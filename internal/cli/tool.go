package cli

import (
	"github.com/glacierzzz26/one-person-company-os/internal/tool"
	"github.com/spf13/cobra"
)

func toolCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "tool", Short: "Query registered tools"}
	cmd.AddCommand(toolListCmd())
	return cmd
}

func toolListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List registered tools",
		RunE: func(_ *cobra.Command, _ []string) error {
			rows := [][]string{}
			for _, t := range tool.List() {
				perm := t.Permission()
				rows = append(rows, []string{t.Name(), t.Risk(), perm.Action + "/" + perm.Resource, t.Description()})
			}
			printTable([]string{"NAME", "RISK", "PERMISSION", "DESCRIPTION"}, rows)
			return nil
		},
	}
}
