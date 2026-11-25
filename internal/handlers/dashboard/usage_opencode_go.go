package dashboard

// OpenCode Go quota — GET https://opencode.ai/inference/go/v1/usage
// Port of upstream open-sse/services/usage/opencode-go.js.
//
// The paid Go lane reports subscription usage as a percentage consumed per
// rolling / weekly / monthly window. Each window is normalized to the 0..100
// pair the dashboard already draws, and the provider's own reset time is kept
// so the countdown is the real one rather than a lock expiry.

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strings"

	"9router/proxy/internal/providers"
)

// openCodeGoUsageURL is the provider default (registry). Tests point it at a
// local server.
var openCodeGoUsageURL = providers.KnownProviders["opencode-go"].UsageURL

var openCodeGoQuotaNames = []struct {
	period string
	name   string
}{
	{"rolling", "Rolling"},
	{"weekly", "Weekly"},
	{"monthly", "Monthly"},
}

func fetchOpenCodeGoUsage(ctx context.Context, apiKey string) usageResult {
	const plan = "OpenCode Go"
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return usageResult{message: "OpenCode Go API key not available. Add a key to view usage."}
	}

	status, _, out, err := usageGet(ctx, openCodeGoUsageURL, map[string]string{
		"Authorization": "Bearer " + apiKey,
		"Accept":        "application/json",
		"User-Agent":    usageUserAgent,
	})
	if err != nil {
		return usageResult{message: fmt.Sprintf("OpenCode Go error: %v", err)}
	}

	switch status {
	case http.StatusUnauthorized:
		return usageResult{plan: plan, message: "OpenCode Go authentication failed. Check the API key."}
	case http.StatusForbidden:
		if openCodeZenIsEntitlementError(out) {
			return usageResult{plan: plan, message: "OpenCode Go subscription required for this API key."}
		}
		return usageResult{plan: plan, message: "OpenCode Go access forbidden for this API key."}
	}
	if status < 200 || status >= 300 {
		return usageResult{plan: plan, message: fmt.Sprintf("OpenCode Go usage API error (%d).", status)}
	}

	usage, ok := usageJSON(out)["usage"].(map[string]any)
	if !ok {
		return usageResult{plan: plan, message: "OpenCode Go usage response did not contain quota data."}
	}

	quotas := map[string]any{}
	for _, q := range openCodeGoQuotaNames {
		window, ok := usage[q.period].(map[string]any)
		if !ok {
			continue
		}
		percent, ok := usageFiniteNum(window["percent"])
		if !ok {
			continue
		}
		used := math.Max(0, math.Min(100, percent))
		quotas[q.name] = map[string]any{
			"used":                used,
			"total":               100.0,
			"remaining":           100 - used,
			"remainingPercentage": 100 - used,
			"resetAt":             usageResetTime(window["resetsAt"]),
			"unlimited":           false,
		}
	}
	if len(quotas) == 0 {
		return usageResult{plan: plan, message: "OpenCode Go usage response did not contain valid quota data."}
	}
	return usageResult{plan: plan, quotas: quotas}
}
