package dashboard

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func withOpenCodeGoUsageServer(t *testing.T, status int, body string) *string {
	t.Helper()
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	prev := openCodeGoUsageURL
	openCodeGoUsageURL = srv.URL + "/inference/go/v1/usage"
	t.Cleanup(func() { openCodeGoUsageURL = prev })
	return &auth
}

func TestFetchOpenCodeGoUsage_MapsSubscriptionWindows(t *testing.T) {
	auth := withOpenCodeGoUsageServer(t, http.StatusOK,
		`{"usage":{"rolling":{"percent":4,"resetsAt":"2026-11-25T14:44:00Z"},
		          "weekly":{"percent":"13","resetsAt":1796000000},
		          "monthly":{"percent":12.5}}}`)

	res := fetchOpenCodeGoUsage(context.Background(), "sk-go")
	if res.plan != "OpenCode Go" {
		t.Errorf("plan = %q", res.plan)
	}
	if len(res.quotas) != 3 {
		t.Fatalf("expected 3 windows, got %d (%+v)", len(res.quotas), res.quotas)
	}
	if *auth != "Bearer sk-go" {
		t.Errorf("expected Bearer auth, got %q", *auth)
	}
	rolling, _ := res.quotas["Rolling"].(map[string]any)
	if rolling["used"] != 4.0 || rolling["remaining"] != 96.0 || rolling["total"] != 100.0 {
		t.Errorf("rolling window mis-mapped: %+v", rolling)
	}
	if rolling["resetAt"] != "2026-11-25T14:44:00Z" {
		t.Errorf("rolling resetAt = %v", rolling["resetAt"])
	}
	weekly, _ := res.quotas["Weekly"].(map[string]any)
	if weekly["used"] != 13.0 {
		t.Errorf("a string percent must parse: %+v", weekly)
	}
	if _, ok := res.quotas["Monthly"]; !ok {
		t.Errorf("monthly window missing: %+v", res.quotas)
	}
}

func TestFetchOpenCodeGoUsage_ReportsFailures(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		key     string
		wantMsg string
	}{
		{"no key", http.StatusOK, `{}`, "  ", "API key not available"},
		{"unauthorized", http.StatusUnauthorized, `{}`, "sk-go", "authentication failed"},
		{"subscription", http.StatusForbidden, `{"error":{"type":"EntitlementError"}}`, "sk-go", "subscription required"},
		{"forbidden", http.StatusForbidden, `{"error":{"type":"Nope"}}`, "sk-go", "access forbidden"},
		{"server error", http.StatusBadGateway, `{}`, "sk-go", "usage API error (502)"},
		{"no usage block", http.StatusOK, `{"ok":true}`, "sk-go", "did not contain quota data"},
		{"no usable windows", http.StatusOK, `{"usage":{"rolling":{}}}`, "sk-go", "valid quota data"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withOpenCodeGoUsageServer(t, tt.status, tt.body)
			res := fetchOpenCodeGoUsage(context.Background(), tt.key)
			if !strings.Contains(res.message, tt.wantMsg) {
				t.Errorf("message = %q, want substring %q", res.message, tt.wantMsg)
			}
		})
	}
}
