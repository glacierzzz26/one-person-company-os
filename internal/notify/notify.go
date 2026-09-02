// Package notify 飞书通知(Phase 6.5,设计 §8「通知 | 飞书机器人 webhook(新增 internal/notify)」)。
//
// 只做一件事:把一段文本推给飞书自定义机器人。文本内容由调用方(service)按消息种类组装。
//
// sink 双形态(env 控制):
//   - https://… / http://…  → POST 飞书机器人文本消息(OS_FEISHU_WEBHOOK)
//   - file:///abs/path      → 追加写入本地邮箱(离线 test-double,等价 OS_ISSUE_SOURCE=fixture
//     的思路;真实飞书验收前用它与单测做确定性冒烟,不依赖外网)
//
// 安全:OS_FEISHU_SECRET 非空时按飞书自定义机器人「加签」规则附带 timestamp+sign,
// 本包与调用方均不把 secret/webhook 入日志。
package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	webhookTimeout = 3 * time.Second
	// digest 与告警文本可能较长,飞书侧对文本消息长度有限制;够用即可。
	maxRespRead = 1 << 10
)

// Notifier 推送句柄。webhook 为空时 NewFromEnv 返回 nil,调用方按 nil 安全处理(=禁用)。
type Notifier struct {
	webhook string // https://open.feishu.cn/…/hook/… 或 file:///abs/path(离线邮箱)
	secret  string // 可选:自定义机器人加签密钥
	httpc   *http.Client
}

// NewFromEnv 从环境构造:OS_FEISHU_WEBHOOK 为空 → nil(禁用);OS_FEISHU_SECRET 可选。
func NewFromEnv() *Notifier {
	wh := strings.TrimSpace(os.Getenv("OS_FEISHU_WEBHOOK"))
	if wh == "" {
		return nil
	}
	return &Notifier{
		webhook: wh,
		secret:  os.Getenv("OS_FEISHU_SECRET"),
		httpc:   &http.Client{Timeout: webhookTimeout},
	}
}

// Enabled 是否已配置推送。nil 安全。
func (n *Notifier) Enabled() bool { return n != nil }

// PostText 推送一段文本。sink=file:// 时写入邮箱文件;否则 POST 飞书机器人。
// 错误上抛由调用方记日志(best-effort),本方法绝不让通知失败拖垮业务。
// nil 接收者(nil *Notifier)视为未启用,直接返回 nil。
func (n *Notifier) PostText(ctx context.Context, text string) error {
	if n == nil {
		return nil
	}
	if strings.HasPrefix(n.webhook, "file://") {
		return n.writeMailbox(text)
	}
	return n.postFeishu(ctx, text)
}

// ---- file:// 离线邮箱(确定性冒烟/test-double) ----

// writeMailbox 把消息追加写入 file:// 邮箱,便于离线断言「收到了什么」。
func (n *Notifier) writeMailbox(text string) error {
	path := strings.TrimPrefix(n.webhook, "file://")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("notify: open mailbox %s: %w", path, err)
	}
	defer f.Close()
	header := "--- " + time.Now().Format(time.RFC3339) + " ---\n"
	if _, err := f.WriteString(header + text + "\n"); err != nil {
		return fmt.Errorf("notify: write mailbox: %w", err)
	}
	return nil
}

// ---- https/webhook POST(真实飞书) ----

// textPayload 是飞书自定义机器人文本消息的请求体;加签时附 timestamp+sign。
type textPayload struct {
	Timestamp string  `json:"timestamp,omitempty"`
	Sign      string  `json:"sign,omitempty"`
	MsgType   string  `json:"msg_type"`
	Content   content `json:"content"`
}

type content struct {
	Text string `json:"text"`
}

// feishuResp 是飞书 webhook 响应(code=0 成功;非 0 如 19021=签名错误)。
type feishuResp struct {
	Code int64  `json:"code"`
	Msg  string `json:"msg"`
}

// postFeishu 组装并 POST 一条文本消息到飞书机器人。
func (n *Notifier) postFeishu(ctx context.Context, text string) error {
	payload := textPayload{MsgType: "text", Content: content{Text: text}}
	if n.secret != "" {
		ts := strconv.FormatInt(time.Now().Unix(), 10)
		payload.Timestamp = ts
		payload.Sign = feishuSign(n.secret, ts)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("notify: marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.webhook, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("notify: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.httpc.Do(req)
	if err != nil {
		return fmt.Errorf("notify: post feishu webhook: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxRespRead))
	var fr feishuResp
	if err := json.Unmarshal(respBody, &fr); err == nil {
		if fr.Code != 0 {
			return fmt.Errorf("notify: feishu code=%d msg=%q", fr.Code, fr.Msg)
		}
		return nil
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("notify: feishu http %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return nil
}

// feishuSign 按飞书自定义机器人「加签」规则:
//   stringToSign = timestamp + "\n" + secret;sign = base64(HMAC-SHA256(secret, stringToSign))。
func feishuSign(secret, timestamp string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "\n" + secret))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}
