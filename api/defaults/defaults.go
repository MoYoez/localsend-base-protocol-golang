package defaults

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

	"github.com/moyoez/localsend-go/api/models"
	"github.com/moyoez/localsend-go/notify"
	"github.com/moyoez/localsend-go/tool"
	"github.com/moyoez/localsend-go/types"
)

// DefaultOnRegister is the default callback for device register.
func DefaultOnRegister(remote *types.VersionMessage) error {
	tool.DefaultLogger.Infof("Received device register request: %s (fingerprint: %s, port: %d)",
		remote.Alias, remote.Fingerprint, remote.Port)
	return nil
}

// DefaultOnPrepareUpload is the default callback for prepare-upload.
func DefaultOnPrepareUpload(request *types.PrepareUploadRequest, pin string) (*types.PrepareUploadResponse, error) {
	tool.DefaultLogger.Infof("Received file transfer prepare request: from %s, file count: %d, PIN: %s",
		request.Info.Alias, len(request.Files), pin)

	askSession := tool.GenerateRandomUUID()
	response := &types.PrepareUploadResponse{
		SessionId: askSession,
		Files:     make(map[string]string),
	}

	pinSetted := tool.GetProgramConfigStatus().Pin
	switch {
	case pinSetted != "" && pin == "":
		notification := &types.Notification{
			Type:    types.NotifyTypePinRequired,
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

	// Text-only message: single file, text/plain, with preview — show dialog, wait for user dismiss, then return 204 (no upload)
	if len(request.Files) == 1 {
		for _, info := range request.Files {
			if strings.TrimSpace(strings.ToLower(info.FileType)) == "text/plain" && info.Preview != "" {
				title := "Text Received"
				if request.Info.Alias != "" {
					title = fmt.Sprintf("From %s", request.Info.Alias)
				}
				textDismissSessionId := tool.GenerateRandomUUID()
				dismissCh := make(chan struct{}, 1)
				models.SetTextReceivedDismissChannel(textDismissSessionId, dismissCh)
				defer models.DeleteTextReceivedDismissChannel(textDismissSessionId)
				if err := notify.SendTextReceivedNotification(request.Info.Alias, title, info.Preview, info.FileName, textDismissSessionId); err != nil {
					tool.DefaultLogger.Errorf("[Notify] Failed to send text_received notification: %v", err)
					return nil, nil
				}
				dismissTimeout := 2 * time.Minute
				select {
				case <-dismissCh:
					tool.DefaultLogger.Infof("[PrepareUpload] Text-only message from %s dismissed by user, returning 204 (no upload)", request.Info.Alias)
				case <-time.After(dismissTimeout):
					tool.DefaultLogger.Infof("[PrepareUpload] Text-only message from %s dismiss timeout, returning 204 (no upload)", request.Info.Alias)
				}
				return nil, nil
			}
			break
		}
	}

	programConfig := tool.GetProgramConfigStatus()
	needConfirmation := !programConfig.AutoSave
	if needConfirmation && programConfig.AutoSaveFromFavorites {
		if tool.IsFavorite(request.Info.Fingerprint) {
			tool.DefaultLogger.Infof("Auto-accepting from favorite device: %s (fingerprint: %s)", request.Info.Alias, request.Info.Fingerprint)
			needConfirmation = false
		}
	}

	if needConfirmation {
		confirmCh := make(chan types.ConfirmResult, 1)
		models.SetConfirmRecvChannel(askSession, confirmCh)
		defer models.DeleteConfirmRecvChannel(askSession)

		files := make([]types.FileInfo, 0, len(request.Files))
		for _, info := range request.Files {
			files = append(files, info)
		}

		notification := &types.Notification{
			Type:    types.NotifyTypeConfirmRecv,
			Title:   "Confirm Receive",
			Message: fmt.Sprintf("Incoming files from %s", request.Info.Alias),
			Data: map[string]any{
				"sessionId": askSession,
				"from":      request.Info.Alias,
				"fileCount": len(request.Files),
				"files":     files,
			},
		}
		tool.DefaultLogger.Infof("[Notify] Sending confirm_recv notification: %v", notification)
		tool.DefaultLogger.Debugf("Accpet by using this link: https://localhost:53317/api/self/v1/confirm-recv?sessionId=%s&confirmed=true", askSession)
		tool.DefaultLogger.Debugf("Reject by using this link: https://localhost:53317/api/self/v1/confirm-recv?sessionId=%s&confirmed=false", askSession)
		if err := notify.SendNotification(notification, ""); err != nil {
			tool.DefaultLogger.Errorf("[Notify] Failed to send confirm_recv notification: %v", err)
		}
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

	models.CreateSessionContext(askSession)

	for fileID := range request.Files {
		response.Files[fileID] = "accepted"
	}

	models.CacheUploadSession(askSession, request.Files)

	return response, nil
}

// DefaultOnUpload is the default callback for file upload.
func DefaultOnUpload(sessionId, fileId, token string, data io.Reader, remoteAddr string) error {
	if models.IsSessionCancelled(sessionId) {
		return fmt.Errorf("session cancelled")
	}

	ctx := models.GetSessionContext(sessionId)
	if ctx == nil {
		ctx = context.Background()
	}

	info, ok := models.LookupFileInfo(sessionId, fileId)
	if !ok {
		return fmt.Errorf("file metadata not found")
	}

	uploadDir := models.DefaultUploadFolder
	if !models.DoNotMakeSessionFolder {
		uploadDir = filepath.Join(models.DefaultUploadFolder, sessionId)
	}
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		return fmt.Errorf("create upload dir failed: %w", err)
	}

	fileName := strings.TrimSpace(info.FileName)
	if fileName == "" {
		fileName = fileId
	}
	// Preserve relative path (e.g. "foldername/subdir/file.txt") for folder uploads
	relativePath := filepath.Clean(filepath.FromSlash(fileName))
	// Prevent path traversal: ensure result stays under uploadDir
	uploadDirAbs, err := filepath.Abs(uploadDir)
	if err != nil {
		return fmt.Errorf("upload dir abs: %w", err)
	}
	targetPath := filepath.Join(uploadDir, relativePath)
	targetPathAbs, err := filepath.Abs(targetPath)
	if err != nil {
		return fmt.Errorf("target path abs: %w", err)
	}
	rel, err := filepath.Rel(uploadDirAbs, targetPathAbs)
	if err != nil || strings.HasPrefix(rel, "..") || rel == ".." {
		return fmt.Errorf("invalid file path: path traversal not allowed")
	}
	// Create parent directories for folder structure
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return fmt.Errorf("create parent dir failed: %w", err)
	}
	if models.DoNotMakeSessionFolder {
		targetPath = tool.NextAvailablePath(filepath.Dir(targetPath), filepath.Base(targetPath))
	}

	file, err := os.Create(targetPath)
	if err != nil {
		return fmt.Errorf("create file failed: %w", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			tool.DefaultLogger.Warnf("Failed to close file: %v", err)
		}
	}()

	hasher := sha256.New()
	writer := io.MultiWriter(file, hasher)

	written, err := tool.CopyWithContext(ctx, writer, data)
	if err != nil {
		if ctx.Err() != nil {
			_ = file.Close()
			_ = os.Remove(targetPath)
			return fmt.Errorf("upload cancelled")
		}
		return fmt.Errorf("write file failed: %w", err)
	}

	if ctx.Err() != nil {
		_ = file.Close()
		_ = os.Remove(targetPath)
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

	models.SetFileSavePath(sessionId, fileId, targetPath)
	tool.DefaultLogger.Infof("Upload saved: sessionId=%s, fileId=%s, path=%s", sessionId, fileId, targetPath)
	return nil
}

// DefaultOnCancel is the default callback for session cancel.
func DefaultOnCancel(sessionId string) error {
	tool.DefaultLogger.Infof("Received file transfer cancel request: sessionId=%s", sessionId)
	if !tool.QuerySessionIsValid(sessionId) {
		return fmt.Errorf("session %s not found", sessionId)
	}
	models.RemoveUploadSession(sessionId)
	tool.DestorySession(sessionId)
	tool.DefaultLogger.Infof("Session %s canceled and all ongoing uploads interrupted", sessionId)
	return nil
}
