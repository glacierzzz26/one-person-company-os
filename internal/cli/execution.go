package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func executionCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "execution", Short: "Query execution records"}
	cmd.AddCommand(executionListCmd())
	cmd.AddCommand(executionShowCmd())
	return cmd
}

func executionListCmd() *cobra.Command {
	var taskID, status string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List executions (optionally filtered)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			list, err := svc.ListExecutions(cmd.Context(), taskID, status)
			if err != nil {
				return err
			}
			rows := [][]string{}
			for _, e := range list {
				rows = append(rows, []string{
					shortID(e.ID), shortID(e.TaskID), e.WorkerID,
					fmt.Sprintf("%d", e.Attempt), e.Status, fmtTime(e.StartedAt),
				})
			}
			printTable([]string{"ID", "TASK", "WORKER", "ATT", "STATUS", "STARTED"}, rows)
			return nil
		},
	}
	cmd.Flags().StringVar(&taskID, "task", "", "filter by task id")
	cmd.Flags().StringVar(&status, "status", "", "filter by status")
	return cmd
}

func executionShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show execution details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := svc.GetExecution(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			finished := "-"
			if e.FinishedAt != nil {
				finished = fmtTime(*e.FinishedAt)
			}
			fmt.Printf("ID:       %s\nTask:     %s\nWorker:   %s\nAttempt:  %d\nStatus:   %s\nStarted:  %s\nFinished: %s\nError:    %s\nResult:\n%s\n",
				e.ID, e.TaskID, e.WorkerID, e.Attempt, e.Status,
				fmtTime(e.StartedAt), finished, strOrDashEmpty(e.Error), e.Result)
			return nil
		},
	}
}
