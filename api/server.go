package api

import (
	"crypto/tls"
	"encoding/pem"
	"fmt"
	"net/http"
	"sync"

	"github.com/charmbracelet/log"
	"github.com/gin-gonic/gin"
	"github.com/moyoez/localsend-base-protocol-golang/api/controllers"
	"github.com/moyoez/localsend-base-protocol-golang/api/models"
	"github.com/moyoez/localsend-base-protocol-golang/tool"
	"github.com/moyoez/localsend-base-protocol-golang/types"
)

// Server represents the HTTP API server for receiving TCP API requests
type Server struct {
	port     int
	protocol string
	handler  *Handler
	engine   *gin.Engine
	server   *http.Server
	mu       sync.RWMutex
}

// SetSelfDevice sets the local device info used for user-side scanning.
func SetSelfDevice(device *types.VersionMessage) {
	models.SetSelfDevice(device)
}

// DefaultUploadFolder is the default folder for uploads
var DefaultUploadFolder = "uploads"

func init() {
	models.DefaultUploadFolder = DefaultUploadFolder
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

// Handler returns the HTTP handler with all registered endpoints.
func (s *Server) Handler() http.Handler {
	return s.setupRoutes()
}

func (s *Server) setupRoutes() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(gin.Recovery())

	// Initialize controllers
	registerCtrl := controllers.NewRegisterController(s.handler)
	uploadCtrl := controllers.NewUploadController(s.handler)
	cancelCtrl := controllers.NewCancelController(s.handler)

	// Register API endpoints
	v2 := engine.Group("/api/localsend/v2")
	{
		v2.POST("/register", registerCtrl.HandleRegister)
		v2.POST("/prepare-upload", uploadCtrl.HandlePrepareUpload)
		v2.POST("/upload", uploadCtrl.HandleUpload)
		v2.POST("/cancel", cancelCtrl.HandleCancel)
	}

	return engine
}

// Start starts the HTTP server
func (s *Server) Start() error {
	engine := s.setupRoutes()

	s.mu.Lock()
	s.engine = engine
	s.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: engine,
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
