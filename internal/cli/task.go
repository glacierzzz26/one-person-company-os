package cli

import (
	"fmt"

	"github.com/glacierzzz26/one-person-company-os/internal/service"
	"github.com/spf13/cobra"
)

func taskCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "task", Short: "Manage tasks"}
	cmd.AddCommand(taskCreateCmd())
	cmd.AddCommand(taskListCmd())
	cmd.AddCommand(taskShowCmd())
	return cmd
}

func taskCreateCmd() *cobra.Command {
	var (
		companyID, title, description, risk string
		capabilityID, workflowID, agentID   string
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a task (status fixed to pending in Phase 0)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if companyID == "" || title == "" {
				return fmt.Errorf("--company and --title are required")
			}
			p := service.TaskParams{
				CompanyID:   companyID,
				Title:       title,
				Description: description,
				Risk:        risk,
			}
			if capabilityID != "" {
				p.CapabilityID = &capabilityID
			}
			if workflowID != "" {
				p.WorkflowID = &workflowID
			}
			if agentID != "" {
				p.AgentID = &agentID
			}
			t, err := svc.CreateTask(cmd.Context(), p)
			if err != nil {
				return err
			}
			fmt.Printf("created task %q status=%s risk=%s\nid: %s\n", t.Title, t.Status, t.Risk, t.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&companyID, "company", "", "company id")
	cmd.Flags().StringVar(&title, "title", "", "task title")
	cmd.Flags().StringVar(&description, "description", "", "task description")
	cmd.Flags().StringVar(&risk, "risk", "low", "low/medium/high")
	cmd.Flags().StringVar(&capabilityID, "capability", "", "capability id")
	cmd.Flags().StringVar(&workflowID, "workflow", "", "workflow id")
	cmd.Flags().StringVar(&agentID, "agent", "", "agent id")
	return cmd
}

func taskListCmd() *cobra.Command {
	var companyID, status string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List tasks (optionally filtered)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			list, err := svc.ListTasks(cmd.Context(), companyID, status)
			if err != nil {
				return err
			}
			rows := [][]string{}
			for _, t := range list {
				rows = append(rows, []string{
					shortID(t.ID), t.Status, t.Risk, t.Title,
					strOrDash(t.AgentID), fmtTime(t.CreatedAt),
				})
			}
			printTable([]string{"ID", "STATUS", "RISK", "TITLE", "AGENT", "CREATED"}, rows)
			return nil
		},
	}
	cmd.Flags().StringVar(&companyID, "company", "", "filter by company id")
	cmd.Flags().StringVar(&status, "status", "", "filter by status")
	return cmd
}

func taskShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show task details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			t, err := svc.GetTask(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			fmt.Printf("ID:          %s\nTitle:       %s\nStatus:      %s\nRisk:        %s\nCompany:     %s\nCapability:  %s\nWorkflow:    %s\nAgent:       %s\nDescription: %s\nCreated:     %s\n",
				t.ID, t.Title, t.Status, t.Risk, shortID(t.CompanyID),
				strOrDash(t.CapabilityID), strOrDash(t.WorkflowID), strOrDash(t.AgentID),
				t.Description, fmtTime(t.CreatedAt))
			return nil
		},
	}
}

func strOrDash(p *string) string {
	if p == nil {
		return "-"
	}
	return shortID(*p)
}
