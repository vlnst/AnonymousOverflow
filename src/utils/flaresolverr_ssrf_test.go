package utils

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func TestCheckPublicURL(t *testing.T) {
	ctx := context.Background()
	for _, raw := range []string{
		"http://127.0.0.1/x",
		"http://169.254.169.254/latest/meta-data/",
		"http://10.0.0.5/x",
		"http://[::ffff:7f00:1]:8080/x",
		"http://[::1]/x",
		"javascript:alert(1)",
		"http:///no-host",
	} {
		if err := CheckPublicURL(ctx, raw); err == nil {
			t.Errorf("CheckPublicURL(%q) = nil, want error", raw)
		}
	}
	for _, raw := range []string{"http://8.8.8.8/x", "https://151.101.1.69/image.png"} {
		if err := CheckPublicURL(ctx, raw); err != nil {
			t.Errorf("CheckPublicURL(%q) = %v, want nil", raw, err)
		}
	}
}

// An internal URL in a signed token must be refused before any connection is
// made and before FlareSolverr is asked to fetch it.
func TestGetWithClearanceRefusesInternalTarget(t *testing.T) {
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	var connections int32
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			atomic.AddInt32(&connections, 1)
			conn.Close()
		}
	}()
	port := l.Addr().(*net.TCPAddr).Port

	var solverCalls int32
	solver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&solverCalls, 1)
	}))
	defer solver.Close()
	t.Setenv("FLARESOLVERR_URL", solver.URL)

	_, err = GetWithClearance(context.Background(), "http://127.0.0.1:"+strconv.Itoa(port)+"/latest/meta-data/")
	if !errors.Is(err, ErrNonPublicAddress) {
		t.Fatalf("err = %v, want ErrNonPublicAddress", err)
	}
	time.Sleep(50 * time.Millisecond)
	if n := atomic.LoadInt32(&connections); n != 0 {
		t.Fatalf("connected to the internal listener %d time(s)", n)
	}
	if n := atomic.LoadInt32(&solverCalls); n != 0 {
		t.Fatalf("FlareSolverr was asked to fetch an internal URL %d time(s)", n)
	}
}

// A public-looking URL that redirects inward must be stopped at connect time.
func TestGetWithClearanceBlocksRedirectToInternal(t *testing.T) {
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	var connections int32
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			atomic.AddInt32(&connections, 1)
			conn.Close()
		}
	}()
	internalPort := l.Addr().(*net.TCPAddr).Port

	public := httptest.NewServer(http.RedirectHandler(
		"http://127.0.0.1:"+strconv.Itoa(internalPort)+"/latest/meta-data/", http.StatusFound))
	defer public.Close()
	publicAddr := netip.MustParseAddrPort(public.Listener.Addr().String())

	origDial, origGuard := dialAllowed, upstreamGuard
	dialAllowed = func(ap netip.AddrPort) bool { return ap == publicAddr }
	upstreamGuard = func(context.Context, string) error { return nil }
	defer func() { dialAllowed, upstreamGuard = origDial, origGuard }()

	// The pre-check is lifted here; only the "public" test server is
	// dialable, so the connect-time guard must stop the redirect.
	t.Setenv("FLARESOLVERR_URL", "")
	target := "http://" + publicAddr.Addr().String() + ":" + strconv.Itoa(int(publicAddr.Port())) + "/img.png"
	_, err = GetWithClearance(context.Background(), target)
	if !errors.Is(err, ErrNonPublicAddress) {
		t.Fatalf("err = %v, want ErrNonPublicAddress", err)
	}
	time.Sleep(50 * time.Millisecond)
	if n := atomic.LoadInt32(&connections); n != 0 {
		t.Fatalf("redirect reached the internal listener %d time(s)", n)
	}
}
