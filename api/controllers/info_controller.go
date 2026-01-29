package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/moyoez/localsend-base-protocol-golang/api/models"
	"github.com/moyoez/localsend-base-protocol-golang/types"
)

// see here https://github.com/localsend/protocol/blob/main/v1.md#22-http-legacy-mode
func HandleLocalsendV1InfoGet(c *gin.Context) {
	selfDevice := models.GetSelfDevice()
	c.JSON(http.StatusOK, types.V1InfoResponse{
		Alias:       selfDevice.Alias,
		Version:     selfDevice.Version, // consider let the remote switch to v2.
		DeviceModel: selfDevice.DeviceModel,
		DeviceType:  selfDevice.DeviceType,
	})
}

func HandleLocalsendV2InfoGet(c *gin.Context) {
	selfDevice := models.GetSelfDevice()
	c.JSON(http.StatusOK, types.V2InfoResponse{
		Alias:       selfDevice.Alias,
		Version:     selfDevice.Version,
		DeviceModel: selfDevice.DeviceModel,
		DeviceType:  selfDevice.DeviceType,
		Fingerprint: selfDevice.Fingerprint,
		Download:    selfDevice.Download, // always false.
	})
}
