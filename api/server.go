package api

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/charmbracelet/log"
	"github.com/moyoez/localsend-base-protocol-golang/boardcast"
	"github.com/moyoez/localsend-base-protocol-golang/tool"
	"github.com/moyoez/localsend-base-protocol-golang/types"
)

// Server represents the HTTP API server for receiving TCP API requests
type Server struct {
	port     int
	protocol string
	handler  *Handler
	server   *http.Server
	mu       sync.RWMutex
}

// Handler contains callback functions for handling API requests
type Handler struct {
	OnRegister      func(remote *types.VersionMessage) error
	OnPrepareUpload func(request *types.PrepareUploadRequest, pin string) (*types.PrepareUploadResponse, error)
	OnUpload        func(sessionId, fileId, token string, data io.Reader, remoteAddr string) error
	OnCancel        func(sessionId string) error
}

const defaultUploadDir = "uploads"

var (
	uploadSessionMu sync.RWMutex
	uploadSessions  = map[string]map[string]types.FileInfo{}
)

func cacheUploadSession(sessionId string, files map[string]types.FileInfo) {
	uploadSessionMu.Lock()
	defer uploadSessionMu.Unlock()
	copied := make(map[string]types.FileInfo, len(files))
	for fileId, info := range files {
		copied[fileId] = info
	}
	uploadSessions[sessionId] = copied
}

func lookupFileInfo(sessionId, fileId string) (types.FileInfo, bool) {
	uploadSessionMu.RLock()
	defer uploadSessionMu.RUnlock()
	files, ok := uploadSessions[sessionId]
	if !ok {
		return types.FileInfo{}, false
	}
	info, exists := files[fileId]
	return info, exists
}

func removeUploadedFile(sessionId, fileId string) {
	uploadSessionMu.Lock()
	defer uploadSessionMu.Unlock()
	files, ok := uploadSessions[sessionId]
	if !ok {
		return
	}
	delete(files, fileId)
	if len(files) == 0 {
		delete(uploadSessions, sessionId)
	}
}

func removeUploadSession(sessionId string) {
	uploadSessionMu.Lock()
	defer uploadSessionMu.Unlock()
	delete(uploadSessions, sessionId)
}

// NewDefaultHandler returns a default Handler implementation.
func NewDefaultHandler() *Handler {
	return &Handler{
		OnRegister: func(remote *types.VersionMessage) error {
			log.Infof("Received device register request: %s (fingerprint: %s, port: %d)",
				remote.Alias, remote.Fingerprint, remote.Port)
			return nil
		},
		OnPrepareUpload: func(request *types.PrepareUploadRequest, pin string) (*types.PrepareUploadResponse, error) {
			log.Infof("Received file transfer prepare request: from %s, file count: %d, PIN: %s",
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

			cacheUploadSession(askSession, request.Files)

			return response, nil
		},
		OnUpload: func(sessionId, fileId, token string, data io.Reader, remoteAddr string) error {
			info, ok := lookupFileInfo(sessionId, fileId)
			if !ok {
				return fmt.Errorf("file metadata not found")
			}

			if err := os.MkdirAll(filepath.Join(defaultUploadDir, sessionId), 0o755); err != nil {
				return fmt.Errorf("create upload dir failed: %w", err)
			}

			fileName := strings.TrimSpace(info.FileName)
			if fileName == "" {
				fileName = fileId
			}
			fileName = filepath.Base(fileName)
			targetPath := filepath.Join(defaultUploadDir, sessionId, fmt.Sprintf("%s_%s", fileId, fileName))

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

			removeUploadedFile(sessionId, fileId)
			log.Infof("Upload saved: sessionId=%s, fileId=%s, path=%s", sessionId, fileId, targetPath)
			return nil
		},
	}
}

// NewServer creates a new API server instance
func NewServer(port int, protocol string, handler *Handler) *Server {
	if handler == nil {
		handler = &Handler{}
	}
	return &Server{
		port:     port,
		protocol: protocol,
		handler:  handler,
	}
}

// Start starts the HTTP server
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// Register API endpoints
	mux.HandleFunc("/api/localsend/v2/register", s.handleRegister)
	mux.HandleFunc("/api/localsend/v2/prepare-upload", s.handlePrepareUpload)
	mux.HandleFunc("/api/localsend/v2/upload", s.handleUpload)
	mux.HandleFunc("/api/localsend/v2/cancel", s.handleCancel)

	s.mu.Lock()
	s.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: mux,
	}
	s.mu.Unlock()

	address := fmt.Sprintf("%s://0.0.0.0:%d", s.protocol, s.port)
	log.Infof("Starting API server on %s", address)

	if s.protocol == "https" {
		// Generate self-signed TLS certificate
		certBytes, keyBytes, err := tool.GenerateTLSCert()
		if err != nil {
			return fmt.Errorf("failed to generate TLS certificate: %v", err)
		}

		// Convert DER format to PEM format
		certPEM := pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: certBytes,
		})

		keyPEM := pem.EncodeToMemory(&pem.Block{
			Type:  "EC PRIVATE KEY",
			Bytes: keyBytes,
		})

		// Load certificate and key for TLS
		cert, err := tls.X509KeyPair(certPEM, keyPEM)
		if err != nil {
			return fmt.Errorf("failed to load TLS certificate: %v", err)
		}

		// Configure TLS
		s.mu.Lock()
		s.server.TLSConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
		}
		s.mu.Unlock()

		log.Infof("TLS certificate generated and configured for HTTPS")
		return s.server.ListenAndServeTLS("", "")
	}

	return s.server.ListenAndServe()
}

// Stop stops the HTTP server
func (s *Server) Stop() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.server != nil {
		return s.server.Close()
	}
	return nil
}

// handleRegister handles the /api/localsend/v2/register endpoint
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Errorf("Failed to read register request body: %v", err)
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Reuse parsing function from boardcast package
	incoming, err := boardcast.ParseVersionMessageFromBody(body)
	if err != nil {
		log.Errorf("Failed to parse register request: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	log.Debugf("Received register request from %s (fingerprint: %s)", incoming.Alias, incoming.Fingerprint)

	// Call the registered callback if available
	if s.handler.OnRegister != nil {
		if err := s.handler.OnRegister(incoming); err != nil {
			log.Errorf("Register callback error: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	response := map[string]string{"status": "ok"}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Errorf("Failed to encode register response: %v", err)
	}
}

// handlePrepareUpload handles the /api/localsend/v2/prepare-upload endpoint
func (s *Server) handlePrepareUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract PIN from query parameters if present
	pin := r.URL.Query().Get("pin")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Errorf("Failed to read prepare-upload request body: %v", err)
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Reuse parsing function from boardcast package
	request, err := boardcast.ParsePrepareUploadRequestFromBody(body)
	if err != nil {
		log.Errorf("Failed to parse prepare-upload request: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	log.Debugf("Received prepare-upload request from %s (pin: %s)", request.Info.Alias, pin)

	// Call the registered callback if available
	var response *types.PrepareUploadResponse
	if s.handler.OnPrepareUpload != nil {
		var callbackErr error
		response, callbackErr = s.handler.OnPrepareUpload(request, pin)
		if callbackErr != nil {
			log.Errorf("Prepare-upload callback error: %v", callbackErr)
			// Map common errors to HTTP status codes
			statusCode := http.StatusInternalServerError
			errorMsg := callbackErr.Error()

			// You can customize error handling based on error types
			switch errorMsg {
			case "pin required", "invalid pin":
				statusCode = http.StatusUnauthorized
			case "rejected":
				statusCode = http.StatusForbidden
			case "blocked by another session":
				statusCode = http.StatusConflict
			case "too many requests":
				statusCode = http.StatusTooManyRequests
			}

			http.Error(w, errorMsg, statusCode)
			return
		}
	} else {
		// Default response if no callback is registered
		response = &types.PrepareUploadResponse{
			SessionId: "default-session",
			Files:     make(map[string]string),
		}
		// Accept all files by default
		for fileID := range request.Files {
			response.Files[fileID] = "accepted"
		}
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Errorf("Failed to encode prepare-upload response: %v", err)
	}
}

// handleUpload handles the /api/localsend/v2/upload endpoint
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract query parameters
	sessionId := r.URL.Query().Get("sessionId")
	fileId := r.URL.Query().Get("fileId")
	token := r.URL.Query().Get("token")

	// Validate required parameters
	if sessionId == "" || fileId == "" || token == "" {
		log.Errorf("Missing required parameters: sessionId=%s, fileId=%s, token=%s", sessionId, fileId, token)
		http.Error(w, "Missing parameters", http.StatusBadRequest)
		return
	}

	// Validate session availability
	if !tool.QuerySessionIsValid(sessionId) {
		log.Errorf("Invalid sessionId: %s", sessionId)
		http.Error(w, "Blocked by another session", http.StatusConflict)
		return
	}

	// Get remote address for IP validation
	remoteAddr := r.RemoteAddr

	log.Debugf("Received upload request: sessionId=%s, fileId=%s, token=%s, remoteAddr=%s", sessionId, fileId, token, remoteAddr)

	// Call the registered callback if available
	if s.handler.OnUpload != nil {
		if err := s.handler.OnUpload(sessionId, fileId, token, r.Body, remoteAddr); err != nil {
			log.Errorf("Upload callback error: %v", err)
			errorMsg := err.Error()

			// Map errors to HTTP status codes
			statusCode := http.StatusInternalServerError
			switch errorMsg {
			case "Invalid token or IP address":
				statusCode = http.StatusForbidden
			case "Blocked by another session":
				statusCode = http.StatusConflict
			case "Unknown receiver error":
				statusCode = http.StatusInternalServerError
			}

			http.Error(w, errorMsg, statusCode)
			return
		}
	}

	// Return success response with no body
	w.WriteHeader(http.StatusOK)
}

// handleCancel handles the /api/localsend/v2/cancel endpoint
func (s *Server) handleCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract sessionId from query parameters
	sessionId := r.URL.Query().Get("sessionId")

	// Validate required parameter
	if sessionId == "" {
		log.Errorf("Missing required parameter: sessionId")
		http.Error(w, "Missing parameters", http.StatusBadRequest)
		return
	}

	log.Debugf("Received cancel request: sessionId=%s", sessionId)

	// Call the registered callback if available
	if s.handler.OnCancel != nil {
		if err := s.handler.OnCancel(sessionId); err != nil {
			log.Errorf("Cancel callback error: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
	}

	// Destroy session on cancel

	removeUploadSession(sessionId)

	// Return success response with no body
	w.WriteHeader(http.StatusOK)
}
