package main

import "testing"

func TestProjectPreviewAPIRouteBoundaries(t *testing.T) {
	api := &projectPreviewAPI{Prefix: "/api"}
	for path, want := range map[string]bool{"/api": true, "/api/orders": true, "/api/v1/orders": true,
		"/apiary": false, "/assets/api": false, "/": false} {
		if got := api.matches(path); got != want {
			t.Errorf("matches(%q) = %v, want %v", path, got, want)
		}
	}
	for _, value := range []string{"http://127.0.0.1:3000/health", "//example.invalid/health", "/health#fragment", "/\\elsewhere"} {
		if validProjectHealthPath(value) {
			t.Errorf("accepted nonlocal health path %q", value)
		}
	}
	if !validProjectHealthPath("/health?ready=1") {
		t.Fatal("rejected same-service health check")
	}
}
