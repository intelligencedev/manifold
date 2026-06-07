package egress

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type staticResolver map[string][]net.IPAddr

func (r staticResolver) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	addrs := r[host]
	if len(addrs) == 0 {
		return nil, fmt.Errorf("unknown host %s", host)
	}
	return addrs, nil
}

func TestBrokerAllowsHTTPForAllowedDomain(t *testing.T) {
	t.Parallel()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "api.example.com" {
			t.Fatalf("unexpected host header %q", r.Host)
		}
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(upstream.Close)
	upstreamURL, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	broker, err := StartBroker(BrokerOptions{
		AllowedDomains: []string{"example.com"},
		Resolver:       staticResolver{"api.example.com": []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}},
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, upstreamURL.Host)
		},
	})
	if err != nil {
		t.Fatalf("StartBroker error: %v", err)
	}
	t.Cleanup(func() { _ = broker.Close() })

	proxyURL, err := url.Parse(broker.ProxyURL())
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	resp, err := client.Get("http://api.example.com/")
	if err != nil {
		t.Fatalf("proxy GET error: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok" {
		t.Fatalf("body = %q, want ok", body)
	}
}

func TestBrokerDeniesDisallowedDomain(t *testing.T) {
	t.Parallel()
	broker, err := StartBroker(BrokerOptions{
		AllowedDomains: []string{"example.com"},
		Resolver:       staticResolver{"evil.test": []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}},
	})
	if err != nil {
		t.Fatalf("StartBroker error: %v", err)
	}
	t.Cleanup(func() { _ = broker.Close() })
	proxyURL, err := url.Parse(broker.ProxyURL())
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	resp, err := client.Get("http://evil.test/")
	if err != nil {
		t.Fatalf("proxy GET error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
}

func TestBrokerDeniesHTTPHostHeaderMismatch(t *testing.T) {
	t.Parallel()
	allowlist, err := NewDomainAllowlist([]string{"example.com"})
	if err != nil {
		t.Fatal(err)
	}
	broker := &Broker{allowlist: allowlist}
	req := httptest.NewRequest(http.MethodGet, "http://api.example.com/", nil)
	req.Host = "evil.test"
	rec := httptest.NewRecorder()
	broker.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestBrokerDeniesPrivateResolvedAddress(t *testing.T) {
	t.Parallel()
	broker, err := StartBroker(BrokerOptions{
		AllowedDomains: []string{"example.com"},
		Resolver:       staticResolver{"api.example.com": []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}},
	})
	if err != nil {
		t.Fatalf("StartBroker error: %v", err)
	}
	t.Cleanup(func() { _ = broker.Close() })
	proxyURL, err := url.Parse(broker.ProxyURL())
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	resp, err := client.Get("http://api.example.com/")
	if err != nil {
		t.Fatalf("proxy GET error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadGateway)
	}
}

func TestBrokerAllowsConnectWithMatchingSNI(t *testing.T) {
	t.Parallel()
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("secure"))
	}))
	t.Cleanup(upstream.Close)
	upstreamURL, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	broker, err := StartBroker(BrokerOptions{
		AllowedDomains: []string{"example.com"},
		Resolver:       staticResolver{"api.example.com": []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}},
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, upstreamURL.Host)
		},
	})
	if err != nil {
		t.Fatalf("StartBroker error: %v", err)
	}
	t.Cleanup(func() { _ = broker.Close() })
	proxyURL, err := url.Parse(broker.ProxyURL())
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
			ServerName:         "api.example.com",
		},
	}}
	resp, err := client.Get("https://api.example.com/")
	if err != nil {
		t.Fatalf("proxy HTTPS GET error: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "secure") {
		t.Fatalf("body = %q, want secure", body)
	}
}

func TestBrokerDeniesConnectWithMismatchedSNI(t *testing.T) {
	t.Parallel()
	dialed := false
	broker, err := StartBroker(BrokerOptions{
		AllowedDomains: []string{"example.com"},
		Resolver:       staticResolver{"api.example.com": []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}},
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			dialed = true
			return nil, fmt.Errorf("should not dial upstream")
		},
	})
	if err != nil {
		t.Fatalf("StartBroker error: %v", err)
	}
	t.Cleanup(func() { _ = broker.Close() })
	proxyURL, err := url.Parse(broker.ProxyURL())
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
			ServerName:         "other.example.com",
		},
	}}
	resp, err := client.Get("https://api.example.com/")
	if err == nil {
		resp.Body.Close()
		t.Fatal("expected proxy HTTPS GET to fail")
	}
	if dialed {
		t.Fatal("mismatched SNI should be denied before upstream dial")
	}
}
