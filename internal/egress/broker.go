package egress

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Resolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

type DialContextFunc func(ctx context.Context, network, address string) (net.Conn, error)

type BrokerOptions struct {
	AllowedDomains []string
	Network        string
	Address        string
	SocketPath     string
	Resolver       Resolver
	DialContext    DialContextFunc
	AllowPrivateIP bool
}

type Broker struct {
	allowlist      DomainAllowlist
	resolver       Resolver
	dialContext    DialContextFunc
	allowPrivateIP bool
	listener       net.Listener
	server         *http.Server
	proxyURL       string
	closeOnce      sync.Once
	done           chan struct{}
}

func StartBroker(opts BrokerOptions) (*Broker, error) {
	allowlist, err := NewDomainAllowlist(opts.AllowedDomains)
	if err != nil {
		return nil, err
	}
	network := strings.TrimSpace(opts.Network)
	if network == "" {
		network = "tcp"
	}
	address := strings.TrimSpace(opts.Address)
	if address == "" && network == "tcp" {
		address = "127.0.0.1:0"
	}
	if network == "unix" {
		if opts.SocketPath == "" {
			return nil, errors.New("unix broker requires socket path")
		}
		_ = os.Remove(opts.SocketPath)
		address = opts.SocketPath
	}
	ln, err := net.Listen(network, address)
	if err != nil {
		return nil, fmt.Errorf("start egress broker: %w", err)
	}
	resolver := opts.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	dialContext := opts.DialContext
	if dialContext == nil {
		dialContext = (&net.Dialer{Timeout: 10 * time.Second}).DialContext
	}
	b := &Broker{
		allowlist:      allowlist,
		resolver:       resolver,
		dialContext:    dialContext,
		allowPrivateIP: opts.AllowPrivateIP,
		listener:       ln,
		done:           make(chan struct{}),
	}
	b.server = &http.Server{Handler: b}
	if network == "tcp" {
		b.proxyURL = "http://" + ln.Addr().String()
	}
	go func() {
		_ = b.server.Serve(ln)
		close(b.done)
	}()
	return b, nil
}

func (b *Broker) ProxyURL() string {
	if b == nil {
		return ""
	}
	return b.proxyURL
}

func (b *Broker) Close() error {
	if b == nil {
		return nil
	}
	var err error
	b.closeOnce.Do(func() {
		err = b.server.Close()
		<-b.done
	})
	return err
}

func (b *Broker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		b.handleConnect(w, r)
		return
	}
	b.handleHTTP(w, r)
}

func (b *Broker) handleHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL == nil || !r.URL.IsAbs() {
		http.Error(w, "proxy request URL must be absolute", http.StatusBadRequest)
		return
	}
	if r.URL.Scheme != "http" {
		http.Error(w, "only HTTP proxy requests and HTTPS CONNECT are supported", http.StatusForbidden)
		return
	}
	host, port, err := splitHostPortDefault(r.URL.Host, "80")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if port != "80" {
		http.Error(w, "HTTP proxy requests are limited to port 80", http.StatusForbidden)
		return
	}
	if r.Host != "" {
		headerHost, _, err := splitHostPortDefault(r.Host, "80")
		if err != nil || headerHost != host {
			http.Error(w, "Host header must match proxy request host", http.StatusForbidden)
			return
		}
	}
	if err := b.validateHost(host); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	req := r.Clone(r.Context())
	req.RequestURI = ""
	removeHopHeaders(req.Header)
	transport := &http.Transport{
		Proxy:       nil,
		DialContext: b.dialAllowed,
	}
	defer transport.CloseIdleConnections()
	resp, err := transport.RoundTrip(req)
	if err != nil {
		http.Error(w, "egress request failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	removeHopHeaders(resp.Header)
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func (b *Broker) handleConnect(w http.ResponseWriter, r *http.Request) {
	host, port, err := splitHostPortDefault(r.Host, "443")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if port != "443" {
		http.Error(w, "HTTPS CONNECT is limited to port 443", http.StatusForbidden)
		return
	}
	if err := b.validateHost(host); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "proxy does not support connection hijacking", http.StatusInternalServerError)
		return
	}
	client, buf, err := hijacker.Hijack()
	if err != nil {
		return
	}
	defer client.Close()
	if _, err := client.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
		return
	}
	prefix, sni, err := readTLSClientHello(buf, client)
	if err != nil {
		return
	}
	if sni != "" {
		normalizedSNI, err := NormalizeRequestHost(sni)
		if err != nil || normalizedSNI != host {
			return
		}
	}
	upstream, err := b.dialAllowed(r.Context(), "tcp", net.JoinHostPort(host, port))
	if err != nil {
		return
	}
	defer upstream.Close()
	if len(prefix) > 0 {
		if _, err := upstream.Write(prefix); err != nil {
			return
		}
	}
	errCh := make(chan error, 2)
	go proxyCopy(errCh, upstream, client)
	go proxyCopy(errCh, client, upstream)
	<-errCh
}

func (b *Broker) dialAllowed(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := splitHostPortDefault(address, "")
	if err != nil {
		return nil, err
	}
	if port == "" {
		return nil, fmt.Errorf("destination port is required")
	}
	if err := b.validateHost(host); err != nil {
		return nil, err
	}
	ips, err := b.resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", host, err)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("resolve %s: no addresses", host)
	}
	var firstErr error
	for _, ip := range ips {
		if !b.allowPrivateIP && !IsPublicIP(ip.IP) {
			if firstErr == nil {
				firstErr = fmt.Errorf("resolved address for %s is not public", host)
			}
			continue
		}
		conn, err := b.dialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
		if err == nil {
			return conn, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return nil, fmt.Errorf("no usable address for %s", host)
}

func (b *Broker) validateHost(host string) error {
	host, err := NormalizeRequestHost(host)
	if err != nil {
		return err
	}
	if !b.allowlist.Allows(host) {
		return fmt.Errorf("host %s is not allowed by allowedDomains", host)
	}
	return nil
}

func splitHostPortDefault(value, defaultPort string) (string, string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", "", fmt.Errorf("host is required")
	}
	host, port, err := net.SplitHostPort(value)
	if err == nil {
		normalizedHost, err := NormalizeRequestHost(host)
		if err != nil {
			return "", "", err
		}
		if err := validatePort(port); err != nil {
			return "", "", err
		}
		return normalizedHost, port, nil
	}
	if strings.Contains(value, ":") && strings.Count(value, ":") > 1 {
		return "", "", fmt.Errorf("IPv6 literal hosts are not allowed")
	}
	host = value
	port = defaultPort
	if strings.Contains(value, ":") {
		pieces := strings.Split(value, ":")
		if len(pieces) != 2 {
			return "", "", fmt.Errorf("invalid host:port")
		}
		host, port = pieces[0], pieces[1]
	}
	normalizedHost, err := NormalizeRequestHost(host)
	if err != nil {
		return "", "", err
	}
	if port != "" {
		if err := validatePort(port); err != nil {
			return "", "", err
		}
	}
	return normalizedHost, port, nil
}

func validatePort(port string) error {
	portNum, err := strconv.Atoi(port)
	if err != nil || portNum <= 0 || portNum > 65535 {
		return fmt.Errorf("invalid destination port")
	}
	return nil
}

func readTLSClientHello(buf *bufio.ReadWriter, client net.Conn) ([]byte, string, error) {
	if err := client.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return nil, "", err
	}
	defer func() { _ = client.SetReadDeadline(time.Time{}) }()
	header := make([]byte, 5)
	if _, err := io.ReadFull(buf, header); err != nil {
		return nil, "", err
	}
	if header[0] != 22 {
		return nil, "", fmt.Errorf("CONNECT only supports TLS")
	}
	length := int(header[3])<<8 | int(header[4])
	if length <= 0 || length > 1<<15 {
		return nil, "", fmt.Errorf("invalid TLS record")
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(buf, body); err != nil {
		return nil, "", err
	}
	prefix := append(header, body...)
	sni, err := parseClientHelloSNI(body)
	if err != nil {
		return nil, "", err
	}
	return prefix, sni, nil
}

func parseClientHelloSNI(body []byte) (string, error) {
	if len(body) < 4 || body[0] != 1 {
		return "", fmt.Errorf("expected TLS ClientHello")
	}
	helloLen := int(body[1])<<16 | int(body[2])<<8 | int(body[3])
	if helloLen+4 > len(body) {
		return "", fmt.Errorf("truncated TLS ClientHello")
	}
	p := body[4 : 4+helloLen]
	if len(p) < 34 {
		return "", fmt.Errorf("invalid TLS ClientHello")
	}
	p = p[34:]
	if len(p) < 1 {
		return "", fmt.Errorf("invalid TLS ClientHello")
	}
	sessionLen := int(p[0])
	p = p[1:]
	if len(p) < sessionLen+2 {
		return "", fmt.Errorf("invalid TLS ClientHello")
	}
	p = p[sessionLen:]
	cipherLen := int(p[0])<<8 | int(p[1])
	p = p[2:]
	if len(p) < cipherLen+1 {
		return "", fmt.Errorf("invalid TLS ClientHello")
	}
	p = p[cipherLen:]
	compressionLen := int(p[0])
	p = p[1:]
	if len(p) < compressionLen {
		return "", fmt.Errorf("invalid TLS ClientHello")
	}
	p = p[compressionLen:]
	if len(p) < 2 {
		return "", nil
	}
	extensionsLen := int(p[0])<<8 | int(p[1])
	p = p[2:]
	if len(p) < extensionsLen {
		return "", fmt.Errorf("invalid TLS ClientHello extensions")
	}
	p = p[:extensionsLen]
	for len(p) >= 4 {
		extType := int(p[0])<<8 | int(p[1])
		extLen := int(p[2])<<8 | int(p[3])
		p = p[4:]
		if len(p) < extLen {
			return "", fmt.Errorf("invalid TLS ClientHello extension")
		}
		ext := p[:extLen]
		p = p[extLen:]
		if extType != 0 {
			continue
		}
		if len(ext) < 2 {
			return "", fmt.Errorf("invalid SNI extension")
		}
		listLen := int(ext[0])<<8 | int(ext[1])
		ext = ext[2:]
		if len(ext) < listLen {
			return "", fmt.Errorf("invalid SNI extension")
		}
		ext = ext[:listLen]
		for len(ext) >= 3 {
			nameType := ext[0]
			nameLen := int(ext[1])<<8 | int(ext[2])
			ext = ext[3:]
			if len(ext) < nameLen {
				return "", fmt.Errorf("invalid SNI name")
			}
			name := string(ext[:nameLen])
			ext = ext[nameLen:]
			if nameType == 0 {
				return name, nil
			}
		}
	}
	return "", nil
}

func removeHopHeaders(h http.Header) {
	for _, key := range []string{"Connection", "Proxy-Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "TE", "Trailer", "Transfer-Encoding", "Upgrade"} {
		h.Del(key)
	}
}

func proxyCopy(ch chan<- error, dst io.Writer, src io.Reader) {
	_, err := io.Copy(dst, src)
	ch <- err
}
