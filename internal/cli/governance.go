package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func policyCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "policy", Short: "Manage policies"}
	cmd.AddCommand(policyAddCmd())
	cmd.AddCommand(policyListCmd())
	return cmd
}

func policyAddCmd() *cobra.Command {
	var companyID, name, kind, statement string
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a policy",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if companyID == "" || name == "" || kind == "" || statement == "" {
				return fmt.Errorf("--company, --name, --kind and --statement are required")
			}
			if kind != "allow" && kind != "deny" {
				return fmt.Errorf("--kind must be allow or deny")
			}
			p, err := svc.CreatePolicy(cmd.Context(), companyID, name, kind, statement)
			if err != nil {
				return err
			}
			fmt.Printf("added policy %q\nid: %s\n", p.Name, p.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&companyID, "company", "", "company id")
	cmd.Flags().StringVar(&name, "name", "", "policy name")
	cmd.Flags().StringVar(&kind, "kind", "", "allow or deny")
	cmd.Flags().StringVar(&statement, "statement", "", "policy statement")
	return cmd
}

func policyListCmd() *cobra.Command {
	var companyID string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List policies of a company",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if companyID == "" {
				return fmt.Errorf("--company is required")
			}
			list, err := svc.ListPolicies(cmd.Context(), companyID)
			if err != nil {
				return err
			}
			rows := [][]string{}
			for _, p := range list {
				enabled := "on"
				if !p.Enabled {
					enabled = "off"
				}
				rows = append(rows, []string{shortID(p.ID), p.Name, p.Kind, enabled, p.Statement})
			}
			printTable([]string{"ID", "NAME", "KIND", "ENABLED", "STATEMENT"}, rows)
			return nil
		},
	}
	cmd.Flags().StringVar(&companyID, "company", "", "company id")
	return cmd
}

func permissionCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "permission", Short: "Manage permissions (stored only in Phase 0)"}
	cmd.AddCommand(permissionAddCmd())
	cmd.AddCommand(permissionListCmd())
	return cmd
}

func permissionAddCmd() *cobra.Command {
	var policyID, subject, action, resource string
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a permission to a policy",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if policyID == "" || subject == "" || action == "" || resource == "" {
				return fmt.Errorf("--policy, --subject, --action and --resource are required")
			}
			p, err := svc.CreatePermission(cmd.Context(), policyID, subject, action, resource)
			if err != nil {
				return err
			}
			fmt.Printf("added permission %s %s %s\nid: %s\n", p.Subject, p.Action, p.Resource, p.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&policyID, "policy", "", "policy id")
	cmd.Flags().StringVar(&subject, "subject", "", "role or agent id")
	cmd.Flags().StringVar(&action, "action", "", "read/write/execute/network/secret/production/admin")
	cmd.Flags().StringVar(&resource, "resource", "", "target resource")
	return cmd
}

func permissionListCmd() *cobra.Command {
	var policyID string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List permissions of a policy",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if policyID == "" {
				return fmt.Errorf("--policy is required")
			}
			list, err := svc.ListPermissions(cmd.Context(), policyID)
			if err != nil {
				return err
			}
			rows := [][]string{}
			for _, p := range list {
				rows = append(rows, []string{shortID(p.ID), p.Subject, p.Action, p.Resource})
			}
			printTable([]string{"ID", "SUBJECT", "ACTION", "RESOURCE"}, rows)
			return nil
		},
	}
	cmd.Flags().StringVar(&policyID, "policy", "", "policy id")
	return cmd
}
