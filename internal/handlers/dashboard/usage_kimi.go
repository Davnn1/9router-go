package dashboard

// Kimi Coding quota — GET https://api.kimi.com/coding/v1/usages
// Contract ported from OmniRoute open-sse/services/usage/kimi.ts (reference
// release v3.8.55) with the upstream decolua/9router kimi.js error semantics.
//
// The endpoint mixes three shapes: a Code 7-day count in `usage`, independent
// rolling windows in `limits[]`, and membership ratio windows in `usages`.
// Newer accounts also publish Claude-like `five_hour` / `seven_day` utilization
// windows. All of them are normalized to the dashboard's 0..100 quota pair.

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strings"

	"9router/proxy/internal/providers"
)

var kimiUsageURL = providers.KnownProviders["kimi"].UsageURL

var kimiPlanLevels = map[string]string{
	"LEVEL_BASIC":        "Moderato",
	"LEVEL_INTERMEDIATE": "Allegretto",
	"LEVEL_ADVANCED":     "Allegro",
	"LEVEL_STANDARD":     "Vivace",
}

func fetchKimiUsage(ctx context.Context, accessToken, apiKey string, psd map[string]any, baseURL string) usageResult {
	accessToken, apiKey = strings.TrimSpace(accessToken), strings.TrimSpace(apiKey)
	if accessToken == "" && apiKey == "" {
		return usageResult{message: "Kimi access token or API key not available."}
	}

	headers := map[string]string{"Accept": "application/json", "Content-Type": "application/json"}
	if apiKey != "" {
		headers["x-api-key"] = apiKey
	} else {
		headers["Authorization"] = "Bearer " + accessToken
		for k, v := range kimiUsageHeaders(psd) {
			headers[k] = v
		}
	}

	status, _, out, err := usageGet(ctx, kimiUsageURLFor(baseURL), headers)
	if err != nil {
		return usageResult{message: fmt.Sprintf("Kimi Coding connected. Unable to fetch usage: %v", err)}
	}

	data := usageJSON(out)
	if status < 200 || status >= 300 {
		return usageResult{plan: "Kimi Coding", message: kimiUsageError(status, data, out)}
	}
	if data == nil {
		return usageResult{plan: "Kimi Coding", message: "Kimi Coding connected. Invalid JSON response from API."}
	}

	quotas := map[string]any{}
	if q := kimiCountQuota(data["usage"]); q != nil {
		q["displayName"] = "Weekly · 7d"
		q["name"] = "Weekly · 7d"
		quotas["code_7d"] = q
	}
	kimiLimitWindows(quotas, data["limits"])
	kimiRatioWindows(quotas, data["usages"])
	kimiUtilizationWindow(quotas, data["five_hour"], "Rolling · 5h")
	kimiUtilizationWindow(quotas, data["seven_day"], "Weekly · 7d")
	for key, value := range data {
		if strings.HasPrefix(key, "seven_day_") && key != "seven_day" {
			kimiUtilizationWindow(quotas, value, "Weekly · 7d "+strings.TrimPrefix(key, "seven_day_"))
		}
	}

	plan := kimiPlan(data)
	if len(quotas) > 0 {
		return usageResult{plan: plan, quotas: quotas}
	}
	return usageResult{plan: plan, message: "Kimi Coding connected. Usage tracked per request."}
}

// kimiUsageURLFor keeps a self-hosted or relayed Kimi endpoint reporting its
// own quotas. Only the official Kimi hosts are rewritten; any other base URL
// keeps the registry default rather than inventing a path.
func kimiUsageURLFor(baseURL string) string {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || u.Host == "" {
		return kimiUsageURL
	}
	switch strings.ToLower(u.Hostname()) {
	case "api.kimi.com", "api.kimi.ai":
		return "https://" + u.Host + "/coding/v1/usages"
	}
	return kimiUsageURL
}

func kimiUsageHeaders(psd map[string]any) map[string]string {
	deviceID := usageStr(psd["deviceId"])
	if deviceID == "" {
		deviceID = "9router"
	}
	return map[string]string{
		"X-Msh-Platform":     "9router",
		"X-Msh-Version":      "1.0",
		"X-Msh-Device-Name":  "unknown",
		"X-Msh-Device-Model": "server",
		"X-Msh-Device-Id":    deviceID,
	}
}

func kimiUsageError(status int, data map[string]any, out []byte) string {
	if status == http.StatusUnauthorized {
		return "Kimi authentication expired. Please re-authorize."
	}
	reason, message := kimiErrorDetail(data)
	snippet := strings.ToLower(message + " " + string(out))
	if status == http.StatusForbidden &&
		(reason == "REASON_FEATURE_NO_PERMISSION" || strings.Contains(snippet, "permission_denied")) {
		return "Kimi connected, but this account has no permission to view usage. Subscribe to Kimi Code to access quota."
	}
	if message == "" {
		message = strings.TrimSpace(string(out))
	}
	if message != "" {
		if len(message) > 100 {
			message = message[:100]
		}
		return fmt.Sprintf("Kimi Coding connected. API Error %d: %s", status, message)
	}
	return fmt.Sprintf("Kimi Coding connected. API Error %d", status)
}

// kimiErrorDetail reads both Connect-RPC shapes: a top-level debug object and
// the details[0] envelope that carries the same reason/localizedMessage.
func kimiErrorDetail(data map[string]any) (reason, message string) {
	if data == nil {
		return "", ""
	}
	readDebug := func(debug map[string]any) {
		if reason == "" {
			reason = usageStr(debug["reason"])
		}
		if localized, ok := debug["localizedMessage"].(map[string]any); ok && message == "" {
			message = usageStr(localized["message"])
		}
	}
	if debug, ok := data["debug"].(map[string]any); ok {
		readDebug(debug)
	}
	if details, ok := data["details"].([]any); ok && len(details) > 0 {
		detail, _ := details[0].(map[string]any)
		if debug, ok := detail["debug"].(map[string]any); ok {
			readDebug(debug)
		}
		if localized, ok := detail["localizedMessage"].(map[string]any); ok && message == "" {
			message = usageStr(localized["message"])
		}
	}
	if message == "" {
		message = usageStr(data["message"])
	}
	if reason == "" {
		reason = usageStr(data["reason"])
	}
	return reason, message
}

// kimiResetTime accepts the reset spellings the endpoint mixes: proto-JSON
// resetTime, Connect-RPC reset_at, and camelCase resetAt.
func kimiResetTime(detail map[string]any) string {
	for _, key := range []string{"resetTime", "reset_time", "reset_at", "resetAt", "ResetTime"} {
		if value := usageResetTime(detail[key]); value != "" {
			return value
		}
	}
	return ""
}

// kimiCountQuota maps a {limit, used|remaining, resetTime} object.
func kimiCountQuota(value any) map[string]any {
	detail, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	limit, ok := usageFiniteNum(detail["limit"])
	if !ok || limit <= 0 {
		return nil
	}
	used, hasUsed := usageFiniteNum(detail["used"])
	remaining, hasRemaining := usageFiniteNum(detail["remaining"])
	if !hasUsed {
		used = 0
		if hasRemaining {
			used = limit - remaining
		}
	}
	used = math.Max(0, math.Min(limit, used))
	return map[string]any{
		"used":                used,
		"total":               limit,
		"remainingPercentage": (limit - used) / limit * 100,
		"resetAt":             kimiResetTime(detail),
		"unlimited":           false,
	}
}

func kimiLimitWindows(quotas map[string]any, value any) {
	limits, ok := value.([]any)
	if !ok {
		return
	}
	for i, raw := range limits {
		item, _ := raw.(map[string]any)
		if item == nil {
			continue
		}
		quota := kimiCountQuota(item["detail"])
		if quota == nil {
			continue
		}
		key, display := kimiWindowLabel(item["window"], i+1)
		key = "code_" + key
		if name := usageStr(item["name"]); name != "" {
			if strings.HasPrefix(strings.ToLower(name), "code") {
				display = name
			} else {
				display = "Code · " + name
			}
		}
		base := key
		for suffix := 2; ; suffix++ {
			if _, exists := quotas[key]; !exists {
				break
			}
			key = fmt.Sprintf("%s_%d", base, suffix)
		}
		quota["displayName"] = display
		quota["name"] = display
		quotas[key] = quota
	}
}

// kimiDurationLabel names a window by the side of the clock it measures. A
// five-hour window is rolling, not daily: Kimi starts a new one the moment the
// old one drains.
func kimiDurationLabel(value int64, unit string) string {
	switch {
	case unit == "h" && value == 5:
		return "Rolling · 5h"
	case (unit == "d" && value == 7) || (unit == "w" && value == 1):
		return "Weekly · 7d"
	case unit == "d":
		return fmt.Sprintf("Rolling · %dd", value)
	case unit == "w":
		return fmt.Sprintf("Weekly · %dw", value)
	case unit == "m":
		return fmt.Sprintf("Rolling · %dm", value)
	default:
		return fmt.Sprintf("Rolling · %d%s", value, unit)
	}
}

func kimiWindowLabel(value any, index int) (string, string) {
	window, _ := value.(map[string]any)
	duration, ok := usageFiniteNum(window["duration"])
	if !ok || duration <= 0 {
		return fmt.Sprintf("limit_%d", index), fmt.Sprintf("Code · Limit %d", index)
	}
	switch usageStr(window["timeUnit"]) {
	case "TIME_UNIT_MINUTE":
		if int64(duration)%60 == 0 {
			return fmt.Sprintf("%dh", int64(duration)/60), fmt.Sprintf("Code · %dh", int64(duration)/60)
		}
		return fmt.Sprintf("%dm", int64(duration)), kimiDurationLabel(int64(duration), "m")
	case "TIME_UNIT_HOUR":
		return fmt.Sprintf("%dh", int64(duration)), kimiDurationLabel(int64(duration), "h")
	case "TIME_UNIT_DAY":
		return fmt.Sprintf("%dd", int64(duration)), kimiDurationLabel(int64(duration), "d")
	case "TIME_UNIT_WEEK":
		return fmt.Sprintf("%dw", int64(duration)), kimiDurationLabel(int64(duration), "w")
	}
	return fmt.Sprintf("limit_%d", index), fmt.Sprintf("Code · Limit %d", index)
}

func kimiRatioWindows(quotas map[string]any, value any) {
	ratios, ok := value.(map[string]any)
	if !ok {
		return
	}
	windows := []struct{ source, key, display string }{
		{"limit_7d", "code_7d", "Weekly · 7d"},
		{"limit_5h", "code_5h", "Rolling · 5h"},
		{"limit_month_total", "monthly", "Monthly"},
	}
	for _, window := range windows {
		detail, ok := ratios[window.source].(map[string]any)
		if !ok {
			continue
		}
		ratio, ok := usageFiniteNum(detail["used_ratio"])
		if !ok || ratio < 0 {
			continue
		}
		used := math.Max(0, math.Min(100, math.Round(ratio*1e6)/1e4))
		quotas[window.key] = map[string]any{
			"used":                used,
			"total":               100.0,
			"remainingPercentage": 100 - used,
			"resetAt":             kimiResetTime(detail),
			"unlimited":           false,
			"displayName":         window.display,
			"name":                window.display,
			"isPercentageOnly":    true,
		}
	}
}

// kimiUtilizationWindow maps a Claude-like {utilization, resets_at} window.
// Kimi reports utilization as the percentage still remaining.
func kimiUtilizationWindow(quotas map[string]any, value any, name string) {
	window, ok := value.(map[string]any)
	if !ok {
		return
	}
	remaining, ok := usageFiniteNum(window["utilization"])
	if !ok {
		return
	}
	remaining = math.Max(0, math.Min(100, remaining))
	quotas[name] = map[string]any{
		"used":                100 - remaining,
		"total":               100.0,
		"remainingPercentage": remaining,
		"resetAt":             usageResetTime(window["resets_at"]),
		"unlimited":           false,
		"name":                name,
		"isPercentageOnly":    true,
	}
}

func kimiPlan(data map[string]any) string {
	user, _ := data["user"].(map[string]any)
	membership, _ := user["membership"].(map[string]any)
	level := usageStr(membership["level"])
	if level == "" {
		return "Kimi Coding"
	}
	if name, ok := kimiPlanLevels[level]; ok {
		return name
	}
	return strings.ToLower(strings.TrimPrefix(level, "LEVEL_"))
}
