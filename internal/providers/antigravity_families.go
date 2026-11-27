package providers

// Antigravity publishes some Gemini families only as per-tier ids
// (gemini-4-neon-low/medium/high). A bare family id is a virtual route the
// gateway can serve, but the tier list belongs to the live catalogue, not to a
// hard-coded table. Sync registers what it actually saw here; the translator
// and the caps endpoint read it back.

import (
	"sort"
	"strings"
	"sync"
)

var antigravityFamilyTiers sync.Map // base -> []string

var antigravityCanonicalTiers = []string{"minimal", "low", "medium", "high", "xhigh", "max", "ultra"}

// RegisterAntigravityFamily records the tier suffixes seen for one bare family.
// It is additive so two accounts reporting different subsets keep the union,
// and idempotent so a repeated sync does not grow the list.
func RegisterAntigravityFamily(base string, tiers []string) {
	base = strings.ToLower(strings.TrimSpace(base))
	if base == "" {
		return
	}
	seen := map[string]bool{}
	if existing, ok := antigravityFamilyTiers.Load(base); ok {
		for _, tier := range existing.([]string) {
			seen[tier] = true
		}
	}
	for _, tier := range tiers {
		tier = strings.ToLower(strings.TrimSpace(tier))
		if isAntigravityTier(tier) {
			seen[tier] = true
		}
	}
	if len(seen) == 0 {
		return
	}
	out := make([]string, 0, len(seen))
	for _, tier := range antigravityCanonicalTiers {
		if seen[tier] {
			out = append(out, tier)
		}
	}
	antigravityFamilyTiers.Store(base, out)
}

// AntigravityFamilyLevels returns the registered effort levels for a bare
// family, in canonical order. Empty when the family was not discovered.
func AntigravityFamilyLevels(base string) []string {
	value, ok := antigravityFamilyTiers.Load(strings.ToLower(strings.TrimSpace(base)))
	if !ok {
		return nil
	}
	tiers := value.([]string)
	return append([]string(nil), tiers...)
}

// AntigravityFamilyServesTier reports whether the live catalogue advertised
// this base/tier pair. The translator uses it before appending the suffix.
func AntigravityFamilyServesTier(base, tier string) bool {
	value, ok := antigravityFamilyTiers.Load(strings.ToLower(strings.TrimSpace(base)))
	if !ok {
		return false
	}
	for _, known := range value.([]string) {
		if known == strings.ToLower(strings.TrimSpace(tier)) {
			return true
		}
	}
	return false
}

func isAntigravityTier(tier string) bool {
	for _, known := range antigravityCanonicalTiers {
		if tier == known {
			return true
		}
	}
	return false
}

// SortedAntigravityFamilies is used by tests and diagnostics; it does not
// expose the internal map.
func SortedAntigravityFamilies() []string {
	var out []string
	antigravityFamilyTiers.Range(func(key, _ any) bool {
		out = append(out, key.(string))
		return true
	})
	sort.Strings(out)
	return out
}
