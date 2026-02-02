package api

import (
	"crypto/tls"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/charmbracelet/log"
	"github.com/gin-gonic/gin"
	"github.com/moyoez/localsend-base-protocol-golang/api/controllers"
	"github.com/moyoez/localsend-base-protocol-golang/api/middlewares"
	"github.com/moyoez/localsend-base-protocol-golang/api/models"
	"github.com/moyoez/localsend-base-protocol-golang/tool"
	"github.com/moyoez/localsend-base-protocol-golang/types"
)

// WebOutPath is the path to the Next.js static export output (web/out)
// Set via -webOutPath flag or defaults to web/out relative to working directory
var WebOutPath = "web/out"

// Server represents the HTTP API server for receiving TCP API requests
type Server struct {
	port       int
	protocol   string
	handler    *Handler
	engine     *gin.Engine
	server     *http.Server
	configPath string // path to config file for TLS cert storage
	mu         sync.RWMutex
}

var (
	DefaultConfigPath        = "config.yaml"
	DefaultUploadFolder      = "uploads"
	DoNotMakeSessionFolder   bool // if true, save under upload folder only; same filename → name-2.ext, name-3.ext, ...
)

// SetDoNotMakeSessionFolder sets whether to skip session subfolder and use numbered filenames when same name exists.
func SetDoNotMakeSessionFolder(v bool) {
	DoNotMakeSessionFolder = v
}

// SetSelfDevice sets the local device info used for user-side scanning.
func SetSelfDevice(device *types.VersionMessage) {
	models.SetSelfDevice(device)
}

// SetDefaultUploadFolder sets the default upload folder for both api and models packages
func SetDefaultUploadFolder(folder string) {
	DefaultUploadFolder = folder
	models.DefaultUploadFolder = folder
}

func init() {
	models.DefaultUploadFolder = DefaultUploadFolder
}

// NewServerWithConfig creates a new API server instance with custom config path
func NewServerWithConfig(port int, protocol string, handler *Handler, configPath string) *Server {
	if handler == nil {
		handler = &Handler{}
	}
	if configPath == "" {
		configPath = DefaultConfigPath
	}
	return &Server{
		port:       port,
		protocol:   protocol,
		handler:    handler,
		configPath: configPath,
	}
}

func (s *Server) setupRoutes() *gin.Engine {
	if tool.DefaultLogger.GetLevel() == log.DebugLevel {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}
	engine := gin.New()
	engine.Use(middlewares.AllowAllCORS())
	engine.Use(gin.Recovery())

	// Initialize controllers
	registerCtrl := controllers.NewRegisterController(s.handler)
	uploadCtrl := controllers.NewUploadController(s.handler)
	cancelCtrl := controllers.NewCancelController(s.handler)

	// Register API endpoints
	v2 := engine.Group("/api/localsend/v2")
	{
		v2.GET("/info", controllers.HandleLocalsendV2InfoGet)
		v2.POST("/register", registerCtrl.HandleRegister)
		v2.POST("/prepare-upload", uploadCtrl.HandlePrepareUpload)
		v2.POST("/upload", uploadCtrl.HandleUpload)
		v2.POST("/cancel", cancelCtrl.HandleCancel)
		// Download API (LocalSend protocol Section 5)
		if selfDevice := models.GetSelfDevice(); selfDevice != nil && selfDevice.Download {
			v2.GET("/prepare-download", controllers.HandlePrepareDownload)
			v2.GET("/download", controllers.HandleDownload)
		}
	}
	// V1 Is Deprecated, but due to some reasons, I support to this ONLY ACCEPT REQUESTS.
	v1 := engine.Group("/api/localsend/v1")
	{
		// no register, register use v2 pls.
		v1.GET("/info", controllers.HandleLocalsendV1InfoGet)
		// DO NOT use PIN, it will be rejected when no pin provided.
		v1.POST("/send-request", uploadCtrl.HandlePrepareV1Upload)
		v1.POST("/send", uploadCtrl.HandleUploadV1Upload)
		v1.POST("/cancel", cancelCtrl.HandleCancelV1Cancel)
	}
	self := engine.Group("/api/self/v1", middlewares.OnlyAllowLocal)
	{
		self.GET("/get-network-info", controllers.UserGetNetworkInfo)  // Get local network info with IP and segment number
		self.GET("/scan-current", controllers.UserScanCurrent)         // Get current scanned devices
		self.GET("/scan-now", controllers.UserScanNow)                 // Trigger immediate scan based on current config
		self.POST("/prepare-upload", controllers.UserPrepareUpload)    // Prepare upload endpoint
		self.POST("/upload", controllers.UserUpload)                   // Actual upload endpoint
		self.POST("/upload-batch", controllers.UserUploadBatch)        // Batch upload endpoint (supports file:/// protocol)
		self.GET("/confirm-recv", controllers.UserConfirmRecv)         // Confirm recv endpoint
		self.GET("/confirm-download", controllers.UserConfirmDownload) // Confirm download endpoint
		self.POST("/cancel", controllers.UserCancelUpload)             // Cancel upload endpoint (sender side)
		self.GET("/get-image", controllers.UserGetImage)
		self.GET("/favorites", controllers.UserFavoritesList)                     // List favorite devices
		self.POST("/favorites", controllers.UserFavoritesAdd)                     // Add a favorite device
		self.DELETE("/favorites/:fingerprint", controllers.UserFavoritesDelete)   // Remove a favorite device
		self.GET("/get-network-interfaces", controllers.UserGetNetworkInterfaces) // Get network interfaces,used same as usergetNetwork Info
		self.POST("/create-share-session", controllers.UserCreateShareSession)    // Create share session for download API
		self.DELETE("/close-share-session", controllers.UserCloseShareSession)    // Close share session
	}

	// Serve Next.js static export for download page at root (when Download enabled and web/out exists)
	if selfDevice := models.GetSelfDevice(); selfDevice != nil && selfDevice.Download {
		indexPage := filepath.Join(tool.GetRunPositionDir(), WebOutPath, "index.html")
		if _, err := os.Stat(indexPage); err == nil {
			engine.StaticFile("/", indexPage)
			nextStatic := filepath.Join(tool.GetRunPositionDir(), WebOutPath, "_next")
			if _, err := os.Stat(nextStatic); err == nil {
				engine.Static("/_next", nextStatic)
			}
			tool.DefaultLogger.Infof("[Server] Serving download page from %s", WebOutPath)
		} else {
			tool.DefaultLogger.Warnf("[Server] Download page not found at %s - run 'cd web && npm run build' first", indexPage)
		}
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
	tool.DefaultLogger.Infof("Starting API server on %s", address)

	if s.protocol == "https" {
		// Get or create TLS certificate from config
		cfg := tool.GetCurrentConfig()
		certBytes, keyBytes, err := tool.GetOrCreateTLSCertFromConfig(cfg)
		if err != nil {
			return fmt.Errorf("failed to get TLS certificate: %v", err)
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

		tool.DefaultLogger.Infof("TLS certificate configured for HTTPS")
		return s.server.ListenAndServeTLS("", "")
	}

	return s.server.ListenAndServe()
}
