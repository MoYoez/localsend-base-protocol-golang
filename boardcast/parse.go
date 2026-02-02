package boardcast

import (
	"fmt"
	"net"

	"github.com/bytedance/sonic"
	"github.com/moyoez/localsend-base-protocol-golang/types"
)

// CastToUDPAddr casts the address to a UDP address.
// Made public for reuse in other packages.
func CastToUDPAddr(addr net.Addr) (*net.UDPAddr, error) {
	udpAddr, ok := addr.(*net.UDPAddr)
	if !ok {
		return nil, fmt.Errorf("unexpected address type: %T", addr)
	}
	return udpAddr, nil
}

// ParseVersionMessageFromBody parses a VersionMessage from HTTP request body.
// Made public for reuse in API server.
func ParseVersionMessageFromBody(body []byte) (*types.VersionMessage, error) {
	var incoming types.VersionMessage
	if err := sonic.Unmarshal(body, &incoming); err != nil {
		return nil, fmt.Errorf("failed to parse version message: %v", err)
	}
	return &incoming, nil
}

// ParsePrepareUploadRequestFromBody parses a PrepareUploadRequest from HTTP request body.
// Made public for reuse in API server.
func ParsePrepareUploadRequestFromBody(body []byte) (*types.PrepareUploadRequest, error) {
	var request types.PrepareUploadRequest
	if err := sonic.Unmarshal(body, &request); err != nil {
		return nil, fmt.Errorf("failed to parse prepare-upload request: %v", err)
	}
	return &request, nil
}
