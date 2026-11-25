package dashboard

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"9router/proxy/internal/models"
)

func withKimiUsageServer(t *testing.T, status int, body string) (url *string, apiKey, bearer *string) {
	t.Helper()
	var key, auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key = r.Header.Get("x-api-key")
		auth = r.Header.Get("Authorization")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	prev := kimiUsageURL
	kimiUsageURL = srv.URL + "/coding/v1/usages"
	t.Cleanup(func() { kimiUsageURL = prev })
	return &kimiUsageURL, &key, &auth
}

func TestFetchKimiUsage_MapsAllWindows(t *testing.T) {
	_, key, _ := withKimiUsageServer(t, http.StatusOK, `{
		"user":{"membership":{"level":"LEVEL_ADVANCED"}},
		"usage":{"limit":100,"used":92,"resetTime":"2026-11-20T00:00:00Z"},
		"limits":[{"name":"Code 5h","detail":{"limit":20,"remaining":3,"reset_at":"2026-11-19T10:00:00Z"},
		           "window":{"duration":5,"timeUnit":"TIME_UNIT_HOUR"}}],
		"usages":{"limit_7d":{"used_ratio":0.25,"reset_time":"2026-11-21T00:00:00Z"}},
		"five_hour":{"utilization":25,"resets_at":"2026-11-19T11:00:00Z"}
	}`)

	res := fetchKimiUsage(context.Background(), "", "sk-kimi", nil, "")
	if res.plan != "Allegro" {
		t.Errorf("plan = %q, want Allegro", res.plan)
	}
	if *key != "sk-kimi" {
		t.Errorf("api key header = %q", *key)
	}
	weekly, _ := res.quotas["code_7d"].(map[string]any)
	if weekly["used"] != 25.0 || weekly["remainingPercentage"] != 75.0 {
		t.Errorf("ratio window must win over the count: %+v", weekly)
	}
	if weekly["resetAt"] != "2026-11-21T00:00:00Z" {
		t.Errorf("ratio reset_time must parse: %+v", weekly)
	}
	limit, _ := res.quotas["code_5h"].(map[string]any)
	if limit["used"] != 17.0 || limit["total"] != 20.0 || limit["displayName"] != "Code 5h" {
		t.Errorf("limits window mis-mapped: %+v", limit)
	}
	if limit["resetAt"] != "2026-11-19T10:00:00Z" {
		t.Errorf("limits reset_at must parse: %+v", limit)
	}
	session, _ := res.quotas["session (5h)"].(map[string]any)
	if session["used"] != 75.0 || session["remainingPercentage"] != 25.0 {
		t.Errorf("utilization is remaining, not used: %+v", session)
	}
}

func TestFetchKimiUsage_UsesOAuthHeaders(t *testing.T) {
	_, _, auth := withKimiUsageServer(t, http.StatusOK, `{"usage":{"limit":10,"used":1}}`)
	res := fetchKimiUsage(context.Background(), "tok", "", map[string]any{"deviceId": "dev-1"}, "")
	if *auth != "Bearer tok" {
		t.Errorf("bearer header = %q", *auth)
	}
	if res.plan != "Kimi Coding" || len(res.quotas) == 0 {
		t.Errorf("unexpected result: %+v", res)
	}
}

func TestFetchKimiUsage_ReportsFailures(t *testing.T) {
	t.Run("unauthorized", func(t *testing.T) {
		withKimiUsageServer(t, http.StatusUnauthorized, `{}`)
		res := fetchKimiUsage(context.Background(), "tok", "", nil, "")
		if !strings.Contains(res.message, "re-authorize") {
			t.Errorf("message = %q", res.message)
		}
	})
	t.Run("no permission", func(t *testing.T) {
		withKimiUsageServer(t, http.StatusForbidden, `{"debug":{"reason":"REASON_FEATURE_NO_PERMISSION"}}`)
		res := fetchKimiUsage(context.Background(), "", "sk", nil, "")
		if !strings.Contains(res.message, "no permission to view usage") {
			t.Errorf("message = %q", res.message)
		}
	})
	t.Run("no quota data", func(t *testing.T) {
		withKimiUsageServer(t, http.StatusOK, `{"user":{"membership":{"level":"LEVEL_BASIC"}}}`)
		res := fetchKimiUsage(context.Background(), "", "sk", nil, "")
		if res.plan != "Moderato" || !strings.Contains(res.message, "tracked per request") {
			t.Errorf("result = %+v", res)
		}
	})
}

func TestFetchKimiUsage_ClampsOverLimit(t *testing.T) {
	withKimiUsageServer(t, http.StatusOK, `{"usage":{"limit":10,"used":50,"resetAt":"2026-11-20T00:00:00Z"}}`)
	res := fetchKimiUsage(context.Background(), "", "sk", nil, "")
	quota, _ := res.quotas["code_7d"].(map[string]any)
	if quota["used"] != 10.0 || quota["remainingPercentage"] != 0.0 {
		t.Errorf("over-limit usage must clamp to the limit: %+v", quota)
	}
}

func TestFetchKimiUsage_ReadsDetailsPermission(t *testing.T) {
	withKimiUsageServer(t, http.StatusForbidden,
		`{"details":[{"debug":{"reason":"REASON_FEATURE_NO_PERMISSION","localizedMessage":{"message":"subscribe"}}}]}`)
	res := fetchKimiUsage(context.Background(), "", "sk", nil, "")
	if !strings.Contains(res.message, "no permission to view usage") {
		t.Errorf("details[0] permission must be recognised: %q", res.message)
	}
}

func TestKimiCodingIsUsageEligible(t *testing.T) {
	for _, auth := range []string{"oauth", "apikey"} {
		conn := &models.ProviderConnection{Provider: "kimi-coding", AuthType: auth}
		if !isUsageEligibleConnection(conn) {
			t.Errorf("kimi-coding %s connection must reach the usage endpoint", auth)
		}
	}
}
