package panel

import "testing"

func TestValidShortUUID(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{"0cb6c0a7beed", true},
		{"user_01-abc", true},
		{"", false},
		{"bad/value", false},
		{"bad..value", false},
		{"bad value", false},
	}
	for _, tt := range tests {
		if got := ValidShortUUID(tt.value); got != tt.want {
			t.Fatalf("ValidShortUUID(%q) = %v, want %v", tt.value, got, tt.want)
		}
	}
}
