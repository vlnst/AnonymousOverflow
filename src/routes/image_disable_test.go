package routes

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestImageProxyHonoursDisabledImages(t *testing.T) {
	t.Setenv("JWT_SIGNING_SECRET", "test-secret")
	t.Setenv("APP_URL", "")
	allowLoopbackFetch(t)

	var fetched int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&fetched, 1)
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("png"))
	}))
	defer upstream.Close()

	// The cookie set by /options/images when a visitor disables images.
	w := serveImageProxy(proxyPathFor(t, upstream.URL+"/x.png"), &http.Cookie{Name: "disable_images", Value: "true"})
	if w.Code == http.StatusOK || atomic.LoadInt32(&fetched) != 0 {
		t.Fatalf("image served with images disabled: status %d, upstream fetches %d", w.Code, fetched)
	}
}
