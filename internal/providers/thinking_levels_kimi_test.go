package providers

import (
	"reflect"
	"testing"
)

// The Kimi plan catalogue publishes effort options per model:
//
//	k3 / k3.1-pro / k3.1-pro-highspeed / k3-256k  low, high, max   (always on)
//	kimi-for-coding                                low, high, max   (default max)
//	kimi-for-coding-highspeed                      Thinking: ON, no options
//
// The wire default for the kimi format is [none, low, medium, high, max], so a
// model without a pattern row offers a "medium" the endpoint does not serve.
func TestKimiPlanThinkingLevels(t *testing.T) {
	cases := []struct {
		model string
		want  []string
	}{
		{"kimi-for-coding", []string{"low", "high", "max"}},
		{"kimi-for-coding-highspeed", nil},
		{"kimi-k2.7-code", nil},
		{"kimi-k2.7-code-highspeed", nil},
		{"k3", []string{"low", "high", "max"}},
		{"kimi-k3", []string{"low", "high", "max"}},
		{"k3.1-pro", []string{"low", "high", "max"}},
		{"k3.1-pro-highspeed", []string{"low", "high", "max"}},
	}
	for _, tc := range cases {
		InvalidateCapabilitiesCache()
		got := GetThinkingLevels("kimi", tc.model)
		if len(got) == 0 && len(tc.want) == 0 {
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("GetThinkingLevels(kimi, %s) = %v, want %v", tc.model, got, tc.want)
		}
	}
}

// A "(level)" suffix is a request override, not part of the id, so it must not
// change the set the picker offers.
func TestKimiPlanThinkingLevelsIgnoreSuffix(t *testing.T) {
	InvalidateCapabilitiesCache()
	if got := GetThinkingLevels("kimi", "kimi-for-coding(low)"); !reflect.DeepEqual(got, []string{"low", "high", "max"}) {
		t.Fatalf("suffixed levels = %v, want low/high/max", got)
	}
}
