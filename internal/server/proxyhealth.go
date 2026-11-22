package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"deepaicode2api/internal/deepai"
	"deepaicode2api/internal/logx"
	"deepaicode2api/internal/proxy"
)

// 出口代理的健康观测与可用性探测。
//
// 线上形态（2026-11-22，16 个账号各绑一个住宅代理）：失败几乎全部发生在
// 代理侧——`Gateway Timeout`(504)、`unexpected EOF`、`context deadline
// exceeded`——而账号本身是好的。这类故障的正确处置是「换出口」，不是「弃号」，
// 所以这里只记代理的连续失败，不做账号级惩罚。

// noteProxyResult 记录一次出口结果：ok=true 表示这次确实拿到了上游响应
// （哪怕是 4xx/5xx —— 那已经证明 DNS/TCP/TLS/CONNECT 全通）。
func (s *Server) noteProxyResult(acct *deepai.Account, ok bool) {
	if s == nil || s.proxyHealth == nil || acct == nil {
		return
	}
	key := proxyKey(acct)
	if key == "" {
		return // 直连没有可观测的出口
	}
	s.proxyHealth.Report(key, ok)
}

// proxyCooling 报告账号绑定的出口是否正处于冷却窗口。
func (s *Server) proxyCooling(acct *deepai.Account) bool {
	if s == nil || s.proxyHealth == nil || acct == nil {
		return false
	}
	return s.proxyHealth.Cooling(proxyKey(acct))
}

// avoidCoolingProxy 是传给 PickAvoiding 的谓词：出口冷却中的账号排到候选末尾。
func (s *Server) avoidCoolingProxy(a *deepai.Account) bool { return s.proxyCooling(a) }

// ProxyCoolingCount 返回当前处于冷却的出口数（概览 / 诊断用）。
func (s *Server) ProxyCoolingCount() int {
	if s == nil || s.proxyHealth == nil {
		return 0
	}
	return s.proxyHealth.CoolingCount()
}

// proxyKey 是出口的台账键：原始代理串（不同凭据不能混合统计）。
func proxyKey(acct *deepai.Account) string {
	if acct == nil {
		return ""
	}
	return strings.TrimSpace(acct.Proxy)
}

// 出口可用性探测。
//
// 为什么需要它：代理串缺 scheme 时旧实现会静默退回全局代理（见
// deepai.NewClientWithFingerprint 的注释）。修好解析之后，一批「从来没被真正
// 用过」的住宅代理会突然生效——线上不能拿它们赌博，所以先用一次 CONNECT
// 探测确认能穿透到上游，**探测通过才启用专属出口**；未知 / 失败一律沿用
// 全局出口（与修复前一致），不会因为一条坏代理把请求打挂。

const (
	// proxyProbeTTL 是探测结论的缓存时长：代理好坏是分钟级事实。
	proxyProbeTTL = 10 * time.Minute
	// proxyProbeTimeout 是单次出口预检预算（CONNECT + 一次真实 GET 共用）。
	//
	// 6s 太短：住宅代理冷启动实测 6–8s（bundle-5 首次全部超时、回探 1.1–4.4s；
	// bundle-7 两条能用的 6.1s / 7.7s），会被误判成不可用。预检完全在后台，
	// 宽松一点只多花一次等待，不影响任何请求。
	proxyProbeTimeout = 14 * time.Second
)

type probeEntry struct {
	ok bool
	at time.Time
}

// proxyUsable 报告账号的专属出口当前是否可用（带缓存与后台探测）。
func (s *Server) proxyUsable(acct *deepai.Account) bool {
	key := proxyKey(acct)
	if key == "" {
		return false
	}
	s.probeMu.Lock()
	if s.probes == nil {
		s.probes = map[string]probeEntry{}
	}
	e, seen := s.probes[key]
	fresh := seen && time.Since(e.at) < proxyProbeTTL
	if !fresh && !s.probeInflight[key] {
		if s.probeInflight == nil {
			s.probeInflight = map[string]bool{}
		}
		s.probeInflight[key] = true
		go s.probeProxy(key)
	}
	s.probeMu.Unlock()
	// 未探测完 / 从未成功：沿用全局出口，避免把请求压到未验证的代理上。
	return fresh && e.ok
}

// probeProxy 在后台跑一次出口预检并落缓存。
//
// 两步都不省：
//  1. CONNECT 到上游 host:443 —— 证明 TCP/TLS/代理隧道通；
//  2. 经出口真发一次 `GET /deepai_code/theos`（不带 Api-Key）—— 证明这个出口 IP
//     没有被上游 WAF 拦。2026-11-22 踩过：6 条新出口 CONNECT 全通，其中 3 条
//     一请求就 `403 <html>nginx/1.24.0`，只有发真请求才能分辨。
func (s *Server) probeProxy(key string) {
	target := s.upstreamTarget()
	ctx, cancel := context.WithTimeout(context.Background(), proxyProbeTimeout)
	defer cancel()
	err := s.probeExit(ctx, key)

	s.probeMu.Lock()
	if s.probes == nil {
		s.probes = map[string]probeEntry{}
	}
	s.probes[key] = probeEntry{ok: err == nil, at: time.Now()}
	delete(s.probeInflight, key)
	s.probeMu.Unlock()

	if err != nil {
		logx.Warn("出口代理探测失败（沿用全局出口）", "proxy", proxy.ProxyLabel(key), "target", target, "err", err.Error())
		return
	}
	logx.Info("出口代理可用，已启用专属出口", "proxy", proxy.ProxyLabel(key))
}

// probeExit 是出口预检的两步组合：CONNECT 隧道 + 一次真实 GET。
func (s *Server) probeExit(ctx context.Context, raw string) error {
	if err := proxy.Probe(ctx, raw, s.upstreamTarget()); err != nil {
		return err
	}
	u, err := proxy.Parse(raw)
	if err != nil {
		return err
	}
	tr := &http.Transport{
		Proxy:               http.ProxyURL(u),
		TLSHandshakeTimeout: 8 * time.Second,
		ForceAttemptHTTP2:   true,
	}
	client := &http.Client{Transport: tr, Timeout: proxyProbeTimeout}

	origin := deepai.DefaultOrigin
	if s.Up != nil && strings.TrimSpace(s.Up.Origin) != "" {
		origin = s.Up.Origin
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(origin, "/")+"/deepai_code/theos", nil)
	if err != nil {
		return err
	}
	// 与桌面端/网关同形：不带 Api-Key（不消耗额度），只为了看出口能不能被上游接受。
	req.Header.Set("accept", "*/*")
	req.Header.Set("user-agent", "node")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return classifyProbeResponse(resp.StatusCode, body)
}

// classifyProbeResponse 把一次出口预检的结果分成「可用 / 被拦」。
//
// 判据只有一条：**HTML 错误页 = 出口被拦/WAF**；JSON 的 4xx/5xx 都是上游业务层的
// 正常响应（426 车道不对、401 缺 Key、429 限流…），说明隧道与出口都是好的。
func classifyProbeResponse(status int, body []byte) error {
	if status == 0 {
		return errors.New("出口预检无响应")
	}
	if looksLikeHTML(body) && status >= 400 {
		return fmt.Errorf("出口被上游拦截（HTTP %d，HTML 错误页）", status)
	}
	return nil
}

// upstreamTarget 是探测用的 CONNECT 目标（上游 host:port）。
func (s *Server) upstreamTarget() string {
	origin := deepai.DefaultOrigin
	if s != nil && s.Up != nil && strings.TrimSpace(s.Up.Origin) != "" {
		origin = s.Up.Origin
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return "api.deepai.org:443"
	}
	if u.Port() != "" {
		return u.Host
	}
	if u.Scheme == "http" {
		return net.JoinHostPort(u.Hostname(), "80")
	}
	return net.JoinHostPort(u.Hostname(), "443")
}

// looksLikeHTML 判断响应体是不是 HTML 错误页（WAF / 代理网关拦的，
// 而不是上游的 JSON 错误）。只看首个非空白字节，避免整段扫描。
func looksLikeHTML(body []byte) bool {
	for _, b := range body {
		switch b {
		case ' ', '\t', '\r', '\n':
			continue
		case '<':
			return true
		default:
			return false
		}
	}
	return false
}
