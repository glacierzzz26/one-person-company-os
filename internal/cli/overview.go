package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

// overviewCmd 一人操作台:全景聚合,突出「需要人决策的事项」(待审批)。
func overviewCmd() *cobra.Command {
	var companyID string
	cmd := &cobra.Command{
		Use:   "overview",
		Short: "Company-wide operational overview (one-person dashboard)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if companyID == "" {
				return fmt.Errorf("--company is required")
			}
			ov, err := svc.Overview(cmd.Context(), companyID)
			if err != nil {
				return err
			}

			fmt.Printf("== Company: %s ==\n", ov.Company.Name)
			fmt.Printf("Vision: %s\n\n", ov.Company.Vision)

			fmt.Println("== Capabilities ==")
			capRows := [][]string{}
			for _, c := range ov.Capabilities {
				capRows = append(capRows, []string{c.Code, c.Name, fmt.Sprintf("%d", c.AgentCount)})
			}
			printTable([]string{"CODE", "NAME", "AGENTS"}, capRows)
			fmt.Println()

			fmt.Println("== Workflows ==")
			wfRows := [][]string{}
			for _, w := range ov.Workflows {
				wfRows = append(wfRows, []string{
					shortID(w.Workflow.ID), w.Workflow.Name, statusSummary(w.Statuses),
				})
			}
			printTable([]string{"ID", "NAME", "TASK STATUS"}, wfRows)
			fmt.Println()

			fmt.Println("== Pending Approvals (需要你决策) ==")
			if len(ov.PendingApprovals) == 0 {
				fmt.Println("  (none)")
			} else {
				apRows := [][]string{}
				for _, a := range ov.PendingApprovals {
					apRows = append(apRows, []string{
						a.Approval.ID, shortID(a.Approval.TaskID), a.Approval.Risk, a.TaskTitle, a.Approval.Reason,
					})
				}
				printTable([]string{"APPROVAL", "TASK", "RISK", "TITLE", "REASON"}, apRows)
			}
			fmt.Println()

			fmt.Println("== Recent Decisions ==")
			decRows := [][]string{}
			for _, d := range ov.RecentDecisions {
				decRows = append(decRows, []string{shortID(d.ID), d.Kind, d.Status, d.Title, fmtTime(d.CreatedAt)})
			}
			printTable([]string{"ID", "KIND", "STATUS", "TITLE", "WHEN"}, decRows)
			fmt.Println()

			fmt.Println("== Recent Tasks ==")
			taskRows := [][]string{}
			for _, t := range ov.RecentTasks {
				wf := ""
				if t.WorkflowID != nil {
					wf = shortID(*t.WorkflowID)
				}
				taskRows = append(taskRows, []string{
					shortID(t.ID), t.Status, t.Risk, wf, firstLine(t.Title), fmtTime(t.CreatedAt),
				})
			}
			printTable([]string{"ID", "STATUS", "RISK", "WORKFLOW", "TITLE", "WHEN"}, taskRows)
			fmt.Println()

			fmt.Println("== Memory Highlights ==")
			memRows := [][]string{}
			for _, m := range ov.MemoryHighlights {
				memRows = append(memRows, []string{shortID(m.ID), m.Type, m.Title, firstLine(m.Content)})
			}
			printTable([]string{"ID", "TYPE", "TITLE", "CONTENT"}, memRows)

			return nil
		},
	}
	cmd.Flags().StringVar(&companyID, "company", "", "company id")
	return cmd
}

// statusSummary 把 workflow 的任务状态计数压成 "pending=2 completed=3" 一行。
func statusSummary(m map[string]int64) string {
	if len(m) == 0 {
		return "(no tasks)"
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := ""
	for i, k := range keys {
		if i > 0 {
			out += " "
		}
		out += fmt.Sprintf("%s=%d", k, m[k])
	}
	return out
}
