package api

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/moyoez/localsend-base-protocol-golang/api/models"
	"github.com/moyoez/localsend-base-protocol-golang/tool"
	"github.com/moyoez/localsend-base-protocol-golang/types"
)

// Handler contains callback functions for handling API requests
type Handler struct {
	onRegister      func(remote *types.VersionMessage) error
	onPrepareUpload func(request *types.PrepareUploadRequest, pin string) (*types.PrepareUploadResponse, error)
	onUpload        func(sessionId, fileId, token string, data io.Reader, remoteAddr string) error
	onCancel        func(sessionId string) error
}

// Ensure Handler implements types.HandlerInterface
var _ types.HandlerInterface = (*Handler)(nil)

// OnRegister implements types.HandlerInterface
func (h *Handler) OnRegister(remote *types.VersionMessage) error {
	if h.onRegister != nil {
		return h.onRegister(remote)
	}
	return nil
}

// OnPrepareUpload implements types.HandlerInterface
func (h *Handler) OnPrepareUpload(request *types.PrepareUploadRequest, pin string) (*types.PrepareUploadResponse, error) {
	if h.onPrepareUpload != nil {
		return h.onPrepareUpload(request, pin)
	}
	return nil, nil
}

// OnUpload implements types.HandlerInterface
func (h *Handler) OnUpload(sessionId, fileId, token string, data io.Reader, remoteAddr string) error {
	if h.onUpload != nil {
		return h.onUpload(sessionId, fileId, token, data, remoteAddr)
	}
	return nil
}

// OnCancel implements types.HandlerInterface
func (h *Handler) OnCancel(sessionId string) error {
	if h.onCancel != nil {
		return h.onCancel(sessionId)
	}
	return nil
}

// NewDefaultHandler returns a default Handler implementation.
func NewDefaultHandler() *Handler {
	return &Handler{
		onRegister: func(remote *types.VersionMessage) error {
			tool.DefaultLogger.Infof("Received device register request: %s (fingerprint: %s, port: %d)",
				remote.Alias, remote.Fingerprint, remote.Port)
			return nil
		},
		onPrepareUpload: func(request *types.PrepareUploadRequest, pin string) (*types.PrepareUploadResponse, error) {
			tool.DefaultLogger.Infof("Received file transfer prepare request: from %s, file count: %d, PIN: %s",
				request.Info.Alias, len(request.Files), pin)
			askSession := tool.GenerateRandomUUID()
			response := &types.PrepareUploadResponse{
				SessionId: askSession,
				Files:     make(map[string]string),
			}

			if err := tool.JoinSession(askSession); err != nil {
				return nil, err
			}

			for fileID := range request.Files {
				response.Files[fileID] = "accepted"
			}

			models.CacheUploadSession(askSession, request.Files)

			return response, nil
		},
		onUpload: func(sessionId, fileId, token string, data io.Reader, remoteAddr string) error {
			info, ok := models.LookupFileInfo(sessionId, fileId)
			if !ok {
				return fmt.Errorf("file metadata not found")
			}

			if err := os.MkdirAll(filepath.Join(models.DefaultUploadFolder, sessionId), 0o755); err != nil {
				return fmt.Errorf("create upload dir failed: %w", err)
			}

			fileName := strings.TrimSpace(info.FileName)
			if fileName == "" {
				fileName = fileId
			}
			fileName = filepath.Base(fileName)
			targetPath := filepath.Join(models.DefaultUploadFolder, sessionId, fmt.Sprintf("%s_%s", fileId, fileName))

			file, err := os.Create(targetPath)
			if err != nil {
				return fmt.Errorf("create file failed: %w", err)
			}
			defer file.Close()

			hasher := sha256.New()
			writer := io.MultiWriter(file, hasher)
			written, err := io.Copy(writer, data)
			if err != nil {
				return fmt.Errorf("write file failed: %w", err)
			}

			if info.Size > 0 && written != info.Size {
				return fmt.Errorf("size mismatch")
			}

			if info.SHA256 != "" {
				actual := hex.EncodeToString(hasher.Sum(nil))
				if !strings.EqualFold(actual, info.SHA256) {
					return fmt.Errorf("hash mismatch")
				}
			}

			tool.DefaultLogger.Infof("Upload saved: sessionId=%s, fileId=%s, path=%s", sessionId, fileId, targetPath)
			return nil
		},
		onCancel: func(sessionId string) error {
			tool.DefaultLogger.Infof("Received file transfer cancel request: sessionId=%s", sessionId)
			if !tool.QuerySessionIsValid(sessionId) {
				return fmt.Errorf("session %s not found", sessionId)
			}
			models.RemoveUploadSession(sessionId)
			tool.DefaultLogger.Infof("Session %s canceled", sessionId)
			return nil
		},
	}
}
