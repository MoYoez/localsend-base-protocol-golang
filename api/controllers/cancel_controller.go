package controllers

import (
	"net/http"

	"github.com/charmbracelet/log"
	"github.com/gin-gonic/gin"
	"github.com/moyoez/localsend-base-protocol-golang/api/models"
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
		log.Errorf("Missing required parameter: sessionId")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing parameters"})
		return
	}

	log.Debugf("Received cancel request: sessionId=%s", sessionId)

	if ctrl.handler != nil {
		if err := ctrl.handler.OnCancel(sessionId); err != nil {
			log.Errorf("Cancel callback error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
			return
		}
	}

	models.RemoveUploadSession(sessionId)
	c.Status(http.StatusOK)
}
