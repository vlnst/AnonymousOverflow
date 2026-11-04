package routes

import (
	"anonymousoverflow/src/utils"
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestImageProxyBoundsBodySize(t *testing.T) {
	t.Setenv("JWT_SIGNING_SECRET", "test-secret")
	t.Setenv("APP_URL", "")
	allowLoopbackFetch(t)
	origMax := maxImageBytes
	maxImageBytes = 1 << 20
	defer func() { maxImageBytes = origMax }()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(make([]byte, 5*maxImageBytes))
	}))
	defer upstream.Close()

	w := serveImageProxy(proxyPathFor(t, upstream.URL+"/huge.png"))
	if int64(w.Body.Len()) > maxImageBytes {
		t.Fatalf("image proxy returned %d bytes, limit is %d", w.Body.Len(), maxImageBytes)
	}
}

func TestImageProxyTimesOut(t *testing.T) {
	t.Setenv("JWT_SIGNING_SECRET", "test-secret")
	t.Setenv("APP_URL", "")
	allowLoopbackFetch(t)
	origTimeout := imageFetchTimeout
	imageFetchTimeout = 200 * time.Millisecond
	defer func() { imageFetchTimeout = origTimeout }()

	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(bytes.Repeat([]byte{0}, 512))
		w.(http.Flusher).Flush()
		select { // never finish the body
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer upstream.Close()
	defer close(release)

	done := make(chan struct{})
	go func() {
		serveImageProxy(proxyPathFor(t, upstream.URL+"/slow.png"))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("image proxy request did not time out on a stalled upstream")
	}
}

func TestImageFetchContextCoversSolve(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/proxy", nil)

	origTimeout := imageFetchTimeout
	imageFetchTimeout = 200 * time.Millisecond
	defer func() { imageFetchTimeout = origTimeout }()

	t.Setenv("FLARESOLVERR_URL", "")
	ctx, cancel := imageFetchContext(c)
	deadline, ok := ctx.Deadline()
	cancel()
	if !ok || time.Until(deadline) > imageFetchTimeout+time.Second {
		t.Fatalf("without FlareSolverr: deadline in %v, want ~%v", time.Until(deadline), imageFetchTimeout)
	}

	t.Setenv("FLARESOLVERR_URL", "http://flaresolverr:8191")
	ctx, cancel = imageFetchContext(c)
	deadline, ok = ctx.Deadline()
	cancel()
	want := imageFetchTimeout + utils.SolveTimeout + 30*time.Second
	if d := time.Until(deadline); !ok || d > want+time.Second || d < want-2*time.Second {
		t.Fatalf("with FlareSolverr: deadline in %v, want ~%v", d, want)
	}
}
