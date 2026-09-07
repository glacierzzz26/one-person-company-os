package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/glacierzzz26/one-person-company-os/internal/server"
	"github.com/spf13/cobra"
)

// serverCmd 常驻:HTTP + webhook + GitHub 轮询 + 每日摘要(研发部通道 B)。首个长驻命令。
// Phase 9.3(runtime-knobs-web.md §3.5)+ 9.4(cli-readonly.md §3.2,决策③):权威源 = app_setting 行
// (Web 设置页),boot 读一次;业务 flag(port/poll/digest/queue-*)9.4 全删 —— os server 零业务参数,
// 仅 --db/--config(数据定位,root)+ --digest-now(boot 一次性触发)。通知按公司机密,不读 env。
func serverCmd() *cobra.Command {
	var digestNow bool
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Run the long-lived server (HTTP + GitHub webhook + poll + daily digest; R&D channel B)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			// 生效配置:DB app_setting 权威(缺行 = 内置默认,与旧 flag 默认一致,存量零参数行为不变)。
			app, err := svc.AppSetting(cmd.Context())
			if err != nil {
				return fmt.Errorf("read app settings: %w", err)
			}
			cfg, err := serverConfigFromApp(app)
			if err != nil {
				return err
			}

			srv := server.New(svc, cfg.PollMin)
			if err := srv.SetDigestTime(cfg.Digest); err != nil {
				return err
			}
			// 主密钥落盘路径(Phase 9.2):os server 的 /setup 首启需写 <db>.key;鉴权 DB 化(console
			// token 哈希读 app_setting,OS_API_TOKEN env 渠道 9.2 已废)。
			srv.SetMasterKeyPath(masterKeyPath)
			// 队列认领循环(Phase 7.2):queue_work 生效值(DB app_setting)开则自消费任务。
			if cfg.QueueWork {
				srv.SetQueueWork(time.Duration(cfg.QueueIntervalSec) * time.Second)
			}

			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			addr := fmt.Sprintf(":%d", cfg.HTTPPort)
			fmt.Printf("os server listening on %s (github poll every %d min; ctrl-c to stop)\n", addr, cfg.PollMin)
			// /api/v1 auth 状态读 DB(Phase 9.2):console_token_hash 非空 = 已初始化要求 Bearer;空 = 开放。
			api := "open"
			if init, err := srv.Initialized(cmd.Context()); err == nil && init {
				api = "on (console token)"
			}
			// 通知源(9.3 决策②):按公司机密 feishu_webhook 逐公司路由(service.notifyCompany/digest fan-out),无全局 env。
			fmt.Printf("feishu notify: per-company (company secret feishu_webhook) | daily digest: %s | /api/v1 auth: %s | queue work: %s\n", srv.DigestTime(), api, queueWorkStatus(srv))

			go srv.PollLoop(ctx)
			go srv.DigestLoop(ctx)
			go srv.QueueLoop(ctx)
			if digestNow {
				fmt.Println("sending daily digest now (--digest-now)...")
				if err := srv.RunDigestNow(ctx); err != nil {
					fmt.Printf("daily digest now: %v\n", err)
				}
			}

			httpSrv := &http.Server{Addr: addr, Handler: srv.Handler()}
			go func() {
				<-ctx.Done()
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = httpSrv.Shutdown(shutdownCtx)
			}()
			err = httpSrv.ListenAndServe()
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				return err
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&digestNow, "digest-now", false, "send the daily digest immediately at boot")
	return cmd
}

// queueWorkStatus 启动日志的队列循环状态文本(不泄漏执行细节)。
func queueWorkStatus(srv *server.Server) string {
	if srv.QueueWorkEnabled() {
		return "on"
	}
	return "off (os queue work drains manually)"
}
