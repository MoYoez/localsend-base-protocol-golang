package types

type PrepareUploadRequest struct {
	Info  DeviceInfo          `json:"info"`
	Files map[string]FileInfo `json:"files"`
}

type PrepareUploadResponse struct {
	SessionId string            `json:"sessionId"`
	Files     map[string]string `json:"files"`
}

type ConfirmResult struct {
	Confirmed bool `json:"confirmed"`
}

// used in https://github.com/localsend/protocol/tree/main?tab=readme-ov-file#5-reverse-file-transfer-http-aka-download-api
type PrepareUploadReverseProxyResp struct {
	Info      DeviceInfoReverseMode `json:"info"`
	SessionId string                `json:"sessionId"`
	Files     map[string]FileInfo   `json:"files"`
}
