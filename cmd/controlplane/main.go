package main

import (
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"forgeos/internal/bao"
	"forgeos/internal/nodetls"
	"forgeos/internal/store"
	_ "github.com/lib/pq"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "configure-owner" {
		if err := configureOwner(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
		return
	}
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
	ownerCredentials, err := loadOwnerCredentials(os.Getenv("FORGEOS_OWNER_CREDENTIALS_FILE"))
	if err != nil {
		return err
	}
	gateway, err := previewGateway(os.Getenv("FORGEOS_GATEWAY_DOMAIN"), os.Getenv("FORGEOS_GATEWAY_PORT"))
	if err != nil {
		return err
	}
	ownerAuth := newOwnerWebAuth(ownerCredentials, owner, gateway)
	channels, err := loadNoticeChannels(os.Getenv("FORGEOS_NOTIFICATION_CHANNELS_FILE"))
	if err != nil {
		return err
	}
	manager := os.Getenv("FORGEOS_MANAGEMENT_TOKEN")
	workers, err := loadWorkers(os.Getenv("FORGEOS_WORKERS_FILE"), os.Getenv("FORGEOS_WORKER_TOKEN"), owner, manager)
	if err != nil {
		return err
	}
	chatNodes, err := loadChatNodes(os.Getenv("FORGEOS_MANAGEMENT_CHAT_NODES_FILE"), owner, manager, workers)
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
	pollers, err := configuredPollers(os.Getenv("FORGEOS_POLL_PROJECT_ID"), os.Getenv("FORGEOS_POLL_VALIDATION"), os.Getenv("FORGEOS_POLL_PROJECTS_FILE"))
	if err != nil {
		return err
	}
	delivery, err := configuredDeliveryTargets(os.Getenv("FORGEOS_DELIVERY_PROJECT_ID"), os.Getenv("FORGEOS_DELIVERY_TARGET_BRANCH"), os.Getenv("FORGEOS_DELIVERY_PROJECTS_FILE"))
	if err != nil {
		return err
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return errors.New("could not open PostgreSQL connection")
	}
	defer db.Close()
	db.SetMaxIdleConns(8)
	data := store.New(db)
	if value := os.Getenv("FORGEOS_PROCESS_LOG_DAYS"); value != "" {
		days, err := strconv.Atoi(value)
		if err != nil || days < 1 || days > 3650 {
			return errors.New("FORGEOS_PROCESS_LOG_DAYS must be between 1 and 3650")
		}
		data.ProcessArchiveDays = days
	}
	initCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := db.PingContext(initCtx); err != nil {
		return errors.New("PostgreSQL is unavailable; check FORGEOS_DATABASE_URL and database permissions")
	}
	if err := data.Migrate(initCtx); err != nil {
		return fmt.Errorf("apply database migration: %w", err)
	}
	if err := data.ConfigureMaterialSearch(initCtx, os.Getenv("FORGEOS_EMBEDDINGS_URL"), os.Getenv("FORGEOS_EMBEDDINGS_MODEL")); err != nil {
		return err
	}

	mux := routes(data, root, owner, workers, delivery)
	ownerAuth.register(mux)
	registerProjectWorkViews(mux, data, owner)
	registerProjectOverviewContext(mux, data, root, owner, pollers)
	registerAgentSettings(mux, data, owner, workers)
	registerProjectChat(mux, data, root, owner, chatNodes, ghRead)
	clones := projectCloneService{data: data, localRoot: root, extraHosts: os.Getenv("FORGEOS_GIT_CLONE_HOSTS")}
	clones.register(mux, owner)
	registerProjectGit(mux, data, root, owner)
	registerGitHubWebhooks(mux, data, owner, pollers)
	stopPreviews, err := startPreviews(mux, data, root, owner, os.Getenv("FORGEOS_PREVIEW_ADDR"))
	if err != nil {
		return err
	}
	defer stopPreviews()
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
	if err := registerTestPlans(mux, data, root, owner, manager, workers); err != nil {
		return err
	}
	if err := registerCodeReviews(mux, data, root, owner, manager, workers); err != nil {
		return err
	}
	if err := registerManagement(mux, data, os.Getenv("FORGEOS_MANAGEMENT_PROJECT_ID"), manager, owner, workers); err != nil {
		return err
	}
	if err := registerCurator(mux, data, owner, manager, workers); err != nil {
		return err
	}
	var servers []*http.Server
	done := make(chan error, 2)
	start := func(listener net.Listener, handler http.Handler) {
		server := &http.Server{
			Handler: handler, ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout: 10 * time.Second, WriteTimeout: 25 * time.Second,
			IdleTimeout: time.Minute, MaxHeaderBytes: 16 << 10,
		}
		servers = append(servers, server)
		go func() { done <- server.Serve(listener) }()
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer listener.Close()
	nodeAddr, identity, ca := os.Getenv("FORGEOS_NODE_TLS_ADDR"), os.Getenv("FORGEOS_NODE_TLS_IDENTITY_FILE"), os.Getenv("FORGEOS_NODE_TLS_CA_FILE")
	if nodeAddr != "" || identity != "" || ca != "" {
		if nodeAddr == "" || workers.file == "" {
			return errors.New("node TLS listener requires FORGEOS_NODE_TLS_ADDR and named Worker grants")
		}
		pair, err := nodetls.Identity(identity)
		if err != nil {
			return err
		}
		pool, err := nodetls.Roots(ca)
		if err != nil {
			return err
		}
		nodeListener, err := tls.Listen("tcp", nodeAddr, &tls.Config{
			MinVersion: tls.VersionTLS13,
			ClientAuth: tls.RequireAndVerifyClientCert,
			ClientCAs:  pool, Certificates: []tls.Certificate{*pair},
		})
		if err != nil {
			return err
		}
		defer nodeListener.Close()
		start(nodeListener, workers.nodeRoutes)
		log.Printf("ForgeOS Worker mTLS listening on %s", nodeListener.Addr())
	}
	runtimeCtx, stopRuntime := context.WithCancel(ctx)
	defer stopRuntime()
	registerRuntimeStatus(runtimeCtx, mux, db, root, owner)
	start(listener, ownerAuth.middleware(mux))
	cloneCtx, stopClones := context.WithCancel(ctx)
	cloneDone := make(chan struct{})
	go func() {
		defer close(cloneDone)
		clones.run(cloneCtx)
	}()
	defer func() {
		stopClones()
		<-cloneDone
	}()
	defer func() {
		for _, s := range servers {
			s.Close()
		}
	}()
	for _, poller := range pollers {
		stopPolling := poller.start(ctx, data, root, ghRead)
		defer stopPolling()
	}
	for project, target := range delivery {
		stopDelivery := (deliveryPolicy{projectID: project, target: target}).start(ctx, data, root)
		defer stopDelivery()
	}
	sweepCtx, stopSweep := context.WithCancel(ctx)
	var notices sync.WaitGroup
	defer notices.Wait()
	defer stopSweep()
	for _, channel := range channels {
		notices.Add(1)
		go func() {
			defer notices.Done()
			channel.run(sweepCtx, data)
		}()
	}
	go sweepArtifacts(sweepCtx, data)
	go watchDocuments(sweepCtx, data, root)
	log.Printf("ForgeOS control plane listening on %s", listener.Addr())
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var result error
		for _, server := range servers {
			result = errors.Join(result, server.Shutdown(shutdown))
		}
		return result
	}
}
