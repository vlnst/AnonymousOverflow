package routes

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestImageProxyRefusesInternalAddresses(t *testing.T) {
	t.Setenv("JWT_SIGNING_SECRET", "test-secret")
	t.Setenv("APP_URL", "")

	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"AccessKeyId":"internal-secret"}`)
	}))
	defer internal.Close()

	w := serveImageProxy(proxyPathFor(t, internal.URL+"/latest/meta-data/iam/security-credentials/role"))
	if w.Code == http.StatusOK || strings.Contains(w.Body.String(), "internal-secret") {
		t.Fatalf("image proxy fetched an internal address: %d %q", w.Code, w.Body.String())
	}
}
