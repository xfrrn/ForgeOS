package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	ownerSessionLifetime = 12 * time.Hour
	ownerLoginWindow     = time.Minute
	ownerLoginAttempts   = 5
	maxOwnerSessions     = 128
)

type ownerSessionContextKey struct{}

type ownerSession struct {
	origin  string
	expires time.Time
}

type ownerWebAuth struct {
	credentials *ownerCredentials
	ownerDigest [sha256.Size]byte
	gatewayHost string

	sessionMu sync.Mutex
	sessions  map[[sha256.Size]byte]ownerSession

	loginMu     sync.Mutex
	loginWindow time.Time
	failures    int
}

func newOwnerWebAuth(credentials *ownerCredentials, owner, gateway string) *ownerWebAuth {
	auth := &ownerWebAuth{
		credentials: credentials,
		ownerDigest: sha256.Sum256([]byte(owner)),
		sessions:    make(map[[sha256.Size]byte]ownerSession),
	}
	if gateway != "" {
		auth.gatewayHost = "forgeos." + gateway
	}
	return auth
}

type ownerOrigin struct {
	url    string
	secure bool
}

func (a *ownerWebAuth) origin(r *http.Request) ownerOrigin {
	peer, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || !net.ParseIP(peer).IsLoopback() {
		return ownerOrigin{}
	}
	host := strings.ToLower(r.Host)
	if gateway := strings.TrimSuffix(host, ":443"); a.gatewayHost != "" && gateway == a.gatewayHost &&
		(r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https") {
		return ownerOrigin{url: "https://" + gateway, secure: true}
	}
	name, _, err := net.SplitHostPort(host)
	if err != nil {
		name = strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
	}
	if name == "localhost" || net.ParseIP(name).IsLoopback() {
		return ownerOrigin{url: "http://" + strings.TrimSuffix(host, ":80")}
	}
	return ownerOrigin{}
}

func (o ownerOrigin) cookie(value string, maxAge int) *http.Cookie {
	name := "forgeos-owner-local"
	if o.secure {
		name = "__Host-forgeos-owner"
	}
	return &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   o.secure,
		SameSite: http.SameSiteStrictMode,
	}
}

func (o ownerOrigin) sessionKey(r *http.Request) ([sha256.Size]byte, bool) {
	var key [sha256.Size]byte
	if o.url == "" {
		return key, false
	}
	cookie, err := r.Cookie(o.cookie("", 0).Name)
	if err != nil || len(cookie.Value) != base64.RawURLEncoding.EncodedLen(32) {
		return key, false
	}
	return sha256.Sum256([]byte(cookie.Value)), true
}

func (a *ownerWebAuth) signedIn(r *http.Request, origin ownerOrigin) bool {
	key, ok := origin.sessionKey(r)
	if !ok {
		return false
	}
	a.sessionMu.Lock()
	defer a.sessionMu.Unlock()
	session := a.sessions[key]
	if !time.Now().Before(session.expires) {
		delete(a.sessions, key)
		return false
	}
	return session.origin == origin.url
}

func (a *ownerWebAuth) signIn(r *http.Request, origin ownerOrigin) (string, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", err
	}
	id := base64.RawURLEncoding.EncodeToString(secret)
	now := time.Now()
	a.sessionMu.Lock()
	defer a.sessionMu.Unlock()
	if previous, ok := origin.sessionKey(r); ok && a.sessions[previous].origin == origin.url {
		delete(a.sessions, previous)
	}
	var oldestKey [sha256.Size]byte
	var oldest time.Time
	for key, session := range a.sessions {
		if !now.Before(session.expires) {
			delete(a.sessions, key)
		} else if oldest.IsZero() || session.expires.Before(oldest) {
			oldestKey, oldest = key, session.expires
		}
	}
	if len(a.sessions) >= maxOwnerSessions {
		delete(a.sessions, oldestKey)
	}
	a.sessions[sha256.Sum256([]byte(id))] = ownerSession{origin.url, now.Add(ownerSessionLifetime)}
	return id, nil
}

func (a *ownerWebAuth) register(mux *http.ServeMux) {
	mux.HandleFunc("GET /auth/session", a.session)
	mux.HandleFunc("POST /auth/login", a.login)
	mux.HandleFunc("POST /auth/logout", a.logout)
}

func (a *ownerWebAuth) session(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !a.signedIn(r, a.origin(r)) {
		problem(w, http.StatusUnauthorized, "sign in required")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"email": a.credentials.Email})
}

func checkOwnerOrigin(w http.ResponseWriter, r *http.Request, origin ownerOrigin) bool {
	if origin.url != "" && r.Header.Get("Origin") == origin.url {
		return true
	}
	problem(w, http.StatusForbidden, "browser request must come from the workbench origin")
	return false
}

func (a *ownerWebAuth) login(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	origin := a.origin(r)
	if !checkOwnerOrigin(w, r, origin) {
		return
	}
	if a.credentials == nil {
		problem(w, http.StatusServiceUnavailable, "Owner email and password are not configured")
		return
	}
	var login struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decodeLimited(w, r, &login, 16<<10) {
		return
	}
	if len(login.Password) > maxOwnerPasswordBytes {
		problem(w, http.StatusBadRequest, "password is too long")
		return
	}
	if !a.loginMu.TryLock() {
		w.Header().Set("Retry-After", "1")
		problem(w, http.StatusTooManyRequests, "try signing in again")
		return
	}
	defer a.loginMu.Unlock()
	now := time.Now()
	if now.After(a.loginWindow) {
		a.failures = 0
		a.loginWindow = now.Add(ownerLoginWindow)
	}
	if a.failures >= ownerLoginAttempts {
		w.Header().Set("Retry-After", "60")
		problem(w, http.StatusTooManyRequests, "too many failed sign-in attempts")
		return
	}
	password := []byte(login.Password)
	defer clear(password)
	digest := ownerPasswordHash(password, a.credentials.Salt)
	valid := subtle.ConstantTimeCompare(digest, a.credentials.Hash) == 1
	if !valid || normalizedOwnerEmail(login.Email) != a.credentials.Email {
		a.failures++
		problem(w, http.StatusUnauthorized, "invalid email or password")
		return
	}
	id, err := a.signIn(r, origin)
	if err != nil {
		problem(w, http.StatusInternalServerError, "could not start a session")
		return
	}
	a.failures = 0
	http.SetCookie(w, origin.cookie(id, 0))
	writeJSON(w, http.StatusOK, map[string]string{"email": a.credentials.Email})
}

func (a *ownerWebAuth) logout(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	origin := a.origin(r)
	if !checkOwnerOrigin(w, r, origin) {
		return
	}
	if key, ok := origin.sessionKey(r); ok {
		a.sessionMu.Lock()
		if a.sessions[key].origin == origin.url {
			delete(a.sessions, key)
		}
		a.sessionMu.Unlock()
	}
	http.SetCookie(w, origin.cookie("", -1))
	w.WriteHeader(http.StatusNoContent)
}

func (a *ownerWebAuth) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if a.credentials != nil && r.Header.Get("Authorization") == "" &&
			path != "/internal" && !strings.HasPrefix(path, "/internal/") && !strings.HasPrefix(path, "/auth/") {
			origin := a.origin(r)
			if a.signedIn(r, origin) {
				if r.Method != http.MethodGet && r.Method != http.MethodHead && !checkOwnerOrigin(w, r, origin) {
					return
				}
				w.Header().Set("Cache-Control", "no-store")
				r = r.WithContext(context.WithValue(r.Context(), ownerSessionContextKey{}, a.ownerDigest))
			}
		}
		next.ServeHTTP(w, r)
	})
}
