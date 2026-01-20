package controllers

import (
	"io"
	"net"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/moyoez/localsend-base-protocol-golang/boardcast"
	"github.com/moyoez/localsend-base-protocol-golang/tool"
	"github.com/moyoez/localsend-base-protocol-golang/types"
)

type RegisterController struct {
	handler types.HandlerInterface
}

func NewRegisterController(handler types.HandlerInterface) *RegisterController {
	return &RegisterController{
		handler: handler,
	}
}

func (ctrl *RegisterController) HandleRegister(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		tool.DefaultLogger.Errorf("Failed to read register request body: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
		return
	}

	incoming, err := boardcast.ParseVersionMessageFromBody(body)
	if err != nil {
		tool.DefaultLogger.Errorf("Failed to parse register request: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	tool.DefaultLogger.Debugf("Received register request from %s (fingerprint: %s)", incoming.Alias, incoming.Fingerprint)

	remoteHost, _, splitErr := net.SplitHostPort(c.ClientIP())
	if splitErr != nil || remoteHost == "" {
		remoteHost = c.ClientIP()
	}

	if ctrl.handler != nil {
		if err := ctrl.handler.OnRegister(incoming); err != nil {
			tool.DefaultLogger.Errorf("Register callback error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
