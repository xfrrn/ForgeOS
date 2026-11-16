package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"forgeos/internal/store"
	_ "github.com/lib/pq"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context) error {
	addr := os.Getenv("FORGEOS_LISTEN_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil || !net.ParseIP(host).IsLoopback() {
		return errors.New("FORGEOS_LISTEN_ADDR must use a loopback IP and port")
	}
	owner, worker := os.Getenv("FORGEOS_OWNER_TOKEN"), os.Getenv("FORGEOS_WORKER_TOKEN")
	if strings.TrimSpace(owner) == "" || strings.TrimSpace(worker) == "" || owner == worker {
		return errors.New("set distinct FORGEOS_OWNER_TOKEN and FORGEOS_WORKER_TOKEN")
	}
	root := os.Getenv("FORGEOS_REPO_ROOT")
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() || !filepath.IsAbs(root) {
		return errors.New("FORGEOS_REPO_ROOT must be an existing absolute directory")
	}
	dsn := os.Getenv("FORGEOS_DATABASE_URL")
	if dsn == "" {
		return errors.New("set FORGEOS_DATABASE_URL to the ForgeOS PostgreSQL database")
	}
	poller, err := pollConfig(os.Getenv("FORGEOS_POLL_PROJECT_ID"), os.Getenv("FORGEOS_POLL_VALIDATION"))
	if err != nil {
		return err
	}
	delivery, err := deliveryConfig(os.Getenv("FORGEOS_DELIVERY_PROJECT_ID"), os.Getenv("FORGEOS_DELIVERY_TARGET_BRANCH"))
	if err != nil {
		return err
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return errors.New("could not open PostgreSQL connection")
	}
	defer db.Close()
	data := store.New(db)
	initCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := db.PingContext(initCtx); err != nil {
		return errors.New("PostgreSQL is unavailable; check FORGEOS_DATABASE_URL and database permissions")
	}
	if err := data.Migrate(initCtx); err != nil {
		return fmt.Errorf("apply database migration: %w", err)
	}

	server := &http.Server{
		Addr:              addr,
		Handler:           routes(data, root, owner, worker, delivery),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      25 * time.Second,
		IdleTimeout:       time.Minute,
		MaxHeaderBytes:    16 << 10,
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	if poller != nil {
		stopPolling := poller.start(ctx, data, root, ghRead)
		defer stopPolling()
	}
	if delivery.projectID != 0 {
		stopDelivery := delivery.start(ctx, data, root)
		defer stopDelivery()
	}
	log.Printf("ForgeOS control plane listening on %s", listener.Addr())
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(shutdown)
	}
}
