package guard

import (
	"testing"
	"time"
)

// TestAdminGuardBackoff 锁住退避曲线：前 Free 次只是 401，之后 base×2^n 递增
// 且封顶在 Max；成功一次即清零。
func TestAdminGuardBackoff(t *testing.T) {
	g := NewAdminGuard()
	now := time.Now()
	p := BackoffPolicy{Free: 2, Base: 10 * time.Millisecond, Max: 100 * time.Millisecond}

	for i := 0; i < p.Free; i++ {
		if d := g.Failure("1.2.3.4", now, p); d != 0 {
			t.Fatalf("第 %d 次失败不应进入退避，实际 %s", i+1, d)
		}
	}
	if d := g.Failure("1.2.3.4", now, p); d != 10*time.Millisecond {
		t.Fatalf("首次退避应为 base（10ms），实际 %s", d)
	}
	if delay, blocked := g.Blocked("1.2.3.4", now); !blocked || delay != 10*time.Millisecond {
		t.Fatalf("退避期内应被拦（10ms），实际 %s / %v", delay, blocked)
	}
	if d := g.Failure("1.2.3.4", now, p); d != 20*time.Millisecond {
		t.Fatalf("第二次退避应翻倍到 20ms，实际 %s", d)
	}
	// 继续翻倍并封顶。
	for _, want := range []time.Duration{40 * time.Millisecond, 80 * time.Millisecond, 100 * time.Millisecond, 100 * time.Millisecond} {
		if d := g.Failure("1.2.3.4", now, p); d != want {
			t.Fatalf("退避应封顶在 %s，实际 %s", want, d)
		}
	}
	// 退避到期后不再拦。
	if _, blocked := g.Blocked("1.2.3.4", now.Add(200*time.Millisecond)); blocked {
		t.Fatal("退避到期后不应再拦")
	}
	// 成功清零：再错一次只是普通失败。
	g.Success("1.2.3.4")
	if _, blocked := g.Blocked("1.2.3.4", now); blocked {
		t.Fatal("成功后应清零")
	}
	if d := g.Failure("1.2.3.4", now, p); d != 0 {
		t.Fatalf("清零后再错一次不应直接退避，实际 %s", d)
	}
}

// TestAdminGuardPerIPIsolation 验证退避按来源隔离。
func TestAdminGuardPerIPIsolation(t *testing.T) {
	g := NewAdminGuard()
	now := time.Now()
	p := BackoffPolicy{Free: 1, Base: time.Second, Max: time.Second}

	g.Failure("10.0.0.9", now, p)
	g.Failure("10.0.0.9", now, p)
	if _, blocked := g.Blocked("10.0.0.9", now); !blocked {
		t.Fatal("攻击来源应处于退避")
	}
	if _, blocked := g.Blocked("127.0.0.1", now); blocked {
		t.Fatal("其它来源不应被牵连")
	}
}

// TestClientIP 验证来源解析：带端口剥端口、解析失败退回原串。
func TestClientIP(t *testing.T) {
	cases := map[string]string{
		"127.0.0.1:34567": "127.0.0.1",
		"[::1]:8080":      "::1",
		"10.0.0.9":        "10.0.0.9",
		"  10.0.0.9  ":    "10.0.0.9",
	}
	for in, want := range cases {
		if got := ClientIP(in); got != want {
			t.Fatalf("ClientIP(%q) = %q，期望 %q", in, got, want)
		}
	}
}

// TestBackoffPolicyDefaults 验证零值走默认策略（服务器只注入 Base 时其余仍生效）。
func TestBackoffPolicyDefaults(t *testing.T) {
	p := BackoffPolicy{Base: 200 * time.Millisecond}.normalized()
	if p.Free != FreeAttempts || p.Max != backoffMax || p.Base != 200*time.Millisecond {
		t.Fatalf("归一化结果不符：%+v", p)
	}
	if d := DefaultBackoffPolicy(); d.Free != FreeAttempts || d.Base != backoffBase || d.Max != backoffMax {
		t.Fatalf("默认策略不符：%+v", d)
	}
}
