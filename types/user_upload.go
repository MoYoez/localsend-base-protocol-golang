package types

// UserPrepareUploadRequest represents the prepare upload request
type UserPrepareUploadRequest struct {
	TargetTo              string               `json:"targetTo"`
	Files                 map[string]FileInput `json:"files,omitempty"`
	UseFolderUpload       bool                 `json:"useFolderUpload,omitempty"`
	FolderPath            string               `json:"folderPath,omitempty"`
	UseFastSender         bool                 `json:"useFastSender,omitempty"`
	UseFastSenderIPSuffex string               `json:"useFastSenderIPSuffex,omitempty"`
	UseFastSenderIp       string               `json:"useFastSenderIp,omitempty"`
}

// UserUploadRequest represents the actual upload request
type UserUploadRequest struct {
	SessionId string `json:"sessionId"`
	FileId    string `json:"fileId"`
	Token     string `json:"token"`
	FileUrl   string `json:"fileUrl"`
}

// UserUploadBatchRequest represents batch upload request
type UserUploadBatchRequest struct {
	SessionId       string             `json:"sessionId"`
	Files           []UserUploadFileItem `json:"files,omitempty"`
	UseFolderUpload bool               `json:"useFolderUpload,omitempty"`
	FolderPath      string             `json:"folderPath,omitempty"`
}

// UserUploadFileItem represents a single file in batch upload
type UserUploadFileItem struct {
	FileId  string `json:"fileId"`
	Token   string `json:"token"`
	FileUrl string `json:"fileUrl"`
}

// UserUploadBatchResult represents the result of a batch upload operation
type UserUploadBatchResult struct {
	Total   int                   `json:"total"`
	Success int                   `json:"success"`
	Failed  int                   `json:"failed"`
	Results []UserUploadItemResult `json:"results"`
}

// UserUploadItemResult represents the result of a single file upload
type UserUploadItemResult struct {
	FileId  string `json:"fileId"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// UserUploadSession stores user upload session information
type UserUploadSession struct {
	Target    UserScanCurrentItem
	SessionId string
	Tokens    map[string]string
}
