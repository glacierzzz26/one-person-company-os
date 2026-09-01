package cli

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/glacierzzz26/one-person-company-os/internal/config"
	"github.com/glacierzzz26/one-person-company-os/internal/service"
	"github.com/glacierzzz26/one-person-company-os/internal/storage"
	"github.com/glacierzzz26/one-person-company-os/internal/storage/repository"
	"github.com/spf13/cobra"
)

var (
	cfgPath string
	dbPath  string
	svc     *service.Service
)

func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "os",
		Short: "One-Person Company OS",
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if svc != nil {
				return nil
			}
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return err
			}
			path := dbPath
			if path == "" {
				path = cfg.DBPath
			}
			db, err := storage.Open(path)
			if err != nil {
				return fmt.Errorf("open database: %w", err)
			}
			svc = service.New(repository.NewStore(db))
			return nil
		},
	}
	root.PersistentFlags().StringVar(&cfgPath, "config", "config/os.yaml", "config file path")
	root.PersistentFlags().StringVar(&dbPath, "db", "", "sqlite db path (overrides config)")
	root.AddCommand(initCmd())
	root.AddCommand(companyCmd())
	root.AddCommand(capabilityCmd())
	root.AddCommand(agentCmd())
	root.AddCommand(policyCmd())
	root.AddCommand(permissionCmd())
	root.AddCommand(workflowCmd())
	root.AddCommand(taskCmd())
	root.AddCommand(queueCmd())
	root.AddCommand(executionCmd())
	root.AddCommand(toolCmd())
	root.AddCommand(approvalCmd())
	root.AddCommand(auditCmd())
	root.AddCommand(providerCmd())
	root.AddCommand(memoryCmd())
	root.AddCommand(decisionCmd())
	return root
}

func initCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize the database (apply migrations)",
		RunE: func(_ *cobra.Command, _ []string) error {
			// PersistentPreRunE 打开数据库即应用迁移。
			fmt.Println("database initialized")
			return nil
		},
	}
}

func printTable(header []string, rows [][]string) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, strings.Join(header, "\t"))
	for _, r := range rows {
		fmt.Fprintln(w, strings.Join(r, "\t"))
	}
	w.Flush()
}

func fmtTime(ts int64) string {
	if ts == 0 {
		return ""
	}
	return time.Unix(ts, 0).Format("2006-01-02 15:04")
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
