package cli

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

func queueCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "queue", Short: "Task queue worker"}
	cmd.AddCommand(queueWorkCmd())
	return cmd
}

func queueWorkCmd() *cobra.Command {
	var workerID string
	var limit int
	cmd := &cobra.Command{
		Use:   "work",
		Short: "Run a worker: consume READY tasks until drained or --limit reached",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if workerID == "" {
				workerID = "worker-" + uuid.NewString()[:8]
			}
			recovered, err := svc.RecoverLeasedTasks(cmd.Context())
			if err != nil {
				return err
			}
			if recovered > 0 {
				fmt.Printf("recovered %d orphaned leased task(s)\n", recovered)
			}
			executed := 0
			for limit <= 0 || executed < limit {
				id, ok, err := svc.LeaseAndExecute(cmd.Context(), workerID)
				if err != nil {
					return err
				}
				if !ok {
					break
				}
				executed++
				fmt.Printf("%s  %s\n", time.Now().Format("15:04:05"), shortID(id))
			}
			fmt.Printf("worker %s done: %d task(s) executed\n", workerID, executed)
			return nil
		},
	}
	cmd.Flags().StringVar(&workerID, "worker", "", "worker id (default: worker-<rand>)")
	cmd.Flags().IntVar(&limit, "limit", 0, "max tasks to execute (0 = until drained)")
	return cmd
}
