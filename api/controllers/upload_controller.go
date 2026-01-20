package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/moyoez/localsend-base-protocol-golang/api/models"
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
		return
	}

	request, err := models.ParsePrepareUploadRequest(body)
	if err != nil {
		tool.DefaultLogger.Errorf("Failed to parse prepare-upload request: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	tool.DefaultLogger.Debugf("Received prepare-upload request from %s (pin: %s)", request.Info.Alias, pin)

	var response *types.PrepareUploadResponse
	if ctrl.handler != nil {
		var callbackErr error
		response, callbackErr = ctrl.handler.OnPrepareUpload(request, pin)
		if callbackErr != nil {
			tool.DefaultLogger.Errorf("Prepare-upload callback error: %v", callbackErr)
			errorMsg := callbackErr.Error()

			switch errorMsg {
			case "PIN required", "Invalid PIN", "pin required", "invalid pin":
				// Return standardized error message
				if errorMsg == "pin required" {
					errorMsg = "PIN required"
				} else if errorMsg == "invalid pin" {
					errorMsg = "Invalid PIN"
				}
				c.JSON(http.StatusUnauthorized, gin.H{"error": errorMsg})
				return
			case "rejected":
				c.JSON(http.StatusForbidden, gin.H{"error": errorMsg})
				return
			case "blocked by another session":
				c.JSON(http.StatusConflict, gin.H{"error": errorMsg})
				return
			case "too many requests":
				c.JSON(http.StatusTooManyRequests, gin.H{"error": errorMsg})
				return
			default:
				c.JSON(http.StatusInternalServerError, gin.H{"error": errorMsg})
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

	c.JSON(http.StatusOK, response)
}

func (ctrl *UploadController) HandleUpload(c *gin.Context) {
	sessionId := c.Query("sessionId")
	fileId := c.Query("fileId")
	token := c.Query("token")

	if sessionId == "" || fileId == "" || token == "" {
		tool.DefaultLogger.Errorf("Missing required parameters: sessionId=%s, fileId=%s, token=%s", sessionId, fileId, token)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing parameters"})
		return
	}

	if !models.IsSessionValidated(sessionId) {
		if !tool.QuerySessionIsValid(sessionId) {
			tool.DefaultLogger.Errorf("Invalid sessionId: %s", sessionId)
			c.JSON(http.StatusConflict, gin.H{"error": "Blocked by another session"})
			return
		}
		models.MarkSessionValidated(sessionId)
	}

	remoteAddr := c.ClientIP()
	tool.DefaultLogger.Debugf("Received upload request: sessionId=%s, fileId=%s, token=%s, remoteAddr=%s", sessionId, fileId, token, remoteAddr)

	if ctrl.handler != nil {
		if err := ctrl.handler.OnUpload(sessionId, fileId, token, c.Request.Body, remoteAddr); err != nil {
			tool.DefaultLogger.Errorf("Upload callback error: %v", err)
			errorMsg := err.Error()

			switch errorMsg {
			case "Invalid token or IP address":
				c.JSON(http.StatusForbidden, gin.H{"error": errorMsg})
				return
			case "Blocked by another session":
				c.JSON(http.StatusConflict, gin.H{"error": errorMsg})
				return
			default:
				c.JSON(http.StatusInternalServerError, gin.H{"error": errorMsg})
				return
			}
		}
	}

	c.Status(http.StatusOK)
}
