package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/moyoez/localsend-base-protocol-golang/api/models"
	"github.com/moyoez/localsend-base-protocol-golang/notify"
	"github.com/moyoez/localsend-base-protocol-golang/tool"
	"github.com/moyoez/localsend-base-protocol-golang/types"
)

const defaultBenchUploadSize = 1 << 20 // 1 MiB

func benchUploadSize() int {
	if value := os.Getenv("BENCH_UPLOAD_SIZE"); value != "" {
		size, err := strconv.Atoi(value)
		if err == nil && size > 0 {
			return size
		}
	}
	return defaultBenchUploadSize
}

func setupBenchmarkServer(b *testing.B, uploadDir string) http.Handler {
	b.Helper()
	notify.UseNotify = false
	tool.SetProgramConfigStatus("", true)
	tool.DefaultLogger.SetLevel(log.ErrorLevel)
	tool.DefaultLogger.SetReportCaller(false)
	SetDefaultUploadFolder(uploadDir)
	server := NewServer(0, "http", NewDefaultHandler())
	return server.Handler()
}

func buildPrepareUploadBody(b *testing.B, fileID, fileName, fileType string, size int64, sha string) []byte {
	b.Helper()
	request := types.PrepareUploadRequest{
		Info: types.DeviceInfo{
			Alias:       "bench-client",
			Version:     "2.1",
			DeviceModel: "bench-model",
			DeviceType:  "desktop",
			Fingerprint: "bench-fingerprint",
			Port:        53317,
			Protocol:    "http",
			Download:    false,
		},
		Files: map[string]types.FileInfo{
			fileID: {
				ID:       fileID,
				FileName: fileName,
				Size:     size,
				FileType: fileType,
				SHA256:   sha,
			},
		},
	}
	body, err := json.Marshal(request)
	if err != nil {
		b.Fatalf("failed to marshal prepare-upload body: %v", err)
	}
	return body
}

func createUploadSession(b *testing.B, handler http.Handler, prepareBody []byte) string {
	b.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/localsend/v2/prepare-upload", bytes.NewReader(prepareBody))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		b.Fatalf("prepare-upload failed: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response types.PrepareUploadResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		b.Fatalf("failed to decode prepare-upload response: %v", err)
	}
	if response.SessionId == "" {
		b.Fatalf("prepare-upload returned empty sessionId")
	}
	return response.SessionId
}

func BenchmarkPrepareUpload(b *testing.B) {
	uploadDir := b.TempDir()
	handler := setupBenchmarkServer(b, uploadDir)

	payloadSize := benchUploadSize()
	payload := bytes.Repeat([]byte("a"), payloadSize)
	hash := sha256.Sum256(payload)
	fileID := "bench-file"
	prepareBody := buildPrepareUploadBody(b, fileID, "bench.bin", "application/octet-stream", int64(len(payload)), hex.EncodeToString(hash[:]))

	b.SetBytes(int64(len(prepareBody)))
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/localsend/v2/prepare-upload", bytes.NewReader(prepareBody))
		req.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusOK {
			b.Fatalf("prepare-upload failed: status=%d body=%s", recorder.Code, recorder.Body.String())
		}

		b.StopTimer()
		var response types.PrepareUploadResponse
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			b.Fatalf("failed to decode prepare-upload response: %v", err)
		}
		if response.SessionId != "" {
			models.RemoveUploadSession(response.SessionId)
			tool.DestorySession(response.SessionId)
		}
		b.StartTimer()
	}
}

func BenchmarkUpload(b *testing.B) {
	uploadDir := b.TempDir()
	handler := setupBenchmarkServer(b, uploadDir)

	payloadSize := benchUploadSize()
	payload := bytes.Repeat([]byte("a"), payloadSize)
	hash := sha256.Sum256(payload)
	fileID := "bench-file"
	prepareBody := buildPrepareUploadBody(b, fileID, "bench.bin", "application/octet-stream", int64(len(payload)), hex.EncodeToString(hash[:]))
	sessionID := createUploadSession(b, handler, prepareBody)
	b.Cleanup(func() {
		models.RemoveUploadSession(sessionID)
		tool.DestorySession(sessionID)
	})

	uploadURL := fmt.Sprintf("/api/localsend/v2/upload?sessionId=%s&fileId=%s&token=%s", sessionID, fileID, "bench-token")

	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, uploadURL, bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/octet-stream")
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusOK {
			b.Fatalf("upload failed: status=%d body=%s", recorder.Code, recorder.Body.String())
		}
	}
}
