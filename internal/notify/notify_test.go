package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// decodeFeishuBody 把收到的请求体还原成 textPayload。
func decodeFeishuBody(t *testing.T, body io.Reader) textPayload {
	t.Helper()
	var p textPayload
	if err := json.NewDecoder(body).Decode(&p); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return p
}

// TestHTTPPostText 未加签的纯文本推送:msg_type=text,content.text 原样,无 timestamp/sign。
func TestHTTPPostText(t *testing.T) {
	var got textPayload
	var method, ctype string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		ctype = r.Header.Get("Content-Type")
		got = decodeFeishuBody(t, r.Body)
		_, _ = w.Write([]byte(`{"code":0,"msg":"success"}`))
	}))
	defer srv.Close()

	n := &Notifier{webhook: srv.URL, httpc: srv.Client()}
	if err := n.PostText(context.Background(), "hello feishu"); err != nil {
		t.Fatalf("PostText: %v", err)
	}
	if method != http.MethodPost {
		t.Errorf("method = %s, want POST", method)
	}
	if ctype != "application/json" {
		t.Errorf("content-type = %s, want application/json", ctype)
	}
	if got.MsgType != "text" || got.Content.Text != "hello feishu" {
		t.Errorf("payload = %+v, want msg_type=text text=hello feishu", got)
	}
	if got.Timestamp != "" || got.Sign != "" {
		t.Errorf("plain send unexpectedly signed: %+v", got)
	}
}

// TestHTTPPostSigned 加签推送:body 带 timestamp+sign,且 sign 与重算一致。
func TestHTTPPostSigned(t *testing.T) {
	const secret = "feishu-test-secret"
	var got textPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = decodeFeishuBody(t, r.Body)
		_, _ = w.Write([]byte(`{"code":0,"msg":"success"}`))
	}))
	defer srv.Close()

	n := &Notifier{webhook: srv.URL, secret: secret, httpc: srv.Client()}
	if err := n.PostText(context.Background(), "signed hello"); err != nil {
		t.Fatalf("PostText: %v", err)
	}
	if got.Timestamp == "" || got.Sign == "" {
		t.Fatalf("signed send missing timestamp/sign: %+v", got)
	}
	if want := feishuSign(secret, got.Timestamp); got.Sign != want {
		t.Errorf("sign = %s, want %s", got.Sign, want)
	}
}

// TestFeishuRespError 飞书返回非 0 code(如签名错误)→ 上抛错误。
func TestFeishuRespError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":19021,"msg":"sign match fail"}`))
	}))
	defer srv.Close()

	n := &Notifier{webhook: srv.URL, httpc: srv.Client()}
	err := n.PostText(context.Background(), "x")
	if err == nil || !strings.Contains(err.Error(), "19021") {
		t.Fatalf("want feishu code error, got %v", err)
	}
}

// TestFileMailbox file:// 邮箱:写入后可读到推送文本(离线确定性冒烟通道)。
func TestFileMailbox(t *testing.T) {
	dir := t.TempDir()
	mailbox := filepath.Join(dir, "fm.txt")
	n := &Notifier{webhook: "file://" + mailbox}
	if err := n.PostText(context.Background(), "line1\nline2"); err != nil {
		t.Fatalf("PostText: %v", err)
	}
	b, err := os.ReadFile(mailbox)
	if err != nil {
		t.Fatalf("read mailbox: %v", err)
	}
	s := string(b)
	if !strings.Contains(s, "line1") || !strings.Contains(s, "line2") {
		t.Errorf("mailbox content missing text:\n%s", s)
	}
}

// TestNilNotifierDisabled nil 通知器(nil *Notifier)与未配置环境均为禁用,PostText 无副作用。
func TestNilNotifierDisabled(t *testing.T) {
	var n *Notifier
	if n.Enabled() {
		t.Error("nil notifier should not be enabled")
	}
	if err := n.PostText(context.Background(), "should be no-op"); err != nil {
		t.Errorf("nil PostText: %v", err)
	}

	t.Setenv("OS_FEISHU_WEBHOOK", "")
	t.Setenv("OS_FEISHU_SECRET", "")
	if got := NewFromEnv(); got != nil {
		t.Errorf("NewFromEnv with empty webhook = %+v, want nil", got)
	}
}
