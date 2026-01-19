package main

import (
	"github.com/charmbracelet/log"
	"github.com/moyoez/localsend-base-protocol-golang/api"
	"github.com/moyoez/localsend-base-protocol-golang/boardcast"
	"github.com/moyoez/localsend-base-protocol-golang/types"
)

// example device information (for broadcast and discovery demonstration)
const (
	alias       = "localsend-base-protocol-golang"
	version     = "2.0"
	deviceModel = "MacBook Pro"
	deviceType  = "headless"
	fingerprint = "1234567890"
	port        = 53317
	protocol    = "https"
	download    = false
	announce    = true
)

func main() {
	message := &types.VersionMessage{
		Alias:       alias,
		Version:     version,
		DeviceModel: deviceModel,
		DeviceType:  deviceType,
		Fingerprint: fingerprint,
		Port:        port,
		Protocol:    protocol,
		Download:    download,
		Announce:    announce,
	}
	log.SetLevel(log.DebugLevel)

	handler := api.NewDefaultHandler()

	apiServer := api.NewServer(port, protocol, handler)
	go func() {
		if err := apiServer.Start(); err != nil {
			log.Fatalf("API server startup failed: %v", err)
		}
	}()

	go boardcast.ListenMulticastUsingUDP(message)
	go boardcast.SendMulticastUsingUDP(message)

	select {}
}
