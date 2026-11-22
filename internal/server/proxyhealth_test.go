package server

import (
	"testing"

	"deepaicode2api/internal/deepai"
)

// TestProxyHealthTracksTransportFailures 固化出口台账的接线：
// 传输失败进入冷却、拿到响应清零、直连（无代理）不记账。
func TestProxyHealthTracksTransportFailures(t *testing.T) {
	s, _ := newTestServer(t, nil)
	viaProxy := &deepai.Account{ID: "a1", Proxy: "http://peer:1"}
	direct := &deepai.Account{ID: "a2"}

	if s.proxyCooling(viaProxy) || s.ProxyCoolingCount() != 0 {
		t.Fatal("初始不应有冷却出口")
	}
	s.noteProxyResult(viaProxy, false)
	if !s.proxyCooling(viaProxy) {
		t.Fatal("传输失败后出口应进入冷却")
	}
	if s.ProxyCoolingCount() != 1 {
		t.Fatalf("冷却出口数 = %d，期望 1", s.ProxyCoolingCount())
	}
	if !s.avoidCoolingProxy(viaProxy) {
		t.Fatal("冷却中的出口应被调度谓词排除")
	}

	s.noteProxyResult(viaProxy, true)
	if s.proxyCooling(viaProxy) || s.ProxyCoolingCount() != 0 {
		t.Fatal("一次成功应立刻解除冷却")
	}

	// 直连不记账：空 proxy 不是「一个出口」。
	s.noteProxyResult(direct, false)
	if s.ProxyCoolingCount() != 0 {
		t.Fatal("直连不应进入出口台账")
	}
	// nil 安全：账号为空 / 服务未初始化时都不 panic。
	s.noteProxyResult(nil, false)
	var nilServer *Server
	nilServer.noteProxyResult(viaProxy, false)
	if nilServer.ProxyCoolingCount() != 0 {
		t.Fatal("nil 服务应返回 0")
	}
}
