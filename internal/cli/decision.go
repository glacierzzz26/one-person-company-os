package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func decisionCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "decision", Short: "Manage company decisions (recorded)"}
	cmd.AddCommand(decisionAddCmd())
	cmd.AddCommand(decisionListCmd())
	cmd.AddCommand(decisionShowCmd())
	return cmd
}

func decisionAddCmd() *cobra.Command {
	var companyID, kind, status, title, body string
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Record a human decision (goal/strategy/policy_change/capital/manual)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if companyID == "" || kind == "" || title == "" {
				return fmt.Errorf("--company, --kind and --title are required")
			}
			if status == "" {
				status = "made"
			}
			d, err := svc.CreateDecision(cmd.Context(), companyID, kind, status, title, body)
			if err != nil {
				return err
			}
			fmt.Printf("decision recorded (%s/%s)\nid: %s\n", d.Kind, d.Status, d.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&companyID, "company", "", "company id")
	cmd.Flags().StringVar(&kind, "kind", "", "decision kind (approval/goal/strategy/policy_change/capital/manual)")
	cmd.Flags().StringVar(&status, "status", "made", "status (made/pending/executed)")
	cmd.Flags().StringVar(&title, "title", "", "title")
	cmd.Flags().StringVar(&body, "body", "", "body")
	return cmd
}

func decisionListCmd() *cobra.Command {
	var companyID, kind string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List decisions (optionally by kind)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if companyID == "" {
				return fmt.Errorf("--company is required")
			}
			list, err := svc.ListDecisions(cmd.Context(), companyID, kind)
			if err != nil {
				return err
			}
			rows := [][]string{}
			for _, d := range list {
				rows = append(rows, []string{
					shortID(d.ID), d.Kind, d.Status, d.Title, firstLine(d.Body), d.Source, fmtTime(d.CreatedAt),
				})
			}
			printTable([]string{"ID", "KIND", "STATUS", "TITLE", "BODY", "SOURCE", "WHEN"}, rows)
			return nil
		},
	}
	cmd.Flags().StringVar(&companyID, "company", "", "company id")
	cmd.Flags().StringVar(&kind, "kind", "", "filter by kind")
	return cmd
}

func decisionShowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show a decision",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := svc.GetDecision(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			rows := [][]string{
				{"ID", d.ID},
				{"COMPANY", d.CompanyID},
				{"KIND", d.Kind},
				{"STATUS", d.Status},
				{"TITLE", d.Title},
				{"BODY", d.Body},
				{"DECIDED_BY", d.DecidedBy},
				{"SOURCE", d.Source},
				{"WHEN", fmtTime(d.CreatedAt)},
			}
			printTable([]string{"FIELD", "VALUE"}, rows)
			return nil
		},
	}
	return cmd
}
