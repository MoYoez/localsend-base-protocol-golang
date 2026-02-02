package controllers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/moyoez/localsend-base-protocol-golang/api/models"
	"github.com/moyoez/localsend-base-protocol-golang/tool"
	"github.com/moyoez/localsend-base-protocol-golang/types"
)

// UserConfirmRecv handles confirm receive request
// GET /api/self/v1/confirm-recv
func UserConfirmRecv(c *gin.Context) {
	sessionId := strings.TrimSpace(c.Query("sessionId"))
	confirmedRaw := strings.TrimSpace(c.Query("confirmed"))
	if sessionId == "" {
		c.JSON(http.StatusBadRequest, tool.FastReturnError("Missing required parameter: sessionId"))
		return
	}
	if confirmedRaw == "" {
		c.JSON(http.StatusBadRequest, tool.FastReturnError("Missing required parameter: confirmed"))
		return
	}

	confirmed, err := strconv.ParseBool(confirmedRaw)
	if err != nil {
		c.JSON(http.StatusBadRequest, tool.FastReturnError("Invalid parameter: confirmed"))
		return
	}

	confirmCh, ok := models.GetConfirmRecvChannel(sessionId)
	if !ok {
		c.JSON(http.StatusNotFound, tool.FastReturnError("Session not found or expired"))
		return
	}

	select {
	case confirmCh <- types.ConfirmResult{Confirmed: confirmed}:
		models.DeleteConfirmRecvChannel(sessionId)
		c.JSON(http.StatusOK, tool.FastReturnSuccess())
	default:
		c.JSON(http.StatusConflict, tool.FastReturnError("Confirm channel busy"))
	}
}

// UserConfirmDownload handles confirm download request
// GET /api/self/v1/confirm-download
func UserConfirmDownload(c *gin.Context) {
	sessionId := strings.TrimSpace(c.Query("sessionId"))
	confirmedRaw := strings.TrimSpace(c.Query("confirmed"))
	if sessionId == "" {
		c.JSON(http.StatusBadRequest, tool.FastReturnError("Missing required parameter: sessionId"))
		return
	}
	if confirmedRaw == "" {
		c.JSON(http.StatusBadRequest, tool.FastReturnError("Missing required parameter: confirmed"))
		return
	}

	confirmed, err := strconv.ParseBool(confirmedRaw)
	if err != nil {
		c.JSON(http.StatusBadRequest, tool.FastReturnError("Invalid parameter: confirmed"))
		return
	}

	confirmCh, ok := models.GetConfirmDownloadChannel(sessionId)
	if !ok {
		c.JSON(http.StatusNotFound, tool.FastReturnError("Session not found or expired"))
		return
	}

	select {
	case confirmCh <- types.ConfirmResult{Confirmed: confirmed}:
		models.DeleteConfirmDownloadChannel(sessionId)
		c.JSON(http.StatusOK, tool.FastReturnSuccess())
	default:
		c.JSON(http.StatusConflict, tool.FastReturnError("Confirm channel busy"))
	}
}
