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

- **Custom model names in the combo picker** — the picker no longer hides a
  custom row whose name differs from its id when the provider also has a static
  catalogue, and a sync upgrades a hand-added `name == id` row to the catalogue's
  display name.
- **Kimi window labels** — `Rolling · 5h` and `Weekly · 7d` instead of the raw
  `code_5h` / `code_7d` keys.
- **Configurable JSON body ceiling** — `MAX_JSON_BODY_MB` raises the 10MB JSON
  cap for clients that send base64 images; values outside 1..256 fall back to
  10MB.
- **Kimi OAuth refresh under the canonical id** — the OAuth client is registered
  for `kimi` as well as the legacy `kimi-coding`, so a migrated connection keeps
  refreshing instead of answering 401.

- **Direct egress as a rotation slot** — `includeDirect` adds the gateway's own
  address to a NoAuth provider's proxy rotation, so a provider can rotate over
  direct + every active pool instead of using direct only after all pools
  refused.

- **Trim images from older turns** — `IMAGE_HISTORY_TURNS=N` keeps images in the
  last N user turns and replaces older image blocks with a short marker before
  forwarding, so a long chat stops carrying every screenshot's base64 and
  vision tokens. The OpenAI, Claude Messages and Responses shapes are handled.
