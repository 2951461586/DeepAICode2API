package server

import (
	"context"
	"deepaicode2api/internal/deepai"
	"deepaicode2api/internal/logx"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// openModel 在账号池上打开一次上游推理请求。
//
// 语义：
//   - 2xx：立即返回响应，调用方负责关闭 Body；
//   - 400/404/422 等请求错误：不重试，直接返回 *deepai.HTTPError；
//   - 401/403/429/5xx：记录账号失败并换号重试；
//   - 409 session_busy：池里有空号就立刻换号，整池都满才按 Retry-After 等一次；
//   - 槽位不够：短暂排队（admissionWait），超时才回 429。
func (s *Server) openModel(ctx context.Context, path string, body []byte, affinity, group string) (*http.Response, *deepai.Account, error) {
	// 只需要号池规模：AccountCounts 不解包快照，避免为了一次 len() 深拷贝全池。
	acctTotal, _ := s.Store.AccountCounts()
	if acctTotal == 0 {
		return nil, nil, errors.New("没有可用账号，请先在控制台导入桌面端凭据或添加 API Key")
	}
	settings := s.Store.Settings()
	maxTries := settings.MaxRetries
	if maxTries <= 0 {
		maxTries = 3
	}
	if maxTries > acctTotal+2 {
		maxTries = acctTotal + 2
	}

	var (
		lastErr error
		used    *deepai.Account
	)
	deadline := time.Now().Add(s.admissionWait)
	for i := 0; i < maxTries; i++ {
		acct, ok, candidates := s.pickFreeAccount(group)
		if !candidates {
			// 池里没有可调度账号（全被拦 / 分组为空）：交给 noAccountError 说明原因。
			break
		}
		if !ok {
			// 全忙：短暂排队而不是立刻 429；等槽位不消耗重试预算。
			lastErr = errAllAccountsBusy()
			if !time.Now().Before(deadline) {
				break
			}
			select {
			case <-time.After(admissionPoll):
			case <-ctx.Done():
				return nil, used, ctx.Err()
			}
			i--
			continue
		}
		// 额度快照过旧时顺手异步刷新：只刷新「已经探过、但结果过期」的账号，
		// 不在热点路径上给从未探测的号新增上游请求（那些交给入池复探 / 后台守护）。
		if acct.Theos != nil && quotaNeedsRefresh(acct, time.Now(), quotaStaleAfter) {
			s.probeAsync(acct)
		}
		used = acct

		client := s.clientFor(acct)
		// 与桌面端一致：推理前先补签到（仅在挑战过期 / 未回传时才会真的发请求）。
		//
		// 这是**同步**调用：最坏占掉 checkInAttemptTimeout（10s）的推理延迟。
		// 没不能改异步：nonce 回传是上游判定「真实客户端」的依据，先发聊天再补
		// 签到会让那次请求落到替身分支（见 docs/recon.md §5.1）。失败不影响本次
		// 推理，但「慢」是用户能感知的，所以超过阈值留一条日志（之前这笔等待
		// 在任何日志里都看不到）。
		if path == deepai.PathChat || path == deepai.PathResponses {
			ciStart := time.Now()
			client.EnsureCheckIn(ctx, acct)
			if d := time.Since(ciStart); d >= checkInSlowWarn {
				logx.Warn("签到阻塞了推理请求", "account", acctLabel(acct), "ms", d.Milliseconds())
			}
		}
		resp, err := client.OpenModel(ctx, acct, path, body, affinity)
		if err != nil {
			s.releaseAccount(acct.ID)
			lastErr = err
			s.Store.MarkAccountFailure(acct.ID, err)
			if ctx.Err() != nil {
				break
			}
			continue
		}

		if resp.StatusCode/100 == 2 {
			s.Store.MarkAccountResult(acct.ID, true, "")
			resp.Body = s.holdResponseBody(resp.Body, acct.ID)
			return resp, acct, nil
		}

		payload, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		_ = resp.Body.Close()
		if readErr != nil {
			s.releaseAccount(acct.ID)
			lastErr = readErr
			s.Store.MarkAccountFailure(acct.ID, readErr)
			continue
		}
		httpErr := &deepai.HTTPError{Op: "上游推理", Status: resp.StatusCode, Body: payload}

		// 上游 0.3.19 对请求体做了形状校验（400 + "This request is not one
		// DeepAI Code sends."）。用桌面端字段白名单 + 补 system 消息同号重试一次。
		if httpErr.BodyRejected() && path == deepai.PathChat {
			if alt, ok := deepai.ChatBodyFallback(body, ""); ok {
				log.Printf("[upstream] 请求体被拒，按桌面端形状重试：account=%s", acctLabel(acct))
				if retry, err2 := client.OpenModel(ctx, acct, path, alt, affinity); err2 == nil {
					if retry.StatusCode/100 == 2 {
						s.Store.MarkAccountResult(acct.ID, true, "")
						retry.Body = s.holdResponseBody(retry.Body, acct.ID)
						return retry, acct, nil
					}
					payload2, _ := io.ReadAll(io.LimitReader(retry.Body, 4<<20))
					_ = retry.Body.Close()
					if len(payload2) > 0 {
						payload = payload2
						httpErr = &deepai.HTTPError{Op: httpErr.Op, Status: retry.StatusCode, Body: payload2}
					}
				} else {
					log.Printf("[upstream] 形状回退重试失败：account=%s err=%v", acctLabel(acct), err2)
				}
			}
		}
		s.releaseAccount(acct.ID)

		// 邮箱未验证：账号登录态无效，换号也救不了它。
		// 标记后继续试下一个号（下次 /theos 成功会自动解封）；
		// 不要把它当普通 403 反复重试。
		if httpErr.EmailUnverified() {
			s.Store.BlockAccount(acct.ID, blockedReason(httpErr))
			lastErr = httpErr
			continue
		}
		// 账号级策略拦截：记 blocked 后直接回传（下一次请求调度会跳过它）。
		if httpErr.SocialLoginRequired() {
			s.Store.BlockAccount(acct.ID, blockedReason(httpErr))
			return nil, acct, httpErr
		}
		// 缺机器指纹：换号也没用（所有账号共用同一个探测结果）。
		if httpErr.MachineRequired() {
			return nil, acct, httpErr
		}
		// 车道不对：先用免鉴权的 /models 探一次，命中新车道就换道重试；
		// 仍在冷却期 / 探测失败时把 426 原样交给调用方（提示升级网关）。
		if httpErr.LaneUnsupported() {
			before := s.CurrentLane()
			if lane, lerr := s.RefreshLane(ctx); lerr == nil && lane != "" && lane != before {
				lastErr = httpErr
				continue
			}
			return nil, acct, httpErr
		}

		// 容量拒绝：池里还有空槽就立刻换号（粘性由 X-Session-Affinity 保证，
		// 不需要在同一个号上干等）；整池都满了才按 Retry-After 等一次。
		if httpErr.CapacityWaiting() {
			lastErr = httpErr
			if s.hasFreeAccount() {
				continue
			}
			wait := deepai.RetryAfter(resp, 2*time.Second)
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				return nil, acct, ctx.Err()
			}
			continue
		}

		// 请求本身的问题：直接回传，不浪费其他账号。
		if !httpErr.Retryable() {
			return nil, acct, httpErr
		}

		lastErr = httpErr
		s.Store.MarkAccountFailure(acct.ID, httpErr)
		if ctx.Err() != nil {
			break
		}
	}
	if lastErr == nil {
		lastErr = s.noAccountError()
	}
	return nil, used, lastErr
}

// checkInSlowWarn 是「签到阻塞推理」的告警阈值。
//
// 正常签到是 1~2 个 RTT（百毫秒级）；超过这个值说明上游变慢或正在走重试，
// 用户能感知到首包延迟，需要留痕。
const checkInSlowWarn = 2 * time.Second

// holdResponseBody 把上游响应体包成两层：外层空闲超时，内层归还账号槽位。
//
// 两条「返回 2xx」的路径（正常响应与请求体形状回退重试）必须完全一致：
// 漏掉任何一层，上游发完首帧后挂住时就会把连接与账号槽位一起挂到客户端断开。
// 历史上回退路径就漏了外层，这是回归测试要锁住的点。
func (s *Server) holdResponseBody(body io.ReadCloser, acctID string) io.ReadCloser {
	return newIdleTimeoutBody(
		&releaseBody{ReadCloser: body, release: func() { s.releaseAccount(acctID) }},
		upstreamIdleTimeout,
	)
}

// callJSON 发起一次非流式上游请求并解析 JSON。
func (s *Server) callJSON(ctx context.Context, path string, payload map[string]any, affinity, group string) (map[string]any, *deepai.Account, error) {
	ctx, cancel := context.WithTimeout(ctx, nonStreamBudget)
	defer cancel()

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("请求序列化失败: %w", err)
	}
	resp, acct, err := s.openModel(ctx, path, body, affinity, group)
	if err != nil {
		return nil, acct, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, acct, fmt.Errorf("读取上游响应失败: %w", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, acct, fmt.Errorf("上游响应解析失败: %w", err)
	}
	return out, acct, nil
}

// writeUpstreamErr 把上游错误原样（或包装后）返回给客户端。
func writeUpstreamErr(w http.ResponseWriter, err error) {
	var he *deepai.HTTPError
	if errors.As(err, &he) {
		if json.Valid(he.Body) && len(he.Body) > 0 {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(he.Status)
			_, _ = w.Write(he.Body)
			return
		}
		writeErr(w, he.Status, he.Error(), "upstream_error")
		return
	}
	if errors.Is(err, context.Canceled) {
		return
	}
	if errors.Is(err, context.DeadlineExceeded) {
		writeErr(w, http.StatusGatewayTimeout, "上游请求超时", "upstream_error")
		return
	}
	writeErr(w, http.StatusBadGateway, err.Error(), "upstream_error")
}

// writeAnthropicUpstreamErr 按 Anthropic 错误结构回传上游错误。
func writeAnthropicUpstreamErr(w http.ResponseWriter, err error) {
	var he *deepai.HTTPError
	if errors.As(err, &he) {
		typ := "api_error"
		switch he.Status {
		case http.StatusBadRequest, http.StatusNotFound, http.StatusUnprocessableEntity:
			typ = "invalid_request_error"
		case http.StatusUnauthorized, http.StatusForbidden:
			typ = "authentication_error"
		case http.StatusTooManyRequests:
			typ = "rate_limit_error"
		}
		writeAnthropicErr(w, he.Status, typ, he.Error())
		return
	}
	if errors.Is(err, context.Canceled) {
		return
	}
	writeAnthropicErr(w, http.StatusBadGateway, "api_error", err.Error())
}
