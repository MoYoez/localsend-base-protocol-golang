package controllers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/moyoez/localsend-go/tool"
	"github.com/skip2/go-qrcode"
)

const (
	defaultQRSize = 200
	maxQRSize     = 512
)

// GenerateQRCode returns a PNG QR code image. Compatible with api.qrserver.com create-qr-code API:
// GET ?size=200x200&data=<url-encoded-content>
func GenerateQRCode(c *gin.Context) {
	data := c.Query("data")
	if data == "" {
		c.JSON(http.StatusBadRequest, tool.FastReturnError("Missing required parameter: data"))
		return
	}

	sizeStr := c.Query("size")
	size := parseSize(sizeStr)
	if size <= 0 {
		size = defaultQRSize
	}
	if size > maxQRSize {
		size = maxQRSize
	}

	png, err := qrcode.Encode(data, qrcode.Medium, size)
	if err != nil {
		c.JSON(http.StatusInternalServerError, tool.FastReturnError("Failed to encode QR code: "+err.Error()))
		return
	}

	c.Data(http.StatusOK, "image/png", png)
}

// parseSize parses size from "200x200" or "200" and returns the pixel dimension.
func parseSize(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if idx := strings.Index(s, "x"); idx > 0 {
		s = strings.TrimSpace(s[:idx])
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}
