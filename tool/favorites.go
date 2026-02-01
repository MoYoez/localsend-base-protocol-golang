package tool

import (
	"os"
	"sync"

	"gopkg.in/yaml.v3"
)

var favoritesMu sync.RWMutex

// AddFavorite adds a device to favorites by fingerprint and alias.
// If the fingerprint already exists, the alias will be updated.
func AddFavorite(fingerprint, alias string) error {
	favoritesMu.Lock()
	defer favoritesMu.Unlock()

	// Check if already exists, update alias if so
	found := false
	for i, fav := range CurrentConfig.FavoriteDevices {
		if fav.Fingerprint == fingerprint {
			CurrentConfig.FavoriteDevices[i].Alias = alias
			found = true
			break
		}
	}

	// Add new entry if not found
	if !found {
		CurrentConfig.FavoriteDevices = append(CurrentConfig.FavoriteDevices, FavoriteDeviceEntry{
			Fingerprint: fingerprint,
			Alias:       alias,
		})
	}

	// Write back to config file
	return writeDefaultConfig(ConfigPath, CurrentConfig)
}

// ListFavorites returns a copy of the current favorite devices list.
func ListFavorites() []FavoriteDeviceEntry {
	favoritesMu.RLock()
	defer favoritesMu.RUnlock()

	// Return a copy to avoid race conditions
	result := make([]FavoriteDeviceEntry, len(CurrentConfig.FavoriteDevices))
	copy(result, CurrentConfig.FavoriteDevices)
	return result
}

// RemoveFavorite removes a device from favorites by fingerprint.
func RemoveFavorite(fingerprint string) error {
	favoritesMu.Lock()
	defer favoritesMu.Unlock()

	// Find and remove the entry
	newList := make([]FavoriteDeviceEntry, 0, len(CurrentConfig.FavoriteDevices))
	for _, fav := range CurrentConfig.FavoriteDevices {
		if fav.Fingerprint != fingerprint {
			newList = append(newList, fav)
		}
	}
	CurrentConfig.FavoriteDevices = newList

	// Write back to config file
	return writeDefaultConfig(ConfigPath, CurrentConfig)
}

// IsFavorite checks if a device with the given fingerprint is in favorites.
// This function reads the config file in real-time to ensure up-to-date state.
func IsFavorite(fingerprint string) bool {
	favoritesMu.RLock()
	defer favoritesMu.RUnlock()

	// Read config file in real-time
	data, err := os.ReadFile(ConfigPath)
	if err != nil {
		DefaultLogger.Debugf("IsFavorite: failed to read config file: %v, falling back to memory", err)
		// Fallback to in-memory check
		for _, fav := range CurrentConfig.FavoriteDevices {
			if fav.Fingerprint == fingerprint {
				return true
			}
		}
		return false
	}

	// Parse only the favoriteDevices field
	var cfg struct {
		FavoriteDevices []FavoriteDeviceEntry `yaml:"favoriteDevices"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		DefaultLogger.Debugf("IsFavorite: failed to parse config file: %v, falling back to memory", err)
		// Fallback to in-memory check
		for _, fav := range CurrentConfig.FavoriteDevices {
			if fav.Fingerprint == fingerprint {
				return true
			}
		}
		return false
	}

	// Check if fingerprint exists in favorites
	for _, fav := range cfg.FavoriteDevices {
		if fav.Fingerprint == fingerprint {
			return true
		}
	}
	return false
}
