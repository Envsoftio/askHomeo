package safefetch

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

type responseConn struct {
	*bytes.Reader
	closed bool
}

func (c *responseConn) Read(p []byte) (int, error)       { return c.Reader.Read(p) }
func (c *responseConn) Write(p []byte) (int, error)      { return len(p), nil }
func (c *responseConn) Close() error                     { c.closed = true; return nil }
func (c *responseConn) LocalAddr() net.Addr              { return &net.TCPAddr{IP: net.IPv4(8, 8, 8, 8), Port: 80} }
func (c *responseConn) RemoteAddr() net.Addr             { return c.LocalAddr() }
func (c *responseConn) SetDeadline(time.Time) error      { return nil }
func (c *responseConn) SetReadDeadline(time.Time) error  { return nil }
func (c *responseConn) SetWriteDeadline(time.Time) error { return nil }
func fixture(responses ...string) *Fetcher {
	index := 0
	return &Fetcher{Resolve: func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
	}, Dial: func(context.Context, string, string) (net.Conn, error) {
		if index >= len(responses) {
			return nil, io.EOF
		}
		v := responses[index]
		index++
		return &responseConn{Reader: bytes.NewReader([]byte(v))}, nil
	}}
}
func TestValidationBlocksUnsafeTargets(t *testing.T) {
	for _, raw := range []string{"http://127.0.0.1/", "http://169.254.169.254/latest/meta-data/", "https://user:pass@example.org/", "http://example.org:8080/", "file:///etc/passwd", "http://localhost/", "http://[::1]/"} {
		if _, err := validate(raw); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
}
func TestFetchHTTPRedirectAndSize(t *testing.T) {
	f := fixture("HTTP/1.1 302 Found\r\nLocation: http://public.example/end\r\nContent-Length: 0\r\n\r\n", "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 17\r\n\r\nhello public page")
	got, err := f.Fetch(context.Background(), "http://public.example/start", PreviewLimit, false)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(got.Body)
	got.Body.Close()
	if err != nil || string(data) != "hello public page" || !got.Redirected || got.FinalURL != "http://public.example/end" || !got.Unencrypted {
		t.Fatalf("result=%+v body=%q err=%v", got, data, err)
	}
	f = fixture("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\n\r\nhello public page")
	got, err = f.Fetch(context.Background(), "http://public.example/end", 4, false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.ReadAll(got.Body)
	got.Body.Close()
	if err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("expected byte limit, got %v", err)
	}
}
func TestRedirectEscapeAndRebindingBlocked(t *testing.T) {
	f := fixture("HTTP/1.1 302 Found\r\nLocation: http://127.0.0.1/private\r\nContent-Length: 0\r\n\r\n")
	if _, err := f.Fetch(context.Background(), "http://public.example/start", PreviewLimit, false); err == nil {
		t.Fatal("followed private redirect")
	}
	f = fixture()
	f.Resolve = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}, {IP: net.ParseIP("127.0.0.1")}}, nil
	}
	connected := false
	f.Dial = func(context.Context, string, string) (net.Conn, error) { connected = true; return nil, nil }
	if _, err := f.Fetch(context.Background(), "http://public.example/", PreviewLimit, false); err == nil || connected {
		t.Fatalf("mixed DNS result connected=%v err=%v", connected, err)
	}
}
func TestTLSFailureDoesNotRetryHTTP(t *testing.T) {
	f := fixture("HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n")
	if _, err := f.Fetch(context.Background(), "https://public.example/", PreviewLimit, false); err == nil {
		t.Fatal("accepted invalid TLS")
	}
}

func TestHTTPSDowngradeRequiresExplicitAllowance(t *testing.T) {
	previous, _ := http.NewRequest(http.MethodGet, "https://public.example/start", nil)
	next, _ := http.NewRequest(http.MethodGet, "http://public.example/end", nil)
	if err := checkRedirect(next, []*http.Request{previous}, false); err == nil {
		t.Fatal("accepted implicit downgrade")
	}
	if err := checkRedirect(next, []*http.Request{previous}, true); err != nil {
		t.Fatal(err)
	}
}

func TestScopedFetchRejectsRedirectBeforeConnecting(t *testing.T) {
	f := fixture("HTTP/1.1 302 Found\r\nLocation: http://public.example/outside/page\r\nContent-Length: 0\r\n\r\n")
	allowed := func(u *url.URL) error {
		if !strings.HasPrefix(u.Path, "/book/") {
			return errors.New("outside collection scope")
		}
		return nil
	}
	if _, err := f.FetchScoped(context.Background(), "http://public.example/book/index", PreviewLimit, false, allowed); err == nil || !strings.Contains(err.Error(), "outside collection scope") {
		t.Fatalf("redirect escape was accepted: %v", err)
	}
}
