package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"net/url"
	"strings"
)

// --- 数据面 / 管理面鉴权 ---

// withAuth 校验 Bearer 密钥。
//
// 尚未签发任何密钥时：
//   - 来自本机回环的请求放行（首次使用的便利，且便于控制台自举）；
//   - 来自非回环来源的请求一律 503 setup_required。
//     否则在监听 0.0.0.0（容器 / 局域网）时，从首次启动到创建第一个密钥之间
//     网关就是一个无需鉴权的开放中继。
//
// 一旦签发了密钥，所有来源都强制校验。
func (s *Server) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.Store.HasKeys() {
			if isLoopbackAddr(r.RemoteAddr) {
				next(w, r)
				return
			}
			writeErr(w, http.StatusServiceUnavailable,
				"网关尚未签发 API 密钥，请先在本机控制台创建密钥（/admin）", "setup_required")
			return
		}
		key := extractKey(r)
		if key == "" {
			writeErr(w, http.StatusUnauthorized, "缺少 API 密钥", "invalid_request_error")
			return
		}
		if !s.Store.ValidKey(key) {
			writeErr(w, http.StatusUnauthorized, "API 密钥无效或已停用", "invalid_request_error")
			return
		}
		// 按密钥隔离的限流：令牌桶（RPS）+ 在途并发。默认 0 = 不限。
		release, ok, retryAfter := s.rateLimitGate(keyIDOf(key))
		if !ok {
			writeRateLimited(w, retryAfter)
			return
		}
		defer release()
		next(w, r)
	}
}

// withAdminAuth 保护控制台接口。
//
// 鉴权策略：
//   - 已设置管理密码：必须通过 X-Admin-Password 或 Authorization: Bearer 提供；
//   - 未设置管理密码：仅允许来自本机环回的请求（避免局域网内裸奔）。
//
// 另外对写操作（非 GET/HEAD/OPTIONS）做同源校验，阻断跨站请求伪造。
func (s *Server) withAdminAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if pw := s.Store.Settings().AdminPassword; pw != "" {
			// 先看退避：处于退避期的来源直接 429，不再给口令对错的提示。
			if !s.checkAdminThrottle(w, r) {
				return
			}
			if !secureEqual(adminPasswordFrom(r), pw) {
				s.noteAdminAuthFailure(r)
				writeErr(w, http.StatusUnauthorized, "管理密码缺失或错误", "invalid_request_error")
				return
			}
			s.noteAdminAuthSuccess(r)
		} else if !isLoopbackAddr(r.RemoteAddr) {
			writeErr(w, http.StatusForbidden, "未设置管理密码，管理接口仅允许本机访问", "invalid_request_error")
			return
		}

		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if !s.adminSameOrigin(r) {
				writeErr(w, http.StatusForbidden, "跨站请求被拒绝", "invalid_request_error")
				return
			}
		}
		next(w, r)
	}
}

func adminPasswordFrom(r *http.Request) string {
	if v := r.Header.Get("X-Admin-Password"); v != "" {
		return v
	}
	if h := r.Header.Get("Authorization"); h != "" {
		if v, ok := strings.CutPrefix(h, "Bearer "); ok {
			return strings.TrimSpace(v)
		}
		return strings.TrimSpace(h)
	}
	return ""
}

func secureEqual(a, b string) bool {
	ha := sha256.Sum256([]byte(a))
	hb := sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(ha[:], hb[:]) == 1
}

func (s *Server) adminSameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	if strings.EqualFold(u.Host, r.Host) {
		return true
	}
	if cfg := strings.TrimSpace(s.Store.Settings().CORSOrigin); cfg != "" {
		for entry := range strings.SplitSeq(cfg, ",") {
			entry = strings.TrimSpace(entry)
			if entry != "" && !hasWildcard(entry) && strings.EqualFold(entry, origin) {
				return true
			}
		}
	}
	return false
}

func (s *Server) resolveKeyRef(ref string) string {
	for _, k := range s.Store.Keys() {
		if k.Key == ref || mask(k.Key) == ref {
			return k.Key
		}
	}
	return ref
}

func extractKey(r *http.Request) string {
	if h := r.Header.Get("Authorization"); h != "" {
		if v, ok := strings.CutPrefix(h, "Bearer "); ok {
			return strings.TrimSpace(v)
		}
		return strings.TrimSpace(h)
	}
	if v := r.Header.Get("x-api-key"); v != "" {
		return strings.TrimSpace(v)
	}
	return ""
}

// handleAdminAuth 校验管理口令本身（登录页用）。
//
// 走 withAdminAuth 注册，所以「口令缺失或错误」已经在那里被拦成 401；
// 能进到这里就说明口令是对的。刻意不返回口令、不返回设置项，只说 ok。
func (s *Server) handleAdminAuth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "仅支持 POST", "invalid_request_error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
