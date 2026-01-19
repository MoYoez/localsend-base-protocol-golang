package models

import (
	"github.com/moyoez/localsend-base-protocol-golang/boardcast"
	"github.com/moyoez/localsend-base-protocol-golang/types"
)

func ParsePrepareUploadRequest(body []byte) (*types.PrepareUploadRequest, error) {
	return boardcast.ParsePrepareUploadRequestFromBody(body)
}
