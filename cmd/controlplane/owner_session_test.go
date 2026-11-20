package main

import (
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testOwnerEmail = "owner@example.test"
const testOwnerPassword = "fixture password only"

func ownerAuthFixture(t *testing.T) (*ownerWebAuth, http.Handler) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "owner.json")
	salt := []byte("test-salt-only!!")
	data, err := json.Marshal(ownerCredentials{Version: 1, Email: testOwnerEmail, Salt: salt, Hash: ownerPasswordHash([]byte(testOwnerPassword), salt)})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	credentials, err := loadOwnerCredentials(path)
	if err != nil {
		t.Fatal(err)
	}
	auth := newOwnerWebAuth(credentials, "owner-machine-key", "example.test:8443")
	mux := http.NewServeMux()
	auth.register(mux)
	done := func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }
	mux.HandleFunc("/tasks", authorized("owner-machine-key", done))
	mux.HandleFunc("/internal/test-agent", authorized("agent-key", done))
	return auth, auth.middleware(mux)
}

func ownerAuthRequest(handler http.Handler, method, base, path, origin, bearer, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, strings.Replace(base, "https://", "http://", 1)+path, strings.NewReader(body))
	request.RemoteAddr = "127.0.0.1:43210"
	if strings.HasPrefix(base, "https://") {
		request.Header.Set("X-Forwarded-Proto", "https")
	}
	if origin != "" {
		request.Header.Set("Origin", origin)
	}
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestOwnerBrowserSession(t *testing.T) {
	for _, base := range []string{"http://127.0.0.1:8080", "https://forgeos.example.test:8443"} {
		t.Run(base, func(t *testing.T) {
			auth, handler := ownerAuthFixture(t)
			login := `{"email":" Owner@Example.Test ","password":"` + testOwnerPassword + `"}`
			response := ownerAuthRequest(handler, "POST", base, "/auth/login", base, "", login, nil)
			if response.Code != http.StatusOK {
				t.Fatalf("login: %d %s", response.Code, response.Body.String())
			}
			cookies := response.Result().Cookies()
			secure := strings.HasPrefix(base, "https://")
			if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].Path != "/" || cookies[0].Domain != "" ||
				cookies[0].SameSite != http.SameSiteStrictMode || cookies[0].Secure != secure ||
				strings.HasPrefix(cookies[0].Name, "__Host-") != secure {
				t.Fatal("login did not set the scoped browser cookie")
			}
			cookie := cookies[0]
			for _, check := range []struct {
				method, path, origin, bearer string
				want                         int
			}{
				{"GET", "/auth/session", "", "", 200},
				{"GET", "/tasks", "", "", 204},
				{"POST", "/tasks", base, "", 204},
				{"POST", "/tasks", "", "", 403},
				{"POST", "/tasks", "https://p-test.example.test:8443", "", 403},
				{"POST", "/tasks", "", "owner-machine-key", 204},
				{"GET", "/tasks", "", "agent-key", 401},
				{"GET", "/internal/test-agent", "", "", 401},
				{"GET", "/internal/test-agent", "", "agent-key", 204},
				{"POST", "/auth/logout", "https://p-test.example.test:8443", "", 403},
			} {
				got := ownerAuthRequest(handler, check.method, base, check.path, check.origin, check.bearer, "", cookie)
				if got.Code != check.want {
					t.Errorf("%s %s origin=%q bearer=%t: %d, want %d", check.method, check.path, check.origin, check.bearer != "", got.Code, check.want)
				}
			}
			forgedOrigin := "http://127.0.0.1:3000"
			forgedCookie := *cookie
			forgedCookie.Name = "forgeos-owner-local"
			if got := ownerAuthRequest(handler, "GET", forgedOrigin, "/tasks", "", "", "", &forgedCookie); got.Code != http.StatusUnauthorized {
				t.Fatal("a different origin accepted the browser session")
			}
			response = ownerAuthRequest(handler, "POST", base, "/auth/logout", base, "", "", cookie)
			if expired := response.Result().Cookies(); response.Code != 204 || len(expired) != 1 || expired[0].MaxAge >= 0 {
				t.Fatal("logout did not clear the cookie")
			}
			if got := ownerAuthRequest(handler, "GET", base, "/tasks", "", "", "", cookie); got.Code != http.StatusUnauthorized {
				t.Fatal("logged out session could be replayed")
			}
			response = ownerAuthRequest(handler, "POST", base, "/auth/login", base, "", login, nil)
			cookie = response.Result().Cookies()[0]
			key := sha256.Sum256([]byte(cookie.Value))
			auth.sessions[key] = ownerSession{origin: base, expires: time.Now().Add(-time.Minute)}
			if got := ownerAuthRequest(handler, "GET", base, "/auth/session", "", "", "", cookie); got.Code != http.StatusUnauthorized {
				t.Fatal("expired session was accepted")
			}
		})
	}
}

func TestOwnerLoginRejectsCrossOriginAndLimitsFailures(t *testing.T) {
	auth, handler := ownerAuthFixture(t)
	const base = "http://127.0.0.1:8080"
	login := `{"email":"` + testOwnerEmail + `","password":"` + testOwnerPassword + `"}`
	for _, origin := range []string{"", "http://127.0.0.1:3000", "https://127.0.0.1:8080"} {
		if got := ownerAuthRequest(handler, "POST", base, "/auth/login", origin, "", login, nil); got.Code != 403 {
			t.Fatalf("login accepted a different origin %q", origin)
		}
	}
	for i := 0; i < ownerLoginAttempts; i++ {
		if got := ownerAuthRequest(handler, "POST", base, "/auth/login", base, "", `{"email":"stranger@example.test","password":"incorrect"}`, nil); got.Code != 401 {
			t.Fatalf("invalid login status: %d", got.Code)
		}
	}
	if got := ownerAuthRequest(handler, "POST", base, "/auth/login", base, "", login, nil); got.Code != 429 {
		t.Fatal("login did not limit repeated failures")
	}
	auth.loginWindow = time.Now().Add(-time.Second)
	if got := ownerAuthRequest(handler, "POST", base, "/auth/login", base, "", login, nil); got.Code != 200 {
		t.Fatalf("login did not recover after the rate limit: %d", got.Code)
	}
}
