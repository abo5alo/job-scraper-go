// Command api serves the job search API.
//
//	go run ./cmd/api
//	curl "localhost:8080/jobs?q=engineer&country=AE&level=senior"
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"job-scraper-go/internal/api"
	"job-scraper-go/internal/store"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// LOG_FILE also appends the log to a file. Docker's own copy of a
	// container's output is lost when the container is replaced, and this
	// one keeps the request history that cmd/stats reports on. A file that
	// can't be opened is a warning, not a reason to keep the site down.
	if path := os.Getenv("LOG_FILE"); path != "" {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			log.Warn("can't open log file; logging to stdout only", "err", err)
		} else {
			defer f.Close()
			log = slog.New(slog.NewTextHandler(io.MultiWriter(os.Stdout, f), nil))
		}
	}

	if err := run(log); err != nil {
		log.Error("api failed", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	// SIGTERM is what Docker and most hosts send to stop a container.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := store.New(ctx, envOr("DATABASE_URL", "postgres://jobs:jobs@localhost:5432/jobs?sslmode=disable"))
	if err != nil {
		return err
	}
	defer db.Close()

	if err := db.Migrate(ctx); err != nil {
		return err
	}

	addr := envOr("ADDR", ":8080")
	srv := &http.Server{
		Addr: addr,
		Handler: api.NewHandler(db, log, api.Options{
			RequestsPerSecond: 5,
			Burst:             20,
			// Behind a reverse proxy like Caddy, the header it passes the
			// client's IP in. Never set it when the API is reachable directly.
			ClientIPHeader: os.Getenv("CLIENT_IP_HEADER"),
		}),
		// Without these, a client that opens a connection and sends bytes
		// very slowly can hold it open forever (a "slowloris" attack).
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Open the port first, so "port already in use" fails right here with a
	// clear error instead of after we've logged that we're listening.
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s (is another copy already running?): %w", addr, err)
	}
	log.Info("listening", "url", "http://localhost"+addr)

	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}

	// Graceful shutdown: stop accepting new connections, then give requests
	// already in progress up to 10 seconds to finish.
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
