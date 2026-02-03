package models

import (
	"github.com/moyoez/localsend-go/boardcast"
	"github.com/moyoez/localsend-go/types"
)

func ParsePrepareUploadRequest(body []byte) (*types.PrepareUploadRequest, error) {
	return boardcast.ParsePrepareUploadRequestFromBody(body)
}
