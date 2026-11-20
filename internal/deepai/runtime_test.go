package deepai

import (
	"testing"
	"time"
)

// TestRuntimeSourceOverridesInstance 锁定「拉取式优先于推送式」的优先级。
//
// 背景：车道与签到预算都能在控制台运行期调整。此前 Server 必须把新值
// 「推」给直连客户端和每个代理客户端（外加新建客户端时再补一次），
// 漏一处就表现为「走代理的号与直连号行为不一致」。改成客户端每次请求
// 现取之后，源一改就立刻生效。
func TestRuntimeSourceOverridesInstance(t *testing.T) {
	c := &Client{}

	// 无 Runtime、无实例值：用编译期车道。
	if got := c.laneVersion(); got != LaneVersion {
		t.Fatalf("laneVersion = %q，期望编译期 %q", got, LaneVersion)
	}
	// 实例值（推送式）在位。
	c.SetLane("5")
	if got := c.laneVersion(); got != "5" {
		t.Fatalf("SetLane 后 laneVersion = %q，期望 5", got)
	}

	// Runtime 优先级更高，且是**每次现取**：源变了立刻生效。
	lane := "6"
	budget := 2 * time.Second
	c.Runtime = func() RuntimeConfig {
		return RuntimeConfig{Lane: lane, CheckInBudget: budget}
	}
	if got := c.laneVersion(); got != "6" {
		t.Fatalf("Runtime 车道 = %q，期望 6", got)
	}
	if got := c.checkInBudget(); got != budget {
		t.Fatalf("Runtime 预算 = %v，期望 %v", got, budget)
	}
	lane = "7"
	budget = 3 * time.Second
	if got := c.laneVersion(); got != "7" {
		t.Fatalf("拉取式应在下次读取时生效，得到 %q", got)
	}
	if got := c.checkInBudget(); got != budget {
		t.Fatalf("拉取式预算应在下次读取时生效，得到 %v", got)
	}
}

// TestRuntimeEmptyFallsBack 验证 Runtime 返回空值时回落到实例值 / 默认值。
func TestRuntimeEmptyFallsBack(t *testing.T) {
	c := &Client{Runtime: func() RuntimeConfig { return RuntimeConfig{} }}
	c.SetLane("5")
	if got := c.laneVersion(); got != "5" {
		t.Fatalf("Runtime 空车道应回落到实例值，得到 %q", got)
	}
	if got := c.checkInBudget(); got != checkInAttemptTimeout {
		t.Fatalf("Runtime 空预算应回落到默认 %v，得到 %v", checkInAttemptTimeout, got)
	}
}
