package server

import (
	"context"
	"net/http"

	"deepaicode2api/internal/store"
)

// --- 请求级设置快照 ---
//
// Settings 是一次 RLock + 一次 ModelAliases 深拷贝。它在单个请求里此前会被
// 读 3~5 次（CORS → 鉴权 → 限流 → handler → 上游），除了多几次加锁与分配，
// 更麻烦的是**同一请求里可能看到前后两份设置**（控制台刚好在这个请求中间
// 改了设置），例如限流按新 RPS 判定、而超时预算按旧值走。
//
// 现在 Handler() 在最外层取一次快照放进 context，请求内所有读取都拿同一份。

type settingsCtxKey struct{}

// withSettingsSnapshot 为每个请求取一份设置快照。
//
// 放在最外层（在 CORS 之外）：CORS 来源校验、鉴权、限流、handler、上游
// 调用都必须看到同一份设置。
func (s *Server) withSettingsSnapshot(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), settingsCtxKey{}, s.Store.Settings())
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// settingsOf 返回本请求的设置快照；没有经过中间件时（测试直接调 handler）
// 回落到实时读取。
func (s *Server) settingsOf(ctx context.Context) store.Settings {
	if ctx != nil {
		if v, ok := ctx.Value(settingsCtxKey{}).(store.Settings); ok {
			return v
		}
	}
	return s.Store.Settings()
}

// settings 是 settingsOf 的 *http.Request 便捷入口。
func (s *Server) settings(r *http.Request) store.Settings {
	if r == nil {
		return s.Store.Settings()
	}
	return s.settingsOf(r.Context())
}
