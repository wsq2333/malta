package cli

import "testing"

func TestServeAccessRequiresExplicitUnauthenticatedOptIn(t *testing.T) {
	for _, tt := range []struct {
		address, token string
		optIn, valid   bool
	}{
		{"127.0.0.1:8080", "", false, true},
		{"[::1]:8080", "", false, true},
		{"0.0.0.0:8080", "", false, false},
		{":8080", "", false, false},
		{"0.0.0.0:8080", "token", false, true},
		{"0.0.0.0:8080", "", true, true},
		{"invalid", "", true, false},
	} {
		if err := validateServeAccess(tt.address, tt.token, tt.optIn); (err == nil) != tt.valid {
			t.Errorf("address=%q optIn=%v: %v", tt.address, tt.optIn, err)
		}
	}
}
