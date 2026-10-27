package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func emptyServer() *Server { return serverWithPlaces(nil) }

func doFrom(s *Server, ip, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = ip + ":1234"
	rec := httptest.NewRecorder()
	s.echo.ServeHTTP(rec, req)
	return rec
}

func TestGlobalRateLimit_BlocksAClientThatBurstsPastTheLimit(t *testing.T) {
	s := emptyServer()
	var limited int
	for i := 0; i < globalBurst+20; i++ {
		if doFrom(s, "203.0.113.5", http.MethodGet, "/places", "").Code == http.StatusTooManyRequests {
			limited++
		}
	}
	if limited == 0 {
		t.Fatal("expected some requests to be rejected with 429")
	}
}

func TestGlobalRateLimit_DoesNotTouchNormalUse(t *testing.T) {
	s := emptyServer()
	for i := 0; i < 10; i++ {
		if code := doFrom(s, "203.0.113.5", http.MethodGet, "/places", "").Code; code != http.StatusOK {
			t.Fatalf("request %d: got %d, want 200", i, code)
		}
	}
}

func TestRateLimit_IsPerClient(t *testing.T) {
	s := emptyServer()
	for i := 0; i < globalBurst+20; i++ {
		doFrom(s, "203.0.113.5", http.MethodGet, "/places", "")
	}
	if code := doFrom(s, "203.0.113.99", http.MethodGet, "/places", "").Code; code != http.StatusOK {
		t.Fatalf("a different client was limited too: got %d", code)
	}
}

func TestStrictRateLimit_CoversCredentialAndPaidRoutes(t *testing.T) {
	for _, path := range []string{"/login", "/register", "/forgot-password", "/reset-password", "/verify-email", "/resend-verification-code"} {
		s := emptyServer()
		limited := false
		for i := 0; i < strictBurst+5; i++ {
			if doFrom(s, "203.0.113.5", http.MethodPost, path, `{}`).Code == http.StatusTooManyRequests {
				limited = true
			}
		}
		if !limited {
			t.Errorf("%s was never rate limited", path)
		}
	}
}

func TestRateLimit_429CarriesRetryAfter(t *testing.T) {
	s := emptyServer()
	var rec *httptest.ResponseRecorder
	for i := 0; i < globalBurst+20; i++ {
		rec = doFrom(s, "203.0.113.5", http.MethodGet, "/places", "")
	}
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("got %d, Retry-After=%q", rec.Code, rec.Header().Get("Retry-After"))
	}
}
