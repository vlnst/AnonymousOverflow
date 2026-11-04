package routes

import (
	"anonymousoverflow/src/utils"
	"context"
	"io"
	"os"
	"time"

	"github.com/gin-gonic/gin"
)

var (
	imageFetchTimeout       = 30 * time.Second
	maxImageBytes     int64 = 10 << 20
)

func imageFetchContext(c *gin.Context) (context.Context, context.CancelFunc) {
	timeout := imageFetchTimeout
	if os.Getenv("FLARESOLVERR_URL") != "" {
		timeout += utils.SolveTimeout + 30*time.Second
	}
	return context.WithTimeout(c.Request.Context(), timeout)
}

func writeImageBody(w io.Writer, body io.Reader) {
	io.Copy(w, io.LimitReader(body, maxImageBytes))
}
