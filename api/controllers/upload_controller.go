package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/moyoez/localsend-base-protocol-golang/api/models"
	"github.com/moyoez/localsend-base-protocol-golang/notify"
	"github.com/moyoez/localsend-base-protocol-golang/tool"
	"github.com/moyoez/localsend-base-protocol-golang/types"
)

type UploadController struct {
	handler types.HandlerInterface
}

func NewUploadController(handler types.HandlerInterface) *UploadController {
	return &UploadController{
		handler: handler,
	}
}

func (ctrl *UploadController) HandlePrepareUpload(c *gin.Context) {
	pin := c.Query("pin")
	body, err := c.GetRawData()
	if err != nil {
		tool.DefaultLogger.Errorf("Failed to read prepare-upload request body: %v", err)
		c.JSON(http.StatusBadRequest, tool.FastReturnError("Failed to read request body"))
		return
	}

	request, err := models.ParsePrepareUploadRequest(body)
	if err != nil {
		tool.DefaultLogger.Errorf("Failed to parse prepare-upload request: %v", err)
		c.JSON(http.StatusBadRequest, tool.FastReturnError("Invalid request body"))
		return
	}

	tool.DefaultLogger.Infof("[PrepareUpload] Received prepare-upload request from %s (pin: %s)", request.Info.Alias, pin)
	tool.DefaultLogger.Infof("[PrepareUpload] Number of files: %d", len(request.Files))

	var response *types.PrepareUploadResponse
	if ctrl.handler != nil {
		tool.DefaultLogger.Infof("[PrepareUpload] Processing prepare-upload callback for device: %s", request.Info.Alias)
		var callbackErr error
		response, callbackErr = ctrl.handler.OnPrepareUpload(request, pin)
		if callbackErr != nil {
			tool.DefaultLogger.Errorf("[PrepareUpload] Prepare-upload callback error: %v", callbackErr)
			errorMsg := callbackErr.Error()
			switch errorMsg {
			case "PIN required", "Invalid PIN", "pin required", "invalid pin":
				// Return standardized error message
				switch errorMsg {
				case "pin required":
					errorMsg = "PIN required"
				case "invalid pin":
					errorMsg = "Invalid PIN"
				}
				c.JSON(http.StatusUnauthorized, tool.FastReturnError(errorMsg))
				return
			case "rejected":
				c.JSON(http.StatusForbidden, tool.FastReturnError(errorMsg))
				return
			case "blocked by another session":
				c.JSON(http.StatusConflict, tool.FastReturnError(errorMsg))
				return
			case "too many requests":
				c.JSON(http.StatusTooManyRequests, tool.FastReturnError(errorMsg))
				return
			default:
				c.JSON(http.StatusInternalServerError, tool.FastReturnError(errorMsg))
				return
			}
		}
	} else {
		response = &types.PrepareUploadResponse{
			SessionId: "default-session",
			Files:     make(map[string]string),
		}
		for fileID := range request.Files {
			response.Files[fileID] = "accepted"
		}
	}

	// Send upload start notifications for each file
	if response != nil && response.SessionId != "" {
		for fileID := range request.Files {
			fileInfo := request.Files[fileID]
			// Send notification asynchronously to avoid blocking the response
			go func(sessionId, fileId string, fileInfo types.FileInfo) {
				tool.DefaultLogger.Infof("[Notify] Sending upload_start notification: sessionId=%s, fileId=%s, fileName=%s",
					sessionId, fileId, fileInfo.FileName)
				if err := notify.SendUploadNotification("upload_start", sessionId, fileId, map[string]any{
					"fileName": fileInfo.FileName,
					"size":     fileInfo.Size,
					"fileType": fileInfo.FileType,
					"sha256":   fileInfo.SHA256,
				}); err != nil {
					tool.DefaultLogger.Errorf("[Notify] Failed to send upload_start notification: %v", err)
				} else {
					tool.DefaultLogger.Infof("[Notify] Successfully sent upload_start notification for file: %s", fileInfo.FileName)
				}
			}(response.SessionId, fileID, fileInfo)
		}
		tool.DefaultLogger.Infof("[PrepareUpload] Successfully prepared upload session: %s", response.SessionId)

	}

	c.JSON(http.StatusOK, response)
}

// HandlePrepareV1Upload handles V1 send-request (metadata only)
// POST /api/localsend/v1/send-request
// V1 differs from V2: simpler device info, response has no sessionId
func (ctrl *UploadController) HandlePrepareV1Upload(c *gin.Context) {
	body, err := c.GetRawData()
	if err != nil {
		tool.DefaultLogger.Errorf("[V1 SendRequest] Failed to read request body: %v", err)
		c.JSON(http.StatusBadRequest, tool.FastReturnError("Failed to read request body"))
		return
	}

	request, err := models.ParsePrepareUploadRequest(body)
	if err != nil {
		tool.DefaultLogger.Errorf("[V1 SendRequest] Failed to parse request: %v", err)
		c.JSON(http.StatusBadRequest, tool.FastReturnError("Invalid request body"))
		return
	}

	remoteAddr := c.ClientIP()
	tool.DefaultLogger.Infof("[V1 SendRequest] Received send-request from %s (IP: %s)", request.Info.Alias, remoteAddr)
	tool.DefaultLogger.Infof("[V1 SendRequest] Number of files: %d", len(request.Files))

	var response *types.PrepareUploadResponse
	if ctrl.handler != nil {
		tool.DefaultLogger.Infof("[V1 SendRequest] Processing callback for device: %s", request.Info.Alias)
		var callbackErr error
		response, callbackErr = ctrl.handler.OnPrepareUpload(request, "")
		if callbackErr != nil {
			tool.DefaultLogger.Errorf("[V1 SendRequest] Callback error: %v", callbackErr)
			errorMsg := callbackErr.Error()
			switch errorMsg {
			case "rejected":
				c.JSON(http.StatusForbidden, tool.FastReturnError(errorMsg))
				return
			case "blocked by another session":
				c.JSON(http.StatusConflict, tool.FastReturnError(errorMsg))
				return
			case "too many requests":
				c.JSON(http.StatusTooManyRequests, tool.FastReturnError(errorMsg))
				return
			default:
				c.JSON(http.StatusInternalServerError, tool.FastReturnError(errorMsg))
				return
			}
		}
	} else {
		response = &types.PrepareUploadResponse{
			SessionId: "default-session",
			Files:     make(map[string]string),
		}
		for fileID := range request.Files {
			response.Files[fileID] = "accepted"
		}
	}

	// Store IP -> sessionId mapping for V1 (since V1 doesn't use sessionId in subsequent requests)
	if response != nil && response.SessionId != "" {
		models.StoreV1Session(remoteAddr, response.SessionId)

		// Send upload start notifications for each file
		for fileID := range request.Files {
			fileInfo := request.Files[fileID]
			go func(sessionId, fileId string, fileInfo types.FileInfo) {
				tool.DefaultLogger.Infof("[V1 Notify] Sending upload_start notification: sessionId=%s, fileId=%s, fileName=%s",
					sessionId, fileId, fileInfo.FileName)
				if err := notify.SendUploadNotification("upload_start", sessionId, fileId, map[string]any{
					"fileName": fileInfo.FileName,
					"size":     fileInfo.Size,
					"fileType": fileInfo.FileType,
					"sha256":   fileInfo.SHA256,
				}); err != nil {
					tool.DefaultLogger.Errorf("[V1 Notify] Failed to send upload_start notification: %v", err)
				}
			}(response.SessionId, fileID, fileInfo)
		}
		tool.DefaultLogger.Infof("[V1 SendRequest] Successfully prepared session: %s for IP: %s", response.SessionId, remoteAddr)
	}

	// V1 response: only returns {fileId: token} mapping, no sessionId
	c.JSON(http.StatusOK, response.Files)
}

// HandleUploadV1Upload handles V1 file upload
// POST /api/localsend/v1/send?fileId=xxx&token=xxx
// V1 differs from V2: no sessionId parameter, uses IP to determine session
func (ctrl *UploadController) HandleUploadV1Upload(c *gin.Context) {
	fileId := c.Query("fileId")
	token := c.Query("token")

	if fileId == "" || token == "" {
		tool.DefaultLogger.Errorf("[V1 Send] Missing required parameters: fileId=%s, token=%s", fileId, token)
		c.JSON(http.StatusBadRequest, tool.FastReturnError("Missing parameters"))
		return
	}

	remoteAddr := c.ClientIP()
	// V1 uses IP address to determine session
	sessionId := models.GetV1Session(remoteAddr)
	if sessionId == "" {
		tool.DefaultLogger.Errorf("[V1 Send] No active session found for IP: %s", remoteAddr)
		c.JSON(http.StatusConflict, tool.FastReturnError("No active session"))
		return
	}

	tool.DefaultLogger.Infof("[V1 Send] Received upload request: fileId=%s, token=%s, remoteAddr=%s, sessionId=%s", fileId, token, remoteAddr, sessionId)

	if ctrl.handler != nil {
		tool.DefaultLogger.Infof("[V1 Send] Processing upload callback for fileId: %s", fileId)
		if err := ctrl.handler.OnUpload(sessionId, fileId, token, c.Request.Body, remoteAddr); err != nil {
			tool.DefaultLogger.Errorf("[V1 Send] Upload callback error: %v", err)
			errorMsg := err.Error()

			switch errorMsg {
			case "Invalid token or IP address":
				c.JSON(http.StatusForbidden, tool.FastReturnError(errorMsg))
				return
			case "Blocked by another session":
				c.JSON(http.StatusConflict, tool.FastReturnError(errorMsg))
				return
			default:
				c.JSON(http.StatusInternalServerError, tool.FastReturnError(errorMsg))
				return
			}
		} else {
			// Upload successful, send upload end notification
			fileInfo, ok := models.LookupFileInfo(sessionId, fileId)
			if !ok {
				tool.DefaultLogger.Errorf("[V1 Send] File info not found for sessionId=%s, fileId=%s", sessionId, fileId)
				c.JSON(http.StatusInternalServerError, tool.FastReturnError("File info not found"))
				return
			}
			go func(sid, fid string, fileInfo types.FileInfo) {
				tool.DefaultLogger.Infof("[V1 Notify] Sending upload_end notification: sessionId=%s, fileId=%s, fileName=%s",
					sid, fid, fileInfo.FileName)
				if err := notify.SendUploadNotification("upload_end", sid, fid, map[string]any{
					"fileName": fileInfo.FileName,
					"size":     fileInfo.Size,
					"fileType": fileInfo.FileType,
					"sha256":   fileInfo.SHA256,
				}); err != nil {
					tool.DefaultLogger.Errorf("[V1 Notify] Failed to send upload_end notification: %v", err)
				}
			}(sessionId, fileId, fileInfo)
			tool.DefaultLogger.Infof("[V1 Send] Successfully uploaded file: %s (sessionId=%s)", fileInfo.FileName, sessionId)
		}
	}

	c.Status(http.StatusOK)
}

func (ctrl *UploadController) HandleUpload(c *gin.Context) {
	sessionId := c.Query("sessionId")
	fileId := c.Query("fileId")
	token := c.Query("token")

	if sessionId == "" || fileId == "" || token == "" {
		tool.DefaultLogger.Errorf("Missing required parameters: sessionId=%s, fileId=%s, token=%s", sessionId, fileId, token)
		c.JSON(http.StatusBadRequest, tool.FastReturnError("Missing parameters"))
		return
	}

	if !models.IsSessionValidated(sessionId) {
		if !tool.QuerySessionIsValid(sessionId) {
			tool.DefaultLogger.Errorf("Invalid sessionId: %s", sessionId)
			c.JSON(http.StatusConflict, tool.FastReturnError("Blocked by another session"))
			return
		}
		models.MarkSessionValidated(sessionId)
	}

	remoteAddr := c.ClientIP()
	tool.DefaultLogger.Infof("[Upload] Received upload request: sessionId=%s, fileId=%s, token=%s, remoteAddr=%s", sessionId, fileId, token, remoteAddr)
	tool.DefaultLogger.Debugf("[Upload] Content-Type: %s", c.GetHeader("Content-Type"))
	if ctrl.handler != nil {
		tool.DefaultLogger.Infof("[Upload] Processing upload callback for fileId: %s", fileId)
		if err := ctrl.handler.OnUpload(sessionId, fileId, token, c.Request.Body, remoteAddr); err != nil {
			tool.DefaultLogger.Errorf("[Upload] Upload callback error: %v", err)
			errorMsg := err.Error()

			switch errorMsg {
			case "Invalid token or IP address":
				c.JSON(http.StatusForbidden, tool.FastReturnError(errorMsg))
				return
			case "Blocked by another session":
				c.JSON(http.StatusConflict, tool.FastReturnError(errorMsg))
				return
			default:
				c.JSON(http.StatusInternalServerError, tool.FastReturnError(errorMsg))
				return
			}
		} else {
			// Upload successful, send upload end notification
			fileInfo, ok := models.LookupFileInfo(sessionId, fileId)
			if !ok {
				tool.DefaultLogger.Errorf("[Upload] File info not found for sessionId=%s, fileId=%s", sessionId, fileId)
				c.JSON(http.StatusInternalServerError, tool.FastReturnError("File info not found"))
				return
			}
			// Send notification asynchronously to avoid blocking the response
			go func(sessionId, fileId string, data types.FileInfo) {
				tool.DefaultLogger.Infof("[Notify] Sending upload_end notification: sessionId=%s, fileId=%s, fileName=%s",
					sessionId, fileId, fileInfo.FileName)
				if err := notify.SendUploadNotification("upload_end", sessionId, fileId, map[string]any{
					"fileName": fileInfo.FileName,
					"size":     fileInfo.Size,
					"fileType": fileInfo.FileType,
					"sha256":   fileInfo.SHA256,
				}); err != nil {
					tool.DefaultLogger.Errorf("[Notify] Failed to send upload_end notification: %v", err)
				} else {
					tool.DefaultLogger.Infof("[Notify] Successfully sent upload_end notification for file: %s", fileInfo.FileName)
				}
			}(sessionId, fileId, fileInfo)
			tool.DefaultLogger.Infof("[Upload] Successfully uploaded file: %s (sessionId=%s)", fileInfo.FileName, sessionId)
		}
	}

	c.Status(http.StatusOK)
}
