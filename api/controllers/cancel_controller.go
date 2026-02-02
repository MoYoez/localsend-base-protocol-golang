package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/moyoez/localsend-base-protocol-golang/api/defaults"
	"github.com/moyoez/localsend-base-protocol-golang/api/models"
	"github.com/moyoez/localsend-base-protocol-golang/tool"
)

type CancelController struct{}

func NewCancelController() *CancelController {
	return &CancelController{}
}

func (ctrl *CancelController) HandleCancel(c *gin.Context) {
	sessionId := c.Query("sessionId")

	if sessionId == "" {
		tool.DefaultLogger.Errorf("Missing required parameter: sessionId")
		c.JSON(http.StatusBadRequest, tool.FastReturnError("Missing parameters"))
		return
	}

	tool.DefaultLogger.Infof("[Cancel] Received cancel request: sessionId=%s", sessionId)

	if err := defaults.DefaultOnCancel(sessionId); err != nil {
		tool.DefaultLogger.Errorf("[Cancel] Cancel callback error: %v", err)
		c.JSON(http.StatusInternalServerError, tool.FastReturnError("Internal server error"))
		return
	}

	models.RemoveUploadSession(sessionId)
	tool.DefaultLogger.Infof("[Cancel] Removed upload session: %s", sessionId)
	c.Status(http.StatusOK)
}

// HandleCancelV1Cancel handles V1 cancel request
// POST /api/localsend/v1/cancel
func (ctrl *CancelController) HandleCancelV1Cancel(c *gin.Context) {
	remoteAddr := c.ClientIP()
	tool.DefaultLogger.Infof("[V1 Cancel] Received cancel request from IP: %s", remoteAddr)

	sessionId := models.GetV1Session(remoteAddr)
	if sessionId == "" {
		tool.DefaultLogger.Warnf("[V1 Cancel] No active session found for IP: %s, but returning OK", remoteAddr)
		c.Status(http.StatusOK)
		return
	}

	tool.DefaultLogger.Infof("[V1 Cancel] Found session %s for IP: %s", sessionId, remoteAddr)

	if err := defaults.DefaultOnCancel(sessionId); err != nil {
		tool.DefaultLogger.Errorf("[V1 Cancel] Cancel callback error: %v", err)
		c.Status(http.StatusInternalServerError)
		return
	}

	models.RemoveUploadSession(sessionId)
	models.RemoveV1Session(remoteAddr)
	tool.DefaultLogger.Infof("[V1 Cancel] Removed upload session: %s and IP mapping for: %s", sessionId, remoteAddr)

	c.Status(http.StatusOK)
}
