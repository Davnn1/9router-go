### Usage tracking parity for OpenCode Go, Kimi Coding and Antigravity

- **OpenCode Go quota** — new `usage_opencode_go.go` reads
  `https://opencode.ai/inference/go/v1/usage` and maps the rolling, weekly and
  monthly percentage windows with the provider's own `resetsAt`. The connection
  previously fell through to the model-lock fallback, which rendered lock
  expiries as `0 / ∞` quota rows. Ported from upstream `decolua/9router`
  `open-sse/services/usage/opencode-go.js` (#3791, endpoint fix 2026-11-08).
- **Kimi Coding quota** — new `usage_kimi.go` maps the Code 7-day count,
  independent `limits[]` windows, membership `usages` ratios, and the
  `five_hour` / `seven_day` utilization windows. The contract follows OmniRoute
  `open-sse/services/usage/kimi.ts` at `release/v3.8.55`; error text follows
  upstream `kimi.js`. Both `kimi` and `kimi-coding` ids dispatch to it.
- **Antigravity presentation** — `fetchAntigravityDashboardUsage` now prefers the
  `retrieveUserQuotaSummary` group/bucket rows (the same family+window shape the
  Antigravity CLI prints through `agy -p /usage`, including `resetTime`), and
  only falls back to the per-model `fetchAvailableModels` projection when the
  summary is empty. Free-tier accounts no longer render a wall of optimistic
  100%-remaining model rows.

Verified with `go test ./internal/handlers/dashboard/...` (new table-driven
handlers tests) and `go build ./cmd/9router-go/`.
