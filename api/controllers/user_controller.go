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
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	ttlworker "github.com/FloatTech/ttl"
	"github.com/gin-gonic/gin"
	"github.com/moyoez/localsend-base-protocol-golang/api/models"
	"github.com/moyoez/localsend-base-protocol-golang/boardcast"
	"github.com/moyoez/localsend-base-protocol-golang/share"
	"github.com/moyoez/localsend-base-protocol-golang/tool"
	"github.com/moyoez/localsend-base-protocol-golang/transfer"
	"github.com/moyoez/localsend-base-protocol-golang/types"
)

// UserPrepareUploadRequest represents the prepare upload request
type UserPrepareUploadRequest struct {
	TargetTo              string                     `json:"targetTo"`                        // Target device identifier
	Files                 map[string]types.FileInput `json:"files,omitempty"`                 // File metadata map, key is fileId (used when useFolderUpload is false)
	UseFolderUpload       bool                       `json:"useFolderUpload,omitempty"`       // If true, prepare upload for all files from FolderPath
	FolderPath            string                     `json:"folderPath,omitempty"`            // Folder path to upload (when useFolderUpload is true)
	UseFastSender         bool                       `json:"useFastSender,omitempty"`         // If true, refer to this,ignore device list check.
	UseFastSenderIPSuffex string                     `json:"useFastSenderIPSuffex,omitempty"` // If true, refer to this,ignore device list check.
	UseFastSenderIp       string                     `json:"useFastSenderIp,omitempty"`       // If true, refer to this,ignore device list check.
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
	SessionId       string               `json:"sessionId"`                 // SessionId returned from prepare-upload
	Files           []UserUploadFileItem `json:"files,omitempty"`           // Array of files to upload (used when useFolderUpload is false)
	UseFolderUpload bool                 `json:"useFolderUpload,omitempty"` // If true, upload all files from FolderPath
	FolderPath      string               `json:"folderPath,omitempty"`      // Folder path to upload (when useFolderUpload is true)
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

// UserFavoritesAddRequest represents the request body for adding a favorite device
type UserFavoritesAddRequest struct {
	Fingerprint string `json:"favorite_fingerprint"` // Required: device fingerprint
	Alias       string `json:"favorite_alias"`       // Optional: device alias for display
}

// CreateShareSessionRequest represents the request body for creating a share session
type CreateShareSessionRequest struct {
	Files      map[string]types.FileInput `json:"files"`         // File metadata map, key is fileId; use fileUrl (file:///path) for local files
	Pin        string                     `json:"pin,omitempty"` // Optional PIN for download access
	AutoAccept bool                       `json:"autoAccept"`    // If true, no confirmation needed when receiver requests download
}

// CreateShareSessionResponse represents the response for create-share-session
type CreateShareSessionResponse struct {
	SessionId   string `json:"sessionId"`
	DownloadUrl string `json:"downloadUrl"`
}

// UserUploadSessionContext holds the context and cancel function for a user upload session
type UserUploadSessionContext struct {
	Ctx    context.Context
	Cancel context.CancelFunc
}

// UserUploadSessions stores user upload sessions using TTL cache (default 30 minutes expiration)
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

// UserGetNetworkInfo returns local network interface information with IP addresses and segment numbers.
// GET /api/self/v1/get-network-info
// Returns: [{ "interface_name": "en0", "ip_address": "192.168.3.12", "number": "#12", "number_int": 12 }, ...]
func UserGetNetworkInfo(c *gin.Context) {
	infos := share.GetSelfNetworkInfos()
	c.JSON(http.StatusOK, tool.FastReturnSuccessWithData(infos))
}

// resolveFastSenderIP resolves the target IP from either a full IP address or an IP suffix.
// Priority: fullIP > ipSuffix
// Returns the resolved IP address or an error.
func resolveFastSenderIP(fullIP, ipSuffix string) (string, error) {
	// If full IP is provided, use it directly
	if fullIP != "" {
		// Validate it's a valid IP
		if ip := net.ParseIP(fullIP); ip != nil {
			return fullIP, nil
		}
		return "", errors.New("invalid IP address format")
	}

	// If IP suffix is provided, resolve it
	if ipSuffix != "" {
		return tool.GetIPFromSuffix(ipSuffix)
	}

	return "", errors.New("either useFastSenderIp or useFastSenderIPSuffex must be provided when useFastSender is true")
}

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

// UserScanNow triggers an immediate device scan based on current configuration.
// GET /api/self/v1/scan-now
// Returns: { "message": "Scan completed", "devices": [...] }
func UserScanNow(c *gin.Context) {
	err := boardcast.ScanNow()
	if err != nil {
		c.JSON(http.StatusInternalServerError, tool.FastReturnError("Scan failed: "+err.Error()))
		return
	}

	// Return the current scan results (same as scan-current)
	keys := share.ListUserScanCurrent()
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

// UserConfirmDownload handles confirm download request
// GET /api/self/v1/confirm-download?sessionId=xxx&confirmed=true|false
func UserConfirmDownload(c *gin.Context) {
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

	confirmCh, ok := models.GetConfirmDownloadChannel(sessionId)
	if !ok {
		c.JSON(http.StatusNotFound, tool.FastReturnError("Session not found or expired"))
		return
	}

	select {
	case confirmCh <- types.ConfirmResult{Confirmed: confirmed}:
		models.DeleteConfirmDownloadChannel(sessionId)
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

	pin := c.Query("pin")
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, tool.FastReturnError("Invalid request body: "+err.Error()))
		return
	}

	var targetItem share.UserScanCurrentItem
	var ok bool

	// Fast sender mode: skip device list check, directly fetch device info
	if request.UseFastSender {
		targetIP, err := resolveFastSenderIP(request.UseFastSenderIp, request.UseFastSenderIPSuffex)
		if err != nil {
			c.JSON(http.StatusBadRequest, tool.FastReturnError("Failed to resolve target IP: "+err.Error()))
			return
		}

		// Default port for LocalSend
		defaultPort := 53317

		tool.DefaultLogger.Infof("[FastSender] Fetching device info from %s:%d", targetIP, defaultPort)

		// Fetch device info from the target
		deviceInfo, protocol, err := transfer.FetchDeviceInfo(targetIP, defaultPort)
		if err != nil {
			c.JSON(http.StatusNotFound, tool.FastReturnError("Failed to fetch device info: "+err.Error()))
			return
		}

		// Construct UserScanCurrentItem from the fetched info
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

		// Optionally cache this device info for future use
		share.SetUserScanCurrent(deviceInfo.Fingerprint, targetItem)
		ok = true
	} else {
		// Normal mode: validate target device exists in scan list
		targetItem, ok = share.GetUserScanCurrent(request.TargetTo)
		if !ok {
			c.JSON(http.StatusNotFound, tool.FastReturnError("Target device not found"))
			return
		}
	}

	// Initialize Files map if nil
	if request.Files == nil {
		request.Files = make(map[string]types.FileInput)
	}

	// Store additional files separately if any were provided
	additionalFiles := make(map[string]types.FileInput)
	maps.Copy(additionalFiles, request.Files)

	// Handle folder upload mode
	if request.UseFolderUpload {
		if request.FolderPath == "" {
			c.JSON(http.StatusBadRequest, tool.FastReturnError("folderPath is required when useFolderUpload is true"))
			return
		}

		tool.DefaultLogger.Infof("[PrepareUpload] Processing folder upload: %s", request.FolderPath)

		// Process the folder and get all files with proper naming
		fileInputMap, _, err := tool.ProcessFolderForUpload(request.FolderPath, false)
		if err != nil {
			c.JSON(http.StatusBadRequest, tool.FastReturnError(fmt.Sprintf("Failed to process folder: %v", err)))
			return
		}

		// Start with folder files
		request.Files = make(map[string]types.FileInput, len(fileInputMap)+len(additionalFiles))
		for fileId, fileInput := range fileInputMap {
			request.Files[fileId] = *fileInput
		}

		tool.DefaultLogger.Infof("[PrepareUpload] Prepared %d files from folder", len(fileInputMap))

		// Merge additional files with folder files
		if len(additionalFiles) > 0 {
			tool.DefaultLogger.Infof("[PrepareUpload] Merging %d additional files with folder files", len(additionalFiles))
			maps.Copy(request.Files, additionalFiles)
		}
	}

	// Process each file input and auto-fill information from fileUrl if provided
	tool.DefaultLogger.Infof("Processing %d total files for prepare-upload", len(request.Files))
	for fileID, fileInput := range request.Files {
		// Only process files that are in additionalFiles (manually provided files)
		// Files from folder processing are already complete
		_, isAdditionalFile := additionalFiles[fileID]
		needsProcessing := !request.UseFolderUpload || isAdditionalFile

		if needsProcessing {
			if err := tool.ProcessFileInput(&fileInput); err != nil {
				c.JSON(http.StatusBadRequest, tool.FastReturnErrorWithData(fmt.Sprintf("Failed to process file %s: %v", fileID, err), map[string]any{
					"fileId": fileID,
				}))
				return
			}
			// Update the map with processed file input
			request.Files[fileID] = fileInput
		}
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
			Protocol:    targetItem.Protocol, // Use target device protocol
			Download:    selfDevice.Download,
		},
		Files: filesMap,
	}

	// Call LocalSend prepare upload endpoint
	targetAddr := &net.UDPAddr{
		IP:   net.ParseIP(targetItem.Ipaddress).To4(),
		Port: targetItem.Port,
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

	// Create session context for cancellation support
	CreateUserUploadSessionContext(prepareResponse.SessionId)

	// Return result
	c.JSON(http.StatusOK, tool.FastReturnSuccessWithData(types.PrepareUploadResponse{
		SessionId: prepareResponse.SessionId,
		Files:     prepareResponse.Files,
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
				tool.DefaultLogger.Debugf("Reading file from local path: %s", filePath)

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
		defer func() {
			if err := c.Request.Body.Close(); err != nil {
				tool.DefaultLogger.Errorf("Failed to close request body: %v", err)
			}
		}()
		fileData = data
	}

	// Validate file data
	if len(fileData) == 0 {
		c.JSON(http.StatusBadRequest, tool.FastReturnError("File data is empty"))
		return
	}

	// Check if session is cancelled
	if IsUserUploadSessionCancelled(sessionId) {
		c.JSON(http.StatusConflict, tool.FastReturnError("Upload session cancelled"))
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

	// Get session context for cancellation support
	ctx := GetUserUploadSessionContext(sessionId)
	if ctx == nil {
		ctx = context.Background()
	}

	// Create reader from file data
	fileReader = bytes.NewReader(fileData)

	// Call LocalSend upload endpoint
	targetAddr := &net.UDPAddr{
		IP:   net.ParseIP(sessionInfo.Target.Ipaddress).To4(),
		Port: sessionInfo.Target.Port,
	}

	tool.DefaultLogger.Infof("Uploading file to %s:%d (sessionId=%s, fileId=%s)",
		targetAddr.IP.String(), targetAddr.Port, sessionId, fileId)

	err := transfer.UploadFileWithContext(
		ctx,
		targetAddr,
		&sessionInfo.Target.VersionMessage,
		sessionId,
		fileId,
		token,
		fileReader,
	)

	if err != nil {
		// Check if it was cancelled
		if ctx.Err() != nil {
			tool.DefaultLogger.Infof("File upload cancelled (sessionId=%s, fileId=%s)", sessionId, fileId)
			c.JSON(http.StatusConflict, tool.FastReturnError("Upload cancelled"))
			return
		}
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

	// Store additional files if any were provided
	additionalFiles := make([]UserUploadFileItem, len(request.Files))
	copy(additionalFiles, request.Files)

	// Handle folder upload mode
	if request.UseFolderUpload {
		if request.FolderPath == "" {
			c.JSON(http.StatusBadRequest, tool.FastReturnError("folderPath is required when useFolderUpload is true"))
			return
		}

		tool.DefaultLogger.Infof("[UploadBatch] Processing folder upload: %s", request.FolderPath)

		// Process the folder and get all files
		_, fileIdToPathMap, err := tool.ProcessFolderForUpload(request.FolderPath, false)
		if err != nil {
			c.JSON(http.StatusBadRequest, tool.FastReturnError(fmt.Sprintf("Failed to process folder: %v", err)))
			return
		}

		// Get user upload session information to get tokens
		sessionInfo := UserUploadSessions.Get(request.SessionId)
		if sessionInfo.SessionId == "" {
			c.JSON(http.StatusNotFound, tool.FastReturnError("Session not found or expired"))
			return
		}

		// Build Files array from the folder contents
		request.Files = make([]UserUploadFileItem, 0, len(fileIdToPathMap)+len(additionalFiles))

		for fileId, filePath := range fileIdToPathMap {
			// Get token for this file from session
			token, ok := sessionInfo.Tokens[fileId]
			if !ok {
				tool.DefaultLogger.Warnf("[UploadBatch] No token found for file %s, skipping", fileId)
				continue
			}

			request.Files = append(request.Files, UserUploadFileItem{
				FileId:  fileId,
				Token:   token,
				FileUrl: "file://" + filePath,
			})
		}

		tool.DefaultLogger.Infof("[UploadBatch] Prepared %d files from folder", len(request.Files))

		// Merge additional files with folder files
		if len(additionalFiles) > 0 {
			tool.DefaultLogger.Infof("[UploadBatch] Merging %d additional files with folder files", len(additionalFiles))
			request.Files = append(request.Files, additionalFiles...)
		}
	}

	if len(request.Files) == 0 {
		c.JSON(http.StatusBadRequest, tool.FastReturnError("No files provided"))
		return
	}

	// Check if session is cancelled
	if IsUserUploadSessionCancelled(request.SessionId) {
		c.JSON(http.StatusConflict, tool.FastReturnError("Upload session cancelled"))
		return
	}

	// Get user upload session information
	sessionInfo := UserUploadSessions.Get(request.SessionId)
	if sessionInfo.SessionId == "" {
		c.JSON(http.StatusNotFound, tool.FastReturnError("Session not found or expired"))
		return
	}

	// Get session context for cancellation support
	ctx := GetUserUploadSessionContext(request.SessionId)
	if ctx == nil {
		ctx = context.Background()
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
		Port: sessionInfo.Target.Port,
	}

	tool.DefaultLogger.Infof("[UploadBatch] Starting batch upload: sessionId=%s, totalFiles=%d",
		request.SessionId, len(request.Files))

	// Process each file
	for _, fileItem := range request.Files {
		// Check if cancelled before each file
		select {
		case <-ctx.Done():
			tool.DefaultLogger.Infof("[UploadBatch] Batch upload cancelled: sessionId=%s", request.SessionId)
			// Mark remaining files as cancelled
			itemResult := UserUploadItemResult{
				FileId:  fileItem.FileId,
				Success: false,
				Error:   "Upload cancelled",
			}
			result.Results = append(result.Results, itemResult)
			result.Failed++
			// Break out of the loop and return partial results
			goto batchComplete
		default:
		}
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

		// Upload file to target device with context support
		err = transfer.UploadFileWithContext(
			ctx,
			targetAddr,
			&sessionInfo.Target.VersionMessage,
			request.SessionId,
			fileItem.FileId,
			fileItem.Token,
			bytes.NewReader(fileData),
		)

		if err != nil {
			// Check if it was cancelled
			if ctx.Err() != nil {
				itemResult.Error = "Upload cancelled"
				result.Results = append(result.Results, itemResult)
				result.Failed++
				tool.DefaultLogger.Infof("[UploadBatch] File %s: upload cancelled", fileItem.FileId)
				goto batchComplete
			}
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

batchComplete:
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

// UserCancelUpload handles cancel upload request (sender side)
// POST /api/self/v1/cancel-upload
// Cancels an ongoing upload session and interrupts all uploads
func UserCancelUpload(c *gin.Context) {
	sessionId := strings.TrimSpace(c.Query("sessionId"))
	if sessionId == "" {
		c.JSON(http.StatusBadRequest, tool.FastReturnError("Missing required parameter: sessionId"))
		return
	}

	// Check if session exists
	sessionInfo := UserUploadSessions.Get(sessionId)
	if sessionInfo.SessionId == "" {
		c.JSON(http.StatusNotFound, tool.FastReturnError("Session not found or expired"))
		return
	}

	tool.DefaultLogger.Infof("[CancelUpload] Cancelling upload session: sessionId=%s", sessionId)

	// Cancel the session (this will interrupt any ongoing uploads)
	CancelUserUploadSession(sessionId)

	// Also notify the target device about the cancellation
	targetAddr := &net.UDPAddr{
		IP:   net.ParseIP(sessionInfo.Target.Ipaddress).To4(),
		Port: sessionInfo.Target.Port,
	}

	// Send cancel request to target device
	if err := transfer.CancelSession(targetAddr, &sessionInfo.Target.VersionMessage, sessionId); err != nil {
		tool.DefaultLogger.Warnf("[CancelUpload] Failed to send cancel request to target: %v", err)
		// Still return success since local cancellation succeeded
	} else {
		tool.DefaultLogger.Infof("[CancelUpload] Cancel request sent to target device")
	}

	tool.DefaultLogger.Infof("[CancelUpload] Upload session cancelled: sessionId=%s", sessionId)
	c.JSON(http.StatusOK, tool.FastReturnSuccess())
}

func UserGetImage(c *gin.Context) {
	fileName := c.Query("fileName")
	if fileName == "" {
		c.JSON(http.StatusBadRequest, tool.FastReturnError("Missing required parameter: fileName"))
		return
	}

	// using file:// protocol
	if strings.HasPrefix(fileName, "file://") {
		parsedURL, err := url.Parse(fileName)
		if err != nil {
			c.JSON(http.StatusBadRequest, tool.FastReturnError("Invalid file URI: "+err.Error()))
			return
		}
		fileName = parsedURL.Path
	}

	// must be .jpg
	if !strings.HasSuffix(strings.ToLower(fileName), ".jpg") {
		c.JSON(http.StatusForbidden, tool.FastReturnError("Only .jpg files are allowed"))
		return
	}

	if strings.HasPrefix(fileName, "~") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			c.JSON(http.StatusInternalServerError, tool.FastReturnError("Failed to get home directory: "+err.Error()))
			return
		}
		fileName = filepath.Join(homeDir, fileName[1:])
	}

	cleanPath := filepath.Clean(fileName)
	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, tool.FastReturnError("Invalid file path: "+err.Error()))
		return
	}

	steamPathPattern := filepath.Join(".local", "share", "Steam", "userdata")

	// find .local/share/Steam/userdata position in path
	steamIndex := strings.Index(absPath, steamPathPattern)
	if steamIndex == -1 {
		c.JSON(http.StatusForbidden, tool.FastReturnError("Access to this path is forbidden: must be in Steam userdata directory"))
		return
	}

	// extract path after userdata
	userdataPath := absPath[steamIndex+len(steamPathPattern):]
	if !strings.HasPrefix(userdataPath, string(filepath.Separator)) {
		c.JSON(http.StatusForbidden, tool.FastReturnError("Invalid path structure"))
		return
	}

	// remove leading separator
	userdataPath = strings.TrimPrefix(userdataPath, string(filepath.Separator))

	// validate path structure: [userid]/760/remote/[gameid]/screenshots/[filename].jpg
	parts := strings.Split(userdataPath, string(filepath.Separator))
	if len(parts) < 6 {
		c.JSON(http.StatusForbidden, tool.FastReturnError("Invalid path structure: insufficient path depth"))
		return
	}

	if parts[1] != "760" || parts[2] != "remote" || parts[4] != "screenshots" {
		c.JSON(http.StatusForbidden, tool.FastReturnError("Invalid path structure: must follow userdata/*/760/remote/*/screenshots/*.jpg"))
		return
	}

	// read image file
	image, err := os.ReadFile(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, tool.FastReturnError("Image not found"))
			return
		}
		c.JSON(http.StatusInternalServerError, tool.FastReturnError("Failed to read image: "+err.Error()))
		return
	}

	c.Data(http.StatusOK, "image/jpeg", image)
}

// UserFavoritesList returns the list of favorite devices.
// GET /api/self/v1/favorites
func UserFavoritesList(c *gin.Context) {
	favorites := tool.ListFavorites()
	c.JSON(http.StatusOK, tool.FastReturnSuccessWithData(favorites))
}

// UserFavoritesAdd adds a device to favorites.
// POST /api/self/v1/favorites
// Body: { "fingerprint": "...", "alias": "..." }
func UserFavoritesAdd(c *gin.Context) {
	var request UserFavoritesAddRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, tool.FastReturnError("Invalid request body: "+err.Error()))
		return
	}

	if request.Fingerprint == "" {
		c.JSON(http.StatusBadRequest, tool.FastReturnError("fingerprint is required"))
		return
	}

	if err := tool.AddFavorite(request.Fingerprint, request.Alias); err != nil {
		c.JSON(http.StatusInternalServerError, tool.FastReturnError("Failed to add favorite: "+err.Error()))
		return
	}

	c.JSON(http.StatusOK, tool.FastReturnSuccess())
}

// UserFavoritesDelete removes a device from favorites.
// DELETE /api/self/v1/favorites/:fingerprint
func UserFavoritesDelete(c *gin.Context) {
	fingerprint := c.Param("fingerprint")
	if fingerprint == "" {
		c.JSON(http.StatusBadRequest, tool.FastReturnError("fingerprint is required"))
		return
	}

	if err := tool.RemoveFavorite(fingerprint); err != nil {
		c.JSON(http.StatusInternalServerError, tool.FastReturnError("Failed to remove favorite: "+err.Error()))
		return
	}

	c.JSON(http.StatusOK, tool.FastReturnSuccess())
}

// UserGetNetworkInterfaces returns the list of network interfaces.
// GET /api/self/v1/get-network-interfaces
// be aware default set to *(Means all)
func UserGetNetworkInterfaces(c *gin.Context) {
	c.JSON(http.StatusOK, tool.FastReturnSuccessWithData(share.GetSelfNetworkInfos()))
}

// UserCreateShareSession creates a share session for the download API
// POST /api/self/v1/create-share-session
// Request body: { "files": { fileId: { id, fileName, size, fileType, fileUrl } }, "pin": "optional", "autoAccept": true/false }
func UserCreateShareSession(c *gin.Context) {
	var request CreateShareSessionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, tool.FastReturnError("Invalid request body: "+err.Error()))
		return
	}

	if len(request.Files) == 0 {
		c.JSON(http.StatusBadRequest, tool.FastReturnError("files is required and must not be empty"))
		return
	}

	// Process each file or folder: resolve fileUrl to path, get file info (folders are expanded to all files inside)
	files := make(map[string]models.ShareFileEntry)
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

		// Verify path exists
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
			// Folder: expand to all files inside and add each as a separate entry
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
				files[id] = models.ShareFileEntry{
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

		// Single file: fill metadata from path and add one entry
		if err := tool.ProcessFileInput(&input); err != nil {
			c.JSON(http.StatusBadRequest, tool.FastReturnError(fmt.Sprintf("Invalid file %s: %v", fileId, err)))
			return
		}
		fileIdVal := input.ID
		if fileIdVal == "" {
			fileIdVal = fileId
		}
		files[fileId] = models.ShareFileEntry{
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

	sessionId := tool.GenerateRandomUUID()
	session := &models.ShareSession{
		SessionId:  sessionId,
		Files:      files,
		CreatedAt:  time.Now(),
		Pin:        request.Pin,
		AutoAccept: request.AutoAccept,
	}
	models.CacheShareSession(session)

	// Build download URL: protocol://ip:port/?session=sessionId
	cfg := tool.GetCurrentConfig()
	protocol := cfg.Protocol
	port := cfg.Port
	host := "localhost"
	if infos := share.GetSelfNetworkInfos(); len(infos) > 0 {
		host = infos[0].IPAddress
	}
	downloadUrl := fmt.Sprintf("%s://%s:%d/?session=%s", protocol, host, port, sessionId)

	tool.DefaultLogger.Infof("[CreateShareSession] Created session %s with %d files, pin=%v, autoAccept=%v",
		sessionId, len(files), request.Pin != "", request.AutoAccept)

	c.JSON(http.StatusOK, tool.FastReturnSuccessWithData(CreateShareSessionResponse{
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
	tool.DefaultLogger.Infof("[CloseShareSession] Closed session %s", sessionId)
	c.JSON(http.StatusOK, tool.FastReturnSuccess())
}
