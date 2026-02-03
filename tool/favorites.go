package tool

import (
	"os"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/moyoez/localsend-go/types"
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
		CurrentConfig.FavoriteDevices = append(CurrentConfig.FavoriteDevices, types.FavoriteDeviceEntry{
			Fingerprint: fingerprint,
			Alias:       alias,
		})
	}

	// Write back to config file
	return writeDefaultConfig(ConfigPath, CurrentConfig)
}

// ListFavorites returns a copy of the current favorite devices list.
func ListFavorites() []types.FavoriteDeviceEntry {
	favoritesMu.RLock()
	defer favoritesMu.RUnlock()

	// Return a copy to avoid race conditions
	result := make([]types.FavoriteDeviceEntry, len(CurrentConfig.FavoriteDevices))
	copy(result, CurrentConfig.FavoriteDevices)
	return result
}

// RemoveFavorite removes a device from favorites by fingerprint.
func RemoveFavorite(fingerprint string) error {
	favoritesMu.Lock()
	defer favoritesMu.Unlock()

	// Find and remove the entry
	newList := make([]types.FavoriteDeviceEntry, 0, len(CurrentConfig.FavoriteDevices))
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
	var favConfig types.FavoriteDevicesYamlFileConfig

	if err := yaml.Unmarshal(data, &favConfig); err != nil {
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
	for _, fav := range favConfig.FavoriteDevices {
		if fav.Fingerprint == fingerprint {
			return true
		}
	}
	return false
}
