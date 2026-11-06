package register

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestMailboxFresh 校验「这个地址是不是本次新建的」判定。
//
// 它决定 pipeline 能不能跳过「邮箱是否已注册」的预检（一次 ~3.3s 的往返）。
func TestMailboxFresh(t *testing.T) {
	// 无收件箱后端：static 是随机前缀（必然新），none 是用户指定地址（可能已存在）。
	static, err := (&staticProvider{domain: "example.com", prefix: "dai"}).Create(context.Background())
	if err != nil {
		t.Fatalf("static Create: %v", err)
	}
	if !static.Fresh() {
		t.Fatal("static 随机地址应标记为新建")
	}

	none, err := (&noneProvider{email: "fixed@example.com"}).Create(context.Background())
	if err != nil {
		t.Fatalf("none Create: %v", err)
	}
	if none.Fresh() {
		t.Fatal("none 固定邮箱不应标记为新建")
	}
}

func TestCFMailboxFresh(t *testing.T) {
	var created int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/mailboxes":
			created++
			_, _ = io.WriteString(w, `{"emails":["daiabc@example.com"]}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/inbox":
			_, _ = io.WriteString(w, `{"mails":[]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	p := &cfProvider{base: srv.URL, adminKey: "k", domain: "example.com"}

	mb, err := p.Create(context.Background())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !mb.Fresh() {
		t.Fatal("刚创建的 CF 邮箱应标记为新建")
	}
	if created != 1 {
		t.Fatalf("应调用一次建邮箱接口，实际 %d", created)
	}

	// 复用已有地址（补跑）→ 必须预检。
	reused, ok := p.Inbox("daiabc@example.com")
	if !ok {
		t.Fatal("Inbox 应能打开同域地址")
	}
	if reused.Fresh() {
		t.Fatal("复用地址不应标记为新建（必须做存在性预检）")
	}

	// 别的域名不接受复用。
	if _, ok := p.Inbox("x@other.com"); ok {
		t.Fatal("跨域地址不应被 cf 后端接受")
	}
}
