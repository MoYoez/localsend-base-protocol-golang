package tool

import (
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/log"
)

var (
	defaultLogDir = "log"
	DefaultLogger = log.Default()
)

func InitLogger() {
	_ = os.MkdirAll(defaultLogDir, 0o755)

	logFile := filepath.Join(defaultLogDir, time.Now().Format("2006-01-02.log"))
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		panic(err)
	}
	DefaultLogger.SetOutput(io.MultiWriter(os.Stdout, f))
	DefaultLogger.SetTimeFormat("2006-01-02 15:04:05")
	DefaultLogger.SetReportCaller(true)
}
