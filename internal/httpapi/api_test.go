package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cave-microclimate-clearance/internal/application"
	"cave-microclimate-clearance/internal/evidence"
	"cave-microclimate-clearance/internal/store"
)

func testAPI(t *testing.T) http.Handler {
	t.Helper()
	repo, err := store.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return New(application.NewService(repo), evidence.NewService(repo)).Handler()
}

func TestHealth(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	testAPI(t).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("healthz=%d", w.Code)
	}
	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("缺少安全响应头")
	}
}

func TestStrictJSONAndContentType(t *testing.T) {
	h := testAPI(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trials", strings.NewReader(`{"unknown":true}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "unknown field") {
		t.Fatalf("未知字段响应: %d %s", w.Code, w.Body.String())
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/trials", strings.NewReader(`{}`))
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "Content-Type") {
		t.Fatalf("媒体类型响应: %d %s", w.Code, w.Body.String())
	}
}
