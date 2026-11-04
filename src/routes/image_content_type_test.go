package routes

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/go-resty/resty/v2"
)

// allowLoopbackFetch lets the handler fetch the loopback test upstream through
// a plain client, without the public-address guard.
func allowLoopbackFetch(t *testing.T) {
	t.Helper()
	orig := fetchImage
	fetchImage = func(ctx context.Context, target string) (*resty.Response, error) {
		return resty.New().R().SetContext(ctx).SetDoNotParseResponse(true).Get(target)
	}
	t.Cleanup(func() { fetchImage = orig })
}

func TestImageProxyDoesNotServeHTML(t *testing.T) {
	t.Setenv("JWT_SIGNING_SECRET", "test-secret")
	t.Setenv("APP_URL", "")
	allowLoopbackFetch(t)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, `<script>alert(document.domain)</script>`)
	}))
	defer upstream.Close()

	w := serveImageProxy(proxyPathFor(t, upstream.URL+"/x.png"))
	if strings.Contains(w.Header().Get("Content-Type"), "text/html") || strings.Contains(w.Body.String(), "<script>") {
		t.Fatalf("image proxy served HTML from the instance origin: %d %q %q",
			w.Code, w.Header().Get("Content-Type"), w.Body.String())
	}
}

func TestSetImageHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		contentType string
		ok          bool
	}{
		{"image/png", true},
		{"image/svg+xml", true},
		{"IMAGE/JPEG", true},
		{"text/html; charset=utf-8", false},
		{"application/javascript", false},
		{"application/octet-stream", false},
		{"", false},
	}
	for _, tc := range cases {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		if got := setImageHeaders(c, tc.contentType); got != tc.ok {
			t.Errorf("setImageHeaders(%q) = %v, want %v", tc.contentType, got, tc.ok)
			continue
		}
		if !tc.ok {
			continue
		}
		if ct := w.Header().Get("Content-Type"); ct != tc.contentType {
			t.Errorf("Content-Type = %q, want %q", ct, tc.contentType)
		}
		if csp := w.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "sandbox") {
			t.Errorf("%s: missing sandbox CSP, got %q", tc.contentType, csp)
		}
	}
}
