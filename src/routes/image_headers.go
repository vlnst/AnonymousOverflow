package routes

import (
	"strings"

	"github.com/gin-gonic/gin"
)

func setImageHeaders(c *gin.Context, contentType string) bool {
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(contentType)), "image/") {
		return false
	}
	c.Header("Content-Type", contentType)
	// SVG is an image type but runs scripts when opened directly.
	c.Header("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	return true
}
