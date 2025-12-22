package server

import "testing"

func TestNormalizeRole(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"ADMIN", RoleAdmin},
		{"reader", RoleReader},
		{"", RoleAdmin},
		{"unknown", RoleAdmin},
	}
	for _, tc := range cases {
		if got := normalizeRole(tc.in); got != tc.want {
			t.Fatalf("normalizeRole(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

func TestHasRequiredRole(t *testing.T) {
	tests := []struct {
		current  string
		required string
		allowed  bool
	}{
		{RoleAdmin, RoleReader, true},
		{RoleAdmin, RoleAdmin, true},
		{RoleReader, RoleReader, true},
		{RoleReader, RoleAdmin, false},
		{"", RoleReader, true},
	}
	for _, tt := range tests {
		if got := hasRequiredRole(tt.current, tt.required); got != tt.allowed {
			t.Fatalf("hasRequiredRole(%q,%q)=%v want %v", tt.current, tt.required, got, tt.allowed)
		}
	}
}
