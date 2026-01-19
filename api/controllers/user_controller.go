package controllers

import (
	"net"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/moyoez/localsend-base-protocol-golang/share"
	"github.com/moyoez/localsend-base-protocol-golang/transfer"
	"github.com/moyoez/localsend-base-protocol-golang/types"
)

type UserFileUploaderRequest struct {
	TargetTo string `json:"targetTo"`
	FileType string `json:"fileType"` // folder / media / text
	Files    []struct {
		FileName string `json:"fileName"`
		FileData []byte `json:"fileData"`
	} `json:"files"`
}

func UserScanCurrent(c *gin.Context) {
	keys := share.ListUserScanCurrent()
	c.JSON(http.StatusOK, gin.H{"data": keys})
}

func UserFileUploader(c *gin.Context) {
	// upload requirement:
	var request UserFileUploaderRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	targetToList, ok := share.GetUserScanCurrent(request.TargetTo)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "Target not found"})
		return
	}
	getTarget := targetToList
	// go on work for this.
	// create files map.
	filesMap := make(map[string]types.FileInfo)
	for _, f := range request.Files {
		filesMap[f.FileName] = types.FileInfo{
			FileName: f.FileName,
			Size:     int64(len(f.FileData)),
			FileType: request.FileType,
			Metadata: nil,
		}
	}
	prepareResponse, err := transfer.ReadyToUploadTo(&net.UDPAddr{IP: net.ParseIP(getTarget.Ipaddress).To4(), Port: getTarget.Port}, &getTarget.VersionMessage, &types.PrepareUploadRequest{
		Info: types.DeviceInfo{
			Alias:       getTarget.VersionMessage.Alias,
			Version:     getTarget.VersionMessage.Version,
			DeviceModel: getTarget.VersionMessage.DeviceModel,
			DeviceType:  getTarget.VersionMessage.DeviceType,
		},
		Files: filesMap,
	}, "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": prepareResponse})
}
