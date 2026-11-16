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
	"syscall"
	"time"

	"forgeos/internal/bao"
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
	owner := os.Getenv("FORGEOS_OWNER_TOKEN")
	manager := os.Getenv("FORGEOS_MANAGEMENT_TOKEN")
	workers, err := loadWorkers(os.Getenv("FORGEOS_WORKERS_FILE"), os.Getenv("FORGEOS_WORKER_TOKEN"), owner, manager)
	if err != nil {
		return err
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

	mux := routes(data, root, owner, workers, delivery)
	vault, err := bao.New(os.Getenv("FORGEOS_BAO_ADDR"), os.Getenv("FORGEOS_BAO_METADATA_TOKEN_FILE"), os.Getenv("FORGEOS_BAO_CA_FILE"))
	if err != nil {
		return err
	}
	registerSecrets(mux, data, owner, workers, vault)
	var broker *bao.Client
	if file := os.Getenv("FORGEOS_BAO_BROKER_TOKEN_FILE"); file != "" {
		if vault == nil || filepath.Clean(file) == filepath.Clean(os.Getenv("FORGEOS_BAO_METADATA_TOKEN_FILE")) {
			return errors.New("configure separate OpenBao metadata and broker identities")
		}
		broker, err = bao.New(os.Getenv("FORGEOS_BAO_ADDR"), file, os.Getenv("FORGEOS_BAO_CA_FILE"))
		if err != nil {
			return err
		}
	}
	registerSecretBroker(mux, data, owner, workers, vault, broker)
	if err := registerCodeReviews(mux, data, root, owner, manager, workers); err != nil {
		return err
	}
	if err := registerManagement(mux, data, os.Getenv("FORGEOS_MANAGEMENT_PROJECT_ID"), manager, owner, workers); err != nil {
		return err
	}
	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
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
