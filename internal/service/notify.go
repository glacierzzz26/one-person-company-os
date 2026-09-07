package service

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/glacierzzz26/one-person-company-os/internal/notify"
	"github.com/glacierzzz26/one-person-company-os/internal/task"
)

// 通知消息种类(Phase 6.5,设计 §8「通知 | 飞书机器人 webhook」)。
// 事件点即时发:requestApproval(熔断 / planner ask / 高险审批门唯一汇点)转 waiting_approval 即推送,
// 保证熔断 ≤1 分钟触达;每日摘要/待审批 recap 由 server 定时发(见 digest.go)。
const (
	notifyKindFuse     = "熔断"  // 工程任务评审连续驳回熔断
	notifyKindApproval = "待审批" // 非熔断的等待人工(ask/高险门)
)

// notifyApproval 在任务转 waiting_approval 时即时通知人工(Phase 6.5)。
// Phase 9.3:按任务归属公司机密 feishu_webhook 路由(notifyCompany);task 无公司归属 → 静默跳过
// (company 隔离下无通道)。best-effort:发送失败仅记日志,绝不回传错误影响审批流。
func (s *Service) notifyApproval(ctx context.Context, t task.Task, approvalID, reason string) {
	if t.CompanyID == "" {
		return
	}
	fuse := strings.HasPrefix(reason, "engineering fuse")
	if err := s.notifyCompany(ctx, t.CompanyID, approvalAlertText(t, approvalID, reason, fuse)); err != nil {
		kind := notifyKindApproval
		if fuse {
			kind = notifyKindFuse
		}
		log.Printf("notify [%s] task %s: %v", kind, short8(t.ID), err)
	}
}

// notifyCompany 把文本推给公司配置的飞书通知。公司无 feishu_webhook 机密 → 静默跳过(返回 nil);
// 有 webhook + 可选 feishu_secret(加签)→ notify.New(...).PostText。仅公司机密,不读 env(契约 §3.3)。
func (s *Service) notifyCompany(ctx context.Context, companyID, text string) error {
	webhook, ok, err := s.OpenSecretCurrent(ctx, companyID, SecretFeishuWebhook)
	if err != nil {
		return err
	}
	if !ok || webhook == "" {
		return nil
	}
	sec, _, _ := s.OpenSecretCurrent(ctx, companyID, SecretFeishuSecret)
	return notify.New(webhook, sec).PostText(ctx, text)
}

// approvalAlertText 组装熔断/待审批告警文本(飞书文本消息,多行)。
func approvalAlertText(t task.Task, approvalID, reason string, fuse bool) string {
	var b strings.Builder
	if fuse {
		fmt.Fprintf(&b, "【研发熔断】task %s\n", short8(t.ID))
	} else {
		fmt.Fprintf(&b, "【待审批】task %s\n", short8(t.ID))
	}
	fmt.Fprintf(&b, "任务: %s\n", firstLine(t.Title))
	fmt.Fprintf(&b, "risk=%s", t.Risk)
	if t.RoundNo > 0 || t.ConflictCount > 0 {
		fmt.Fprintf(&b, " | round=%d conflict=%d", t.RoundNo, t.ConflictCount)
	}
	fmt.Fprintf(&b, "\n原因: %s\n", firstLine(reason))
	if fuse {
		fmt.Fprintf(&b, "评审连续驳回达阈值,已转人工审批。\n")
	}
	fmt.Fprintf(&b, "处理: os approval list → os approval approve|reject|changes %s\n", short8(approvalID))
	return b.String()
}
