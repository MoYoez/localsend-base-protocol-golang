package controllers

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/moyoez/localsend-base-protocol-golang/share"
	"github.com/moyoez/localsend-base-protocol-golang/tool"
)

// UserGetImage returns an image file (Steam screenshot path validation).
// GET /api/self/v1/get-image?fileName=...
func UserGetImage(c *gin.Context) {
	fileName := c.Query("fileName")
	if fileName == "" {
		c.JSON(http.StatusBadRequest, tool.FastReturnError("Missing required parameter: fileName"))
		return
	}
	if strings.HasPrefix(fileName, "file://") {
		parsedURL, err := url.Parse(fileName)
		if err != nil {
			c.JSON(http.StatusBadRequest, tool.FastReturnError("Invalid file URI: "+err.Error()))
			return
		}
		fileName = parsedURL.Path
	}
	if !strings.HasSuffix(strings.ToLower(fileName), ".jpg") {
		c.JSON(http.StatusForbidden, tool.FastReturnError("Only .jpg files are allowed"))
		return
	}
	if strings.HasPrefix(fileName, "~") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			c.JSON(http.StatusInternalServerError, tool.FastReturnError("Failed to get home directory: "+err.Error()))
			return
		}
		fileName = filepath.Join(homeDir, fileName[1:])
	}
	cleanPath := filepath.Clean(fileName)
	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, tool.FastReturnError("Invalid file path: "+err.Error()))
		return
	}
	steamPathPattern := filepath.Join(".local", "share", "Steam", "userdata")
	steamIndex := strings.Index(absPath, steamPathPattern)
	if steamIndex == -1 {
		c.JSON(http.StatusForbidden, tool.FastReturnError("Access to this path is forbidden: must be in Steam userdata directory"))
		return
	}
	userdataPath := absPath[steamIndex+len(steamPathPattern):]
	if !strings.HasPrefix(userdataPath, string(filepath.Separator)) {
		c.JSON(http.StatusForbidden, tool.FastReturnError("Invalid path structure"))
		return
	}
	userdataPath = strings.TrimPrefix(userdataPath, string(filepath.Separator))
	parts := strings.Split(userdataPath, string(filepath.Separator))
	if len(parts) < 6 {
		c.JSON(http.StatusForbidden, tool.FastReturnError("Invalid path structure: insufficient path depth"))
		return
	}
	if parts[1] != "760" || parts[2] != "remote" || parts[4] != "screenshots" {
		c.JSON(http.StatusForbidden, tool.FastReturnError("Invalid path structure: must follow userdata/*/760/remote/*/screenshots/*.jpg"))
		return
	}
	image, err := os.ReadFile(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, tool.FastReturnError("Image not found"))
			return
		}
		c.JSON(http.StatusInternalServerError, tool.FastReturnError("Failed to read image: "+err.Error()))
		return
	}
	c.Data(http.StatusOK, "image/jpeg", image)
}

// UserGetNetworkInterfaces returns the list of network interfaces.
// GET /api/self/v1/get-network-interfaces
func UserGetNetworkInterfaces(c *gin.Context) {
	c.JSON(http.StatusOK, tool.FastReturnSuccessWithData(share.GetSelfNetworkInfos()))
}
