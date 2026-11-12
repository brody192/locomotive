package railway

import "testing"

func TestParseTokenKeepsTheTokenRailwayIssued(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"legacy uuid token", "0b3f6a52-7c1e-4d7a-9a51-2f4c8e9d1a6b", "0b3f6a52-7c1e-4d7a-9a51-2f4c8e9d1a6b"},
		{"uppercase uuid token is lowercased as before", "0B3F6A52-7C1E-4D7A-9A51-2F4C8E9D1A6B", "0b3f6a52-7c1e-4d7a-9a51-2f4c8e9d1a6b"},
		{"account token", "rlwy_acct_3kQ9xV7mN2pL8rT4wY6zB1cD5fG0hJ", "rlwy_acct_3kQ9xV7mN2pL8rT4wY6zB1cD5fG0hJ"},
		{"workspace token", "rlwy_wksp_8nR2tY5uI9oP3aS7dF1gH4jK6lZ0xC", "rlwy_wksp_8nR2tY5uI9oP3aS7dF1gH4jK6lZ0xC"},
		{"surrounding whitespace is trimmed", "  rlwy_acct_abc123\n", "rlwy_acct_abc123"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseToken(tc.raw)
			if err != nil {
				t.Fatalf("ParseToken(%q) returned error: %v", tc.raw, err)
			}

			if got != tc.want {
				t.Errorf("ParseToken(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestParseTokenRejectsABlankToken(t *testing.T) {
	for _, raw := range []string{"", "   ", "\n\t"} {
		if _, err := ParseToken(raw); err == nil || err.Error() != "token must not be blank" {
			t.Errorf("ParseToken(%q) error = %v, want \"token must not be blank\"", raw, err)
		}
	}
}
