package poolsvc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"deepaicode2api/internal/deepai"
	"deepaicode2api/internal/store"
)

// 本文件只有测试装配：临时状态文件、假上游、账号工厂。
//
// 为什么 poolsvc 需要真实 *store.Store：Config.Store 是具体类型（不做接口
// 抽象是刻意的，见 poolsvc.go 的注释），所以白盒用例必须落一个真的状态
// 文件，顺带覆盖「先写内存再 Save()」这条路径。

// newTestStore 在临时目录里打开一个状态文件。
func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatalf("打开状态文件: %v", err)
	}
	return st
}

// seedAccounts 写入 n 个健康账号，返回它们的 ID（acc-0 … acc-n-1）。
func seedAccounts(t *testing.T, st *store.Store, n int) []string {
	t.Helper()
	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		a := &deepai.Account{
			ID:      fmt.Sprintf("acc-%d", i),
			Name:    fmt.Sprintf("acc-%d", i),
			APIKey:  fmt.Sprintf("key-%d", i),
			Enabled: true,
			Status:  "ok",
			Theos:   &deepai.Allowance{Enabled: true, Balance: "10", Granted: "20"},
		}
		if !st.UpsertAccount(a) {
			t.Fatalf("写入账号 %s 失败（被判为已存在）", a.ID)
		}
		ids = append(ids, a.ID)
	}
	return ids
}

// newService 构造一个绑定到临时 store 的 Service。
//
// clientFor 为 nil 时用一个「永远失败」的客户端，避免用例意外打到真实上游
// （DEEPAI_API_ORIGIN 未指向 httptest 时，网络请求会真的发出去）。
func newService(t *testing.T, st *store.Store, clientFor func(*deepai.Account) *deepai.Client) *Service {
	t.Helper()
	if clientFor == nil {
		clientFor = func(*deepai.Account) *deepai.Client { return nil }
	}
	p := New(Config{Store: st, ClientFor: clientFor, Save: st.Save})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		p.Close(ctx)
	})
	return p
}

// upstreamStub 起一个假上游，并把 DEEPAI_API_ORIGIN 指向它。
//
// 必须在创建 *deepai.Client 之前调用：客户端在构造时读一次 Origin。
func upstreamStub(t *testing.T, h func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(h))
	t.Cleanup(srv.Close)
	t.Setenv("DEEPAI_API_ORIGIN", srv.URL)
	return srv
}

// newStubClient 构造一个指向 upstreamStub 的上游客户端。
func newStubClient(t *testing.T) *deepai.Client {
	t.Helper()
	return deepai.NewClient()
}

// theosOK 返回一份合法的 /theos 响应（余额 10，可调度）。
func theosOK(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"enabled":true,"balance":"10","granted":"20"}`))
}

// theosFail 返回一个 500（可重试的瞬时错误，会走 RecordProbeFailure）。
func theosFail(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	_, _ = w.Write([]byte(`{"error":{"message":"boom"}}`))
}

// theosSocial 返回策略拦截（social_login_required）。
func theosSocial(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte(`{"error":{"message":"use social login","code":"social_login_required"}}`))
}

// concurrencyMeter 记录假上游上的最大并发数与总请求数。
type concurrencyMeter struct {
	cur  atomic.Int64
	peak atomic.Int64
	hits atomic.Int64
}

// track 包一层 handler：进入时 +1、记录峰值、退出时 -1。
func (m *concurrencyMeter) track(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		n := m.cur.Add(1)
		for {
			peak := m.peak.Load()
			if n <= peak || m.peak.CompareAndSwap(peak, n) {
				break
			}
		}
		m.hits.Add(1)
		defer m.cur.Add(-1)
		next(w, r)
	}
}

// gate 让 handler 阻塞到 release 关闭，用于观测并发上限。
type gate struct {
	once    sync.Once
	entered chan struct{}
	release chan struct{}
}

func newGate(buffer int) *gate {
	return &gate{entered: make(chan struct{}, buffer), release: make(chan struct{})}
}

func (g *gate) open() { g.once.Do(func() { close(g.release) }) }

// wait 让 handler 阻塞，直到 open 或超时（超时用例失败而不是挂死）。
func (g *gate) wait(t *testing.T) {
	t.Helper()
	g.entered <- struct{}{}
	select {
	case <-g.release:
	case <-time.After(5 * time.Second):
		t.Error("假上游等待放行超时")
	}
}

// decodeBody 解一份 JSON（用例断言用）。
func decodeBody(t *testing.T, raw []byte, dst any) {
	t.Helper()
	if err := json.Unmarshal(raw, dst); err != nil {
		t.Fatalf("解析 JSON: %v", err)
	}
}
