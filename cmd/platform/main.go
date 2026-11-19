// Command platform runs the AgentsView Teams Platform.
package main

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/applied-chaos/agentsview-team/internal/admin"
	"github.com/applied-chaos/agentsview-team/internal/auth"
	"github.com/applied-chaos/agentsview-team/internal/clock"
	"github.com/applied-chaos/agentsview-team/internal/config"
	"github.com/applied-chaos/agentsview-team/internal/hosts"
	"github.com/applied-chaos/agentsview-team/internal/instance"
	"github.com/applied-chaos/agentsview-team/internal/replay"
	"github.com/applied-chaos/agentsview-team/internal/secrets"
	"github.com/applied-chaos/agentsview-team/internal/server"
	"github.com/applied-chaos/agentsview-team/internal/stats"
	"github.com/applied-chaos/agentsview-team/internal/store"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck())
	}
	if len(os.Args) > 1 && os.Args[1] == "admin" {
		os.Exit(adminCLI(os.Args[2:], os.Stdout, os.Stderr))
	}
	if len(os.Args) > 1 && os.Args[1] == "secrets" {
		os.Exit(secretsCLI(os.Args[2:], os.Stdout, os.Stderr))
	}
	if err := run(); err != nil {
		slog.Error("platform stopped", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}
	if clock.TestBuild && cfg.Production {
		return errors.New("test-clock build refused in production (PLATFORM_ENV=production)")
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	st, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer st.Close()
	clk := clock.New()
	a, err := auth.New(ctx, cfg, st, clk)
	if err != nil {
		return err
	}
	box, err := secrets.NewBox(cfg.SecretKey)
	if err != nil {
		return err
	}
	inst, err := instance.New(ctx, cfg, st, box)
	if err != nil {
		return err
	}
	defer inst.Wait()
	rp, err := replay.New(st, box, inst, inst, clk, filepath.Join(cfg.DataDir, "spool"))
	if err != nil {
		return err
	}
	inst.Tee = rp.Tee()
	rp.Start(ctx)
	defer rp.Stop()
	go a.Run(ctx)
	a.OnSignIn = func(u store.User) { inst.Ensure(u.ID) }
	a.OnDisabled = func(ctx context.Context, id string) {
		if err := inst.Disable(ctx, id); err != nil {
			slog.Error("revoke devices", "user", id, "err", err)
		}
	}
	adm := admin.New(a, st, clk)
	adm.Instances = inst
	adm.Replay = rp
	ts := stats.New(st, clk, a, inst, cfg.DataDir)
	adm.Stats = ts
	hs := hosts.New(st, inst, inst, clk, a, cfg.BaseURL, instance.PinnedVersion)
	hs.Space = ts.UserSpace
	go ts.Run(ctx)
	go hs.Run(ctx)
	srv := &http.Server{Addr: cfg.Listen, Handler: server.New(a, adm, clk, inst, hs, rp, ts, cfg.WebDir), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	slog.Info("platform listening", "addr", cfg.Listen, "testClock", clock.TestBuild)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// healthcheck is the container probe: the image has no curl or wget.
func healthcheck() int {
	addr := os.Getenv("PLATFORM_LISTEN")
	if addr == "" || addr[0] == ':' {
		addr = "127.0.0.1" + cmp.Or(addr, ":8080")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/healthz", nil)
	if err != nil {
		return 1
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 1
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		return 1
	}
	return 0
}
