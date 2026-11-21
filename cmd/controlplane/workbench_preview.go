package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"forgeos/internal/localrepo"
	"forgeos/internal/store"
)

var workbenchPreviewReadPath = regexp.MustCompile(`^(/projects/[1-9][0-9]*(/(overview-context|planning|chat|chat/provider|execution-queue|test-checks))?|/tasks/[1-9][0-9]*)$`)

func runWorkbenchPreview(ctx context.Context, sampleRepo string) error {
	appPort, appErr := strconv.Atoi(os.Getenv("PORT"))
	dbPort, dbErr := strconv.Atoi(os.Getenv("FORGEOS_SELF_PREVIEW_DB_PORT"))
	password := os.Getenv("FORGEOS_SELF_PREVIEW_DB_PASSWORD")
	if os.Getenv("HOST") != "127.0.0.1" || appErr != nil || appPort < 1024 || appPort > 65535 ||
		dbErr != nil || dbPort < 1024 || dbPort > 65535 || dbPort == appPort || len(password) != 64 {
		return errors.New("start workbench-preview from the repository preview script with its temporary database")
	}
	root := filepath.Dir(sampleRepo)
	startup, finishStartup := context.WithTimeout(ctx, 25*time.Second)
	defer finishStartup()
	if _, err := localrepo.ResolveRepo(startup, root, sampleRepo); err != nil {
		return errors.New("the preview sample repository is unavailable")
	}
	dbURL := (&url.URL{
		Scheme: "postgres", User: url.UserPassword("forgeos_preview", password),
		Host: net.JoinHostPort("127.0.0.1", strconv.Itoa(dbPort)),
		Path: "/postgres", RawQuery: "sslmode=disable",
	}).String()
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return errors.New("could not open temporary preview database")
	}
	defer db.Close()
	data := store.New(db)
	if err := data.Migrate(startup); err != nil {
		return fmt.Errorf("migrate temporary preview database: %w", err)
	}
	project, err := data.CreateProject(startup, sampleRepo)
	if err != nil {
		return fmt.Errorf("create sample preview project: %w", err)
	}
	if _, err := data.CreateTask(startup, project.ID); err != nil {
		return fmt.Errorf("create sample preview task: %w", err)
	}
	finishStartup()

	var key [64]byte
	if _, err := rand.Read(key[:]); err != nil {
		return err
	}
	owner := hex.EncodeToString(key[:32])
	mux := routes(data, root, owner, legacyWorker(hex.EncodeToString(key[32:])), nil)
	registerProjectWorkViews(mux, data, owner)
	registerProjectOverviewContext(mux, data, root, owner, nil)
	registerProjectChat(mux, data, root, owner, nil, nil, nil, nil)
	registerRuntimeStatus(ctx, mux, db, root, owner)
	server := &http.Server{
		Addr:              net.JoinHostPort("127.0.0.1", strconv.Itoa(appPort)),
		Handler:           workbenchPreviewAccess(mux, owner),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       time.Minute,
		MaxHeaderBytes:    16 << 10,
	}
	listener, err := net.Listen("tcp4", server.Addr)
	if err != nil {
		return err
	}
	stop := context.AfterFunc(ctx, func() { _ = server.Close() })
	defer stop()
	err = server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// The private preview URL grants access only to disposable sample data.
// Keep external integrations and Worker and repository mutation routes out of this preview.
func workbenchPreviewAccess(next http.Handler, owner string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		peer, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil || !net.ParseIP(peer).IsLoopback() {
			http.NotFound(w, r)
			return
		}
		path := r.URL.Path
		w.Header().Set("Cache-Control", "no-store")
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			switch {
			case path == "/auth/session":
				writeJSON(w, http.StatusOK, map[string]string{"email": "preview@forgeos.test"})
				return
			case path == "/" || path == "/ui/" || strings.HasPrefix(path, "/ui/assets/"):
			case path == "/projects" || path == "/tasks" || path == "/workers" ||
				path == "/workers/control-plane" || path == "/workers/model-budgets":
			case workbenchPreviewReadPath.MatchString(path):
			default:
				http.NotFound(w, r)
				return
			}
		} else if r.Method != http.MethodPost || path != "/tasks" {
			http.NotFound(w, r)
			return
		}
		r.Header.Set("Authorization", "Bearer "+owner)
		next.ServeHTTP(w, r)
	})
}
