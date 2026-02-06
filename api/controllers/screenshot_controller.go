package controllers

import (
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	ttlworker "github.com/FloatTech/ttl"
	"github.com/gin-gonic/gin"
	"github.com/moyoez/localsend-go/tool"
	"github.com/moyoez/localsend-go/types"
)

const (
	screenshotCacheKey   = "steam_screenshots"
	duplicateMinSizeKB   = 30
	duplicateMinSizeByte = duplicateMinSizeKB * 1024
)

var (
	screenshotsDir   = "760/remote"
	slashScreenshots = "/screenshots/"
	screenshotCache  = ttlworker.NewCache[string, []types.ScreenshotItem](time.Hour)
)

// GetUserScreenShot returns Steam screenshots with time filter and pagination.
// GET /api/self/v1/get-user-screenshot?page=1&pageSize=20&since=&until=
func GetUserScreenShot(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 500 {
		pageSize = 500
	}

	var sinceTs, untilTs *int64
	if s := c.Query("since"); s != "" {
		if v, err := strconv.ParseInt(s, 10, 64); err == nil {
			sinceTs = &v
		}
	}
	if u := c.Query("until"); u != "" {
		if v, err := strconv.ParseInt(u, 10, 64); err == nil {
			untilTs = &v
		}
	}
	refreshNow := strings.EqualFold(c.Query("refresh-now"), "1") || strings.EqualFold(c.Query("refresh-now"), "true")
	if refreshNow {
		screenshotCache.Delete(screenshotCacheKey)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		c.JSON(http.StatusInternalServerError, tool.FastReturnError("Failed to get home directory: "+err.Error()))
		return
	}
	steamUserdataRoot := filepath.Join(homeDir, ".local", "share", "Steam", "userdata")
	if _, err := os.Stat(steamUserdataRoot); err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusOK, tool.FastReturnSuccessWithData(types.ScreenshotListResponse{
				Screenshots: []types.ScreenshotItem{},
				Count:       0,
				Total:       0,
			}))
			return
		}
		c.JSON(http.StatusInternalServerError, tool.FastReturnError("Failed to access Steam userdata: "+err.Error()))
		return
	}

	var list []types.ScreenshotItem
	if !refreshNow {
		list = screenshotCache.Get(screenshotCacheKey)
	}
	if list == nil {
		err = filepath.WalkDir(steamUserdataRoot, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				tool.DefaultLogger.Warnf("WalkDir %s: %v", path, err)
				return nil
			}
			if d.IsDir() {
				return nil
			}
			base := filepath.Base(path)
			if !strings.HasSuffix(strings.ToLower(base), ".jpg") {
				return nil
			}
			pathSlash := filepath.ToSlash(path)
			idx := strings.Index(pathSlash, screenshotsDir)
			if idx == -1 {
				return nil
			}
			after := pathSlash[idx+len(screenshotsDir):]
			if !strings.Contains(after, slashScreenshots) {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				tool.DefaultLogger.Warnf("Stat %s: %v", path, err)
				return nil
			}
			mtime := info.ModTime().Unix()
			list = append(list, types.ScreenshotItem{
				Path:     path,
				Filename: base,
				Size:     info.Size(),
				Mtime:    mtime,
				MtimeStr: time.Unix(mtime, 0).Format("2006-01-02 15:04:05"),
			})
			return nil
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, tool.FastReturnError("Failed to scan screenshots: "+err.Error()))
			return
		}
		sort.Slice(list, func(i, j int) bool { return list[i].Mtime > list[j].Mtime })
		screenshotCache.Set(screenshotCacheKey, list)
	}

	var filtered []types.ScreenshotItem
	for _, item := range list {
		if sinceTs != nil && item.Mtime < *sinceTs {
			continue
		}
		if untilTs != nil && item.Mtime > *untilTs {
			continue
		}
		filtered = append(filtered, item)
	}

	// remove lower than duplicateMinSizeByte
	byName := make(map[string][]types.ScreenshotItem)
	for _, item := range filtered {
		byName[item.Filename] = append(byName[item.Filename], item)
	}
	filtered = filtered[:0]
	for _, items := range byName {
		if len(items) > 1 {
			for _, item := range items {
				if item.Size >= duplicateMinSizeByte {
					filtered = append(filtered, item)
				}
			}
		} else {
			filtered = append(filtered, items[0])
		}
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].Mtime > filtered[j].Mtime })

	total := len(filtered)
	offset := (page - 1) * pageSize
	if offset >= total {
		c.JSON(http.StatusOK, tool.FastReturnSuccessWithData(types.ScreenshotListResponse{
			Screenshots: []types.ScreenshotItem{},
			Count:       0,
			Total:       total,
		}))
		return
	}
	end := min(offset+pageSize, total)
	pageList := filtered[offset:end]

	c.JSON(http.StatusOK, tool.FastReturnSuccessWithData(types.ScreenshotListResponse{
		Screenshots: pageList,
		Count:       len(pageList),
		Total:       total,
	}))
}
