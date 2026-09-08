package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func pipelineCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "pipeline", Short: "Manage declarative pipelines (intent text -> engineering driver run)"}
	cmd.AddCommand(pipelineListCmd())
	cmd.AddCommand(pipelineShowCmd())
	cmd.AddCommand(pipelineRunCmd())
	cmd.AddCommand(pipelineDeleteCmd())
	// 注:作者面(create)在 Web(Phase 9 起 Web 唯一配置入口);CLI 只读 + run 触发(9.4 只读族)。
	return cmd
}

func pipelineListCmd() *cobra.Command {
	var projectID string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List pipelines in a project",
		RunE: func(cmd *cobra.Command, _ []string) error {
			list, err := svc.ListPipelines(cmd.Context(), projectID)
			if err != nil {
				return err
			}
			if len(list) == 0 {
				fmt.Println("no pipelines")
				return nil
			}
			rows := [][]string{}
			for _, p := range list {
				rows = append(rows, []string{shortID(p.ID), p.Name, p.Kind, p.Risk, p.Status, strOrDashEmpty(p.Description), fmtTime(p.CreatedAt)})
			}
			printTable([]string{"ID", "NAME", "KIND", "RISK", "STATUS", "DESCRIPTION", "WHEN"}, rows)
			return nil
		},
	}
	cmd.Flags().StringVar(&projectID, "project", "", "project id")
	return cmd
}

func pipelineShowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show a pipeline",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := svc.GetPipeline(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			rows := [][]string{
				{"ID", p.ID},
				{"PROJECT", p.ProjectID},
				{"NAME", p.Name},
				{"KIND", p.Kind},
				{"RISK", p.Risk},
				{"STATUS", p.Status},
				{"DESCRIPTION", p.Description},
				{"SCHEDULE", p.Schedule},
				{"WHEN", fmtTime(p.CreatedAt)},
			}
			printTable([]string{"FIELD", "VALUE"}, rows)
			return nil
		},
	}
	return cmd
}

func pipelineRunCmd() *cobra.Command {
	var request string
	cmd := &cobra.Command{
		Use:   "run <id>",
		Short: "Trigger a pipeline run (creates one engineering task in the project directory)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			t, err := svc.RunPipeline(cmd.Context(), args[0], request)
			if err != nil {
				return err
			}
			fmt.Printf("run triggered\npipeline: %s\ntask_id: %s\nstatus: %s\n", shortID(args[0]), t.ID, t.Status)
			return nil
		},
	}
	cmd.Flags().StringVar(&request, "request", "", "run request text (overrides pipeline description when set)")
	return cmd
}

func pipelineDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a pipeline (its historical run tasks remain)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := svc.DeletePipeline(cmd.Context(), args[0]); err != nil {
				return err
			}
			fmt.Printf("pipeline %s deleted (historical run tasks kept)\n", shortID(args[0]))
			return nil
		},
	}
	return cmd
}
