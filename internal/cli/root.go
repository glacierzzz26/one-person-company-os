package cli

import (
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/glacierzzz26/one-person-company-os/internal/config"
	"github.com/glacierzzz26/one-person-company-os/internal/endpoint"
	"github.com/glacierzzz26/one-person-company-os/internal/service"
	"github.com/glacierzzz26/one-person-company-os/internal/settings"
	"github.com/glacierzzz26/one-person-company-os/internal/storage"
	"github.com/glacierzzz26/one-person-company-os/internal/storage/repository"
	"github.com/spf13/cobra"
)

var (
	cfgPath string
	dbPath  string
	svc     *service.Service

	// masterKeyPath 当前开库的主密钥文件路径(<dbPath>.key,settings.KeyPath);PersistentPreRunE 解析后
	// 供 os server 的 /setup 落盘用(server.New 无路径知识,经 SetMasterKeyPath 注入)。
	masterKeyPath string
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
			// 主密钥注入(Phase 9.2 + 9.3):开库后若有 <db>.key(首启 /setup 生成)→ LoadKey 解码 → 进程级
			// holder:endpoint(解端点 token)与 settings(解公司机密 secret)。无 key 文件 = 未初始化,
			// 静默跳过。通知不走 env(9.3 起按公司机密 feishu_webhook 解析,见 service.notifyCompany)。
			masterKeyPath = settings.KeyPath(path)
			if k, err := settings.LoadKey(masterKeyPath); err == nil {
				if kb, derr := hex.DecodeString(strings.TrimSpace(k)); derr == nil && len(kb) == 32 {
					endpoint.UseMasterKey(kb)
					settings.UseMasterKey(kb)
				}
			}
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
	root.AddCommand(overviewCmd())
	root.AddCommand(endpointCmd())
	root.AddCommand(intakeCmd())
	root.AddCommand(projectCmd())
	root.AddCommand(pipelineCmd())
	root.AddCommand(serverCmd())
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
