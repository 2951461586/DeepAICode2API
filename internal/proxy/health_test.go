package proxy

import (
	"testing"
	"time"
)

// TestHealthCooldownBacksOff 固化「连续失败指数退避 + 上限」：
// 一次失败 30s，两次 60s，四次 240s，到上限后不再增长。
func TestHealthCooldownBacksOff(t *testing.T) {
	now := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	h := NewHealth()
	h.Now = func() time.Time { return now }

	if h.Cooling("p1") {
		t.Fatal("未失败过的出口不应处于冷却")
	}
	steps := []struct {
		want int
		step time.Duration
	}{
		{1, 30 * time.Second},
		{2, 60 * time.Second},
		{3, 120 * time.Second},
	}
	for _, c := range steps {
		h.Report("p1", false)
		if got := h.Fails("p1"); got != c.want {
			t.Fatalf("连续失败 = %d，期望 %d", got, c.want)
		}
		if !h.Cooling("p1") {
			t.Fatalf("第 %d 次失败后应处于冷却", c.want)
		}
		// 差一毫秒仍未到期，整段走完即到期。
		now = now.Add(c.step - time.Millisecond)
		if !h.Cooling("p1") {
			t.Fatalf("第 %d 次失败：冷却提前结束（%v）", c.want, c.step)
		}
		now = now.Add(time.Millisecond)
		if h.Cooling("p1") {
			t.Fatalf("第 %d 次失败：冷却没有在 %v 后结束", c.want, c.step)
		}
	}

	// 退避上限：失败次数堆到 20 次也不过 Max。
	for i := 0; i < 20; i++ {
		h.Report("p1", false)
	}
	h.Report("p1", false)
	now = now.Add(defaultHealthMax - time.Millisecond)
	if !h.Cooling("p1") {
		t.Fatal("冷却应被 Max 截断在 10m")
	}
	now = now.Add(time.Millisecond)
	if h.Cooling("p1") {
		t.Fatal("超过 Max 后应解除冷却")
	}
}

// TestHealthSuccessResets 固化「一次成功立刻清零」：成功之后马上再失败，
// 冷却回到最短档，而不是接着之前的失败次数继续翻倍。
func TestHealthSuccessResets(t *testing.T) {
	now := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	h := NewHealth()
	h.Now = func() time.Time { return now }

	h.Report("p", false)
	h.Report("p", false)
	h.Report("p", false)
	if got := h.Fails("p"); got != 3 {
		t.Fatalf("连续失败 = %d，期望 3", got)
	}
	h.Report("p", true)
	if got := h.Fails("p"); got != 0 {
		t.Fatalf("成功应清零，实际 %d", got)
	}
	if h.Cooling("p") {
		t.Fatal("成功后不应冷却")
	}
	h.Report("p", false)
	now = now.Add(defaultHealthBase)
	if h.Cooling("p") {
		t.Fatal("清零后第一次失败只应冷却 Base")
	}
}

// TestHealthIsolatesKeys 固化「按代理串区分」：同一主机不同凭据互不影响，
// 空 key（直连）不参与台账。
func TestHealthIsolatesKeys(t *testing.T) {
	now := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	h := NewHealth()
	h.Now = func() time.Time { return now }

	h.Report("http://u1:pw@host:1", false)
	if h.Cooling("http://u2:pw@host:1") {
		t.Fatal("不同凭据的出口不应共享冷却")
	}
	if h.Cooling("") {
		t.Fatal("空 key（直连）不应被判为冷却")
	}
	h.Report("", false)
	if h.CoolingCount() != 1 {
		t.Fatalf("冷却出口数 = %d，期望 1", h.CoolingCount())
	}
}

// TestHealthNilSafe 固化「未配置代理池时也不 panic」：服务端字段可能是 nil。
func TestHealthNilSafe(t *testing.T) {
	var h *Health
	h.Report("p", false)
	if h.Cooling("p") || h.Fails("p") != 0 || h.CoolingCount() != 0 {
		t.Fatal("nil 台账应全部返回零值")
	}
}
