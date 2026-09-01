package cli

import (
	"github.com/spf13/cobra"
)

func auditCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "audit", Short: "Query audit records (append-only)"}
	cmd.AddCommand(auditListCmd())
	return cmd
}

func auditListCmd() *cobra.Command {
	var entityType string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List audit records (optionally filtered)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			list, err := svc.ListAudits(cmd.Context(), entityType)
			if err != nil {
				return err
			}
			rows := [][]string{}
			for _, a := range list {
				rows = append(rows, []string{
					shortID(a.ID), a.EntityType, shortID(a.EntityID), a.Action, a.Actor, fmtTime(a.CreatedAt),
				})
			}
			printTable([]string{"ID", "ENTITY", "ENTITY_ID", "ACTION", "ACTOR", "WHEN"}, rows)
			return nil
		},
	}
	cmd.Flags().StringVar(&entityType, "entity-type", "", "filter by entity type (company/task/policy/...)")
	return cmd
}
