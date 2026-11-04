package utils

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sync"
	"syscall"
	"time"
)

var ErrNonPublicAddress = errors.New("destination address is not public")

var nonPublicPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),      // "this" network
	netip.MustParsePrefix("100.64.0.0/10"),  // carrier-grade NAT
	netip.MustParsePrefix("192.0.0.0/24"),   // IETF protocol assignments
	netip.MustParsePrefix("198.18.0.0/15"),  // benchmarking
	netip.MustParsePrefix("240.0.0.0/4"),    // reserved
	netip.MustParsePrefix("64:ff9b::/96"),   // NAT64, can reach internal IPv4
	netip.MustParsePrefix("64:ff9b:1::/48"), // local-use NAT64
}

func IsPublicAddr(addr netip.Addr) bool {
	addr = addr.Unmap()
	if !addr.IsGlobalUnicast() || addr.IsPrivate() {
		return false
	}
	for _, p := range nonPublicPrefixes {
		if p.Contains(addr) {
			return false
		}
	}
	return true
}

var (
	dialAllowed = func(ap netip.AddrPort) bool { return IsPublicAddr(ap.Addr()) }
	envProxy    = http.ProxyFromEnvironment
)

func NewPublicOnlyTransport() *http.Transport {
	d := &publicOnlyDialer{
		guarded: &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second, Control: publicOnlyControl},
		plain:   &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second},
	}
	return &http.Transport{
		Proxy:                 d.proxy,
		DialContext:           d.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}

type publicOnlyDialer struct {
	guarded *net.Dialer
	plain   *net.Dialer
	proxies sync.Map // "host:port" of proxies returned by d.proxy
}

func (d *publicOnlyDialer) proxy(req *http.Request) (*url.URL, error) {
	p, err := envProxy(req)
	if err != nil || p == nil {
		return p, err
	}
	if err := checkHostIsPublic(req.Context(), req.URL.Hostname()); err != nil {
		return nil, err
	}
	d.proxies.Store(proxyAddr(p), struct{}{})
	return p, nil
}

func (d *publicOnlyDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	if _, ok := d.proxies.Load(addr); ok {
		return d.plain.DialContext(ctx, network, addr)
	}
	return d.guarded.DialContext(ctx, network, addr)
}

func publicOnlyControl(network, address string, _ syscall.RawConn) error {
	ap, err := netip.ParseAddrPort(address)
	if err != nil {
		return err
	}
	if !dialAllowed(ap) {
		return fmt.Errorf("%w: %s", ErrNonPublicAddress, ap.Addr())
	}
	return nil
}

func checkHostIsPublic(ctx context.Context, host string) error {
	if addr, err := netip.ParseAddr(host); err == nil {
		if !IsPublicAddr(addr) {
			return fmt.Errorf("%w: %s", ErrNonPublicAddress, addr)
		}
		return nil
	}
	addrs, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil
	}
	for _, addr := range addrs {
		if !IsPublicAddr(addr) {
			return fmt.Errorf("%w: %s resolves to %s", ErrNonPublicAddress, host, addr)
		}
	}
	return nil
}

func proxyAddr(p *url.URL) string {
	port := p.Port()
	if port == "" {
		port = map[string]string{"http": "80", "https": "443", "socks5": "1080", "socks5h": "1080"}[p.Scheme]
	}
	return net.JoinHostPort(p.Hostname(), port)
}

func CheckPublicURL(ctx context.Context, rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("unsupported URL scheme %q", u.Scheme)
	}
	if u.Hostname() == "" {
		return fmt.Errorf("URL has no host")
	}
	return checkHostIsPublic(ctx, u.Hostname())
}
