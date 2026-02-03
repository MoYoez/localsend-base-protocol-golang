package tool

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/moyoez/localsend-go/types"
)

// processFileInput processes a FileInput and fills missing information from fileUrl if provided
func ProcessFileInput(fileInput *types.FileInput) error {
	// If fileUrl is provided, auto-fill missing information
	if fileInput.FileUrl != "" {
		parsedUrl, err := url.Parse(fileInput.FileUrl)
		if err != nil {
			return fmt.Errorf("invalid fileUrl: %v", err)
		}

		if parsedUrl.Scheme != "file" {
			return fmt.Errorf("only file:// protocol is supported for fileUrl")
		}

		filePath := parsedUrl.Path
		DefaultLogger.Infof("Reading file info from: %s", filePath)

		// Determine if we need to calculate SHA256
		calculateSHA := fileInput.SHA256 == ""

		// Get file information
		fileName, fileSize, fileType, sha256Hash, err := GetFileInfoFromPath(filePath, calculateSHA)
		if err != nil {
			return err
		}

		// Fill missing fields
		if fileInput.FileName == "" {
			fileInput.FileName = fileName
			DefaultLogger.Debugf("Auto-detected fileName: %s", fileName)
		}
		if fileInput.Size == 0 {
			fileInput.Size = fileSize
			DefaultLogger.Debugf("Auto-detected size: %d bytes", fileSize)
		}
		if fileInput.FileType == "" {
			fileInput.FileType = fileType
			DefaultLogger.Debugf("Auto-detected fileType: %s", fileType)
		}
		if fileInput.SHA256 == "" && sha256Hash != "" {
			fileInput.SHA256 = sha256Hash
			DefaultLogger.Debugf("Auto-calculated SHA256: %s", sha256Hash)
		}
	}

	// Validate required fields
	if fileInput.FileName == "" {
		return fmt.Errorf("fileName is required")
	}
	if fileInput.Size == 0 {
		return fmt.Errorf("size is required or must be > 0")
	}
	if fileInput.FileType == "" {
		return fmt.Errorf("fileType is required")
	}

	return nil
}

// getFileInfoFromPath reads file information from local filesystem
// Returns fileName, size, fileType, sha256, error
func GetFileInfoFromPath(filePath string, calculateSHA bool) (string, int64, string, string, error) {
	// Get file info
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return "", 0, "", "", fmt.Errorf("failed to stat file: %v", err)
	}

	// Check if it's a file (not directory)
	if fileInfo.IsDir() {
		return "", 0, "", "", fmt.Errorf("path is a directory, not a file")
	}

	// Get file name
	fileName := filepath.Base(filePath)

	// Get file size
	fileSize := fileInfo.Size()

	// Detect file type (MIME type) from extension
	fileType := mime.TypeByExtension(filepath.Ext(filePath))
	if fileType == "" {
		fileType = "application/octet-stream" // Default MIME type
	}

	// Calculate SHA256 if requested
	var sha256Hash string
	if calculateSHA {
		file, err := os.Open(filePath)
		if err != nil {
			return fileName, fileSize, fileType, "", fmt.Errorf("failed to open file for hashing: %v", err)
		}
		defer func() {
			if err := file.Close(); err != nil {
				DefaultLogger.Errorf("Failed to close file: %v", err)
			}
		}()

		hasher := sha256.New()
		if _, err := io.Copy(hasher, file); err != nil {
			return fileName, fileSize, fileType, "", fmt.Errorf("failed to calculate SHA256: %v", err)
		}
		sha256Hash = hex.EncodeToString(hasher.Sum(nil))
	}

	return fileName, fileSize, fileType, sha256Hash, nil
}

// ProcessFolderForUpload recursively processes a folder and returns file information for upload.
// Returns a map of fileId -> FileInput with filenames in "foldername/subfolder/file.txt" format.
// folderPath: absolute path to the folder to process
// fileIdToPathMap: output map of fileId to actual file path on disk (for later reading)
func ProcessFolderForUpload(folderPath string, calculateSHA bool) (map[string]*types.FileInput, map[string]string, error) {
	// Get folder info
	info, err := os.Stat(folderPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to stat folder: %v", err)
	}

	if !info.IsDir() {
		return nil, nil, fmt.Errorf("path is not a directory: %s", folderPath)
	}

	// Get the folder name to use as prefix
	folderName := filepath.Base(folderPath)

	fileInputMap := make(map[string]*types.FileInput)
	fileIdToPathMap := make(map[string]string)

	err = filepath.WalkDir(folderPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories themselves, only process files
		if d.IsDir() {
			return nil
		}

		// Calculate relative path from the folder
		relPath, err := filepath.Rel(folderPath, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %v", err)
		}

		// Combine folder name with relative path: "foldername/subfolder/file.txt"
		// Use forward slashes for cross-platform compatibility (LocalSend protocol uses forward slashes)
		fileName := folderName + "/" + filepath.ToSlash(relPath)

		// Get file info
		fileInfo, err := os.Stat(path)
		if err != nil {
			DefaultLogger.Warnf("Skipping file %s: failed to stat: %v", path, err)
			return nil // Continue processing other files
		}

		// Detect file type (MIME type) from extension
		fileType := mime.TypeByExtension(filepath.Ext(path))
		if fileType == "" {
			fileType = "application/octet-stream"
		}

		// Generate unique ID based on the full path
		fileId := GenerateFileID(path)

		fileInput := &types.FileInput{
			ID:       fileId,
			FileName: fileName,
			Size:     fileInfo.Size(),
			FileType: fileType,
		}

		// Calculate SHA256 if requested
		if calculateSHA {
			file, err := os.Open(path)
			if err != nil {
				DefaultLogger.Warnf("Skipping file %s: failed to open for hashing: %v", path, err)
				return nil
			}
			defer func() {
				if err := file.Close(); err != nil {
					DefaultLogger.Errorf("Failed to close file: %v", err)
				}
			}()

			hasher := sha256.New()
			if _, err := io.Copy(hasher, file); err != nil {
				DefaultLogger.Warnf("Skipping file %s: failed to calculate SHA256: %v", path, err)
				return nil
			}
			fileInput.SHA256 = hex.EncodeToString(hasher.Sum(nil))
		}

		fileInputMap[fileId] = fileInput
		fileIdToPathMap[fileId] = path

		DefaultLogger.Debugf("Processed file: %s -> %s (size: %d, type: %s)", path, fileName, fileInfo.Size(), fileType)
		return nil
	})

	if err != nil {
		return nil, nil, fmt.Errorf("failed to walk folder: %v", err)
	}

	if len(fileInputMap) == 0 {
		return nil, nil, fmt.Errorf("no files found in folder: %s", folderPath)
	}

	DefaultLogger.Infof("Processed folder %s: found %d files", folderPath, len(fileInputMap))
	return fileInputMap, fileIdToPathMap, nil
}

// GenerateFileID generates a unique file ID based on file path
func GenerateFileID(filePath string) string {
	hasher := sha256.New()
	hasher.Write([]byte(filePath))
	return hex.EncodeToString(hasher.Sum(nil))[:16]
}

// ProcessPathInput processes a path (file or folder) and returns file information.
// If path is a file, returns a single-item map.
// If path is a folder, returns all files in the folder with proper naming.
func ProcessPathInput(path string, calculateSHA bool) (map[string]*types.FileInput, map[string]string, error) {
	// Handle file:// URL
	if strings.HasPrefix(path, "file://") {
		parsedUrl, err := url.Parse(path)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid file URL: %v", err)
		}
		path = parsedUrl.Path
	}

	// Get file/folder info
	info, err := os.Stat(path)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to stat path: %v", err)
	}

	// If it's a file, process as single file
	if !info.IsDir() {
		fileName := filepath.Base(path)
		fileType := mime.TypeByExtension(filepath.Ext(path))
		if fileType == "" {
			fileType = "application/octet-stream"
		}

		fileId := GenerateFileID(path)
		fileInput := &types.FileInput{
			ID:       fileId,
			FileName: fileName,
			Size:     info.Size(),
			FileType: fileType,
		}

		if calculateSHA {
			file, err := os.Open(path)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to open file for hashing: %v", err)
			}
			defer func() {
				if err := file.Close(); err != nil {
					DefaultLogger.Errorf("Failed to close file: %v", err)
				}
			}()

			hasher := sha256.New()
			if _, err := io.Copy(hasher, file); err != nil {
				return nil, nil, fmt.Errorf("failed to calculate SHA256: %v", err)
			}
			fileInput.SHA256 = hex.EncodeToString(hasher.Sum(nil))
		}

		fileInputMap := map[string]*types.FileInput{fileId: fileInput}
		fileIdToPathMap := map[string]string{fileId: path}

		return fileInputMap, fileIdToPathMap, nil
	}

	// It's a directory, recursively collect all files
	return ProcessFolderForUpload(path, calculateSHA)
}
