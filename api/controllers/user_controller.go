package controllers

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	ttlworker "github.com/FloatTech/ttl"
	"github.com/gin-gonic/gin"
	"github.com/moyoez/localsend-base-protocol-golang/api/models"
	"github.com/moyoez/localsend-base-protocol-golang/share"
	"github.com/moyoez/localsend-base-protocol-golang/tool"
	"github.com/moyoez/localsend-base-protocol-golang/transfer"
	"github.com/moyoez/localsend-base-protocol-golang/types"
)

// UserPrepareUploadRequest represents the prepare upload request
type UserPrepareUploadRequest struct {
	TargetTo string                     `json:"targetTo"` // Target device identifier
	Files    map[string]types.FileInput `json:"files"`    // File metadata map, key is fileId
}

// UserUploadRequest represents the actual upload request
type UserUploadRequest struct {
	SessionId string `json:"sessionId"` // SessionId returned from prepare-upload
	FileId    string `json:"fileId"`    // File ID
	Token     string `json:"token"`     // File token
	FileUrl   string `json:"fileUrl"`   // Optional file URL (supports file:/// protocol)
}

// UserUploadBatchRequest represents batch upload request
type UserUploadBatchRequest struct {
	SessionId string               `json:"sessionId"` // SessionId returned from prepare-upload
	Files     []UserUploadFileItem `json:"files"`     // Array of files to upload
}

// UserConfirmRecvRequest represents confirm receive request
type UserConfirmRecvRequest struct {
	SessionId string `json:"sessionId"`
	Confirmed bool   `json:"confirmed"`
}

// UserUploadFileItem represents a single file in batch upload
type UserUploadFileItem struct {
	FileId  string `json:"fileId"`  // File ID
	Token   string `json:"token"`   // File token
	FileUrl string `json:"fileUrl"` // File URL (supports file:/// protocol)
}

// UserUploadBatchResult represents the result of a batch upload operation
type UserUploadBatchResult struct {
	Total   int                    `json:"total"`   // Total number of files
	Success int                    `json:"success"` // Number of successful uploads
	Failed  int                    `json:"failed"`  // Number of failed uploads
	Results []UserUploadItemResult `json:"results"` // Detailed results for each file
}

// UserUploadItemResult represents the result of a single file upload
type UserUploadItemResult struct {
	FileId  string `json:"fileId"`          // File ID
	Success bool   `json:"success"`         // Whether upload was successful
	Error   string `json:"error,omitempty"` // Error message if failed
}

// UserUploadSession stores user upload session information
type UserUploadSession struct {
	Target    share.UserScanCurrentItem // Target device information
	SessionId string                    // SessionId returned from LocalSend
	Tokens    map[string]string         // File ID to token mapping
}

// UserUploadSessions stores user upload sessions using TTL cache (default 30 minutes expiration)
var (
	UserUploadSessionTTL = 30 * time.Minute
	UserUploadSessions   = ttlworker.NewCache[string, UserUploadSession](UserUploadSessionTTL)
)

func UserScanCurrent(c *gin.Context) {
	keys := share.ListUserScanCurrent()
	// get key and values.
	values := make([]share.UserScanCurrentItem, 0)
	for _, key := range keys {
		item, ok := share.GetUserScanCurrent(key)
		if !ok {
			continue
		}
		values = append(values, item)
	}
	c.JSON(http.StatusOK, tool.FastReturnSuccessWithData(values))
}

// UserConfirmRecv handles confirm receive request
// GET /api/self/v1/confirm-recv
func UserConfirmRecv(c *gin.Context) {
	sessionId := strings.TrimSpace(c.Query("sessionId"))
	confirmedRaw := strings.TrimSpace(c.Query("confirmed"))
	if sessionId == "" {
		c.JSON(http.StatusBadRequest, tool.FastReturnError("Missing required parameter: sessionId"))
		return
	}
	if confirmedRaw == "" {
		c.JSON(http.StatusBadRequest, tool.FastReturnError("Missing required parameter: confirmed"))
		return
	}

	confirmed, err := strconv.ParseBool(confirmedRaw)
	if err != nil {
		c.JSON(http.StatusBadRequest, tool.FastReturnError("Invalid parameter: confirmed"))
		return
	}

	confirmCh, ok := models.GetConfirmRecvChannel(sessionId)
	if !ok {
		c.JSON(http.StatusNotFound, tool.FastReturnError("Session not found or expired"))
		return
	}

	select {
	case confirmCh <- types.ConfirmResult{Confirmed: confirmed}:
		models.DeleteConfirmRecvChannel(sessionId)
		c.JSON(http.StatusOK, tool.FastReturnSuccess())
	default:
		c.JSON(http.StatusConflict, tool.FastReturnError("Confirm channel busy"))
	}
}

// UserPrepareUpload handles prepare upload request
// POST /api/self/v1/prepare-upload
// Receives file metadata, sends prepare upload request to target device, returns sessionId and tokens
func UserPrepareUpload(c *gin.Context) {
	var request UserPrepareUploadRequest
	var pin string
	pin = c.Query("pin")
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, tool.FastReturnError("Invalid request body: "+err.Error()))
		return
	}

	// Validate target device exists
	targetItem, ok := share.GetUserScanCurrent(request.TargetTo)
	if !ok {
		c.JSON(http.StatusNotFound, tool.FastReturnError("Target device not found"))
		return
	}

	// Process each file input and auto-fill information from fileUrl if provided
	tool.DefaultLogger.Infof("Processing %d files for prepare-upload", len(request.Files))
	for fileID, fileInput := range request.Files {
		// Process file input (auto-fill from fileUrl if provided)
		if err := tool.ProcessFileInput(&fileInput); err != nil {
			c.JSON(http.StatusBadRequest, tool.FastReturnErrorWithData(fmt.Sprintf("Failed to process file %s: %v", fileID, err), map[string]any{
				"fileId": fileID,
			}))
			return
		}
		// Update the map with processed file input
		request.Files[fileID] = fileInput
		tool.DefaultLogger.Infof("File %s: %s (%d bytes, %s)", fileID, fileInput.FileName, fileInput.Size, fileInput.FileType)
	}

	// Build file info map
	filesMap := make(map[string]types.FileInfo)
	for fileID, fileInput := range request.Files {
		filesMap[fileID] = types.FileInfo{
			ID:       fileInput.ID,
			FileName: fileInput.FileName,
			Size:     fileInput.Size,
			FileType: fileInput.FileType,
			SHA256:   fileInput.SHA256,
			Preview:  fileInput.Preview,
		}
	}

	// Get local device information
	selfDevice := models.GetSelfDevice()
	if selfDevice == nil {
		c.JSON(http.StatusInternalServerError, tool.FastReturnError("Local device information not configured"))
		return
	}

	// Build prepare upload request
	prepareRequest := &types.PrepareUploadRequest{
		Info: types.DeviceInfo{
			Alias:       selfDevice.Alias,
			Version:     selfDevice.Version,
			DeviceModel: selfDevice.DeviceModel,
			DeviceType:  selfDevice.DeviceType,
			Fingerprint: selfDevice.Fingerprint,
			Port:        selfDevice.Port,
			Protocol:    targetItem.VersionMessage.Protocol, // Use target device protocol
			Download:    selfDevice.Download,
		},
		Files: filesMap,
	}

	// Call LocalSend prepare upload endpoint
	targetAddr := &net.UDPAddr{
		IP:   net.ParseIP(targetItem.Ipaddress).To4(),
		Port: targetItem.VersionMessage.Port,
	}

	prepareResponse, err := transfer.ReadyToUploadTo(
		targetAddr,
		&targetItem.VersionMessage,
		prepareRequest,
		pin, // PIN, can be added if needed
	)

	if err != nil {
		errorMsg := err.Error()
		// Check if it's a PIN-related error
		errorMsgLower := strings.ToLower(errorMsg)
		if strings.Contains(errorMsgLower, "pin required") || strings.Contains(errorMsgLower, "invalid pin") {
			c.JSON(http.StatusUnauthorized, tool.FastReturnError("PIN required / Invalid PIN"))
			return
		}
		c.JSON(http.StatusInternalServerError, tool.FastReturnError("Prepare upload failed: "+errorMsg))
		return
	}

	// If response is nil, it means no file transfer needed (204 status)
	if prepareResponse == nil {
		c.Status(http.StatusNoContent)
		return
	}

	// Store user upload session information
	sessionInfo := UserUploadSession{
		Target:    targetItem,
		SessionId: prepareResponse.SessionId,
		Tokens:    prepareResponse.Files,
	}

	// Store using sessionId as key
	UserUploadSessions.Set(prepareResponse.SessionId, sessionInfo)

	// Return result
	c.JSON(http.StatusOK, tool.FastReturnSuccessWithData(map[string]any{
		"sessionId": prepareResponse.SessionId,
		"files":     prepareResponse.Files,
	}))
}

// UserUpload handles actual file upload request
// POST /api/self/v1/upload
// Supports two request formats:
// 1. Query params (sessionId, fileId, token) + binary file data in body
// 2. JSON body with sessionId, fileId, token, and fileUrl (supports file:/// protocol)
func UserUpload(c *gin.Context) {
	var sessionId, fileId, token string
	var fileReader io.Reader
	var fileData []byte

	// Check Content-Type to determine request format
	contentType := c.GetHeader("Content-Type")

	if strings.Contains(contentType, "application/json") {
		// JSON request format (supports file:/// protocol)
		var request UserUploadRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, tool.FastReturnError("Invalid JSON request: "+err.Error()))
			return
		}

		sessionId = request.SessionId
		fileId = request.FileId
		token = request.Token
		fileUrl := request.FileUrl

		// Validate required parameters
		if sessionId == "" || fileId == "" || token == "" {
			c.JSON(http.StatusBadRequest, tool.FastReturnError("Missing required parameters: sessionId, fileId, token"))
			return
		}

		// Handle file:/// protocol
		if fileUrl != "" {
			parsedUrl, err := url.Parse(fileUrl)
			if err != nil {
				c.JSON(http.StatusBadRequest, tool.FastReturnError("Invalid fileUrl: "+err.Error()))
				return
			}

			if parsedUrl.Scheme == "file" {
				// Extract file path from file:/// URL
				filePath := parsedUrl.Path
				tool.DefaultLogger.Infof("Reading file from local path: %s", filePath)

				// Read file from local filesystem
				data, err := os.ReadFile(filePath)
				if err != nil {
					c.JSON(http.StatusBadRequest, tool.FastReturnErrorWithData(fmt.Sprintf("Failed to read file from %s: %v", filePath, err), map[string]any{
						"filePath": filePath,
					}))
					return
				}
				fileData = data
				tool.DefaultLogger.Infof("Successfully read %d bytes from %s", len(fileData), filePath)
			} else {
				c.JSON(http.StatusBadRequest, tool.FastReturnError("Only file:// protocol is supported for fileUrl"))
				return
			}
		} else {
			c.JSON(http.StatusBadRequest, tool.FastReturnError("fileUrl is required in JSON request"))
			return
		}
	} else {
		// Traditional binary upload format (query params + binary body)
		sessionId = c.Query("sessionId")
		fileId = c.Query("fileId")
		token = c.Query("token")

		// Validate required parameters
		if sessionId == "" || fileId == "" || token == "" {
			c.JSON(http.StatusBadRequest, tool.FastReturnError("Missing required query parameters: sessionId, fileId, token"))
			return
		}

		// Read file data from request body (binary data)
		data, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, tool.FastReturnError("Failed to read file data: "+err.Error()))
			return
		}
		defer c.Request.Body.Close()
		fileData = data
	}

	// Validate file data
	if len(fileData) == 0 {
		c.JSON(http.StatusBadRequest, tool.FastReturnError("File data is empty"))
		return
	}

	// Get user upload session information
	sessionInfo := UserUploadSessions.Get(sessionId)
	if sessionInfo.SessionId == "" {
		c.JSON(http.StatusNotFound, tool.FastReturnError("Session not found or expired"))
		return
	}

	// Validate token matches
	expectedToken, ok := sessionInfo.Tokens[fileId]
	if !ok || expectedToken != token {
		c.JSON(http.StatusForbidden, tool.FastReturnError("Invalid file ID or token"))
		return
	}

	// Create reader from file data
	fileReader = bytes.NewReader(fileData)

	// Call LocalSend upload endpoint
	targetAddr := &net.UDPAddr{
		IP:   net.ParseIP(sessionInfo.Target.Ipaddress).To4(),
		Port: sessionInfo.Target.VersionMessage.Port,
	}

	tool.DefaultLogger.Infof("Uploading file to %s:%d (sessionId=%s, fileId=%s)",
		targetAddr.IP.String(), targetAddr.Port, sessionId, fileId)

	err := transfer.UploadFile(
		targetAddr,
		&sessionInfo.Target.VersionMessage,
		sessionId,
		fileId,
		token,
		fileReader,
	)

	if err != nil {
		tool.DefaultLogger.Errorf("File upload failed: %v", err)
		c.JSON(http.StatusInternalServerError, tool.FastReturnError("File upload failed: "+err.Error()))
		return
	}

	tool.DefaultLogger.Infof("File uploaded successfully (sessionId=%s, fileId=%s)", sessionId, fileId)
	c.JSON(http.StatusOK, gin.H{"message": "File uploaded successfully"})
}

// UserUploadBatch handles batch file upload request
// POST /api/self/v1/upload-batch
// Uploads multiple files in a single request using file:/// protocol
func UserUploadBatch(c *gin.Context) {
	var request UserUploadBatchRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, tool.FastReturnError("Invalid JSON request: "+err.Error()))
		return
	}

	// Validate required parameters
	if request.SessionId == "" {
		c.JSON(http.StatusBadRequest, tool.FastReturnError("Missing required parameter: sessionId"))
		return
	}

	if len(request.Files) == 0 {
		c.JSON(http.StatusBadRequest, tool.FastReturnError("No files provided"))
		return
	}

	// Get user upload session information
	sessionInfo := UserUploadSessions.Get(request.SessionId)
	if sessionInfo.SessionId == "" {
		c.JSON(http.StatusNotFound, tool.FastReturnError("Session not found or expired"))
		return
	}

	// Prepare result tracking
	result := UserUploadBatchResult{
		Total:   len(request.Files),
		Success: 0,
		Failed:  0,
		Results: make([]UserUploadItemResult, 0, len(request.Files)),
	}

	// Get target address for upload
	targetAddr := &net.UDPAddr{
		IP:   net.ParseIP(sessionInfo.Target.Ipaddress).To4(),
		Port: sessionInfo.Target.VersionMessage.Port,
	}

	tool.DefaultLogger.Infof("[UploadBatch] Starting batch upload: sessionId=%s, totalFiles=%d",
		request.SessionId, len(request.Files))

	// Process each file
	for _, fileItem := range request.Files {
		itemResult := UserUploadItemResult{
			FileId:  fileItem.FileId,
			Success: false,
		}

		// Validate file parameters
		if fileItem.FileId == "" || fileItem.Token == "" || fileItem.FileUrl == "" {
			itemResult.Error = "Missing required parameters: fileId, token, or fileUrl"
			result.Results = append(result.Results, itemResult)
			result.Failed++
			tool.DefaultLogger.Errorf("[UploadBatch] File %s: missing parameters", fileItem.FileId)
			continue
		}

		// Validate token matches
		expectedToken, ok := sessionInfo.Tokens[fileItem.FileId]
		if !ok || expectedToken != fileItem.Token {
			itemResult.Error = "Invalid file ID or token"
			result.Results = append(result.Results, itemResult)
			result.Failed++
			tool.DefaultLogger.Errorf("[UploadBatch] File %s: invalid token", fileItem.FileId)
			continue
		}

		// Parse and validate file URL
		parsedUrl, err := url.Parse(fileItem.FileUrl)
		if err != nil {
			itemResult.Error = fmt.Sprintf("Invalid fileUrl: %v", err)
			result.Results = append(result.Results, itemResult)
			result.Failed++
			tool.DefaultLogger.Errorf("[UploadBatch] File %s: invalid URL: %v", fileItem.FileId, err)
			continue
		}

		if parsedUrl.Scheme != "file" {
			itemResult.Error = "Only file:// protocol is supported"
			result.Results = append(result.Results, itemResult)
			result.Failed++
			tool.DefaultLogger.Errorf("[UploadBatch] File %s: unsupported protocol: %s", fileItem.FileId, parsedUrl.Scheme)
			continue
		}

		// Extract file path and read file
		filePath := parsedUrl.Path
		tool.DefaultLogger.Infof("[UploadBatch] File %s: reading from %s", fileItem.FileId, filePath)

		fileData, err := os.ReadFile(filePath)
		if err != nil {
			itemResult.Error = fmt.Sprintf("Failed to read file: %v", err)
			result.Results = append(result.Results, itemResult)
			result.Failed++
			tool.DefaultLogger.Errorf("[UploadBatch] File %s: failed to read file: %v", fileItem.FileId, err)
			continue
		}

		if len(fileData) == 0 {
			itemResult.Error = "File data is empty"
			result.Results = append(result.Results, itemResult)
			result.Failed++
			tool.DefaultLogger.Errorf("[UploadBatch] File %s: file is empty", fileItem.FileId)
			continue
		}

		tool.DefaultLogger.Infof("[UploadBatch] File %s: read %d bytes, uploading to %s:%d",
			fileItem.FileId, len(fileData), targetAddr.IP.String(), targetAddr.Port)

		// Upload file to target device
		err = transfer.UploadFile(
			targetAddr,
			&sessionInfo.Target.VersionMessage,
			request.SessionId,
			fileItem.FileId,
			fileItem.Token,
			bytes.NewReader(fileData),
		)

		if err != nil {
			itemResult.Error = fmt.Sprintf("Upload failed: %v", err)
			result.Results = append(result.Results, itemResult)
			result.Failed++
			tool.DefaultLogger.Errorf("[UploadBatch] File %s: upload failed: %v", fileItem.FileId, err)
		} else {
			itemResult.Success = true
			result.Results = append(result.Results, itemResult)
			result.Success++
			tool.DefaultLogger.Infof("[UploadBatch] File %s: uploaded successfully", fileItem.FileId)
		}
	}

	tool.DefaultLogger.Infof("[UploadBatch] Batch upload completed: sessionId=%s, success=%d, failed=%d",
		request.SessionId, result.Success, result.Failed)

	// Return result with appropriate status code
	if result.Failed == result.Total {
		// All files failed
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":  "All files failed to upload",
			"result": result,
		})
	} else if result.Failed > 0 {
		// Some files failed
		c.JSON(http.StatusMultiStatus, gin.H{
			"message": "Batch upload completed with some failures",
			"result":  result,
		})
	} else {
		// All files succeeded
		c.JSON(http.StatusOK, gin.H{
			"message": "All files uploaded successfully",
			"result":  result,
		})
	}
}
