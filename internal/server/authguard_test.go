package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"deepaicode2api/internal/store"
)

// setAdminPassword 打开管理口令校验。
func setAdminPassword(t *testing.T, s *Server, pw string) {
	t.Helper()
	s.Store.UpdateSettings(func(st *store.Settings) { st.AdminPassword = pw })
}

// hit 从一个指定来源发一次管理鉴权请求，返回状态码。
func hit(s *Server, remoteAddr, pw string) int {
	req := httptest.NewRequest(http.MethodPost, "/admin/api/auth", nil)
	req.RemoteAddr = remoteAddr
	if pw != "" {
		req.Header.Set("Authorization", "Bearer "+pw)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec.Code
}

// TestAdminPasswordThrottle 是 P1-2 的回归防线。
//
// 连续失败超过阈值后必须进入退避：期间即使口令正确也回 429（不给出对错提示），
// 退避结束后恢复；成功一次即清零。
func TestAdminPasswordThrottle(t *testing.T) {
	s, _ := newTestServer(t, nil)
	s.adminGuardBase = 200 * time.Millisecond
	setAdminPassword(t, s, "s3cret")

	const ip = "127.0.0.1:34567" // newTestServer 的默认来源
	for i := 0; i < adminAuthFreeAttempts; i++ {
		if code := hit(s, ip, "wrong"); code != http.StatusUnauthorized {
			t.Fatalf("第 %d 次错误口令应回 401，实际 %d", i+1, code)
		}
	}
	// 第 6 次仍然“只是错了一次”：退避从这一次开始计时。
	if code := hit(s, ip, "wrong"); code != http.StatusUnauthorized {
		t.Fatalf("触发退避的那次仍应回 401，实际 %d", code)
	}

	// 退避期：错误口令与正确口令都回 429，避免成为口令探测器。
	if code := hit(s, ip, "wrong"); code != http.StatusTooManyRequests {
		t.Fatalf("超过阈值后应回 429，实际 %d", code)
	}
	if code := hit(s, ip, "s3cret"); code != http.StatusTooManyRequests {
		t.Fatalf("退避期内正确口令也应回 429，实际 %d", code)
	}

	// 退避到期后恢复。
	time.Sleep(260 * time.Millisecond)
	if code := hit(s, ip, "s3cret"); code != http.StatusOK {
		t.Fatalf("退避结束后应放行，实际 %d", code)
	}

	// 成功后计数清零：再错一次只是普通 401，不应立即 429。
	if code := hit(s, ip, "wrong"); code != http.StatusUnauthorized {
		t.Fatalf("成功后计数应清零，实际 %d", code)
	}
}

// TestAdminPasswordThrottlePerIP 验证退避按来源隔离：别的机器不受影响。
func TestAdminPasswordThrottlePerIP(t *testing.T) {
	s, _ := newTestServer(t, nil)
	s.adminGuardBase = 30 * time.Millisecond
	setAdminPassword(t, s, "s3cret")

	const attacker = "10.0.0.9:1234"
	for i := 0; i < adminAuthFreeAttempts+2; i++ {
		hit(s, attacker, "wrong")
	}
	if code := hit(s, attacker, "s3cret"); code != http.StatusTooManyRequests {
		t.Fatalf("攻击来源应处于退避，实际 %d", code)
	}
	// 另一个来源仍然可以正常验证。
	if code := hit(s, "127.0.0.1:34567", "s3cret"); code != http.StatusOK {
		t.Fatalf("其它来源不应被牵连，实际 %d", code)
	}
}

// TestAdminPasswordNotConfiguredLoopbackOnly 守住“未设口令时仅本机可管理”。
func TestAdminPasswordNotConfiguredLoopbackOnly(t *testing.T) {
	s, _ := newTestServer(t, nil)
	// newTestServer 未设管理口令。
	req := httptest.NewRequest(http.MethodGet, "/admin/api/overview", nil)
	req.RemoteAddr = "10.1.2.3:5555"
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("非回环来源应被拒（403），实际 %d", rec.Code)
	}
}
