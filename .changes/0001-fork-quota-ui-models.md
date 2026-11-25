### Quota view controls, Kimi labels, catalog merge, and model discovery

- **Used / remaining percentages** — the quota tracker menu now switches every
  row between the remaining share and the used share, stored per browser in
  `localStorage`. Rows that only publish a percentage hide the redundant
  `used / total` pair.
- **Kimi labels** — `code_5h` / `code_7d` now render as `Code · 5h` /
  `Code · 7d`, and percentage-only windows render as `18.4% used` instead of
  `18.41 / 100`. The default quota parser also prefers a provider's
  `displayName` over the raw map key, which fixes Antigravity's family rows.
- **One Kimi provider card** — `kimi-coding` is a legacy id; the catalog hides
  its duplicate card and `MigrateLegacyProviderIDs` moves stored connections to
  `kimi` on boot, so the existing account appears under Kimi.
- **Model discovery on sync** — `POST /api/models/sync` now stores live
  catalogue entries the static registry does not know as custom models, so a
  provider that ships a new model appears in the picker after one Sync Models
  click. Static rows, existing custom rows and passthrough providers are left
  untouched.

Verified with `go test ./internal/handlers/dashboard/... ./internal/db/...`,
`go build ./cmd/9router-go/`, and the full Docker web build (`tsc -b && vite
build`).

- **Kimi concurrency slot in-router** — `ForwardKimi` serialises one credential
  with a two-second slot wait and answers 429 when the slot is held, replacing
  the queue sidecar's only job. Combo members therefore fail over to the next
  model instead of waiting out a long queue.
