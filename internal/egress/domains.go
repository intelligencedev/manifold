package egress

import (
	"fmt"
	"net"
	"net/netip"
	"strings"

	"golang.org/x/net/idna"
)

type DomainAllowlist struct {
	domains []string
}

func NewDomainAllowlist(entries []string) (DomainAllowlist, error) {
	out := DomainAllowlist{domains: make([]string, 0, len(entries))}
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		domain, err := NormalizeDomainEntry(entry)
		if err != nil {
			return DomainAllowlist{}, err
		}
		if _, ok := seen[domain]; ok {
			continue
		}
		seen[domain] = struct{}{}
		out.domains = append(out.domains, domain)
	}
	if len(out.domains) == 0 {
		return DomainAllowlist{}, fmt.Errorf("allowedDomains must contain at least one domain")
	}
	return out, nil
}

func NormalizeDomainEntry(entry string) (string, error) {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return "", fmt.Errorf("allowed domain is empty")
	}
	if strings.Contains(entry, "://") || strings.ContainsAny(entry, "/?#@") || strings.Contains(entry, "*") {
		return "", fmt.Errorf("allowed domain %q must be a hostname, not a URL or wildcard", entry)
	}
	if strings.Contains(entry, ":") {
		return "", fmt.Errorf("allowed domain %q must not include a port", entry)
	}
	entry = strings.TrimSuffix(strings.ToLower(entry), ".")
	if entry == "" || strings.Contains(entry, "..") || strings.HasPrefix(entry, ".") {
		return "", fmt.Errorf("allowed domain %q is invalid", entry)
	}
	if ip := net.ParseIP(entry); ip != nil {
		return "", fmt.Errorf("allowed domain %q must not be an IP literal", entry)
	}
	ascii, err := idna.Lookup.ToASCII(entry)
	if err != nil {
		return "", fmt.Errorf("allowed domain %q is invalid: %w", entry, err)
	}
	ascii = strings.TrimSuffix(strings.ToLower(ascii), ".")
	if ascii == "" || len(ascii) > 253 {
		return "", fmt.Errorf("allowed domain %q is invalid", entry)
	}
	labels := strings.Split(ascii, ".")
	if len(labels) < 2 {
		return "", fmt.Errorf("allowed domain %q must include at least two labels", entry)
	}
	for _, label := range labels {
		if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return "", fmt.Errorf("allowed domain %q is invalid", entry)
		}
	}
	return ascii, nil
}

func NormalizeRequestHost(host string) (string, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return "", fmt.Errorf("host is required")
	}
	if strings.Contains(host, "://") {
		return "", fmt.Errorf("host must not include a scheme")
	}
	if strings.HasPrefix(host, "[") {
		withoutPort, _, err := net.SplitHostPort(host)
		if err == nil {
			host = withoutPort
		}
	}
	host = strings.Trim(host, "[]")
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if ip := net.ParseIP(host); ip != nil {
		return "", fmt.Errorf("IP literal hosts are not allowed")
	}
	ascii, err := idna.Lookup.ToASCII(host)
	if err != nil {
		return "", fmt.Errorf("host %q is invalid: %w", host, err)
	}
	ascii = strings.TrimSuffix(strings.ToLower(ascii), ".")
	if ascii == "" || strings.Contains(ascii, "..") {
		return "", fmt.Errorf("host %q is invalid", host)
	}
	return ascii, nil
}

func (a DomainAllowlist) Allows(host string) bool {
	host, err := NormalizeRequestHost(host)
	if err != nil {
		return false
	}
	for _, domain := range a.domains {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

func IsPublicIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	addr = addr.Unmap()
	return addr.IsGlobalUnicast() && !addr.IsPrivate() && !addr.IsLoopback() && !addr.IsLinkLocalUnicast()
}
