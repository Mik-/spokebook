// Package spokebook wires up configuration, database and HTTP server.
package spokebook

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Mik-/spokebook/internal/app/spokebook/rides/garmin"
	"github.com/Mik-/spokebook/internal/app/spokebook/secret"
	"github.com/Mik-/spokebook/internal/app/spokebook/server"
	"github.com/Mik-/spokebook/internal/app/spokebook/store/sqlite"
)

// Run starts the app and blocks until ctx is done.
func Run(ctx context.Context, log *slog.Logger) error {
	addr := env("APP_ADDR", "localhost:8080")
	dbPath := env("APP_DB_PATH", "data/dev.sqlite")
	baseURL := strings.TrimSuffix(env("APP_BASE_URL", "http://localhost:8080"), "/")
	proxies, err := server.ParseTrustedProxies(os.Getenv("APP_TRUSTED_PROXIES"))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o750); err != nil {
		return err
	}

	st, err := sqlite.Open(ctx, dbPath)
	if err != nil {
		return err
	}
	defer func() {
		if err := st.Close(); err != nil {
			log.Error("close database", "err", err)
		}
	}()

	var box *secret.Box
	if key := os.Getenv("APP_SECRET_KEY"); key != "" {
		if box, err = secret.New(key); err != nil {
			return fmt.Errorf("APP_SECRET_KEY: %w", err)
		}
	}
	syncer := garmin.NewSyncer(st, box, garmin.NewClient(garmin.BaseURL), log)
	go syncer.Run(ctx)

	version := AppVersion()
	app, err := server.New(ctx, st, log, server.Config{
		BaseURL: baseURL, TrustedProxies: proxies, Version: version, SourceURL: SourceURL,
		Garmin: syncer,
	})
	if err != nil {
		return err
	}
	srv := &http.Server{
		Addr:              addr,
		Handler:           app.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	log.Info("listening", "addr", "http://"+addr, "url", baseURL, "version", version)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// NewLogger writes text logs to w at the level in APP_LOG_LEVEL (debug, info,
// warn or error; default info).
func NewLogger(w io.Writer) (*slog.Logger, error) {
	var level slog.Level
	if err := level.UnmarshalText([]byte(env("APP_LOG_LEVEL", "info"))); err != nil {
		return nil, fmt.Errorf("APP_LOG_LEVEL: %w", err)
	}
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level})), nil
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
