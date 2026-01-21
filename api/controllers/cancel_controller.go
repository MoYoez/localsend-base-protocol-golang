package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/moyoez/localsend-base-protocol-golang/api/models"
	"github.com/moyoez/localsend-base-protocol-golang/tool"
	"github.com/moyoez/localsend-base-protocol-golang/types"
)

type CancelController struct {
	handler types.HandlerInterface
}

func NewCancelController(handler types.HandlerInterface) *CancelController {
	return &CancelController{
		handler: handler,
	}
}

func (ctrl *CancelController) HandleCancel(c *gin.Context) {
	sessionId := c.Query("sessionId")

	if sessionId == "" {
		tool.DefaultLogger.Errorf("Missing required parameter: sessionId")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing parameters"})
		return
	}

	tool.DefaultLogger.Infof("[Cancel] Received cancel request: sessionId=%s", sessionId)

	if ctrl.handler != nil {
		tool.DefaultLogger.Infof("[Cancel] Processing cancel callback for session: %s", sessionId)
		if err := ctrl.handler.OnCancel(sessionId); err != nil {
			tool.DefaultLogger.Errorf("[Cancel] Cancel callback error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
			return
		}
		tool.DefaultLogger.Infof("[Cancel] Successfully cancelled session: %s", sessionId)
	}

	models.RemoveUploadSession(sessionId)
	tool.DefaultLogger.Infof("[Cancel] Removed upload session: %s", sessionId)
	c.Status(http.StatusOK)
}
