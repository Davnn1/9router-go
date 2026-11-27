package providers

import (
	"reflect"
	"testing"
)

func TestAntigravityFamilyRegistry(t *testing.T) {
	RegisterAntigravityFamily("Gemini-5-Neon", []string{"low", "high", "max"})
	got := AntigravityFamilyLevels("gemini-5-neon")
	if !reflect.DeepEqual(got, []string{"low", "high", "max"}) {
		t.Fatalf("levels = %v, want low/high/max", got)
	}
	if !AntigravityFamilyServesTier("gemini-5-neon", "max") {
		t.Error("max tier must be served once the catalogue reported it")
	}

	// Additive and idempotent: another account reporting a subset must not
	// remove a tier, and a repeated sync must not duplicate one.
	RegisterAntigravityFamily("gemini-5-neon", []string{"low", "max"})
	if got := AntigravityFamilyLevels("gemini-5-neon"); !reflect.DeepEqual(got, []string{"low", "high", "max"}) {
		t.Fatalf("after repeat levels = %v", got)
	}
}
