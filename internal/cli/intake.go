package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func intakeCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "intake", Short: "R&D intake (channel B: GitHub issues -> engineering tasks)"}
	cmd.AddCommand(intakeSyncCmd())
	return cmd
}

// intakeSyncCmd 单次手动同步(server 轮询内部亦走 SyncRepos;无真实 GitHub token 时
// 用 OS_ISSUE_SOURCE=fixture + OS_FIXTURE_ISSUES 离线冒烟)。
func intakeSyncCmd() *cobra.Command {
	var companyID string
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Pull open issues from registered repos and triage into the queue",
		RunE: func(cmd *cobra.Command, _ []string) error {
			results, err := svc.SyncRepos(cmd.Context(), companyID)
			total := 0
			created := 0
			for _, r := range results {
				rows := [][]string{}
				for _, d := range []string{"direct_work", "ask", "skip", "merge"} {
					n := r.ByDisp[d]
					if n == 0 {
						continue
					}
					rows = append(rows, []string{r.Repo, d, fmt.Sprintf("%d", n)})
				}
				if len(rows) > 0 {
					printTable([]string{"REPO", "DISPOSITION", "COUNT"}, rows)
				}
				fmt.Printf("repo %s: %d issue(s) seen, %d task(s) created (queue ready)\n",
					r.Repo, r.IssuesSeen, len(r.CreatedTasks))
				total += r.IssuesSeen
				created += len(r.CreatedTasks)
			}
			fmt.Printf("intake sync: %d issue(s), %d engineering task(s) in queue\n", total, created)
			return err
		},
	}
	cmd.Flags().StringVar(&companyID, "company", "", "company id (default: all companies' repos)")
	return cmd
}
