package cli

import (
	"fmt"

	"github.com/glacierzzz26/one-person-company-os/internal/service"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

func taskCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "task", Short: "Manage tasks"}
	cmd.AddCommand(taskCreateCmd())
	cmd.AddCommand(taskListCmd())
	cmd.AddCommand(taskShowCmd())
	cmd.AddCommand(taskRunCmd())
	return cmd
}

func taskCreateCmd() *cobra.Command {
	var (
		companyID, title, description, risk, toolName string
		capabilityID, workflowID, agentID             string
		workspace                                     string
		maxAttempts, timeoutSec                       int64
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a task (status pending, qstatus ready)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if companyID == "" || title == "" {
				return fmt.Errorf("--company and --title are required")
			}
			p := service.TaskParams{
				CompanyID:   companyID,
				Title:       title,
				Description: description,
				ToolName:    toolName,
				Risk:        risk,
				MaxAttempts: maxAttempts,
				TimeoutSec:  timeoutSec,
				Workspace:   workspace,
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
	cmd.Flags().StringVar(&description, "description", "", "task description (tool command: shell command / git subcommand / write <path>... / read <path>)")
	cmd.Flags().StringVar(&toolName, "tool", "shell", "tool name: shell|git|file-read|file-write")
	cmd.Flags().StringVar(&risk, "risk", "low", "low/medium/high")
	cmd.Flags().Int64Var(&maxAttempts, "max-attempts", 1, "max attempts before terminal failed")
	cmd.Flags().Int64Var(&timeoutSec, "timeout", 0, "execution timeout in seconds (0 = none)")
	cmd.Flags().StringVar(&workspace, "workspace", "", "task workspace directory")
	cmd.Flags().StringVar(&capabilityID, "capability", "", "capability id")
	cmd.Flags().StringVar(&workflowID, "workflow", "", "workflow id")
	cmd.Flags().StringVar(&agentID, "agent", "", "agent id")
	return cmd
}

func taskListCmd() *cobra.Command {
	var companyID, status, risk string
	var attemptMin int64
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List tasks (optionally filtered)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			list, err := svc.ListTasks(cmd.Context(), companyID, status, risk, attemptMin)
			if err != nil {
				return err
			}
			rows := [][]string{}
			for _, t := range list {
				rows = append(rows, []string{
					shortID(t.ID), t.Status, t.Risk, fmt.Sprintf("%d", t.Attempt),
					firstLine(t.Title), strOrDash(t.AgentID), fmtTime(t.CreatedAt),
				})
			}
			printTable([]string{"ID", "STATUS", "RISK", "ATT", "TITLE", "AGENT", "CREATED"}, rows)
			return nil
		},
	}
	cmd.Flags().StringVar(&companyID, "company", "", "filter by company id")
	cmd.Flags().StringVar(&status, "status", "", "filter by status")
	cmd.Flags().StringVar(&risk, "risk", "", "filter by risk")
	cmd.Flags().Int64Var(&attemptMin, "attempt", -1, "filter by attempt >= n (-1 = no filter)")
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
			fmt.Printf("ID:           %s\nTitle:        %s\nStatus:       %s\nQueueStatus:  %s\nRisk:         %s\nAttempt:      %d\nMaxAttempts:  %d\nCompany:      %s\nCapability:   %s\nWorkflow:     %s\nAgent:        %s\nWorkspace:    %s\nDescription:  %s\nLastError:    %s\nCreated:      %s\n",
				t.ID, t.Title, t.Status, t.QStatus, t.Risk, t.Attempt, t.MaxAttempts,
				shortID(t.CompanyID), strOrDash(t.CapabilityID), strOrDash(t.WorkflowID),
				strOrDash(t.AgentID), strOrDashEmpty(t.WorkspacePath), firstLine(t.Description),
				strOrDashEmpty(t.LastError), fmtTime(t.CreatedAt))
			if t.Status == "completed" && t.Result != "" {
				fmt.Printf("Result:\n%s\n", t.Result)
			}
			return nil
		},
	}
}

func taskRunCmd() *cobra.Command {
	var worker string
	cmd := &cobra.Command{
		Use:   "run <id>",
		Short: "Run a single task (claim and execute)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if worker == "" {
				worker = "worker-" + uuid.NewString()[:8]
			}
			if err := svc.ExecuteTask(cmd.Context(), worker, args[0]); err != nil {
				return err
			}
			t, err := svc.GetTask(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			fmt.Printf("task %s status=%s attempt=%d\n", shortID(t.ID), t.Status, t.Attempt)
			if t.Status == "completed" {
				fmt.Printf("result:\n%s\n", t.Result)
			} else if t.Status == "failed" {
				fmt.Printf("error: %s\n", t.LastError)
			} else if t.Status == "pending" {
				fmt.Printf("requeued for retry\n")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&worker, "worker", "", "worker id (default: worker-<rand>)")
	return cmd
}

func firstLine(s string) string {
	for i, c := range s {
		if c == '\n' {
			return s[:i]
		}
	}
	return s
}

func strOrDashEmpty(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func strOrDash(p *string) string {
	if p == nil {
		return "-"
	}
	return shortID(*p)
}
