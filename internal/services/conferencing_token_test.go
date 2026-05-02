package services

import "testing"

func TestIsPermanentTokenError(t *testing.T) {
	cases := []struct {
		code string
		want bool
	}{
		{"invalid_grant", true},        // refresh token revoked or expired beyond grace
		{"invalid_request", true},      // malformed / missing param
		{"invalid_client", true},       // wrong client credentials
		{"unauthorized_client", true},  // client not allowed this grant
		{"unsupported_grant_type", true},

		{"", false},
		{"server_error", false},        // 5xx-ish, retry later
		{"temporarily_unavailable", false},
		{"rate_limited", false},
	}
	for _, tc := range cases {
		if got := isPermanentTokenError(tc.code); got != tc.want {
			t.Errorf("isPermanentTokenError(%q) = %v, want %v", tc.code, got, tc.want)
		}
	}
}
