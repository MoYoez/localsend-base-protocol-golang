package controllers

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/moyoez/localsend-base-protocol-golang/api/defaults"
	"github.com/moyoez/localsend-base-protocol-golang/api/models"
	"github.com/moyoez/localsend-base-protocol-golang/boardcast"
	"github.com/moyoez/localsend-base-protocol-golang/tool"
	"github.com/moyoez/localsend-base-protocol-golang/types"
)

type RegisterController struct{}

func NewRegisterController() *RegisterController {
	return &RegisterController{}
}

func (ctrl *RegisterController) HandleRegister(c *gin.Context) {
	self := models.GetSelfDevice()
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		tool.DefaultLogger.Errorf("Failed to read register request body: %v", err)
		c.JSON(http.StatusBadRequest, tool.FastReturnError("Failed to read request body"))
		return
	}

	incoming, err := boardcast.ParseVersionMessageFromBody(body)
	if err != nil {
		tool.DefaultLogger.Errorf("Failed to parse register request: %v", err)
		c.JSON(http.StatusBadRequest, tool.FastReturnError("Invalid request body"))
		return
	}

	if tool.CheckFingerPrintIsSame(incoming.Fingerprint) {
		tool.DefaultLogger.Infof("Fingerprint is the same as the local device, bypass it.")
		c.JSON(http.StatusForbidden, tool.FastReturnError("Fingerprint is the same as the local device, ban it."))
		return
	}
	tool.DefaultLogger.Infof("[Register] Received register request from %s (fingerprint: %s)", incoming.Alias, incoming.Fingerprint)

	if err := defaults.DefaultOnRegister(incoming); err != nil {
		tool.DefaultLogger.Errorf("[Register] Register callback error: %v", err)
		c.JSON(http.StatusInternalServerError, tool.FastReturnError("Internal server error"))
		return
	}

	c.JSON(http.StatusOK, types.CallbackVersionMessageHTTP{
		Alias:       self.Alias,
		Version:     self.Version,
		DeviceModel: self.DeviceModel,
		DeviceType:  self.DeviceType,
		Fingerprint: self.Fingerprint,
		Download:    self.Download,
	})
}
