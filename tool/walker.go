package tool

import (
	"os"
	"path/filepath"
)

func GetRunPositionDir() string {
	exePath, err := os.Executable()
	if err != nil {
		return ""
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return ""
	}
	return filepath.Dir(exePath)
}
