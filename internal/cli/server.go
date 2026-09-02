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
func serverCmd() *cobra.Command {
	var port, pollMin int
	var digest, digestFlag string
	var digestNow bool
	var queueWork bool
	var queueIntervalSec int
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Run the long-lived server (HTTP + GitHub webhook + poll + daily digest; R&D channel B)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			srv := server.New(svc, pollMin)
			// 摘要时刻解析优先级:--digest > OS_FEISHU_DIGEST > 09:00;--digest off 关闭。
			digest = digestFlag
			if digest == "" {
				digest = os.Getenv("OS_FEISHU_DIGEST")
			}
			if digest == "" {
				digest = "09:00"
			}
			if err := srv.SetDigestTime(digest); err != nil {
				return err
			}
			// /api/v1 访问令牌(Phase 7.1):设了 OS_API_TOKEN → API 要求 Bearer;空 = 开放。
			srv.SetAPIToken(os.Getenv("OS_API_TOKEN"))
			// 队列认领循环(Phase 7.2):--queue-work 开启后 server 自己消费任务(免手动 os queue work)。
			if queueWork {
				srv.SetQueueWork(time.Duration(queueIntervalSec) * time.Second)
			}

			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			addr := fmt.Sprintf(":%d", port)
			feishu := "off"
			if svc.NotifyEnabled() {
				feishu = "on (OS_FEISHU_WEBHOOK)"
			}
			fmt.Printf("os server listening on %s (github poll every %d min; ctrl-c to stop)\n", addr, pollMin)
			api := "open"
			if srv.APITokenSet() {
				api = "on (OS_API_TOKEN)"
			}
			fmt.Printf("feishu notify: %s | daily digest: %s | /api/v1 auth: %s | queue work: %s\n", feishu, srv.DigestTime(), api, queueWorkStatus(srv))

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
			err := httpSrv.ListenAndServe()
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				return err
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&port, "port", 8787, "http listen port")
	cmd.Flags().IntVar(&pollMin, "poll", 5, "GitHub issue poll interval in minutes")
	cmd.Flags().StringVar(&digestFlag, "digest", "", `daily digest time "HH:MM" (default 09:00; "off" to disable)`)
	cmd.Flags().BoolVar(&digestNow, "digest-now", false, "send the daily digest immediately at boot")
	cmd.Flags().BoolVar(&queueWork, "queue-work", false, "server consumes the task queue itself (LeaseAndExecute; no manual os queue work)")
	cmd.Flags().IntVar(&queueIntervalSec, "queue-interval", 10, "queue consumption interval in seconds (with --queue-work)")
	return cmd
}

// queueWorkStatus 启动日志的队列循环状态文本(不泄漏执行细节)。
func queueWorkStatus(srv *server.Server) string {
	if srv.QueueWorkEnabled() {
		return "on"
	}
	return "off (os queue work drains manually)"
}
