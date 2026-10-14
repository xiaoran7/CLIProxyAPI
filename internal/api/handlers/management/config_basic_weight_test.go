package management

import "testing"

func TestNormalizeRoutingStrategyWeightedRoundRobin(t *testing.T) {
	for _, input := range []string{"weighted-round-robin", "weightedroundrobin", "wrr"} {
		got, ok := normalizeRoutingStrategy(input)
		if !ok || got != "weighted-round-robin" {
			t.Fatalf("normalizeRoutingStrategy(%q) = %q, %v; want weighted-round-robin, true", input, got, ok)
		}
	}
}

func TestNormalizeRoutingStrategySmartAndPace(t *testing.T) {
	for _, input := range []string{"smart", "smart-routing", "SMART"} {
		got, ok := normalizeRoutingStrategy(input)
		if !ok || got != "smart" {
			t.Fatalf("normalizeRoutingStrategy(%q) = %q, %v; want smart, true", input, got, ok)
		}
	}
	for _, input := range []string{"pace", "pace-routing", "PACE"} {
		got, ok := normalizeRoutingStrategy(input)
		if !ok || got != "pace" {
			t.Fatalf("normalizeRoutingStrategy(%q) = %q, %v; want pace, true", input, got, ok)
		}
	}
}
