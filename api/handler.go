package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/moyoez/localsend-base-protocol-golang/api/models"
	"github.com/moyoez/localsend-base-protocol-golang/notify"
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

// nextAvailablePath returns the first path under dir that does not exist, using fileName
// and if it exists, trying base-2.ext, base-3.ext, ... (e.g. txt.txt -> txt-2.txt, txt-3.txt).
func nextAvailablePath(dir, fileName string) string {
	ext := filepath.Ext(fileName)
	base := strings.TrimSuffix(filepath.Base(fileName), ext)
	if base == "" {
		base = fileName
		ext = ""
	}
	try := filepath.Join(dir, fileName)
	if _, err := os.Stat(try); os.IsNotExist(err) {
		return try
	}
	for n := 2; ; n++ {
		try = filepath.Join(dir, fmt.Sprintf("%s-%d%s", base, n, ext))
		if _, err := os.Stat(try); os.IsNotExist(err) {
			return try
		}
	}
}

// copyWithContext copies from src to dst while respecting context cancellation.
// It checks the context periodically during the copy operation.
func copyWithContext(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	buf := make([]byte, 2*1024*1024) // 2MB buffer
	var written int64
	for {
		// Check if context is cancelled before each read
		select {
		case <-ctx.Done():
			return written, ctx.Err()
		default:
		}

		nr, readErr := src.Read(buf)
		if nr > 0 {
			nw, writeErr := dst.Write(buf[0:nr])
			if nw < 0 || nr < nw {
				nw = 0
				if writeErr == nil {
					writeErr = fmt.Errorf("invalid write result")
				}
			}
			written += int64(nw)
			if writeErr != nil {
				return written, writeErr
			}
			if nr != nw {
				return written, io.ErrShortWrite
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				return written, nil
			}
			return written, readErr
		}
	}
}

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

			// generated uuid, ready to check before updating files.

			// this is for pin support.
			pinSetted := tool.GetProgramConfigStatus().Pin
			switch {
			case pinSetted != "" && pin == "":
				notification := &notify.Notification{
					Type:    "pin_required",
					Title:   "PIN Required",
					Message: fmt.Sprintf("PIN required for incoming files from %s", request.Info.Alias),
					Data: map[string]any{
						"from":      request.Info.Alias,
						"fileCount": len(request.Files),
					},
				}
				tool.DefaultLogger.Infof("[Notify] Sending pin_required notification: %v", notification)
				if err := notify.SendNotification(notification, ""); err != nil {
					tool.DefaultLogger.Errorf("[Notify] Failed to send pin_required notification: %v", err)
				}
				return nil, fmt.Errorf("pin required")
			case pinSetted != "" && pin != pinSetted:
				return nil, fmt.Errorf("invalid PIN")
			}

			programConfig := tool.GetProgramConfigStatus()
			// Check if we need user confirmation:
			// - If AutoSave is true: no confirmation needed
			// - If AutoSaveFromFavorites is true AND sender is in favorites: no confirmation needed
			// - Otherwise: need confirmation
			needConfirmation := !programConfig.AutoSave
			if needConfirmation && programConfig.AutoSaveFromFavorites {
				// Check if sender is in favorites (real-time read from config)
				if tool.IsFavorite(request.Info.Fingerprint) {
					tool.DefaultLogger.Infof("Auto-accepting from favorite device: %s (fingerprint: %s)", request.Info.Alias, request.Info.Fingerprint)
					needConfirmation = false
				}
			}

			if needConfirmation {
				// user is required to confirm before recv.
				confirmCh := make(chan types.ConfirmResult, 1)
				models.SetConfirmRecvChannel(askSession, confirmCh)
				defer models.DeleteConfirmRecvChannel(askSession)

				files := make([]types.FileInfo, 0, len(request.Files))
				for _, info := range request.Files {
					files = append(files, info)
				}

				notification := &notify.Notification{
					Type:    "confirm_recv",
					Title:   "Confirm Receive",
					Message: fmt.Sprintf("Incoming files from %s", request.Info.Alias),
					Data: map[string]any{
						"sessionId": askSession,
						"from":      request.Info.Alias,
						"fileCount": len(request.Files),
						"files":     files,
					},
				}
				// send notify to user.
				tool.DefaultLogger.Infof("[Notify] Sending confirm_recv notification: %v", notification)
				tool.DefaultLogger.Debugf("Accpet by using this link: https://localhost:53317/api/self/v1/confirm-recv?sessionId=%s&confirmed=true", askSession)
				tool.DefaultLogger.Debugf("Reject by using this link: https://localhost:53317/api/self/v1/confirm-recv?sessionId=%s&confirmed=false", askSession)
				if err := notify.SendNotification(notification, ""); err != nil {
					tool.DefaultLogger.Errorf("[Notify] Failed to send confirm_recv notification: %v", err)
				}
				// timeout is 30
				confirmTimeout := 30 * time.Second
				confirmTimeOuttimer := time.NewTimer(confirmTimeout)

				defer confirmTimeOuttimer.Stop()
				select {
				case result := <-confirmCh:
					if !result.Confirmed {
						return nil, fmt.Errorf("rejected")
					}
				case <-confirmTimeOuttimer.C:
					return nil, fmt.Errorf("rejected")
				}
			}

			if err := tool.JoinSession(askSession); err != nil {
				return nil, err
			}

			// Create session context for cancellation support
			models.CreateSessionContext(askSession)

			for fileID := range request.Files {
				response.Files[fileID] = "accepted"
			}

			models.CacheUploadSession(askSession, request.Files)

			return response, nil
		},
		onUpload: func(sessionId, fileId, token string, data io.Reader, remoteAddr string) error {
			// Check if session is cancelled before starting
			if models.IsSessionCancelled(sessionId) {
				return fmt.Errorf("session cancelled")
			}

			// Get session context for cancellation support
			ctx := models.GetSessionContext(sessionId)
			if ctx == nil {
				// Fallback: create a background context if session context not found
				ctx = context.Background()
			}

			info, ok := models.LookupFileInfo(sessionId, fileId)
			if !ok {
				return fmt.Errorf("file metadata not found")
			}

			uploadDir := models.DefaultUploadFolder
			if !DoNotMakeSessionFolder {
				uploadDir = filepath.Join(models.DefaultUploadFolder, sessionId)
			}
			if err := os.MkdirAll(uploadDir, 0o755); err != nil {
				return fmt.Errorf("create upload dir failed: %w", err)
			}

			fileName := strings.TrimSpace(info.FileName)
			if fileName == "" {
				fileName = fileId
			}
			fileName = filepath.Base(fileName)
			targetPath := filepath.Join(uploadDir, fileName)
			if DoNotMakeSessionFolder {
				targetPath = nextAvailablePath(uploadDir, fileName)
			}

			file, err := os.Create(targetPath)
			if err != nil {
				return fmt.Errorf("create file failed: %w", err)
			}
			defer func() {
				if err := file.Close(); err != nil {
					tool.DefaultLogger.Errorf("Failed to close file: %v", err)
				}
			}()

			hasher := sha256.New()
			writer := io.MultiWriter(file, hasher)

			// Use context-aware copy to support cancellation
			written, err := copyWithContext(ctx, writer, data)
			if err != nil {
				// Check if it was cancelled
				if ctx.Err() != nil {
					// Clean up the partial file
					if err := file.Close(); err != nil {
						tool.DefaultLogger.Errorf("Failed to close file: %v", err)
					}
					err := os.Remove(targetPath)
					if err != nil {
						tool.DefaultLogger.Errorf("Failed to remove partial file: %v", err)
					}
					return fmt.Errorf("upload cancelled")
				}
				return fmt.Errorf("write file failed: %w", err)
			}

			// Check if cancelled after copy
			if ctx.Err() != nil {
				if err := file.Close(); err != nil {
					tool.DefaultLogger.Errorf("Failed to close file: %v", err)
				}
				err := os.Remove(targetPath)
				if err != nil {
					tool.DefaultLogger.Errorf("Failed to remove partial file: %v", err)
				}
				return fmt.Errorf("upload cancelled")
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
			// This will cancel the session context, interrupting any ongoing uploads
			models.RemoveUploadSession(sessionId)
			// Also destroy the session from the tool cache
			tool.DestorySession(sessionId)
			tool.DefaultLogger.Infof("Session %s canceled and all ongoing uploads interrupted", sessionId)
			return nil
		},
	}
}
