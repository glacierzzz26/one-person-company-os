package cli

import (
	"github.com/glacierzzz26/one-person-company-os/internal/config"
	"github.com/spf13/cobra"
)

func providerCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "provider", Short: "Query model providers (from config)"}
	cmd.AddCommand(providerListCmd())
	return cmd
}

func providerListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List model providers declared in config (stub, no real calls in Phase 2)",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return err
			}
			rows := [][]string{}
			for _, p := range cfg.Providers {
				rows = append(rows, []string{p.Name, p.Type, p.Model, p.Endpoint, p.APIKeyEnv})
			}
			printTable([]string{"NAME", "TYPE", "MODEL", "ENDPOINT", "API_KEY_ENV"}, rows)
			return nil
		},
	}
}
