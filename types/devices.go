package types

type DeviceInfo struct {
	Alias       string `json:"alias"`
	Version     string `json:"version"`
	DeviceModel string `json:"deviceModel,omitempty"`
	DeviceType  string `json:"deviceType,omitempty"`
	Fingerprint string `json:"fingerprint"`
	Port        int    `json:"port"`
	Protocol    string `json:"protocol"`
	Download    bool   `json:"download,omitempty"`
}

// DeviceInfoReverseMode documents the fields relevant when a device acts as a receiver (reverse mode)
// This struct matches DeviceInfo, but the field notes are tailored for reverse (download) mode.
type DeviceInfoReverseMode struct {
	Alias       string `json:"alias"`                 // e.g. "Nice Orange"
	Version     string `json:"version"`               // protocol version (major.minor)
	DeviceModel string `json:"deviceModel,omitempty"` // Optional, e.g. "Samsung"
	DeviceType  string `json:"deviceType,omitempty"`  // Optional, e.g. "mobile", "desktop", "web", etc.
	Fingerprint string `json:"fingerprint"`           // Device identifier (ignored in HTTPS mode)
	Download    bool   `json:"download,omitempty"`    // If download API (5.2, 5.3) is active (optional, default: false)
}

// UserScanCurrentItem holds discovered device info with IP address
type UserScanCurrentItem struct {
	Ipaddress string `json:"ip_address"`
	VersionMessage
}

// SelfNetworkInfo represents the local device's network information
// including IP address and broadcast segment number
type SelfNetworkInfo struct {
	InterfaceName string `json:"interface_name"` // network interface name
	IPAddress     string `json:"ip_address"`     // ip address
	Number        string `json:"number"`         // number
	NumberInt     int    `json:"number_int"`     // number int
}

