package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func approvalCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "approval", Short: "Query and decide approval requests"}
	cmd.AddCommand(approvalListCmd())
	cmd.AddCommand(approvalShowCmd())
	cmd.AddCommand(approvalDecideCmd("approve", "Approve a pending approval; task re-enqueues for execution"))
	cmd.AddCommand(approvalDecideCmd("reject", "Reject a pending approval; task fails"))
	cmd.AddCommand(approvalDecideCmd("changes", "Request changes; task fails with note"))
	return cmd
}

func approvalListCmd() *cobra.Command {
	var status string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List approvals (optionally filtered)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			list, err := svc.ListApprovals(cmd.Context(), status)
			if err != nil {
				return err
			}
			rows := [][]string{}
			for _, a := range list {
				rows = append(rows, []string{
					a.ID, shortID(a.TaskID), a.Risk, a.Status, shortID(a.RequestedBy), fmtTime(a.CreatedAt), a.Reason,
				})
			}
			printTable([]string{"ID", "TASK", "RISK", "STATUS", "REQ_BY", "CREATED", "REASON"}, rows)
			return nil
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "filter by status (pending/approved/rejected/changes)")
	return cmd
}

func approvalShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show approval details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := svc.GetApproval(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			decided := "-"
			if a.DecidedAt != nil {
				decided = fmtTime(*a.DecidedAt)
			}
			fmt.Printf("ID:        %s\nTask:      %s\nRisk:      %s\nStatus:    %s\nRequested: %s\nDecidedBy: %s\nNote:      %s\nCreated:   %s\nDecided:   %s\nReason:    %s\n",
				a.ID, a.TaskID, a.Risk, a.Status, shortID(a.RequestedBy),
				strOrDashEmpty(a.DecidedBy), strOrDashEmpty(a.DecisionNote),
				fmtTime(a.CreatedAt), decided, a.Reason)
			return nil
		},
	}
}

func approvalDecideCmd(decision, usage string) *cobra.Command {
	var note string
	cmd := &cobra.Command{
		Use:   decision + " <id>",
		Short: usage,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := svc.DecideApproval(cmd.Context(), args[0], decision, note)
			if err != nil {
				return err
			}
			fmt.Printf("approval %s status=%s (task %s)\n", shortID(a.ID), a.Status, shortID(a.TaskID))
			return nil
		},
	}
	cmd.Flags().StringVar(&note, "note", "", "decision note")
	return cmd
}
