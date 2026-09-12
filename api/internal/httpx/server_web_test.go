package httpx

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestWebServingKeepsAPIIsolatedAndFallsBackToSPA(t *testing.T) {
	dist := t.TempDir()
	if err := os.WriteFile(filepath.Join(dist, "index.html"), []byte("spa index"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dist, "main.js"), []byte("bundle"), 0o600); err != nil {
		t.Fatal(err)
	}
	api := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	handler := serveWeb(api, dist)

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantBody   string
	}{
		{name: "static file", method: http.MethodGet, path: "/main.js", wantStatus: http.StatusOK, wantBody: "bundle"},
		{name: "SPA fallback", method: http.MethodGet, path: "/properties/123", wantStatus: http.StatusOK, wantBody: "spa index"},
		{name: "API base stays on API handler", method: http.MethodGet, path: "/api/v1", wantStatus: http.StatusTeapot},
		{name: "API stays on API handler", method: http.MethodGet, path: "/api/v1/missing", wantStatus: http.StatusTeapot},
		{name: "non-GET stays on API handler", method: http.MethodPost, path: "/properties/123", wantStatus: http.StatusTeapot},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			if res.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", res.Code, tt.wantStatus)
			}
			if tt.wantBody != "" && res.Body.String() != tt.wantBody {
				t.Fatalf("body = %q, want %q", res.Body.String(), tt.wantBody)
			}
		})
	}

	t.Run("symlink escape falls back", func(t *testing.T) {
		outsideFile := filepath.Join(t.TempDir(), "attachment")
		if err := os.WriteFile(outsideFile, []byte("must not be served"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outsideFile, filepath.Join(dist, "attachment")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		request := httptest.NewRequest(http.MethodGet, "/attachment", nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK || response.Body.String() != "spa index" {
			t.Fatalf("response = (%d, %q), want (%d, %q)", response.Code, response.Body.String(), http.StatusOK, "spa index")
		}
	})
}
