package safefetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const PreviewLimit int64 = 10 << 20
const PDFLimit int64 = 250 << 20

// Result owns the response stream. Close it after reading, including on errors.
type Result struct {
	Body             io.ReadCloser
	RequestedURL     string
	FinalURL         string
	ContentType      string
	ContentLength    int64
	Redirected       bool
	TransportChanged bool
	Downgraded       bool
	Unencrypted      bool
}

type Fetcher struct {
	Resolver *net.Resolver
	Dialer   *net.Dialer
	// Resolve and Dial are injectable only for deterministic connection-policy fixtures.
	Resolve func(context.Context, string) ([]net.IPAddr, error)
	Dial    func(context.Context, string, string) (net.Conn, error)
}

func validate(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.Opaque != "" || u.Fragment != "" {
		return nil, errors.New("enter a public HTTP or HTTPS URL without credentials or a fragment")
	}
	port := u.Port()
	if (u.Scheme == "http" && port != "" && port != "80") || (u.Scheme == "https" && port != "" && port != "443") {
		return nil, errors.New("destination port is not allowed")
	}
	if strings.HasSuffix(strings.ToLower(u.Hostname()), ".local") || strings.EqualFold(u.Hostname(), "localhost") {
		return nil, errors.New("destination is not public")
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil && !allowedIP(ip) {
		return nil, errors.New("destination is not public")
	}
	return u, nil
}

func allowedIP(ip net.IP) bool {
	a, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	a = a.Unmap()
	if !a.IsGlobalUnicast() || a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast() {
		return false
	}
	for _, s := range []string{"0.0.0.0/8", "100.64.0.0/10", "169.254.0.0/16", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4", "2001:db8::/32", "2001::/32", "fc00::/7", "fe80::/10"} {
		if netip.MustParsePrefix(s).Contains(a) {
			return false
		}
	}
	return true
}

func (f *Fetcher) Fetch(ctx context.Context, raw string, limit int64, allowDowngrade bool) (*Result, error) {
	return f.FetchScoped(ctx, raw, limit, allowDowngrade, nil)
}

// FetchScoped enforces an additional caller-defined URL policy before the
// initial request and before every redirect. Collection path limits must be
// checked here, before a redirect can contact an out-of-scope destination.
func (f *Fetcher) FetchScoped(ctx context.Context, raw string, limit int64, allowDowngrade bool, allowed func(*url.URL) error) (*Result, error) {
	initial, err := validate(raw)
	if err != nil {
		return nil, err
	}
	if allowed != nil {
		if err := allowed(initial); err != nil {
			return nil, err
		}
	}
	if limit < 1 || limit > PDFLimit {
		return nil, errors.New("invalid fetch limit")
	}
	resolver := net.DefaultResolver
	if f != nil && f.Resolver != nil {
		resolver = f.Resolver
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	if f != nil && f.Dialer != nil {
		dialer = f.Dialer
	}
	transport := &http.Transport{Proxy: nil, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 20 * time.Second, IdleConnTimeout: 5 * time.Second, DisableKeepAlives: true}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil || (port != "80" && port != "443") {
			return nil, errors.New("destination port is not allowed")
		}
		var ips []net.IPAddr
		if f != nil && f.Resolve != nil {
			ips, err = f.Resolve(ctx, host)
		} else {
			ips, err = resolver.LookupIPAddr(ctx, host)
		}
		if err != nil {
			return nil, err
		}
		if len(ips) == 0 {
			return nil, errors.New("destination has no address")
		}
		for _, ip := range ips {
			if !allowedIP(ip.IP) {
				return nil, errors.New("destination resolves to a non-public address")
			}
		}
		var last error
		for _, ip := range ips {
			target := net.JoinHostPort(ip.IP.String(), port)
			if f != nil && f.Dial != nil {
				var c net.Conn
				c, last = f.Dial(ctx, network, target)
				if last == nil {
					return c, nil
				}
			} else {
				var c net.Conn
				c, last = dialer.DialContext(ctx, network, target)
				if last == nil {
					return c, nil
				}
			}
		}
		return nil, last
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, 210*time.Second)
	client := &http.Client{Transport: transport, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		req.Header.Del("Authorization")
		req.Header.Del("Cookie")
		req.Header.Del("Referer")
		if err := checkRedirect(req, via, allowDowngrade); err != nil {
			return err
		}
		if allowed != nil {
			return allowed(req.URL)
		}
		return nil
	}}
	req, err := http.NewRequestWithContext(timeoutCtx, http.MethodGet, initial.String(), nil)
	if err != nil {
		cancel()
		return nil, err
	}
	req.Header.Set("User-Agent", "HomeopathPOC/0.1")
	req.Header.Set("Accept", "application/pdf,text/html,text/plain,application/xml,text/xml;q=0.9,*/*;q=0.1")
	res, err := client.Do(req)
	if err != nil {
		cancel()
		transport.CloseIdleConnections()
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		cancel()
		return nil, fmt.Errorf("source returned HTTP %d", res.StatusCode)
	}
	if res.ContentLength > limit {
		res.Body.Close()
		cancel()
		return nil, fmt.Errorf("source exceeds %s MiB limit", strconv.FormatInt(limit/(1<<20), 10))
	}
	body := &boundedBody{source: res.Body, remaining: limit, closeFunc: func() { cancel(); transport.CloseIdleConnections() }}
	return &Result{Body: body, RequestedURL: initial.String(), FinalURL: res.Request.URL.String(), ContentType: res.Header.Get("Content-Type"), ContentLength: res.ContentLength, Redirected: res.Request.URL.String() != initial.String(), TransportChanged: initial.Scheme != res.Request.URL.Scheme, Downgraded: initial.Scheme == "https" && res.Request.URL.Scheme == "http", Unencrypted: res.Request.URL.Scheme == "http"}, nil
}

func checkRedirect(req *http.Request, via []*http.Request, allowDowngrade bool) error {
	if len(via) >= 5 {
		return errors.New("too many redirects")
	}
	u, err := validate(req.URL.String())
	if err != nil {
		return err
	}
	if len(via) > 0 && via[len(via)-1].URL.Scheme == "https" && u.Scheme == "http" && !allowDowngrade {
		return errors.New("HTTPS to HTTP redirect requires explicit allowance")
	}
	return nil
}

type boundedBody struct {
	source    io.ReadCloser
	remaining int64
	closeFunc func()
}

func (b *boundedBody) Read(p []byte) (int, error) {
	if b.remaining < 0 {
		return 0, errors.New("source exceeds byte limit")
	}
	if int64(len(p)) > b.remaining+1 {
		p = p[:b.remaining+1]
	}
	n, err := b.source.Read(p)
	b.remaining -= int64(n)
	if b.remaining < 0 {
		return n, errors.New("source exceeds byte limit")
	}
	return n, err
}
func (b *boundedBody) Close() error { b.closeFunc(); return b.source.Close() }
