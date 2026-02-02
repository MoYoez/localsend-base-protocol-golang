package controllers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
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
	TargetTo              string                     `json:"targetTo"`
	Files                 map[string]types.FileInput `json:"files,omitempty"`
	UseFolderUpload       bool                       `json:"useFolderUpload,omitempty"`
	FolderPath            string                     `json:"folderPath,omitempty"`
	UseFastSender         bool                       `json:"useFastSender,omitempty"`
	UseFastSenderIPSuffex string                     `json:"useFastSenderIPSuffex,omitempty"`
	UseFastSenderIp       string                     `json:"useFastSenderIp,omitempty"`
}

// UserUploadRequest represents the actual upload request
type UserUploadRequest struct {
	SessionId string `json:"sessionId"`
	FileId    string `json:"fileId"`
	Token     string `json:"token"`
	FileUrl   string `json:"fileUrl"`
}

// UserUploadBatchRequest represents batch upload request
type UserUploadBatchRequest struct {
	SessionId       string               `json:"sessionId"`
	Files           []UserUploadFileItem `json:"files,omitempty"`
	UseFolderUpload bool                 `json:"useFolderUpload,omitempty"`
	FolderPath      string               `json:"folderPath,omitempty"`
}

// UserUploadFileItem represents a single file in batch upload
type UserUploadFileItem struct {
	FileId  string `json:"fileId"`
	Token   string `json:"token"`
	FileUrl string `json:"fileUrl"`
}

// UserUploadBatchResult represents the result of a batch upload operation
type UserUploadBatchResult struct {
	Total   int                    `json:"total"`
	Success int                    `json:"success"`
	Failed  int                    `json:"failed"`
	Results []UserUploadItemResult `json:"results"`
}

// UserUploadItemResult represents the result of a single file upload
type UserUploadItemResult struct {
	FileId  string `json:"fileId"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// UserUploadSession stores user upload session information
type UserUploadSession struct {
	Target    share.UserScanCurrentItem
	SessionId string
	Tokens    map[string]string
}

// UserUploadSessionContext holds the context and cancel function for a user upload session
type UserUploadSessionContext struct {
	Ctx    context.Context
	Cancel context.CancelFunc
}

var (
	UserUploadSessionTTL      = 60 * time.Minute
	UserUploadSessions        = ttlworker.NewCache[string, UserUploadSession](UserUploadSessionTTL)
	userUploadSessionContexts = ttlworker.NewCache[string, *UserUploadSessionContext](UserUploadSessionTTL)
	userUploadSessionMu       sync.RWMutex
)

// CreateUserUploadSessionContext creates a new context for the user upload session
func CreateUserUploadSessionContext(sessionId string) context.Context {
	userUploadSessionMu.Lock()
	defer userUploadSessionMu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	userUploadSessionContexts.Set(sessionId, &UserUploadSessionContext{
		Ctx:    ctx,
		Cancel: cancel,
	})
	return ctx
}

// GetUserUploadSessionContext returns the context for the user upload session
func GetUserUploadSessionContext(sessionId string) context.Context {
	userUploadSessionMu.RLock()
	defer userUploadSessionMu.RUnlock()
	sessCtx := userUploadSessionContexts.Get(sessionId)
	if sessCtx == nil {
		return nil
	}
	return sessCtx.Ctx
}

// CancelUserUploadSession cancels the user upload session and removes it
func CancelUserUploadSession(sessionId string) {
	userUploadSessionMu.Lock()
	defer userUploadSessionMu.Unlock()
	if sessCtx := userUploadSessionContexts.Get(sessionId); sessCtx != nil {
		sessCtx.Cancel()
		userUploadSessionContexts.Delete(sessionId)
	}
	UserUploadSessions.Delete(sessionId)
}

// IsUserUploadSessionCancelled checks if the user upload session has been cancelled
func IsUserUploadSessionCancelled(sessionId string) bool {
	ctx := GetUserUploadSessionContext(sessionId)
	if ctx == nil {
		return true
	}
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

func resolveFastSenderIP(fullIP, ipSuffix string) (string, error) {
	if fullIP != "" {
		if ip := net.ParseIP(fullIP); ip != nil {
			return fullIP, nil
		}
		return "", errors.New("invalid IP address format")
	}
	if ipSuffix != "" {
		return tool.GetIPFromSuffix(ipSuffix)
	}
	return "", errors.New("either useFastSenderIp or useFastSenderIPSuffex must be provided when useFastSender is true")
}

// UserPrepareUpload handles prepare upload request
// POST /api/self/v1/prepare-upload
func UserPrepareUpload(c *gin.Context) {
	var request UserPrepareUploadRequest
	pin := c.Query("pin")
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, tool.FastReturnError("Invalid request body: "+err.Error()))
		return
	}

	var targetItem share.UserScanCurrentItem
	var ok bool

	if request.UseFastSender {
		targetIP, err := resolveFastSenderIP(request.UseFastSenderIp, request.UseFastSenderIPSuffex)
		if err != nil {
			c.JSON(http.StatusBadRequest, tool.FastReturnError("Failed to resolve target IP: "+err.Error()))
			return
		}
		defaultPort := 53317
		tool.DefaultLogger.Infof("[FastSender] Fetching device info from %s:%d", targetIP, defaultPort)
		deviceInfo, protocol, err := transfer.FetchDeviceInfo(targetIP, defaultPort)
		if err != nil {
			c.JSON(http.StatusNotFound, tool.FastReturnError("Failed to fetch device info: "+err.Error()))
			return
		}
		targetItem = share.UserScanCurrentItem{
			Ipaddress: targetIP,
			VersionMessage: types.VersionMessage{
				Alias:       deviceInfo.Alias,
				Version:     deviceInfo.Version,
				DeviceModel: deviceInfo.DeviceModel,
				DeviceType:  deviceInfo.DeviceType,
				Fingerprint: deviceInfo.Fingerprint,
				Port:        defaultPort,
				Protocol:    protocol,
				Download:    deviceInfo.Download,
				Announce:    true,
			},
		}
		tool.DefaultLogger.Infof("[FastSender] Successfully fetched device info: %s (fingerprint: %s) at %s",
			deviceInfo.Alias, deviceInfo.Fingerprint, targetIP)
		share.SetUserScanCurrent(deviceInfo.Fingerprint, targetItem)
	} else {
		targetItem, ok = share.GetUserScanCurrent(request.TargetTo)
		if !ok {
			c.JSON(http.StatusNotFound, tool.FastReturnError("Target device not found"))
			return
		}
	}

	if request.Files == nil {
		request.Files = make(map[string]types.FileInput)
	}
	additionalFiles := make(map[string]types.FileInput)
	maps.Copy(additionalFiles, request.Files)

	if request.UseFolderUpload {
		if request.FolderPath == "" {
			c.JSON(http.StatusBadRequest, tool.FastReturnError("folderPath is required when useFolderUpload is true"))
			return
		}
		tool.DefaultLogger.Infof("[PrepareUpload] Processing folder upload: %s", request.FolderPath)
		fileInputMap, _, err := tool.ProcessFolderForUpload(request.FolderPath, false)
		if err != nil {
			c.JSON(http.StatusBadRequest, tool.FastReturnError(fmt.Sprintf("Failed to process folder: %v", err)))
			return
		}
		request.Files = make(map[string]types.FileInput, len(fileInputMap)+len(additionalFiles))
		for fileId, fileInput := range fileInputMap {
			request.Files[fileId] = *fileInput
		}
		tool.DefaultLogger.Infof("[PrepareUpload] Prepared %d files from folder", len(fileInputMap))
		if len(additionalFiles) > 0 {
			maps.Copy(request.Files, additionalFiles)
		}
	}

	tool.DefaultLogger.Infof("Processing %d total files for prepare-upload", len(request.Files))
	for fileID, fileInput := range request.Files {
		_, isAdditionalFile := additionalFiles[fileID]
		needsProcessing := !request.UseFolderUpload || isAdditionalFile
		if needsProcessing {
			if err := tool.ProcessFileInput(&fileInput); err != nil {
				c.JSON(http.StatusBadRequest, tool.FastReturnErrorWithData(fmt.Sprintf("Failed to process file %s: %v", fileID, err), map[string]any{"fileId": fileID}))
				return
			}
			request.Files[fileID] = fileInput
		}
	}

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

	selfDevice := models.GetSelfDevice()
	if selfDevice == nil {
		c.JSON(http.StatusInternalServerError, tool.FastReturnError("Local device information not configured"))
		return
	}

	prepareRequest := &types.PrepareUploadRequest{
		Info: types.DeviceInfo{
			Alias:       selfDevice.Alias,
			Version:     selfDevice.Version,
			DeviceModel: selfDevice.DeviceModel,
			DeviceType:  selfDevice.DeviceType,
			Fingerprint: selfDevice.Fingerprint,
			Port:        selfDevice.Port,
			Protocol:    targetItem.Protocol,
			Download:    selfDevice.Download,
		},
		Files: filesMap,
	}

	targetAddr := &net.UDPAddr{
		IP:   net.ParseIP(targetItem.Ipaddress).To4(),
		Port: targetItem.Port,
	}

	prepareResponse, err := transfer.ReadyToUploadTo(targetAddr, &targetItem.VersionMessage, prepareRequest, pin)
	if err != nil {
		errorMsg := err.Error()
		if strings.Contains(errorMsg, "prepare-upload request rejected") {
			c.JSON(http.StatusForbidden, tool.FastReturnError("Upload request rejected"))
			return
		}
		errorMsgLower := strings.ToLower(errorMsg)
		if strings.Contains(errorMsgLower, "pin required") || strings.Contains(errorMsgLower, "invalid pin") {
			c.JSON(http.StatusUnauthorized, tool.FastReturnError("PIN required / Invalid PIN"))
			return
		}
		c.JSON(http.StatusInternalServerError, tool.FastReturnError("Prepare upload failed: "+errorMsg))
		return
	}

	if prepareResponse == nil {
		c.Status(http.StatusNoContent)
		return
	}

	sessionInfo := UserUploadSession{
		Target:    targetItem,
		SessionId: prepareResponse.SessionId,
		Tokens:    prepareResponse.Files,
	}
	UserUploadSessions.Set(prepareResponse.SessionId, sessionInfo)
	CreateUserUploadSessionContext(prepareResponse.SessionId)

	c.JSON(http.StatusOK, tool.FastReturnSuccessWithData(types.PrepareUploadResponse{
		SessionId: prepareResponse.SessionId,
		Files:     prepareResponse.Files,
	}))
}

// UserUpload handles actual file upload request
// POST /api/self/v1/upload
func UserUpload(c *gin.Context) {
	var sessionId, fileId, token string
	var fileReader io.Reader
	var fileData []byte
	contentType := c.GetHeader("Content-Type")

	if strings.Contains(contentType, "application/json") {
		var request UserUploadRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, tool.FastReturnError("Invalid JSON request: "+err.Error()))
			return
		}
		sessionId = request.SessionId
		fileId = request.FileId
		token = request.Token
		fileUrl := request.FileUrl
		if sessionId == "" || fileId == "" || token == "" {
			c.JSON(http.StatusBadRequest, tool.FastReturnError("Missing required parameters: sessionId, fileId, token"))
			return
		}
		if fileUrl != "" {
			parsedUrl, err := url.Parse(fileUrl)
			if err != nil {
				c.JSON(http.StatusBadRequest, tool.FastReturnError("Invalid fileUrl: "+err.Error()))
				return
			}
			if parsedUrl.Scheme == "file" {
				filePath := parsedUrl.Path
				data, err := os.ReadFile(filePath)
				if err != nil {
					c.JSON(http.StatusBadRequest, tool.FastReturnErrorWithData(fmt.Sprintf("Failed to read file from %s: %v", filePath, err), map[string]any{"filePath": filePath}))
					return
				}
				fileData = data
			} else {
				c.JSON(http.StatusBadRequest, tool.FastReturnError("Only file:// protocol is supported for fileUrl"))
				return
			}
		} else {
			c.JSON(http.StatusBadRequest, tool.FastReturnError("fileUrl is required in JSON request"))
			return
		}
	} else {
		sessionId = c.Query("sessionId")
		fileId = c.Query("fileId")
		token = c.Query("token")
		if sessionId == "" || fileId == "" || token == "" {
			c.JSON(http.StatusBadRequest, tool.FastReturnError("Missing required query parameters: sessionId, fileId, token"))
			return
		}
		data, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, tool.FastReturnError("Failed to read file data: "+err.Error()))
			return
		}
		defer c.Request.Body.Close()
		fileData = data
	}

	if len(fileData) == 0 {
		c.JSON(http.StatusBadRequest, tool.FastReturnError("File data is empty"))
		return
	}
	if IsUserUploadSessionCancelled(sessionId) {
		c.JSON(http.StatusConflict, tool.FastReturnError("Upload session cancelled"))
		return
	}
	sessionInfo := UserUploadSessions.Get(sessionId)
	if sessionInfo.SessionId == "" {
		c.JSON(http.StatusNotFound, tool.FastReturnError("Session not found or expired"))
		return
	}
	expectedToken, ok := sessionInfo.Tokens[fileId]
	if !ok || expectedToken != token {
		c.JSON(http.StatusForbidden, tool.FastReturnError("Invalid file ID or token"))
		return
	}
	ctx := GetUserUploadSessionContext(sessionId)
	if ctx == nil {
		ctx = context.Background()
	}
	fileReader = bytes.NewReader(fileData)
	targetAddr := &net.UDPAddr{
		IP:   net.ParseIP(sessionInfo.Target.Ipaddress).To4(),
		Port: sessionInfo.Target.Port,
	}
	err := transfer.UploadFileWithContext(ctx, targetAddr, &sessionInfo.Target.VersionMessage, sessionId, fileId, token, fileReader)
	if err != nil {
		if ctx.Err() != nil {
			c.JSON(http.StatusConflict, tool.FastReturnError("Upload cancelled"))
			return
		}
		c.JSON(http.StatusInternalServerError, tool.FastReturnError("File upload failed: "+err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "File uploaded successfully"})
}

// UserUploadBatch handles batch file upload request
// POST /api/self/v1/upload-batch
func UserUploadBatch(c *gin.Context) {
	var request UserUploadBatchRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, tool.FastReturnError("Invalid JSON request: "+err.Error()))
		return
	}
	if request.SessionId == "" {
		c.JSON(http.StatusBadRequest, tool.FastReturnError("Missing required parameter: sessionId"))
		return
	}
	additionalFiles := make([]UserUploadFileItem, len(request.Files))
	copy(additionalFiles, request.Files)

	if request.UseFolderUpload {
		if request.FolderPath == "" {
			c.JSON(http.StatusBadRequest, tool.FastReturnError("folderPath is required when useFolderUpload is true"))
			return
		}
		_, fileIdToPathMap, err := tool.ProcessFolderForUpload(request.FolderPath, false)
		if err != nil {
			c.JSON(http.StatusBadRequest, tool.FastReturnError(fmt.Sprintf("Failed to process folder: %v", err)))
			return
		}
		sessionInfo := UserUploadSessions.Get(request.SessionId)
		if sessionInfo.SessionId == "" {
			c.JSON(http.StatusNotFound, tool.FastReturnError("Session not found or expired"))
			return
		}
		request.Files = make([]UserUploadFileItem, 0, len(fileIdToPathMap)+len(additionalFiles))
		for fileId, filePath := range fileIdToPathMap {
			token, ok := sessionInfo.Tokens[fileId]
			if !ok {
				continue
			}
			request.Files = append(request.Files, UserUploadFileItem{
				FileId:  fileId,
				Token:   token,
				FileUrl: "file://" + filePath,
			})
		}
		if len(additionalFiles) > 0 {
			request.Files = append(request.Files, additionalFiles...)
		}
	}

	if len(request.Files) == 0 {
		c.JSON(http.StatusBadRequest, tool.FastReturnError("No files provided"))
		return
	}
	if IsUserUploadSessionCancelled(request.SessionId) {
		c.JSON(http.StatusConflict, tool.FastReturnError("Upload session cancelled"))
		return
	}
	sessionInfo := UserUploadSessions.Get(request.SessionId)
	if sessionInfo.SessionId == "" {
		c.JSON(http.StatusNotFound, tool.FastReturnError("Session not found or expired"))
		return
	}
	ctx := GetUserUploadSessionContext(request.SessionId)
	if ctx == nil {
		ctx = context.Background()
	}

	result := UserUploadBatchResult{
		Total:   len(request.Files),
		Success: 0,
		Failed:  0,
		Results: make([]UserUploadItemResult, 0, len(request.Files)),
	}
	targetAddr := &net.UDPAddr{
		IP:   net.ParseIP(sessionInfo.Target.Ipaddress).To4(),
		Port: sessionInfo.Target.Port,
	}

	for _, fileItem := range request.Files {
		select {
		case <-ctx.Done():
			result.Results = append(result.Results, UserUploadItemResult{FileId: fileItem.FileId, Success: false, Error: "Upload cancelled"})
			result.Failed++
			goto batchComplete
		default:
		}
		itemResult := UserUploadItemResult{FileId: fileItem.FileId, Success: false}
		if fileItem.FileId == "" || fileItem.Token == "" || fileItem.FileUrl == "" {
			itemResult.Error = "Missing required parameters: fileId, token, or fileUrl"
			result.Results = append(result.Results, itemResult)
			result.Failed++
			continue
		}
		expectedToken, ok := sessionInfo.Tokens[fileItem.FileId]
		if !ok || expectedToken != fileItem.Token {
			itemResult.Error = "Invalid file ID or token"
			result.Results = append(result.Results, itemResult)
			result.Failed++
			continue
		}
		parsedUrl, err := url.Parse(fileItem.FileUrl)
		if err != nil {
			itemResult.Error = fmt.Sprintf("Invalid fileUrl: %v", err)
			result.Results = append(result.Results, itemResult)
			result.Failed++
			continue
		}
		if parsedUrl.Scheme != "file" {
			itemResult.Error = "Only file:// protocol is supported"
			result.Results = append(result.Results, itemResult)
			result.Failed++
			continue
		}
		filePath := parsedUrl.Path
		fileData, err := os.ReadFile(filePath)
		if err != nil {
			itemResult.Error = fmt.Sprintf("Failed to read file: %v", err)
			result.Results = append(result.Results, itemResult)
			result.Failed++
			continue
		}
		if len(fileData) == 0 {
			itemResult.Error = "File data is empty"
			result.Results = append(result.Results, itemResult)
			result.Failed++
			continue
		}
		err = transfer.UploadFileWithContext(ctx, targetAddr, &sessionInfo.Target.VersionMessage, request.SessionId, fileItem.FileId, fileItem.Token, bytes.NewReader(fileData))
		if err != nil {
			if ctx.Err() != nil {
				itemResult.Error = "Upload cancelled"
				result.Results = append(result.Results, itemResult)
				result.Failed++
				goto batchComplete
			}
			itemResult.Error = fmt.Sprintf("Upload failed: %v", err)
			result.Results = append(result.Results, itemResult)
			result.Failed++
		} else {
			itemResult.Success = true
			result.Results = append(result.Results, itemResult)
			result.Success++
		}
	}
batchComplete:
	if result.Failed == result.Total {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "All files failed to upload", "result": result})
	} else if result.Failed > 0 {
		c.JSON(http.StatusMultiStatus, gin.H{"message": "Batch upload completed with some failures", "result": result})
	} else {
		c.JSON(http.StatusOK, gin.H{"message": "All files uploaded successfully", "result": result})
	}
}

// UserCancelUpload handles cancel upload request (sender side)
// POST /api/self/v1/cancel-upload
func UserCancelUpload(c *gin.Context) {
	sessionId := strings.TrimSpace(c.Query("sessionId"))
	if sessionId == "" {
		c.JSON(http.StatusBadRequest, tool.FastReturnError("Missing required parameter: sessionId"))
		return
	}
	sessionInfo := UserUploadSessions.Get(sessionId)
	if sessionInfo.SessionId == "" {
		c.JSON(http.StatusNotFound, tool.FastReturnError("Session not found or expired"))
		return
	}
	CancelUserUploadSession(sessionId)
	targetAddr := &net.UDPAddr{
		IP:   net.ParseIP(sessionInfo.Target.Ipaddress).To4(),
		Port: sessionInfo.Target.Port,
	}
	if err := transfer.CancelSession(targetAddr, &sessionInfo.Target.VersionMessage, sessionId); err != nil {
		tool.DefaultLogger.Warnf("[CancelUpload] Failed to send cancel request to target: %v", err)
	}
	c.JSON(http.StatusOK, tool.FastReturnSuccess())
}
