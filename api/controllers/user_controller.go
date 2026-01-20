package controllers

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	ttlworker "github.com/FloatTech/ttl"
	"github.com/gin-gonic/gin"
	"github.com/moyoez/localsend-base-protocol-golang/api/models"
	"github.com/moyoez/localsend-base-protocol-golang/share"
	"github.com/moyoez/localsend-base-protocol-golang/transfer"
	"github.com/moyoez/localsend-base-protocol-golang/types"
)

// UserPrepareUploadRequest represents the prepare upload request
type UserPrepareUploadRequest struct {
	TargetTo string               `json:"targetTo"` // Target device identifier
	Files    map[string]FileInput `json:"files"`    // File metadata map, key is fileId
}

// FileInput represents file input information
type FileInput struct {
	ID       string `json:"id"`                // File ID
	FileName string `json:"fileName"`          // File name
	Size     int64  `json:"size"`              // File size in bytes
	FileType string `json:"fileType"`          // File type, e.g., "image/jpeg"
	SHA256   string `json:"sha256,omitempty"`  // SHA256 hash value (optional)
	Preview  string `json:"preview,omitempty"` // Preview data (optional)
}

// UserUploadRequest represents the actual upload request
type UserUploadRequest struct {
	SessionId string `json:"sessionId"` // SessionId returned from prepare-upload
	FileId    string `json:"fileId"`    // File ID
	Token     string `json:"token"`     // File token
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
	c.JSON(http.StatusOK, gin.H{"data": values})
}

// UserPrepareUpload handles prepare upload request
// POST /api/self/v1/prepare-upload
// Receives file metadata, sends prepare upload request to target device, returns sessionId and tokens
func UserPrepareUpload(c *gin.Context) {
	var request UserPrepareUploadRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body: " + err.Error()})
		return
	}

	// Validate target device exists
	targetItem, ok := share.GetUserScanCurrent(request.TargetTo)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "Target device not found"})
		return
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Local device information not configured"})
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
		"", // PIN, can be added if needed
	)

	if err != nil {
		errorMsg := err.Error()
		// Check if it's a PIN-related error
		errorMsgLower := strings.ToLower(errorMsg)
		if strings.Contains(errorMsgLower, "pin required") || strings.Contains(errorMsgLower, "invalid pin") {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "PIN required / Invalid PIN"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Prepare upload failed: " + errorMsg})
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
	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"sessionId": prepareResponse.SessionId,
			"files":     prepareResponse.Files,
		},
	})
}

// UserUpload handles actual file upload request
// POST /api/self/v1/upload?sessionId=xxx&fileId=xxx&token=xxx
// Request body contains binary file data (complies with LocalSend protocol specification)
// Receives file data and sessionId/fileId/token, sends file to target device
func UserUpload(c *gin.Context) {
	// Get required parameters from query params (complies with LocalSend protocol)
	sessionId := c.Query("sessionId")
	fileId := c.Query("fileId")
	token := c.Query("token")

	// Validate required parameters
	if sessionId == "" || fileId == "" || token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing required query parameters: sessionId, fileId, token"})
		return
	}

	// Get user upload session information
	sessionInfo := UserUploadSessions.Get(sessionId)
	if sessionInfo.SessionId == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "Session not found or expired"})
		return
	}

	// Validate token matches
	expectedToken, ok := sessionInfo.Tokens[fileId]
	if !ok || expectedToken != token {
		c.JSON(http.StatusForbidden, gin.H{"error": "Invalid file ID or token"})
		return
	}

	// Read file data from request body (binary data)
	fileData, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read file data: " + err.Error()})
		return
	}
	defer c.Request.Body.Close()

	if len(fileData) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File data is empty"})
		return
	}

	// Call LocalSend upload endpoint
	targetAddr := &net.UDPAddr{
		IP:   net.ParseIP(sessionInfo.Target.Ipaddress).To4(),
		Port: sessionInfo.Target.VersionMessage.Port,
	}

	err = transfer.UploadFile(
		targetAddr,
		&sessionInfo.Target.VersionMessage,
		sessionId,
		fileId,
		token,
		bytes.NewReader(fileData),
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "File upload failed: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "File uploaded successfully"})
}
