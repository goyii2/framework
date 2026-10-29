package authclient

import (
	"strings"
	"testing"
)

func TestGenerateStateIsRandomAndURLSafe(t *testing.T) {
	seen := make(map[string]struct{}, 100)
	for i := 0; i < 100; i++ {
		state := GenerateState()
		if state == "" {
			t.Fatal("GenerateState returned an empty state")
		}
		if len(state) != 43 { // 32 bytes, raw URL-safe base64, no padding
			t.Fatalf("state length = %d, want 43: %q", len(state), state)
		}
		if strings.ContainsAny(state, "+/= \t\n") {
			t.Fatalf("state %q is not URL-safe", state)
		}
		seen[state] = struct{}{}
	}
	if len(seen) != 100 {
		t.Fatalf("GenerateState repeated a value: %d distinct out of 100", len(seen))
	}
}

func TestValidState(t *testing.T) {
	tests := []struct {
		name      string
		submitted string
		expected  string
		want      bool
	}{
		{"match", "the-state", "the-state", true},
		{"mismatch", "the-state", "other-state", false},
		{"empty submitted", "", "the-state", false},
		{"empty expected", "the-state", "", false},
		{"both empty", "", "", false},
		{"prefix is not equality", "the-state-extra", "the-state", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidState(tt.submitted, tt.expected); got != tt.want {
				t.Fatalf("ValidState(%q, %q) = %v, want %v", tt.submitted, tt.expected, got, tt.want)
			}
		})
	}
}
