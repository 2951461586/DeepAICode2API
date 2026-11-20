package deepai

import (
	"context"
	"io"
	"net/http"
	"testing"

	"deepaicode2api/internal/machine"
)

// 账号级机器指纹必须覆盖进程级默认值 —— 这是免费额度能按号发放的前提。
func TestClientPerAccountMachine(t *testing.T) {
	var got []string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Header.Get(HeaderMachine))
		_, _ = io.WriteString(w, `{}`)
	})
	c.Machine = machine.Fingerprint("global-machine")

	acctA := &Account{APIKey: "k1", MachineID: "aaaaaaaa-1111-4111-8111-111111111111"}
	acctB := &Account{APIKey: "k2", MachineID: "bbbbbbbb-2222-4222-8222-222222222222"}
	noMachine := &Account{APIKey: "k3"}

	for _, acct := range []*Account{acctA, acctB, noMachine} {
		if _, err := c.OpenModel(context.Background(), acct, PathChat, []byte(`{}`), ""); err != nil {
			t.Fatalf("OpenModel: %v", err)
		}
	}

	if len(got) != 3 {
		t.Fatalf("请求数 = %d", len(got))
	}
	if want := machine.Fingerprint(acctA.MachineID); got[0] != want {
		t.Fatalf("账号 A 的指纹 = %q，期望 %q", got[0], want)
	}
	if want := machine.Fingerprint(acctB.MachineID); got[1] != want {
		t.Fatalf("账号 B 的指纹 = %q，期望 %q", got[1], want)
	}
	if got[0] == got[1] {
		t.Fatal("两个账号不应共用同一机器指纹（会导致额度互斥）")
	}
	// 没有账号级指纹时回退到进程级。
	if got[2] != c.Machine {
		t.Fatalf("无账号指纹时应回退到进程级 %q，实际 %q", c.Machine, got[2])
	}
}

func TestTheosUsesAccountMachine(t *testing.T) {
	// /theos 决定额度归属，必须用账号自己的指纹。
	var seen string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get(HeaderMachine)
		_, _ = io.WriteString(w, `{"enabled":true,"balance":"1"}`)
	})
	c.Machine = machine.Fingerprint("global")

	acct := &Account{APIKey: "k", MachineID: "cccccccc-3333-4333-8333-333333333333"}
	if _, err := c.Theos(context.Background(), acct); err != nil {
		t.Fatalf("Theos: %v", err)
	}
	if want := machine.Fingerprint(acct.MachineID); seen != want {
		t.Fatalf("/theos 的机器指纹 = %q，期望 %q", seen, want)
	}
}

// 推理侧必须复用注册时的浏览器指纹，否则“Chrome 注册 + 空 UA 推理”本身就是异常信号。
func TestClientPerAccountFingerprintHeaders(t *testing.T) {
	var got http.Header
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		_, _ = io.WriteString(w, `{}`)
	})

	acct := &Account{
		APIKey:         "k",
		UserAgent:      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/156.0.0.0",
		AcceptLanguage: "en-US,en;q=0.9",
		SecCHUA:        `"Not:A-Brand";v="8", "Chromium";v="156"`,
		Platform:       `"Windows"`,
	}
	if _, err := c.OpenModel(context.Background(), acct, PathChat, []byte(`{}`), ""); err != nil {
		t.Fatalf("OpenModel: %v", err)
	}

	if v := got.Get("User-Agent"); v != acct.UserAgent {
		t.Fatalf("User-Agent = %q，期望 %q", v, acct.UserAgent)
	}
	if v := got.Get("Accept-Language"); v != acct.AcceptLanguage {
		t.Fatalf("Accept-Language = %q", v)
	}
	if v := got.Get("sec-ch-ua"); v != acct.SecCHUA {
		t.Fatalf("sec-ch-ua = %q", v)
	}
	if v := got.Get("sec-ch-ua-platform"); v != acct.Platform {
		t.Fatalf("sec-ch-ua-platform = %q", v)
	}
	if v := got.Get("sec-ch-ua-mobile"); v != "?0" {
		t.Fatalf("sec-ch-ua-mobile = %q", v)
	}
}

func TestClientFallsBackToClientFingerprint(t *testing.T) {
	// 没有账号级指纹时（桌面端导入 / 手工添加）回退到客户端默认值。
	var got http.Header
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		_, _ = io.WriteString(w, `{}`)
	})
	c.UserAgent = "fallback-ua/2"
	if _, err := c.OpenModel(context.Background(), &Account{APIKey: "k"}, PathChat, []byte(`{}`), ""); err != nil {
		t.Fatalf("OpenModel: %v", err)
	}
	if v := got.Get("User-Agent"); v != "fallback-ua/2" {
		t.Fatalf("User-Agent = %q，期望回退值", v)
	}
	// 账号没有指纹时不发指纹头，避免发空值。
	if v := got.Get("Accept-Language"); v != "" {
		t.Fatalf("无账号指纹时不应发送 Accept-Language，实际 %q", v)
	}
}
