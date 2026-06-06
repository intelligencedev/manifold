package egress

import "testing"

func TestDomainAllowlistMatchesDomainAndSubdomains(t *testing.T) {
	t.Parallel()
	allowlist, err := NewDomainAllowlist([]string{"Example.COM"})
	if err != nil {
		t.Fatalf("NewDomainAllowlist error: %v", err)
	}
	for _, host := range []string{"example.com", "api.example.com", "deep.api.example.com"} {
		if !allowlist.Allows(host) {
			t.Fatalf("expected %s to be allowed", host)
		}
	}
	for _, host := range []string{"badexample.com", "example.com.evil.test", "127.0.0.1"} {
		if allowlist.Allows(host) {
			t.Fatalf("expected %s to be denied", host)
		}
	}
}

func TestNormalizeDomainEntryRejectsUnsafeForms(t *testing.T) {
	t.Parallel()
	for _, entry := range []string{
		"https://example.com",
		"example.com/path",
		"*.example.com",
		"example.com:443",
		"127.0.0.1",
		"localhost",
		".example.com",
	} {
		if _, err := NormalizeDomainEntry(entry); err == nil {
			t.Fatalf("expected %q to be rejected", entry)
		}
	}
}
