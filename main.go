// RustDex: a minimal RustDesk API server that only keeps an address book.
package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alexbn71/RustDex/internal/config"
	"github.com/alexbn71/RustDex/internal/server"
	"github.com/alexbn71/RustDex/internal/store"
)

//go:embed web
var webFS embed.FS

// Set at build time with -ldflags "-X main.version=..."
var version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "healthcheck":
			os.Exit(healthcheck())
		case "version", "--version", "-v":
			fmt.Println(version)
			return
		}
	}
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := dropPrivileges(cfg.DataDir); err != nil {
		return err
	}
	st, err := store.Open(cfg.DataDir, cfg.SecretKey)
	if err != nil {
		return err
	}
	defer st.Close()

	static, err := fs.Sub(webFS, "web")
	if err != nil {
		return err
	}
	handler, err := server.New(cfg, st, static, version)
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errc := make(chan error, 1)
	go func() {
		log.Printf("RustDex %s listening on %s (data in %s)", version, cfg.Listen, cfg.DataDir)
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	log.Print("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// healthcheck is used by the Docker HEALTHCHECK (the image has no curl).
func healthcheck() int {
	listen := os.Getenv("RUSTDEX_LISTEN")
	if listen == "" {
		listen = ":21114"
	}
	_, port, err := net.SplitHostPort(listen)
	if err != nil {
		return 1
	}
	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/healthz")
	if err != nil {
		return 1
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
