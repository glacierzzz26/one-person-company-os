package service

import (
	"strings"
	"testing"
)

// TestFormatDigestEmpty 空摘要:各计数行为 0,无 intake 行,提示含 approval list。
func TestFormatDigestEmpty(t *testing.T) {
	text := formatDigest(digestStats{Title: "2026-09-02 09:00", Ledger: map[string]int{}})
	for _, want := range []string{
		"【OS 研发日报 2026-09-02 09:00】",
		"新建任务 0 | 完成 0 | 失败 0 | 熔断(当前待人工) 0",
		"当前待审批: 0 起(含熔断 0)",
		"昨日审批: 通过 0 / 驳回 0 / 打回 0 | 决策落库 0 条",
		"os approval list",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("digest missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "研发 intake") {
		t.Errorf("empty ledger should omit intake line:\n%s", text)
	}
}

// TestFormatDigestFull 计数 + 处置分布:intake 行按 key 排序,单公司提示带 --company id。
func TestFormatDigestFull(t *testing.T) {
	st := digestStats{
		Title: "2026-09-02 09:00", Created: 5, Completed: 3, Failed: 1, Fused: 2, Pending: 4,
		ApprovalOK: 2, ApprovalReject: 1, ApprovalChg: 1, Decisions: 3,
		Companies: 1, CompanyID: "company-x",
		Ledger:    map[string]int{"merge": 1, "direct_work": 2, "skip": 1}, LedgerSeen: 4,
	}
	text := formatDigest(st)
	for _, want := range []string{
		"新建任务 5 | 完成 3 | 失败 1 | 熔断(当前待人工) 2",
		"当前待审批: 4 起(含熔断 2)",
		"通过 2 / 驳回 1 / 打回 1 | 决策落库 3 条",
		"研发 intake(近24h, 4 条 issue): direct_work=2 merge=1 skip=1",
		"os overview --company company-x",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("digest missing %q:\n%s", want, text)
		}
	}
}
