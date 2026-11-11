package server

import (
	"deepaicode2api/internal/deepai"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
)

// maxInFlightPerAccount 是单账号同时进行的上游请求上限。
//
// 调度只决定「这一轮选到谁」，不保证并发不会集中到少数账号；
// 实测 12 并发时可能全落在 1~2 个号上，一碰上上游限流就是长尾。
const maxInFlightPerAccount = 2

// releaseBody 在响应体关闭时归还账号的在途配额。
//
// 所有上游调用点都以 defer resp.Body.Close() 收尾，所以槽位会随 Body 释放：
// 流式响应可能持续数分钟，不能在建连成功时就归还。
type releaseBody struct {
	io.ReadCloser
	once    sync.Once
	release func()
}

func (b *releaseBody) Close() error {
	b.once.Do(b.release)
	return b.ReadCloser.Close()
}

// pickFreeAccount 扫一圈号池，找一个还有在途槽位的账号。
//
// 不能用 max_retries 次贪心选号：饱和时几次尝试大概率全撞在忙号上，
// 于是对外 429 而池里其实还有空槽（docs/performance.md §14）。
// 返回值第三个标记池里是否有可选账号：区分「全忙」与「没有可调度账号」，
// 后者要交给 noAccountError 给出可行动的原因（social_login / 未验证…）。
//
// 实现要点：选号在 store 里一次读完（只拷贝被选中的那个），忙号通过纯谓词
// 排除；acquireAccount 只处理「检查到占用」的窄竞态，失败就排除后重试。
func (s *Server) pickFreeAccount(group string) (*deepai.Account, bool, bool) {
	// excluded 用切片而不是 map：重试只在「检查与占用撞车」时发生，通常 0~1 次，
	// 切片比 map 少一次分配，而选号是每个请求一次的热路径。
	var excluded []string
	skip := func(a *deepai.Account) bool {
		for _, id := range excluded {
			if id == a.ID {
				return true
			}
		}
		return s.accountBusy(a.ID)
	}
	for {
		a, candidates := s.Store.PickAccount(group, skip)
		if a == nil {
			return nil, false, candidates
		}
		if s.acquireAccount(a.ID) {
			return a, true, true
		}
		// 竞态：谓词判定空闲后、占用前被别人抢满。排除它再看下一个。
		excluded = append(excluded, a.ID)
	}
}

// accountBusy 报告账号的在途槽位是否已满（纯读，不占用）。
func (s *Server) accountBusy(id string) bool {
	s.inflightMu.Lock()
	defer s.inflightMu.Unlock()
	return s.inflight[id] >= maxInFlightPerAccount
}

// acquireAccount 尝试占用一个账号的在途槽位；已满时返回 false。
func (s *Server) acquireAccount(id string) bool {
	s.inflightMu.Lock()
	defer s.inflightMu.Unlock()
	if s.inflight == nil {
		s.inflight = map[string]int{}
	}
	if s.inflight[id] >= maxInFlightPerAccount {
		return false
	}
	s.inflight[id]++
	return true
}

// releaseAccount 归还一个在途槽位。
func (s *Server) releaseAccount(id string) {
	s.inflightMu.Lock()
	defer s.inflightMu.Unlock()
	if s.inflight[id] <= 0 {
		delete(s.inflight, id)
		return
	}
	s.inflight[id]--
	if s.inflight[id] == 0 {
		delete(s.inflight, id)
	}
}

// hasFreeAccount 报告池里是否还有账号有空在途槽位。
//
// 409 之后用它区分「立即换号」与「整池都满，等一等」：以前一律先
// sleep(Retry-After) 再换号，最坏白等 30s，还把非流式预算（90s）吃掉。
// 只做一次指针扫描 + map 查询，不复制整个号池。
func (s *Server) hasFreeAccount() bool {
	return s.Store.AnySchedulable(func(a *deepai.Account) bool {
		return !s.accountBusy(a.ID)
	})
}

// errAllAccountsBusy 表示所有候选账号都达到了并发上限。
//
// 用 429 而不是 502：这是可重试的背压，客户端应稍后重试。
func errAllAccountsBusy() error {
	return &deepai.HTTPError{
		Op:     "账号调度",
		Status: http.StatusTooManyRequests,
		Body:   []byte(`{"error":{"message":"所有账号并发已满，请稍后重试","type":"rate_limit_error"}}`),
	}
}

// blockedReason 把上游错误码拼进账号的 last_err。
//
// 光存 message 会丢掉 code：全池被拦时要靠它区分 social_login_required /
// email_unverified，才能给调用方一个能行动的错误。
func blockedReason(he *deepai.HTTPError) string {
	if code := he.Code(); code != "" {
		return code + ": " + he.Error()
	}
	return he.Error()
}

// noAccountError 在没有可选账号时给出可执行的错误。
//
// 池里全部账号都被上游要求改用 Google/Apple 登录时，不能只回一句
// 「全部账号均不可用」——那是运营层面的策略变更，必须让调用方看见原因。
func (s *Server) noAccountError() error {
	accts := s.Store.Accounts()
	if len(accts) == 0 {
		return errors.New("没有可用账号，请先在控制台导入桌面端凭据或添加 API Key")
	}
	blocked := 0
	for _, a := range accts {
		if a.Status == deepai.StatusBlocked {
			blocked++
		}
	}
	if blocked > 0 && blocked == len(accts) {
		// 全部被拦时要把原因说清楚：只报“全部不可用”会让人以为网关坏了。
		social, unverified := 0, 0
		for _, a := range accts {
			switch {
			case strings.Contains(a.LastErr, deepai.CodeSocialLogin):
				social++
			case strings.Contains(a.LastErr, deepai.CodeEmailUnverified):
				unverified++
			}
		}
		if unverified > 0 && social == 0 {
			return &deepai.HTTPError{
				Op:     "账号调度",
				Status: http.StatusServiceUnavailable,
				Body:   []byte(`{"error":{"message":"全部账号都被上游判定为邮箱未验证（email_unverified），请先验证邮箱后重试","type":"email_unverified","code":"email_unverified"}}`),
			}
		}
		if social > 0 {
			return &deepai.HTTPError{
				Op:     "账号调度",
				Status: http.StatusServiceUnavailable,
				Body:   []byte(`{"error":{"message":"全部账号已被上游要求改用 Google/Apple 登录（social_login_required）；邮箱密码注册的账号无法再推理","type":"social_login_required","code":"social_login_required"}}`),
			}
		}
		return &deepai.HTTPError{
			Op:     "账号调度",
			Status: http.StatusServiceUnavailable,
			Body:   []byte(`{"error":{"message":"全部账号都被上游拦截（策略 / 登录态），请到控制台查看各账号的 last_err","type":"account_blocked","code":"account_blocked"}}`),
		}
	}
	return errors.New("全部账号均不可用")
}
