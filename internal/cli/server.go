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

			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			addr := fmt.Sprintf(":%d", port)
			feishu := "off"
			if svc.NotifyEnabled() {
				feishu = "on (OS_FEISHU_WEBHOOK)"
			}
			fmt.Printf("os server listening on %s (github poll every %d min; ctrl-c to stop)\n", addr, pollMin)
			fmt.Printf("feishu notify: %s | daily digest: %s\n", feishu, srv.DigestTime())

			go srv.PollLoop(ctx)
			go srv.DigestLoop(ctx)
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
	return cmd
}
