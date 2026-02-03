package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/moyoez/localsend-go/boardcast"
	"github.com/moyoez/localsend-go/share"
	"github.com/moyoez/localsend-go/tool"
	"github.com/moyoez/localsend-go/types"
)

// UserGetNetworkInfo returns local network interface information with IP addresses and segment numbers.
// GET /api/self/v1/get-network-info
func UserGetNetworkInfo(c *gin.Context) {
	infos := share.GetSelfNetworkInfos()
	c.JSON(http.StatusOK, tool.FastReturnSuccessWithData(infos))
}

// UserScanCurrent returns the current scanned devices.
// GET /api/self/v1/scan-current
func UserScanCurrent(c *gin.Context) {
	keys := share.ListUserScanCurrent()
	values := make([]types.UserScanCurrentItem, 0)
	for _, key := range keys {
		item, ok := share.GetUserScanCurrent(key)
		if !ok {
			continue
		}
		values = append(values, item)
	}
	c.JSON(http.StatusOK, tool.FastReturnSuccessWithData(values))
}

// UserScanNow triggers an immediate device scan based on current configuration.
// GET /api/self/v1/scan-now
func UserScanNow(c *gin.Context) {
	err := boardcast.ScanNow()
	if err != nil {
		c.JSON(http.StatusInternalServerError, tool.FastReturnError("Scan failed: "+err.Error()))
		return
	}
	keys := share.ListUserScanCurrent()
	values := make([]types.UserScanCurrentItem, 0)
	for _, key := range keys {
		item, ok := share.GetUserScanCurrent(key)
		if !ok {
			continue
		}
		values = append(values, item)
	}
	c.JSON(http.StatusOK, tool.FastReturnSuccessWithData(values))
}
