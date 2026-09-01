package cli

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

func capabilityCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "capability", Short: "Manage capabilities"}
	cmd.AddCommand(capabilityAddCmd())
	cmd.AddCommand(capabilityListCmd())
	return cmd
}

func capabilityAddCmd() *cobra.Command {
	var companyID, code, name, description string
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a capability to a company",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if companyID == "" || code == "" || name == "" {
				return fmt.Errorf("--company, --code and --name are required")
			}
			c, err := svc.CreateCapability(cmd.Context(), companyID, code, name, description)
			if err != nil {
				return err
			}
			fmt.Printf("added capability %q\nid: %s\n", c.Name, c.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&companyID, "company", "", "company id")
	cmd.Flags().StringVar(&code, "code", "", "capability code (e.g. engineering)")
	cmd.Flags().StringVar(&name, "name", "", "capability name")
	cmd.Flags().StringVar(&description, "description", "", "description")
	return cmd
}

func capabilityListCmd() *cobra.Command {
	var companyID string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List capabilities of a company",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if companyID == "" {
				return fmt.Errorf("--company is required")
			}
			list, err := svc.ListCapabilities(cmd.Context(), companyID)
			if err != nil {
				return err
			}
			rows := [][]string{}
			for _, c := range list {
				rows = append(rows, []string{shortID(c.ID), c.Code, c.Name, c.Description})
			}
			printTable([]string{"ID", "CODE", "NAME", "DESCRIPTION"}, rows)
			return nil
		},
	}
	cmd.Flags().StringVar(&companyID, "company", "", "company id")
	return cmd
}

func agentCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "agent", Short: "Manage agents"}
	cmd.AddCommand(agentAddCmd())
	cmd.AddCommand(agentListCmd())
	return cmd
}

func agentAddCmd() *cobra.Command {
	var capabilityID, name, role string
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add an agent to a capability",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if capabilityID == "" || name == "" || role == "" {
				return fmt.Errorf("--capability, --name and --role are required")
			}
			a, err := svc.CreateAgent(cmd.Context(), capabilityID, name, role)
			if err != nil {
				return err
			}
			fmt.Printf("added agent %q\nid: %s\n", a.Name, a.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&capabilityID, "capability", "", "capability id")
	cmd.Flags().StringVar(&name, "name", "", "agent name")
	cmd.Flags().StringVar(&role, "role", "", "agent role (architect/coding/qa/review)")
	return cmd
}

func agentListCmd() *cobra.Command {
	var capabilityID string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List agents of a capability",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if capabilityID == "" {
				return fmt.Errorf("--capability is required")
			}
			list, err := svc.ListAgents(cmd.Context(), capabilityID)
			if err != nil {
				return err
			}
			rows := [][]string{}
			for _, a := range list {
				rows = append(rows, []string{shortID(a.ID), a.Name, a.Role, a.ModelHint})
			}
			printTable([]string{"ID", "NAME", "ROLE", "MODEL"}, rows)
			return nil
		},
	}
	cmd.Flags().StringVar(&capabilityID, "capability", "", "capability id")
	return cmd
}

func workflowCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "workflow", Short: "Manage workflows"}
	cmd.AddCommand(workflowAddCmd())
	cmd.AddCommand(workflowListCmd())
	cmd.AddCommand(workflowRunCmd())
	return cmd
}

func workflowAddCmd() *cobra.Command {
	var companyID, name, description, definition string
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a workflow (definition stored, not executed in Phase 0)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if companyID == "" || name == "" {
				return fmt.Errorf("--company and --name are required")
			}
			w, err := svc.CreateWorkflow(cmd.Context(), companyID, name, description, definition)
			if err != nil {
				return err
			}
			fmt.Printf("added workflow %q\nid: %s\n", w.Name, w.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&companyID, "company", "", "company id")
	cmd.Flags().StringVar(&name, "name", "", "workflow name")
	cmd.Flags().StringVar(&description, "description", "", "description")
	cmd.Flags().StringVar(&definition, "definition", "", "workflow definition JSON (stored only)")
	return cmd
}

func workflowListCmd() *cobra.Command {
	var companyID string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List workflows of a company",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if companyID == "" {
				return fmt.Errorf("--company is required")
			}
			list, err := svc.ListWorkflows(cmd.Context(), companyID)
			if err != nil {
				return err
			}
			rows := [][]string{}
			for _, w := range list {
				rows = append(rows, []string{shortID(w.ID), w.Name, w.Description, fmtTime(w.CreatedAt)})
			}
			printTable([]string{"ID", "NAME", "DESCRIPTION", "CREATED"}, rows)
			return nil
		},
	}
	cmd.Flags().StringVar(&companyID, "company", "", "company id")
	return cmd
}

func workflowRunCmd() *cobra.Command {
	var worker string
	cmd := &cobra.Command{
		Use:   "run <id>",
		Short: "Run a workflow: create and execute node tasks in order (aborts on node failure)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if worker == "" {
				worker = "worker-" + uuid.NewString()[:8]
			}
			if err := svc.RunWorkflow(cmd.Context(), worker, args[0]); err != nil {
				return err
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&worker, "worker", "", "worker id (default: worker-<rand>)")
	return cmd
}
