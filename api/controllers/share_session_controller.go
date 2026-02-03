package controllers

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/moyoez/localsend-go/api/models"
	"github.com/moyoez/localsend-go/share"
	"github.com/moyoez/localsend-go/tool"
	"github.com/moyoez/localsend-go/types"
)

// UserCreateShareSession creates a share session for the download API
// POST /api/self/v1/create-share-session
func UserCreateShareSession(c *gin.Context) {
	var request types.CreateShareSessionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, tool.FastReturnError("Invalid request body: "+err.Error()))
		return
	}
	if len(request.Files) == 0 {
		c.JSON(http.StatusBadRequest, tool.FastReturnError("files is required and must not be empty"))
		return
	}

	files := make(map[string]types.ShareFileEntry)
	for fileId, fileInput := range request.Files {
		input := fileInput
		if input.FileUrl == "" {
			c.JSON(http.StatusBadRequest, tool.FastReturnError(fmt.Sprintf("fileUrl is required for %s", fileId)))
			return
		}
		parsedUrl, err := url.Parse(input.FileUrl)
		if err != nil || parsedUrl.Scheme != "file" {
			c.JSON(http.StatusBadRequest, tool.FastReturnError(fmt.Sprintf("Invalid fileUrl for %s: must be file:// path", fileId)))
			return
		}
		localPath := parsedUrl.Path

		info, err := os.Stat(localPath)
		if err != nil {
			if os.IsNotExist(err) {
				c.JSON(http.StatusBadRequest, tool.FastReturnError(fmt.Sprintf("File or folder not found: %s", localPath)))
				return
			}
			c.JSON(http.StatusBadRequest, tool.FastReturnError(fmt.Sprintf("Failed to access %s: %v", localPath, err)))
			return
		}

		if info.IsDir() {
			fileInputMap, pathMap, err := tool.ProcessPathInput(localPath, true)
			if err != nil {
				c.JSON(http.StatusBadRequest, tool.FastReturnError(fmt.Sprintf("Invalid folder %s: %v", fileId, err)))
				return
			}
			for id, inp := range fileInputMap {
				entryPath := pathMap[id]
				idVal := inp.ID
				if idVal == "" {
					idVal = id
				}
				files[id] = types.ShareFileEntry{
					FileInfo: types.FileInfo{
						ID:       idVal,
						FileName: inp.FileName,
						Size:     inp.Size,
						FileType: inp.FileType,
						SHA256:   inp.SHA256,
						Preview:  inp.Preview,
					},
					LocalPath: entryPath,
				}
			}
			continue
		}

		if err := tool.ProcessFileInput(&input); err != nil {
			c.JSON(http.StatusBadRequest, tool.FastReturnError(fmt.Sprintf("Invalid file %s: %v", fileId, err)))
			return
		}
		fileIdVal := input.ID
		if fileIdVal == "" {
			fileIdVal = fileId
		}
		files[fileId] = types.ShareFileEntry{
			FileInfo: types.FileInfo{
				ID:       fileIdVal,
				FileName: input.FileName,
				Size:     input.Size,
				FileType: input.FileType,
				SHA256:   input.SHA256,
				Preview:  input.Preview,
			},
			LocalPath: localPath,
		}
	}

	sessionId := tool.GenerateShortSessionID()
	session := &types.ShareSession{
		SessionId:  sessionId,
		Files:      files,
		CreatedAt:  time.Now(),
		Pin:        request.Pin,
		AutoAccept: request.AutoAccept,
	}
	models.CacheShareSession(session)

	selfDeviceInfo := models.GetSelfDevice()
	if selfDeviceInfo == nil {
		c.JSON(http.StatusInternalServerError, tool.FastReturnError("Local device information not configured"))
		return
	}
	protocol := selfDeviceInfo.Protocol
	port := 53317
	host := "localhost"
	if infos := share.GetSelfNetworkInfos(); len(infos) > 0 {
		host = infos[0].IPAddress
	}
	downloadUrl := fmt.Sprintf("%s://%s:%d/?session=%s", protocol, host, port, sessionId)

	c.JSON(http.StatusOK, tool.FastReturnSuccessWithData(types.CreateShareSessionResponse{
		SessionId:   sessionId,
		DownloadUrl: downloadUrl,
	}))
}

// UserCloseShareSession closes a share session
// DELETE /api/self/v1/close-share-session?sessionId=xxx
func UserCloseShareSession(c *gin.Context) {
	sessionId := strings.TrimSpace(c.Query("sessionId"))
	if sessionId == "" {
		c.JSON(http.StatusBadRequest, tool.FastReturnError("Missing required parameter: sessionId"))
		return
	}
	_, ok := models.GetShareSession(sessionId)
	if !ok {
		c.JSON(http.StatusNotFound, tool.FastReturnError("Session not found or expired"))
		return
	}
	models.RemoveShareSession(sessionId)
	c.JSON(http.StatusOK, tool.FastReturnSuccess())
}
