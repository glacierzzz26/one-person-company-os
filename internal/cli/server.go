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
// Phase 9.3(契约 runtime-knobs-web.md §3.5,决策「server 零参数」):权威源 = app_setting 行
// (Web 设置页),boot 读一次;flags 保留仅显式覆盖,标记 deprecated(9.4 CLI 收口删)。通知不再读 env。
func serverCmd() *cobra.Command {
	var port, pollMin int
	var digestFlag string
	var digestNow bool
	var queueWork bool
	var queueIntervalSec int
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Run the long-lived server (HTTP + GitHub webhook + poll + daily digest; R&D channel B)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			// 生效配置:DB app_setting(缺行 = 内置默认,与旧 flag 默认一致,存量零参数行为不变);
			// flag 显式传(Changed)才覆盖 DB 值。digest 时刻来自 DB(OS_FEISHU_DIGEST 已不再读取)。
			app, err := svc.AppSetting(cmd.Context())
			if err != nil {
				return fmt.Errorf("read app settings: %w", err)
			}
			cfg, err := effectiveServerConfig(app, flagVals{
				port: port, pollMin: pollMin, queueIntervalSec: queueIntervalSec,
				queueWork: queueWork, digest: digestFlag,
			}, cmd.Flags().Changed)
			if err != nil {
				return err
			}

			srv := server.New(svc, cfg.PollMin)
			if err := srv.SetDigestTime(cfg.Digest); err != nil {
				return err
			}
			// 主密钥落盘路径(Phase 9.2):os server 的 /setup 首启需写 <db>.key;鉴权已 DB 化(console
			// token 哈希,读 app_setting),OS_API_TOKEN 不再是 /api/v1 权威来源(9.4 CLI 收口删)。
			srv.SetMasterKeyPath(masterKeyPath)
			// 队列认领循环(Phase 7.2):queue_work 生效值(DB 默认 / --queue-work 覆盖)开则自消费任务。
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
	// deprecated(9.3):已 DB 化,Web 设置页为权威源;显式传此 flag 仅作覆盖,9.4 收口删。
	cmd.Flags().IntVar(&port, "port", 8787, "http listen port (deprecated: 已 DB 化,Web 设置页 http_port 为权威源;显式传此 flag 仅作覆盖)")
	cmd.Flags().IntVar(&pollMin, "poll", 5, "GitHub issue poll interval in minutes (deprecated: 已 DB 化,Web 设置页 poll_min 为权威源;显式传此 flag 仅作覆盖)")
	cmd.Flags().StringVar(&digestFlag, "digest", "", `daily digest time "HH:MM" (default from app_setting digest_time; "off" to disable) (deprecated: 已 DB 化,Web 设置页 digest_time 为权威源;显式传此 flag 仅作覆盖)`)
	cmd.Flags().BoolVar(&digestNow, "digest-now", false, "send the daily digest immediately at boot")
	cmd.Flags().BoolVar(&queueWork, "queue-work", false, "server consumes the task queue itself (LeaseAndExecute; no manual os queue work) (deprecated: 已 DB 化,Web 设置页 queue_work 为权威源;显式传此 flag 仅作覆盖)")
	cmd.Flags().IntVar(&queueIntervalSec, "queue-interval", 10, "queue consumption interval in seconds (with queue_work) (deprecated: 已 DB 化,Web 设置页 queue_interval_sec 为权威源;显式传此 flag 仅作覆盖)")
	return cmd
}

// queueWorkStatus 启动日志的队列循环状态文本(不泄漏执行细节)。
func queueWorkStatus(srv *server.Server) string {
	if srv.QueueWorkEnabled() {
		return "on"
	}
	return "off (os queue work drains manually)"
}
