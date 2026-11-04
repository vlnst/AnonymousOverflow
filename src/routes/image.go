package routes

import (
	"anonymousoverflow/src/types"
	"anonymousoverflow/src/utils"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
)

var fetchImage = utils.GetWithClearance

func GetImage(c *gin.Context) {

	if c.MustGet("disable_images").(bool) {
		c.AbortWithStatus(403)
		return
	}

	authorization := c.Query("auth")
	if authorization == "" {
		c.String(400, "Missing auth token")
		return
	}

	// validate the auth token
	token, err := jwt.ParseWithClaims(authorization, &types.ImageProxyClaims{}, func(token *jwt.Token) (interface{}, error) {

		// validate the signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		return []byte(os.Getenv("JWT_SIGNING_SECRET")), nil
	})
	if err != nil {
		c.String(400, err.Error())
		return
	}

	claims, ok := token.Claims.(*types.ImageProxyClaims)
	if !ok || !token.Valid {
		c.String(400, "Invalid token")
		return
	}

	if claims.Action != "imageProxy" {
		c.String(400, "Invalid action")
		return
	}

	if claims.Exp < time.Now().Unix() {
		c.String(400, "Token expired")
		return
	}

	ctx, cancel := imageFetchContext(c)
	defer cancel()

	// download the image
	resp, err := fetchImage(ctx, claims.ImageURL)
	if err != nil {
		c.AbortWithStatus(500)
		return
	}
	if resp == nil || resp.StatusCode() != http.StatusOK {
		c.AbortWithStatus(http.StatusBadGateway)
		return
	}
	defer resp.RawBody().Close()

	if !setImageHeaders(c, resp.Header().Get("Content-Type")) {
		c.AbortWithStatus(502)
		return
	}

	writeImageBody(c.Writer, resp.RawBody())
}
