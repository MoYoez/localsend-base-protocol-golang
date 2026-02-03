package types

// FavoriteDeviceEntry represents a favorite device with only fingerprint and alias
type FavoriteDeviceEntry struct {
	Fingerprint string `yaml:"favorite_fingerprint" json:"favorite_fingerprint"`
	Alias       string `yaml:"favorite_alias" json:"favorite_alias"`
}

// FavoriteDevicesYamlFileConfig is used for YAML unmarshaling of favorites
type FavoriteDevicesYamlFileConfig struct {
	FavoriteDevices []FavoriteDeviceEntry `yaml:"favoriteDevices"`
}

// UserFavoritesAddRequest represents the request body for adding a favorite device
type UserFavoritesAddRequest struct {
	Fingerprint string `json:"favorite_fingerprint"`
	Alias       string `json:"favorite_alias"`
}
