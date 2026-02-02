package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/moyoez/localsend-base-protocol-golang/tool"
)

// UserFavoritesAddRequest represents the request body for adding a favorite device
type UserFavoritesAddRequest struct {
	Fingerprint string `json:"favorite_fingerprint"`
	Alias       string `json:"favorite_alias"`
}

// UserFavoritesList returns the list of favorite devices.
// GET /api/self/v1/favorites
func UserFavoritesList(c *gin.Context) {
	favorites := tool.ListFavorites()
	c.JSON(http.StatusOK, tool.FastReturnSuccessWithData(favorites))
}

// UserFavoritesAdd adds a device to favorites.
// POST /api/self/v1/favorites
func UserFavoritesAdd(c *gin.Context) {
	var request UserFavoritesAddRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, tool.FastReturnError("Invalid request body: "+err.Error()))
		return
	}
	if request.Fingerprint == "" {
		c.JSON(http.StatusBadRequest, tool.FastReturnError("fingerprint is required"))
		return
	}
	if err := tool.AddFavorite(request.Fingerprint, request.Alias); err != nil {
		c.JSON(http.StatusInternalServerError, tool.FastReturnError("Failed to add favorite: "+err.Error()))
		return
	}
	c.JSON(http.StatusOK, tool.FastReturnSuccess())
}

// UserFavoritesDelete removes a device from favorites.
// DELETE /api/self/v1/favorites/:fingerprint
func UserFavoritesDelete(c *gin.Context) {
	fingerprint := c.Param("fingerprint")
	if fingerprint == "" {
		c.JSON(http.StatusBadRequest, tool.FastReturnError("fingerprint is required"))
		return
	}
	if err := tool.RemoveFavorite(fingerprint); err != nil {
		c.JSON(http.StatusInternalServerError, tool.FastReturnError("Failed to remove favorite: "+err.Error()))
		return
	}
	c.JSON(http.StatusOK, tool.FastReturnSuccess())
}
