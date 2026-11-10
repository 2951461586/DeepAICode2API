package server

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"deepaicode2api/internal/deepai"
)

// 浏览器登录（RFC 8252 本机回调）与桌面端完全同款：
//
//	GET  https://api.deepai.org/deepai_code/desktop_auth/?state=…&port=…
//	  → 浏览器完成登录后回调 http://127.0.0.1:<port>/auth-callback?code=…&state=…
//	POST https://api.deepai.org/deepai_code/desktop_auth/exchange/  {"code": …}
//	  → {api_key, username, is_pro}
const loginTTL = 10 * time.Minute

type loginResult struct {
	Creds *deepai.Credentials
	Err   string
}

type loginSession struct {
	ID      string
	State   string
	Port    int
	Created time.Time
	srv     *http.Server

	mu         sync.Mutex
	result     *loginResult
	exchanging bool // 已有一路在用 code 换 Key（本机回调与「粘贴回调 URL」互斥）
	consumed   bool // 已入池并收尾；poll 与粘贴只允许一方完成收尾
}

// setResult 记录兑换结果；已有结果时不覆盖（先到先得）。
func (s *loginSession) setResult(r *loginResult) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.result != nil {
		return false
	}
	s.result = r
	return true
}

// snapshotResult 返回当前结果（nil 表示仍在等待）。
func (s *loginSession) snapshotResult() *loginResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.result
}

// beginExchange 抢占「用 code 换 Key」的唯一名额。
//
// 本机回调与粘贴回调 URL 是两条并行入口，而同一个 code 只能兑换一次
// （上游对第二次报 400/401）。这里把兑换串行化，谁先拿到谁兑换。
func (s *loginSession) beginExchange() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.consumed || s.exchanging || s.result != nil {
		return false
	}
	s.exchanging = true
	return true
}

// claim 收尾：返回本次结果并把会话标记为已消费（重复调用返回 nil）。
func (s *loginSession) claim() *loginResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.consumed {
		return nil
	}
	s.consumed = true
	return s.result
}

type loginSessions struct {
	mu sync.Mutex
	m  map[string]*loginSession
}

func newLoginSessions() *loginSessions { return &loginSessions{m: map[string]*loginSession{}} }

func (ls *loginSessions) put(s *loginSession) {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	for k, v := range ls.m {
		if time.Since(v.Created) > loginTTL {
			delete(ls.m, k)
			go shutdownLoginServer(v.srv)
		}
	}
	ls.m[s.ID] = s
}

func (ls *loginSessions) get(id string) *loginSession {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return ls.m[id]
}

func (ls *loginSessions) remove(id string) *loginSession {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	s := ls.m[id]
	delete(ls.m, id)
	return s
}

func shutdownLoginServer(srv *http.Server) {
	if srv == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

// handleLoginStart 启动一次浏览器登录，返回可打开的 URL。
func (s *Server) handleLoginStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "仅支持 POST", "invalid_request_error")
		return
	}

	id := deepai.RandomHex(16)
	state := deepai.RandomToken(32)
	session := &loginSession{ID: id, State: state, Created: time.Now()}

	mux := http.NewServeMux()
	mux.HandleFunc("/auth-callback", func(cw http.ResponseWriter, cr *http.Request) {
		code := cr.URL.Query().Get("code")
		if cr.Method != http.MethodGet || code == "" || len(code) > 4096 ||
			cr.URL.Query().Get("state") != state || !secureEqual(cr.URL.Query().Get("state"), state) {
			cw.WriteHeader(http.StatusBadRequest)
			_, _ = cw.Write([]byte("Invalid sign-in callback."))
			return
		}
		// 用户可能已经在控制台里粘贴同一段回调，谁先兑换谁算数。
		if !session.beginExchange() {
			cw.WriteHeader(http.StatusConflict)
			_, _ = cw.Write([]byte("This sign-in link was already used."))
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		creds, err := s.Up.ExchangeCode(ctx, code)
		cancel()
		if err != nil {
			s.finishLogin(session, nil, err)
			cw.WriteHeader(http.StatusBadGateway)
			_, _ = cw.Write([]byte("Sign-in failed. Return to the console and try again."))
			return
		}
		s.finishLogin(session, creds, nil)
		cw.Header().Set("Content-Type", "text/html; charset=utf-8")
		cw.Header().Set("Cache-Control", "no-store")
		_, _ = cw.Write([]byte("<h2>Signed in to DeepAI Code</h2><p>You can close this tab and return to the console.</p>"))
	})

	// 本机回调只是「自动收」的那条路；绑不上回环时（典型：Docker 里
	// 127.0.0.1 属于容器，宿主机浏览器打不开）流程不能断，改用粘贴回调 URL。
	if ln, err := net.Listen("tcp", "127.0.0.1:0"); err != nil {
		session.Port = fallbackLoginPort()
	} else {
		session.Port = ln.Addr().(*net.TCPAddr).Port
		session.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
		go func() { _ = session.srv.Serve(ln) }()
	}
	s.Login.put(session)

	writeJSON(w, http.StatusOK, map[string]any{
		"session_id": id,
		"open_url":   "/login/open?id=" + url.QueryEscape(id),
		"port":       session.Port,
		"loopback":   session.srv != nil,
		"expires_in": int(loginTTL.Seconds()),
	})
}

// fallbackLoginPort 在绑不上本机回环时给上游一个「看起来合法」的端口。
//
// 上游只把这个值拼进 http://127.0.0.1:<port>/auth-callback；回调打不开时
// 浏览器地址栏仍保留完整 URL（含 code 与 state），正好可以走粘贴回调。
func fallbackLoginPort() int {
	var b [2]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 47821
	}
	return 45000 + (int(b[0])<<8|int(b[1]))%10000
}

// handleLoginOpen 跳转到上游登录页。
//
// 跳转目标完全由服务端根据会话里的 state/port 重建，并再次校验
// scheme/host/path，绝不使用请求参数拼 URL。
func (s *Server) handleLoginOpen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "仅支持 GET", "invalid_request_error")
		return
	}
	session := s.Login.get(strings.TrimSpace(r.URL.Query().Get("id")))
	if session == nil || session.Port == 0 {
		writeErr(w, http.StatusNotFound, "登录会话不存在或已过期", "invalid_request_error")
		return
	}
	if time.Since(session.Created) > loginTTL {
		s.Login.remove(session.ID)
		shutdownLoginServer(session.srv)
		writeErr(w, http.StatusGone, "登录会话已过期", "invalid_request_error")
		return
	}
	target := s.Up.SignInURL(session.State, session.Port)
	if !isSafeLoginURL(target, s.Up.Origin) {
		writeErr(w, http.StatusInternalServerError, "登录地址校验失败", "server_error")
		return
	}
	h := w.Header()
	h.Set("Cache-Control", "no-store")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Location", target)
	w.WriteHeader(http.StatusFound)
}

// isSafeLoginURL 校验登录跳转目标是否落在配置的上游源上。
func isSafeLoginURL(raw, origin string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	o, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if u.Scheme != o.Scheme || !strings.EqualFold(u.Host, o.Host) {
		return false
	}
	return u.Path == "/deepai_code/desktop_auth/" || u.Path == "/deepai_code/desktop_auth"
}

// handleLoginPoll 轮询浏览器登录结果。
func (s *Server) handleLoginPoll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "仅支持 POST", "invalid_request_error")
		return
	}
	var body struct {
		SessionID string `json:"session_id"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body)
	if strings.TrimSpace(body.SessionID) == "" {
		writeErr(w, http.StatusBadRequest, "缺少 session_id", "invalid_request_error")
		return
	}

	session := s.Login.get(body.SessionID)
	if session == nil {
		writeErr(w, http.StatusNotFound, "登录会话不存在或已过期", "invalid_request_error")
		return
	}
	if time.Since(session.Created) > loginTTL {
		s.Login.remove(body.SessionID)
		shutdownLoginServer(session.srv)
		writeJSON(w, http.StatusOK, map[string]any{"status": "expired"})
		return
	}

	result := session.snapshotResult()
	if result == nil {
		writeJSON(w, http.StatusOK, map[string]any{"status": "pending"})
		return
	}
	writeJSON(w, http.StatusOK, s.completeLogin(session))
}

// handleLoginCallback 用「粘贴回来的回调 URL」完成登录。
//
// Docker / 远程部署时，上游把浏览器送到 http://127.0.0.1:<port>/auth-callback?code=…
// —— 那个 127.0.0.1 是宿主机自己的回环，而监听在容器里（而且是随机会话端口，
// 就算 -p 也没法映射到容器的回环地址），所以这一页必定打不开。但地址栏里的
// code/state 是有效的：本接口替用户把 code 换成 Key 并完成入池，等价于本机回调。
func (s *Server) handleLoginCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "仅支持 POST", "invalid_request_error")
		return
	}
	var body struct {
		SessionID string `json:"session_id"`
		Callback  string `json:"callback"`
		Code      string `json:"code"`
		State     string `json:"state"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "请求体不是合法 JSON", "invalid_request_error")
		return
	}
	session := s.Login.get(strings.TrimSpace(body.SessionID))
	if session == nil {
		writeErr(w, http.StatusNotFound, "登录会话不存在、已结束或已过期（同一个 code 只能兑换一次）", "invalid_request_error")
		return
	}
	if time.Since(session.Created) > loginTTL {
		s.Login.remove(session.ID)
		shutdownLoginServer(session.srv)
		writeJSON(w, http.StatusOK, map[string]any{"status": "expired"})
		return
	}

	code, state, err := parseLoginCallback(body.Callback, body.Code, body.State)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error(), "invalid_request_error")
		return
	}
	if !secureEqual(state, session.State) {
		writeErr(w, http.StatusBadRequest, "回调里的 state 与本次登录不匹配：请确认粘贴的是本次登录的地址栏 URL", "invalid_request_error")
		return
	}
	if !session.beginExchange() {
		writeErr(w, http.StatusConflict, "这次登录已经处理过（同一个 code 只能兑换一次），请重新发起登录", "invalid_request_error")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	creds, xerr := s.Up.ExchangeCode(ctx, code)
	cancel()
	if xerr != nil {
		s.finishLogin(session, nil, xerr)
		_ = s.completeLogin(session) // 收尾：删会话、关本机监听
		status, msg := loginExchangeError(xerr)
		writeErr(w, status, msg, "invalid_request_error")
		return
	}
	s.finishLogin(session, creds, nil)
	writeJSON(w, http.StatusOK, s.completeLogin(session))
}

// loginExchangeError 把上游兑换失败翻译成控制台可直接展示的提示。
func loginExchangeError(err error) (int, string) {
	var he *deepai.HTTPError
	if errors.As(err, &he) {
		switch {
		case he.Status == http.StatusBadRequest, he.Status == http.StatusUnauthorized,
			he.Status == http.StatusForbidden:
			return http.StatusBadRequest, "这段回调已经用过或已过期（code 只能兑换一次），请重新发起登录"
		case he.Status == http.StatusTooManyRequests:
			return http.StatusTooManyRequests, "登录尝试过多，稍后再试"
		}
	}
	return http.StatusBadGateway, "兑换失败：" + errString(err)
}

// parseLoginCallback 从「粘贴的内容」里取出 code 与 state。
//
// 接受两种形态：完整回调 URL（地址栏直接复制）与裸 query 串（只复制了 ? 后面）。
// 只粘 code 会被拒：那样就拿不到 state，无法确认它属于本次登录。
func parseLoginCallback(callback, code, state string) (string, string, error) {
	code, state = strings.TrimSpace(code), strings.TrimSpace(state)
	raw := strings.TrimSpace(callback)
	if raw != "" {
		c, st, ok := splitCallbackQuery(raw)
		if !ok {
			return "", "", errors.New("无法从粘贴内容里解析出 code：请粘贴地址栏里完整的 http://127.0.0.1:…/auth-callback?code=…&state=… 链接")
		}
		code, state = c, st
	}
	if code == "" || len(code) > 4096 {
		return "", "", errors.New("缺少 code：请粘贴完整的回调 URL")
	}
	if state == "" {
		return "", "", errors.New("回调里没有 state：请粘贴完整的回调 URL，不要只复制 code")
	}
	return code, state, nil
}

// splitCallbackQuery 兼容「完整 URL」与「只有 query 串」两种粘贴形态。
func splitCallbackQuery(raw string) (string, string, bool) {
	switch {
	case strings.Contains(raw, "://"):
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			return "", "", false
		}
		q := u.Query()
		return q.Get("code"), q.Get("state"), q.Get("code") != ""
	case strings.Contains(raw, "="):
		q, err := url.ParseQuery(strings.TrimPrefix(raw, "?"))
		if err != nil {
			return "", "", false
		}
		return q.Get("code"), q.Get("state"), q.Get("code") != ""
	}
	return "", "", false
}

// completeLogin 把一次已换回凭据的登录收尾：入池 + 探测 + 关闭会话。
//
// 轮询与本机回调驱动它，粘贴回调 URL 也驱动它；claim 保证同一个会话
// 只会入池一次（否则并发两条入口会把同一个号写两遍、探活两次）。
func (s *Server) completeLogin(session *loginSession) map[string]any {
	result := session.claim()
	if result == nil {
		return map[string]any{"status": "pending"}
	}
	// 无论成败，会话都到此结束：code 是一次性的，监听端口也不该继续开着。
	s.Login.remove(session.ID)
	shutdownLoginServer(session.srv)

	if result.Err != "" || result.Creds == nil {
		return map[string]any{"status": "error", "error": result.Err}
	}
	acct, err := deepai.AccountFromCredentials(result.Creds, "oauth")
	if err != nil {
		return map[string]any{"status": "error", "error": err.Error()}
	}
	// 浏览器登录（含 Google/Apple）只换来 API Key，不带机器指纹。若沿用本机
	// 指纹，多个号会共用同一份每日免费额度，只有第一个能领到；入池时补一个
	// 独立 UUID，与 action=enroll 的规则保持一致。
	if strings.TrimSpace(acct.MachineID) == "" {
		acct.MachineID = deepai.NewMachineUUID()
	}
	// 浏览器登录默认走直连；同一直连 IP 的每日免费额度是共享的，
	// 配了代理池就按轮询分一个出口，避免多号挤在同一个 IP 上。
	// 必须在 UpsertAccount 前写入：入池后的探活会直接复用它。
	if strings.TrimSpace(acct.Proxy) == "" {
		if pool := s.enrollProxyPool(); pool.Len() > 0 {
			acct.Proxy = pool.Next()
		}
	}
	isNew := s.Store.UpsertAccount(acct)
	_ = s.Store.Save()
	s.probeAndSaveAsync(acct)

	return map[string]any{
		"status": "ok", "new": isNew, "id": acct.ID,
		"username": acct.Username, "is_pro": acct.IsPro,
	}
}

// finishLogin 记录兑换结果并唤醒等待方（首个结果生效）。
func (s *Server) finishLogin(session *loginSession, creds *deepai.Credentials, err error) {
	session.setResult(&loginResult{Creds: creds, Err: errString(err)})
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%v", err)
}
