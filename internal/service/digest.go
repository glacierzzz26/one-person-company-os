package service

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"
)

// 昨日摘要(Phase 6.5,设计 §8「定时 | 每日摘要 9:00」)。
// 窗口 = 最近 24h:9:00 例行即"昨日 9:00 → 今日 9:00",首日/任意时刻启动都可给出有意义内容。
// 数据全部复用既有查询,Go 内按时间窗口过滤(single-operator 量级可接受)。

const digestWindowHours = 24

// digestStats 是摘要的计数输入(纯数据,便于单测构造断言文案)。
type digestStats struct {
	Title          string         // 例如 2026-09-02 09:00
	Created        int            // 窗口内新建任务
	Completed      int            // 窗口内完成
	Failed         int            // 窗口内失败
	Fused          int            // 当前熔断(conflict 达阈值待人工)
	Pending        int            // 当前待审批(waiting_approval)
	ApprovalOK     int            // 窗口内审批通过
	ApprovalReject int            // 窗口内审批驳回
	ApprovalChg    int            // 窗口内审批打回(changes)
	Decisions      int            // 窗口内 decision 自动落库
	Companies      int            // 公司在册数(多公司摘要不含具体 id 提示)
	CompanyID      string         // 单公司时给出 overview 提示用
	Ledger         map[string]int // 窗口内通道 B issue_sync 处置分布
	LedgerSeen     int            // 窗口内已同步 issue 数(去重计数)
}

// SendDailyDigest 组装并推送昨日摘要。未配置通知器 → 记日志跳过(服务化通道的摘要仍可随 server 常驻启用)。
func (s *Service) SendDailyDigest(ctx context.Context) error {
	now := time.Now()
	st, err := s.gatherDigest(ctx, now.Add(-digestWindowHours*time.Hour).Unix(), now.Unix())
	if err != nil {
		return fmt.Errorf("gather daily digest: %w", err)
	}
	text := formatDigest(st)
	if !s.NotifyEnabled() {
		log.Printf("daily digest skipped: OS_FEISHU_WEBHOOK unset (no feishu notifications)")
		return nil
	}
	if err := s.notify.PostText(ctx, text); err != nil {
		return fmt.Errorf("send daily digest: %w", err)
	}
	return nil
}

// gatherDigest 聚合摘要计数:全公司任务/审批/决策/通道 B 账本,按 [start,end] 窗口过滤。
func (s *Service) gatherDigest(ctx context.Context, start, end int64) (digestStats, error) {
	st := digestStats{Ledger: map[string]int{}, Title: time.Now().Format("2006-01-02 15:04")}

	tasks, err := s.store.ListTasks(ctx, "", "", "", 0)
	if err != nil {
		return st, err
	}
	for _, t := range tasks {
		if t.CreatedAt >= start && t.CreatedAt <= end {
			st.Created++
		}
		if t.UpdatedAt >= start && t.UpdatedAt <= end {
			switch t.Status {
			case "completed":
				st.Completed++
			case "failed":
				st.Failed++
			}
		}
		if t.Status == "waiting_approval" {
			st.Pending++
			if t.ConflictCount >= engFuseMax {
				st.Fused++
			}
		}
	}

	apps, err := s.store.ListApprovals(ctx, "")
	if err != nil {
		return st, err
	}
	for _, a := range apps {
		if a.DecidedAt == nil || *a.DecidedAt < start || *a.DecidedAt > end {
			continue
		}
		switch a.Status {
		case "approved":
			st.ApprovalOK++
		case "rejected":
			st.ApprovalReject++
		case "changes":
			st.ApprovalChg++
		}
	}

	decs, err := s.store.ListDecisions(ctx, "", "")
	if err != nil {
		return st, err
	}
	for _, d := range decs {
		if d.CreatedAt >= start && d.CreatedAt <= end {
			st.Decisions++
		}
	}

	comps, err := s.store.ListCompanies(ctx)
	if err != nil {
		return st, err
	}
	st.Companies = len(comps)
	if len(comps) == 1 {
		st.CompanyID = comps[0].ID
	}
	for _, c := range comps {
		iss, err := s.store.ListIssueSync(ctx, c.ID)
		if err != nil {
			return st, err
		}
		for _, is := range iss {
			if is.CreatedAt >= start && is.CreatedAt <= end {
				st.Ledger[is.Disposition]++
				st.LedgerSeen++
			}
		}
	}
	return st, nil
}

// formatDigest 把计数排版成飞书文本消息(纯函数,供单测断言)。
func formatDigest(st digestStats) string {
	var b strings.Builder
	fmt.Fprintf(&b, "【OS 研发日报 %s】\n", st.Title)
	fmt.Fprintf(&b, "昨日(近24h): 新建任务 %d | 完成 %d | 失败 %d | 熔断(当前待人工) %d\n",
		st.Created, st.Completed, st.Failed, st.Fused)
	fmt.Fprintf(&b, "当前待审批: %d 起(含熔断 %d)\n", st.Pending, st.Fused)
	fmt.Fprintf(&b, "昨日审批: 通过 %d / 驳回 %d / 打回 %d | 决策落库 %d 条\n",
		st.ApprovalOK, st.ApprovalReject, st.ApprovalChg, st.Decisions)
	if st.LedgerSeen > 0 {
		fmt.Fprintf(&b, "研发 intake(近24h, %d 条 issue): %s\n", st.LedgerSeen, kvSorted(st.Ledger))
	}
	comp := fmt.Sprintf("%d 家公司", st.Companies)
	if st.CompanyID != "" {
		comp = "os overview --company " + st.CompanyID
	}
	fmt.Fprintf(&b, "处理: os approval list 处理待审批;全景见 %s\n", comp)
	return b.String()
}

// kvSorted 把 "key=value" 按 key 排序拼成一行(处置分布等)。
func kvSorted(m map[string]int) string {
	if len(m) == 0 {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteString(" ")
		}
		fmt.Fprintf(&b, "%s=%d", k, m[k])
	}
	return b.String()
}
