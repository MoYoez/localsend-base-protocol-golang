package types

type FileMetadata struct {
	Modified string `json:"modified,omitempty"`
	Accessed string `json:"accessed,omitempty"`
}

type FileInfo struct {
	ID       string        `json:"id"`
	FileName string        `json:"fileName"`
	Size     int64         `json:"size"`
	FileType string        `json:"fileType"`
	SHA256   string        `json:"sha256,omitempty"`
	Preview  string        `json:"preview,omitempty"`
	Metadata *FileMetadata `json:"metadata,omitempty"`
}
