package dashboard

import (
	"testing"

	"9router/proxy/internal/providers"
)

func TestAddDiscoveredModelsStoresOnlyUnknownIDs(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()
	h := NewDashboardHandler(repo)

	staticID := "gemini-4-argon"
	if len(providers.GetProviderModels("antigravity")) == 0 {
		t.Skip("antigravity has no static model catalogue")
	}
	entries := []customModelBatchEntry{
		{ID: staticID, Name: "static"},
		{ID: "gemini-9-test", Name: "Gemini 9 Test", Vision: true},
		{ID: "", Name: "empty"},
	}

	added, err := h.addDiscoveredModels("antigravity", entries)
	if err != nil {
		t.Fatalf("addDiscoveredModels: %v", err)
	}
	if added != 1 {
		t.Fatalf("added = %d, want 1", added)
	}
	scope, err := repo.GetKVScope("customModels")
	if err != nil {
		t.Fatalf("GetKVScope: %v", err)
	}
	if _, ok := scope[customModelKey("antigravity", "gemini-9-test", "llm")]; !ok {
		t.Errorf("discovered model missing from customModels: %v", scope)
	}
	if _, ok := scope[customModelKey("antigravity", staticID, "llm")]; ok {
		t.Errorf("static catalogue row must not become a custom model")
	}

	// A second pass is idempotent.
	added, err = h.addDiscoveredModels("antigravity", entries)
	if err != nil || added != 0 {
		t.Errorf("second pass added=%d err=%v, want 0/nil", added, err)
	}
}

func TestAddDiscoveredModelsSkipsPassthroughProvider(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()
	h := NewDashboardHandler(repo)
	added, err := h.addDiscoveredModels("opencode-zen", []customModelBatchEntry{{ID: "brand-new"}})
	if err != nil || added != 0 {
		t.Errorf("passthrough provider added=%d err=%v, want 0/nil", added, err)
	}
}

func TestAntigravityVirtualFamiliesDeriveBareIDs(t *testing.T) {
	entries := []customModelBatchEntry{
		{ID: "gemini-4-neon-low"},
		{ID: "gemini-4-neon-medium"},
		{ID: "gemini-4-neon-high"},
		{ID: "gemini-4-hydrogen-medium"},
		{ID: "gemini-4-hydrogen-high"},
		{ID: "gemini-4-argon"},
	}
	got := antigravityVirtualFamilies(entries)
	ids := map[string]string{}
	for _, entry := range got {
		ids[entry.ID] = entry.Name
	}
	if ids["gemini-4-neon"] == "" || ids["gemini-4-hydrogen"] == "" {
		t.Fatalf("virtual families = %v, want neon and hydrogen", ids)
	}
	if _, ok := ids["gemini-4-argon"]; ok {
		t.Errorf("a family that already exists upstream must not be re-added: %v", ids)
	}
}
