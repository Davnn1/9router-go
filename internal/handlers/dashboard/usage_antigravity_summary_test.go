package dashboard

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchAntigravityDashboardUsage_PrefersQuotaSummary(t *testing.T) {
	var modelCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1internal:loadCodeAssist":
			_, _ = w.Write([]byte(`{"currentTier":{"name":"Free"},"paidTier":{"id":"free-tier"}}`))
		case "/v1internal:retrieveUserQuotaSummary":
			_, _ = w.Write([]byte(`{"groups":[
				{"displayName":"Gemini Models","buckets":[
					{"bucketId":"gemini-5h","window":"5h","remainingFraction":0.85,"resetTime":"2026-11-19T15:00:00Z"},
					{"bucketId":"gemini-weekly","window":"weekly","remainingFraction":0.92,"resetTime":"2026-11-25T00:00:00Z"}]},
				{"displayName":"Claude and GPT Models","buckets":[
					{"bucketId":"claude-5h","window":"5h","remainingFraction":0.40,"resetTime":"2026-11-19T14:30:00Z"},
					{"bucketId":"claude-weekly","window":"weekly","remainingFraction":0.65,"resetTime":"2026-11-23T08:00:00Z"}]}
			]}`))
		case "/v1internal:fetchAvailableModels":
			modelCalls++
			_, _ = w.Write([]byte(`{"models":{}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	prevDashboard, prevDaily := antigravityDashboardBaseURL, antigravityDailyBaseURL
	antigravityDashboardBaseURL, antigravityDailyBaseURL = srv.URL, srv.URL
	t.Cleanup(func() {
		antigravityDashboardBaseURL, antigravityDailyBaseURL = prevDashboard, prevDaily
	})

	res := fetchAntigravityDashboardUsage(context.Background(), "tok", "proj")
	if modelCalls != 0 {
		t.Errorf("the per-model projection must not be read when summary buckets exist (calls=%d)", modelCalls)
	}
	tests := []struct {
		key        string
		used, rem  float64
		wantReset  string
		wantWindow string
	}{
		{"gemini_session", 15, 85, "2026-11-19T15:00:00Z", "5h"},
		{"gemini_weekly", 8, 92, "2026-11-25T00:00:00Z", "weekly"},
		{"claude_gpt_session", 60, 40, "2026-11-19T14:30:00Z", "5h"},
		{"claude_gpt_weekly", 35, 65, "2026-11-23T08:00:00Z", "weekly"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			quota, ok := res.quotas[tt.key].(map[string]any)
			if !ok {
				t.Fatalf("missing %s in %+v", tt.key, res.quotas)
			}
			if quota["used"] != tt.used || quota["remainingPercentage"] != tt.rem {
				t.Errorf("quota = %+v, want used=%v rem=%v", quota, tt.used, tt.rem)
			}
			if quota["resetAt"] != tt.wantReset || quota["window"] != tt.wantWindow {
				t.Errorf("reset/window = %v/%v", quota["resetAt"], quota["window"])
			}
		})
	}
}
