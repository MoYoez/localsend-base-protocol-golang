package types

/*
 UDP Multicast Message example

{
  "alias": "Nice Orange",
  "version": "2.0", // 协议版本（major.minor）
  "deviceModel": "Samsung", // nullable
  "deviceType": "mobile", // mobile | desktop | web | headless | server, nullable
  "fingerprint": "随机字符串",
  "port": 53317,
  "protocol": "https", // http | https
  "download": true, // 下载 API（5.2 和 5.3）是否激活（可选，默认为 false）
  "announce": true
}

*/

// VersionMessage is a unified device information container.
// Used as the payload for UDP multicast broadcast and /register requests.
type VersionMessage struct {
	Alias       string `json:"alias"`
	Version     string `json:"version"`
	DeviceModel string `json:"deviceModel"`
	DeviceType  string `json:"deviceType"`
	Fingerprint string `json:"fingerprint"`
	Port        int    `json:"port"`
	Protocol    string `json:"protocol"`
	Download    bool   `json:"download"`
	Announce    bool   `json:"announce"`
}
