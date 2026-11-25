package dashboard

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"fmt"
	"net/http"
	"strings"

	"9router/proxy/internal/db"
	"9router/proxy/internal/handlerutil"
	"9router/proxy/internal/log"
	"9router/proxy/internal/models"
	"9router/proxy/internal/providers"
)

// HandleSyncProviderModels handles POST /api/models/sync.
//
// It re-reads each connection's upstream catalogue and reconciles the result
// against the recorded deprecations: a model the provider still publishes is
// alive and loses its badge.
//
// A model the catalogue *omits* is deliberately not badged. Catalogues are
// frequently partial — a scoped key, a paginated feed, a node behind a feature
// flag — so absence is weak evidence, and acting on it would blacklist healthy
// models. A model is retired only when a live request gets a 410 naming it, or
// when one comes back by serving a request (#179).
//
// The provider query parameter is optional: "" syncs every provider that can
// list models.
func (h *DashboardHandler) HandleSyncProviderModels(w http.ResponseWriter, r *http.Request) {
	provider := strings.TrimSpace(r.URL.Query().Get("provider"))
	conns, err := h.Repo.GetProviderConnections(provider, true)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(conns) == 0 {
		handlerutil.WriteJSONError(w, http.StatusNotFound, "no active connections to sync")
		return
	}

	synced, failed, skipped, revived, added := 0, 0, 0, 0, 0
	var lastErr error
	for _, conn := range conns {
		if conn == nil {
			continue
		}
		live, entries, fetchErr, listable := h.fetchLiveModelIDs(r, conn)
		if !listable {
			// The provider has no catalogue endpoint at all. That is a
			// property of the provider, not a failed sync, and reporting it as
			// one would turn every Sync Models click on such a provider into a
			// 502 the operator cannot act on.
			skipped++
			continue
		}
		if fetchErr != nil {
			failed++
			lastErr = fetchErr
			log.Warn("models-sync", "catalogue unreachable", "provider", conn.Provider, "conn", conn.ID, "error", fetchErr)
			continue
		}
		if len(live) == 0 {
			continue
		}
		cleared, clearErr := h.Repo.RetainLiveModels(conn.Provider, live)
		if clearErr != nil {
			failed++
			lastErr = clearErr
			continue
		}
		discovered, addErr := h.addDiscoveredModels(conn.Provider, entries)
		if addErr != nil {
			failed++
			lastErr = addErr
			log.Warn("models-sync", "could not store discovered models", "provider", conn.Provider, "error", addErr)
			continue
		}
		synced++
		revived += cleared
		added += discovered
		log.Info("models-sync", "catalogue refreshed", "provider", conn.Provider, "models", len(live), "revived", cleared, "added", discovered)
	}

	if synced == 0 && skipped == 0 && failed > 0 {
		handlerutil.WriteJSONError(w, http.StatusBadGateway, "model sync failed: "+lastErr.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"success":      true,
		"provider":     provider,
		"synced":       synced,
		"failed":       failed,
		"skipped":      skipped,
		"revived":      revived,
		"added":        added,
		"lastError":    errText(lastErr),
		"catalogState": providers.GetCatalogState(),
	})
}

// errText renders an optional error for a JSON field.
func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// fetchLiveModelIDs returns the set of model ids a connection's upstream
// currently publishes, keyed the way deprecations are keyed.
//
// listable is false when the provider has no catalogue endpoint, which is a
// property of the provider rather than a sync failure — the dashboard answers
// 400 for those, and treating that as an error would make every provider
// without a catalogue report a broken sync.
func (h *DashboardHandler) fetchLiveModelIDs(r *http.Request, conn *models.ProviderConnection) (map[string]bool, []customModelBatchEntry, error, bool) {
	capture := newModelsCapture()
	h.writeConnectionModels(capture, r, conn.ID)
	if capture.status == http.StatusBadRequest {
		return nil, nil, nil, false
	}
	if capture.status != http.StatusOK {
		return nil, nil, fmt.Errorf("catalogue unavailable: %s", strings.TrimSpace(capture.body.String())), true
	}

	var resp struct {
		Models []struct {
			ID           string          `json:"id"`
			Model        string          `json:"model"`
			Name         string          `json:"name"`
			Capabilities map[string]bool `json:"capabilities"`
		} `json:"models"`
	}
	if err := json.Unmarshal(capture.body.Bytes(), &resp); err != nil {
		return nil, nil, fmt.Errorf("invalid catalogue response: %w", err), true
	}

	live := make(map[string]bool, len(resp.Models))
	entries := make([]customModelBatchEntry, 0, len(resp.Models))
	for _, m := range resp.Models {
		id := m.ID
		if id == "" {
			id = m.Model
		}
		if id == "" {
			continue
		}
		live[providers.DeprecationKey(conn.Provider, id)] = true
		entries = append(entries, customModelBatchEntry{
			ID:        id,
			Name:      m.Name,
			Type:      "llm",
			Vision:    m.Capabilities["vision"],
			Reasoning: m.Capabilities["reasoning"],
		})
	}
	return live, entries, nil, true
}

// addDiscoveredModels stores catalogue entries the static registry does not
// know yet as custom models, so a provider that ships a new model appears in
// the picker without a code release. A static row, an existing custom row and
// a passthrough provider are all left alone: the first two already resolve,
// and the third lists its live catalogue at request time.
func (h *DashboardHandler) addDiscoveredModels(providerID string, entries []customModelBatchEntry) (int, error) {
	if len(entries) == 0 {
		return 0, nil
	}
	if meta, ok := providers.GetProviderMeta(providerID); ok && meta.PassthroughModels {
		return 0, nil
	}
	static := make(map[string]bool)
	for _, id := range providers.GetProviderModels(providerID) {
		static[id] = true
	}
	existing, err := h.Repo.GetKVScope("customModels")
	if err != nil {
		return 0, err
	}
	writes := make([]db.KVEntry, 0, len(entries))
	seen := make(map[string]bool)
	for _, entry := range entries {
		if entry.ID == "" || static[entry.ID] || seen[entry.ID] {
			continue
		}
		key := customModelKey(providerID, entry.ID, "llm")
		if raw, exists := existing[key]; exists {
			// A hand-added row often carries the id as its name. The catalogue
			// knows the display name, so upgrade that row instead of leaving the
			// picker on a raw id forever.
			var current db.CustomModel
			if err := json.Unmarshal([]byte(raw), &current); err == nil &&
				(current.Name == "" || current.Name == current.ID) &&
				entry.Name != "" && entry.Name != entry.ID {
				value, err := json.Marshal(customModelValue(providerID, entry.ID, "llm", entry))
				if err != nil {
					return len(writes), fmt.Errorf("encode discovered model %q: %w", entry.ID, err)
				}
				writes = append(writes, db.KVEntry{Key: key, Value: string(value)})
			}
			continue
		}
		value, err := json.Marshal(customModelValue(providerID, entry.ID, "llm", entry))
		if err != nil {
			return len(writes), fmt.Errorf("encode discovered model %q: %w", entry.ID, err)
		}
		seen[entry.ID] = true
		writes = append(writes, db.KVEntry{Key: key, Value: string(value)})
	}
	if err := h.Repo.SetKVMany(context.Background(), "customModels", writes); err != nil {
		return 0, err
	}
	return len(writes), nil
}

// modelsCapture is a ResponseWriter that keeps what the discovery handler wrote
// instead of sending it anywhere, so sync can reuse that handler and read the
// catalogue off it.
type modelsCapture struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func newModelsCapture() *modelsCapture {
	return &modelsCapture{header: make(http.Header), status: http.StatusOK}
}

func (c *modelsCapture) Header() http.Header { return c.header }

func (c *modelsCapture) Write(p []byte) (int, error) { return c.body.Write(p) }

func (c *modelsCapture) WriteHeader(status int) { c.status = status }
