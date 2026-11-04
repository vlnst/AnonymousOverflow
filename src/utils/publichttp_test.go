package utils

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"testing"
)

func TestIsPublicAddr(t *testing.T) {
	blocked := []string{
		"127.0.0.1", "10.1.2.3", "172.16.0.1", "192.168.1.1", "169.254.169.254",
		"100.64.0.1", "0.0.0.0", "255.255.255.255", "224.0.0.1",
		"::1", "::", "fe80::1", "fc00::1", "fd00:ec2::254",
		"::ffff:127.0.0.1", "::ffff:169.254.169.254", "64:ff9b::a9fe:a9fe",
	}
	for _, s := range blocked {
		if IsPublicAddr(netip.MustParseAddr(s)) {
			t.Errorf("%s should not be public", s)
		}
	}
	for _, s := range []string{"151.101.1.69", "8.8.8.8", "2606:4700::1111"} {
		if !IsPublicAddr(netip.MustParseAddr(s)) {
			t.Errorf("%s should be public", s)
		}
	}
}

func publicOnlyClient() *http.Client {
	return &http.Client{Transport: NewPublicOnlyTransport()}
}

func TestPublicOnlyTransportBlocksLoopback(t *testing.T) {
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "internal secret")
	}))
	defer internal.Close()

	u, _ := url.Parse(internal.URL)
	for _, target := range []string{internal.URL, "http://localhost:" + u.Port()} {
		_, err := publicOnlyClient().Get(target)
		if !errors.Is(err, ErrNonPublicAddress) {
			t.Errorf("GET %s: expected ErrNonPublicAddress, got %v", target, err)
		}
	}
}

func TestPublicOnlyTransportBlocksRedirectToInternal(t *testing.T) {
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "internal secret")
	}))
	defer internal.Close()
	// Stands in for an attacker's public host that redirects inward.
	public := httptest.NewServer(http.RedirectHandler(internal.URL+"/latest/meta-data/", http.StatusFound))
	defer public.Close()

	publicAddr := netip.MustParseAddrPort(public.Listener.Addr().String())
	orig := dialAllowed
	dialAllowed = func(ap netip.AddrPort) bool { return ap == publicAddr }
	defer func() { dialAllowed = orig }()

	_, err := publicOnlyClient().Get(public.URL)
	if !errors.Is(err, ErrNonPublicAddress) {
		t.Fatalf("expected redirect to internal host to be blocked, got %v", err)
	}
}

func TestPublicOnlyTransportProxy(t *testing.T) {
	var proxied []string
	// The operator's proxy on a private address must stay usable.
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxied = append(proxied, r.URL.String())
		io.WriteString(w, "via proxy")
	}))
	defer proxy.Close()
	proxyURL, _ := url.Parse(proxy.URL)

	orig := envProxy
	envProxy = func(*http.Request) (*url.URL, error) { return proxyURL, nil }
	defer func() { envProxy = orig }()

	resp, err := publicOnlyClient().Get("http://151.101.1.69/image.png")
	if err != nil {
		t.Fatalf("public target through private proxy failed: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "via proxy" {
		t.Fatalf("unexpected body %q", body)
	}

	_, err = publicOnlyClient().Get("http://169.254.169.254/latest/meta-data/")
	if !errors.Is(err, ErrNonPublicAddress) {
		t.Fatalf("expected internal target to be refused before reaching the proxy, got %v", err)
	}
	if len(proxied) != 1 {
		t.Fatalf("proxy saw %d requests (%v), want 1", len(proxied), proxied)
	}
}
