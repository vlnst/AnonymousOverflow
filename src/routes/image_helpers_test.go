package routes

import (
	"anonymousoverflow/src/middleware"
	"anonymousoverflow/src/utils"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/gin-gonic/gin"
)

var proxyPathRegex = regexp.MustCompile(`/proxy\?auth=[^"]+`)

// proxyPathFor returns the /proxy URL the app emits for an <img> found in
// upstream post HTML.
func proxyPathFor(t *testing.T, src string) string {
	t.Helper()
	out := utils.ReplaceImgTags(`<p><img src="` + src + `" alt=""></p>`)
	path := proxyPathRegex.FindString(out)
	if path == "" {
		t.Fatalf("no proxy URL in %q", out)
	}
	return path
}

func serveImageProxy(path string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.OptionsMiddleware())
	r.GET("/proxy", GetImage)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}
