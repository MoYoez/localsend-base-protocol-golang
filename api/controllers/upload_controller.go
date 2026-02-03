package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/moyoez/localsend-base-protocol-golang/api/defaults"
	"github.com/moyoez/localsend-base-protocol-golang/api/models"
	"github.com/moyoez/localsend-base-protocol-golang/notify"
	"github.com/moyoez/localsend-base-protocol-golang/tool"
	"github.com/moyoez/localsend-base-protocol-golang/types"
)

type UploadController struct{}

func NewUploadController() *UploadController {
	return &UploadController{}
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

	response, callbackErr := defaults.DefaultOnPrepareUpload(request, pin)
	if callbackErr != nil {
		tool.DefaultLogger.Errorf("[PrepareUpload] Prepare-upload callback error: %v", callbackErr)
		errorMsg := callbackErr.Error()
		switch errorMsg {
		case "PIN required", "Invalid PIN", "pin required", "invalid pin":
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

	// Initialize session stats and send upload start notification (single notification for all files)
	if response != nil && response.SessionId != "" {
		// Initialize upload statistics for this session
		models.InitSessionStats(response.SessionId, len(request.Files))

		// Collect all file info for single notification
		filesList := make([]map[string]any, 0, len(request.Files))
		var totalSize int64
		for fileID, fileInfo := range request.Files {
			filesList = append(filesList, map[string]any{
				"fileId":   fileID,
				"fileName": fileInfo.FileName,
				"size":     fileInfo.Size,
				"fileType": fileInfo.FileType,
			})
			totalSize += fileInfo.Size
		}

		// Send single notification asynchronously
		go func(sessionId string, files []map[string]any, totalFiles int, totalSize int64) {
			tool.DefaultLogger.Infof("[Notify] Sending upload_start notification: sessionId=%s, totalFiles=%d",
				sessionId, totalFiles)
			if err := notify.SendUploadNotification("upload_start", sessionId, "", map[string]any{
				"totalFiles": totalFiles,
				"totalSize":  totalSize,
				"files":      files,
			}); err != nil {
				tool.DefaultLogger.Errorf("[Notify] Failed to send upload_start notification: %v", err)
			} else {
				tool.DefaultLogger.Infof("[Notify] Successfully sent upload_start notification for session: %s", sessionId)
			}
		}(response.SessionId, filesList, len(request.Files), totalSize)

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

	response, callbackErr := defaults.DefaultOnPrepareUpload(request, "")
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

	// Store IP -> sessionId mapping for V1 (since V1 doesn't use sessionId in subsequent requests)
	if response != nil && response.SessionId != "" {
		models.StoreV1Session(remoteAddr, response.SessionId)

		// Initialize upload statistics for this session
		models.InitSessionStats(response.SessionId, len(request.Files))

		// Collect all file info for single notification
		filesList := make([]map[string]any, 0, len(request.Files))
		var totalSize int64
		for fileID, fileInfo := range request.Files {
			filesList = append(filesList, map[string]any{
				"fileId":   fileID,
				"fileName": fileInfo.FileName,
				"size":     fileInfo.Size,
				"fileType": fileInfo.FileType,
			})
			totalSize += fileInfo.Size
		}

		// Send single notification asynchronously
		go func(sessionId string, files []map[string]any, totalFiles int, totalSize int64) {
			tool.DefaultLogger.Infof("[V1 Notify] Sending upload_start notification: sessionId=%s, totalFiles=%d",
				sessionId, totalFiles)
			if err := notify.SendUploadNotification("upload_start", sessionId, "", map[string]any{
				"totalFiles": totalFiles,
				"totalSize":  totalSize,
				"files":      files,
			}); err != nil {
				tool.DefaultLogger.Errorf("[V1 Notify] Failed to send upload_start notification: %v", err)
			} else {
				tool.DefaultLogger.Infof("[V1 Notify] Successfully sent upload_start notification for session: %s", sessionId)
			}
		}(response.SessionId, filesList, len(request.Files), totalSize)

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

	// Get file info before processing (needed for both success and failure cases)
	fileInfo, hasFileInfo := models.LookupFileInfo(sessionId, fileId)

	uploadErr := defaults.DefaultOnUpload(sessionId, fileId, token, c.Request.Body, remoteAddr)
	if uploadErr != nil {
		tool.DefaultLogger.Errorf("[V1 Send] Upload callback error: %v", uploadErr)

		// Mark file as failed and check if all files are done
		remaining, isLast, stats := models.MarkFileUploadedAndCheckComplete(sessionId, fileId, false)
		tool.DefaultLogger.Infof("[V1 Send] File failed: %s, remaining files: %d, isLast: %v", fileId, remaining, isLast)

		// Send notification when all files are processed (even if some failed)
		if isLast && stats != nil {
			go func(sid string, stats *models.SessionUploadStats, remoteAddr string) {
				// remove session
				models.RemoveV1Session(remoteAddr)
				tool.DefaultLogger.Infof("[V1 Notify] Sending upload_end notification (all files processed): sessionId=%s, success=%d, failed=%d",
					sid, stats.SuccessFiles, stats.FailedFiles)
				if err := notify.SendUploadNotification("upload_end", sid, "", map[string]any{
					"totalFiles":    stats.TotalFiles,
					"successFiles":  stats.SuccessFiles,
					"failedFiles":   stats.FailedFiles,
					"failedFileIds": stats.FailedFileIds,
				}); err != nil {
					tool.DefaultLogger.Errorf("[V1 Notify] Failed to send upload_end notification: %v", err)
				}

				models.CleanupSessionStats(sid)
				models.RemoveUploadSession(sid)
			}(sessionId, stats, remoteAddr)
		}

		errorMsg := uploadErr.Error()
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
	}
	// Upload successful
	if !hasFileInfo {
		tool.DefaultLogger.Errorf("[V1 Send] File info not found for sessionId=%s, fileId=%s", sessionId, fileId)
		c.JSON(http.StatusInternalServerError, tool.FastReturnError("File info not found"))
		return
	}
	tool.DefaultLogger.Infof("[V1 Send] Successfully uploaded file: %s (sessionId=%s)", fileInfo.FileName, sessionId)

	remaining, isLast, stats := models.MarkFileUploadedAndCheckComplete(sessionId, fileId, true)
	tool.DefaultLogger.Infof("[V1 Send] File completed: %s, remaining files: %d, isLast: %v", fileInfo.FileName, remaining, isLast)

	if isLast && stats != nil {
		go func(sid, fid string, fileInfo types.FileInfo, stats *models.SessionUploadStats) {
			tool.DefaultLogger.Infof("[V1 Notify] Sending upload_end notification (all files processed): sessionId=%s, success=%d, failed=%d",
				sid, stats.SuccessFiles, stats.FailedFiles)
			if err := notify.SendUploadNotification("upload_end", sid, fid, map[string]any{
				"fileName":      fileInfo.FileName,
				"fileType":      fileInfo.FileType,
				"totalFiles":    stats.TotalFiles,
				"successFiles":  stats.SuccessFiles,
				"failedFiles":   stats.FailedFiles,
				"failedFileIds": stats.FailedFileIds,
			}); err != nil {
				tool.DefaultLogger.Errorf("[V1 Notify] Failed to send upload_end notification: %v", err)
			}
			models.CleanupSessionStats(sid)
			models.RemoveUploadSession(sid)
		}(sessionId, fileId, fileInfo, stats)
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

	// Get file info before processing (needed for both success and failure cases)
	fileInfo, hasFileInfo := models.LookupFileInfo(sessionId, fileId)

	uploadErr := defaults.DefaultOnUpload(sessionId, fileId, token, c.Request.Body, remoteAddr)
	if uploadErr != nil {
		tool.DefaultLogger.Errorf("[Upload] Upload callback error: %v", uploadErr)

		remaining, isLast, stats := models.MarkFileUploadedAndCheckComplete(sessionId, fileId, false)
		tool.DefaultLogger.Infof("[Upload] File failed: %s, remaining files: %d, isLast: %v", fileId, remaining, isLast)

		if isLast && stats != nil {
			go func(sid string, stats *models.SessionUploadStats) {
				tool.DefaultLogger.Infof("[Notify] Sending upload_end notification (all files processed): sessionId=%s, success=%d, failed=%d",
					sid, stats.SuccessFiles, stats.FailedFiles)
				if err := notify.SendUploadNotification("upload_end", sid, "", map[string]any{
					"totalFiles":    stats.TotalFiles,
					"successFiles":  stats.SuccessFiles,
					"failedFiles":   stats.FailedFiles,
					"failedFileIds": stats.FailedFileIds,
				}); err != nil {
					tool.DefaultLogger.Errorf("[Notify] Failed to send upload_end notification: %v", err)
				}
				models.CleanupSessionStats(sid)
				models.RemoveUploadSession(sid)
			}(sessionId, stats)
		}

		errorMsg := uploadErr.Error()
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
	}

	if !hasFileInfo {
		tool.DefaultLogger.Errorf("[Upload] File info not found for sessionId=%s, fileId=%s", sessionId, fileId)
		c.JSON(http.StatusInternalServerError, tool.FastReturnError("File info not found"))
		return
	}
	tool.DefaultLogger.Infof("[Upload] Successfully uploaded file: %s (sessionId=%s)", fileInfo.FileName, sessionId)

	remaining, isLast, stats := models.MarkFileUploadedAndCheckComplete(sessionId, fileId, true)
	tool.DefaultLogger.Infof("[Upload] File completed: %s, remaining files: %d, isLast: %v", fileInfo.FileName, remaining, isLast)

	if isLast && stats != nil {
		go func(sid, fid string, fileInfo types.FileInfo, stats *models.SessionUploadStats) {
			tool.DefaultLogger.Infof("[Notify] Sending upload_end notification (all files processed): sessionId=%s, success=%d, failed=%d",
				sid, stats.SuccessFiles, stats.FailedFiles)
			if err := notify.SendUploadNotification("upload_end", sid, fid, map[string]any{
				"fileName":      fileInfo.FileName,
				"fileType":      fileInfo.FileType,
				"totalFiles":    stats.TotalFiles,
				"successFiles":  stats.SuccessFiles,
				"failedFiles":   stats.FailedFiles,
				"failedFileIds": stats.FailedFileIds,
			}); err != nil {
				tool.DefaultLogger.Errorf("[Notify] Failed to send upload_end notification: %v", err)
			} else {
				tool.DefaultLogger.Infof("[Notify] Successfully sent upload_end notification for session: %s", sid)
			}
			models.CleanupSessionStats(sid)
			models.RemoveUploadSession(sid)
		}(sessionId, fileId, fileInfo, stats)
	}

	c.Status(http.StatusOK)
}
