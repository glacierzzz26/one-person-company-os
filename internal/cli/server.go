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

// serverCmd 常驻:HTTP + webhook + GitHub 轮询(研发部通道 B)。首个长驻命令。
func serverCmd() *cobra.Command {
	var port, pollMin int
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Run the long-lived server (HTTP + GitHub webhook + poll; R&D intake channel B)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			srv := server.New(svc, pollMin)
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			addr := fmt.Sprintf(":%d", port)
			fmt.Printf("os server listening on %s (github poll every %d min; ctrl-c to stop)\n", addr, pollMin)

			go srv.PollLoop(ctx)
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
	return cmd
}
