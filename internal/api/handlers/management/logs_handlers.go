package management

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/logging"
)

const (
	defaultLogFileName      = "main.log"
	logScannerInitialBuffer = 64 * 1024
	logScannerMaxBuffer     = 8 * 1024 * 1024
	logCursorVersion        = 1
	logCursorFingerprintMax = 4 * 1024
)

// GetLogs returns log lines with optional incremental loading.
//
// The legacy timestamp path keeps line-count as the total scanned line count for
// compatibility. Cursor and tail reads avoid scanning older files, so line-count
// is the number of returned lines there. A cursor emitted by the legacy path
// points at the latest complete log boundary; combining after with limit is
// therefore tail semantics and does not replay lines trimmed by limit.
func (h *Handler) GetLogs(c *gin.Context) {
	if h == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "handler unavailable"})
		return
	}
	if h.cfg == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "configuration unavailable"})
		return
	}
	if !h.cfg.LoggingToFile {
		c.JSON(http.StatusBadRequest, gin.H{"error": "logging to file disabled"})
		return
	}

	logDir := h.logDirectory()
	if strings.TrimSpace(logDir) == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "log directory not configured"})
		return
	}

	rawCursor := strings.TrimSpace(c.Query("cursor"))
	files, err := h.collectLogFiles(logDir)
	if err != nil {
		if os.IsNotExist(err) {
			cutoff := parseCutoff(c.Query("after"))
			latest := cutoff
			if rawCursor != "" {
				if cursor, errCursor := decodeLogCursor(rawCursor); errCursor == nil && cursor.LatestTimestamp > latest {
					latest = cursor.LatestTimestamp
				}
			}
			writeLogsResponse(c, []string{}, 0, latest, "", rawCursor != "")
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to list log files: %v", err)})
		return
	}

	limit, errLimit := parseLimit(c.Query("limit"))
	if errLimit != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid limit: %v", errLimit)})
		return
	}

	cutoff := parseCutoff(c.Query("after"))
	if rawCursor != "" {
		result, reset, errCursor := readLogFilesFromCursor(logDir, files, rawCursor, limit)
		if errCursor != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to read log files: %v", errCursor)})
			return
		}
		if reset {
			result, errCursor = tailLogFiles(files, limit, result.latest)
			if errCursor != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to read log files: %v", errCursor)})
				return
			}
			writeLogsResponse(c, result.lines, len(result.lines), result.latest, result.nextCursor, true)
			return
		}
		writeLogsResponse(c, result.lines, len(result.lines), result.latest, result.nextCursor, false)
		return
	}

	if cutoff == 0 && limit > 0 {
		result, errTail := tailLogFiles(files, limit, 0)
		if errTail != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to read log files: %v", errTail)})
			return
		}
		writeLogsResponse(c, result.lines, len(result.lines), result.latest, result.nextCursor, false)
		return
	}

	acc := newLogAccumulator(cutoff, limit)
	for i := range files {
		if errProcess := acc.consumeFile(files[i]); errProcess != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to read log file: %v", errProcess)})
			return
		}
	}

	lines, total, latest := acc.result()
	if latest == 0 || latest < cutoff {
		latest = cutoff
	}
	nextCursor, errCursor := cursorForLatestLogFile(files, latest)
	if errCursor != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to prepare log cursor: %v", errCursor)})
		return
	}
	writeLogsResponse(c, lines, total, latest, nextCursor, false)
}

// DeleteLogs removes all rotated log files and truncates the active log.
func (h *Handler) DeleteLogs(c *gin.Context) {
	if h == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "handler unavailable"})
		return
	}
	if h.cfg == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "configuration unavailable"})
		return
	}
	if !h.cfg.LoggingToFile {
		c.JSON(http.StatusBadRequest, gin.H{"error": "logging to file disabled"})
		return
	}

	dir := h.logDirectory()
	if strings.TrimSpace(dir) == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "log directory not configured"})
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "log directory not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to list log directory: %v", err)})
		return
	}

	removed := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		fullPath := filepath.Join(dir, name)
		if name == defaultLogFileName {
			if errTrunc := os.Truncate(fullPath, 0); errTrunc != nil && !os.IsNotExist(errTrunc) {
				c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to truncate log file: %v", errTrunc)})
				return
			}
			continue
		}
		if isRotatedLogFile(name) {
			if errRemove := os.Remove(fullPath); errRemove != nil && !os.IsNotExist(errRemove) {
				c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to remove %s: %v", name, errRemove)})
				return
			}
			removed++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Logs cleared successfully",
		"removed": removed,
	})
}

// GetRequestErrorLogs lists error request log files when RequestLog is disabled.
// It returns an empty list when RequestLog is enabled.
func (h *Handler) GetRequestErrorLogs(c *gin.Context) {
	if h == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "handler unavailable"})
		return
	}
	if h.cfg == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "configuration unavailable"})
		return
	}
	if h.cfg.RequestLog {
		c.JSON(http.StatusOK, gin.H{"files": []any{}})
		return
	}

	dir := h.logDirectory()
	if strings.TrimSpace(dir) == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "log directory not configured"})
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusOK, gin.H{"files": []any{}})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to list request error logs: %v", err)})
		return
	}

	type errorLog struct {
		Name     string `json:"name"`
		Size     int64  `json:"size"`
		Modified int64  `json:"modified"`
	}

	files := make([]errorLog, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "error-") || !strings.HasSuffix(name, ".log") {
			continue
		}
		info, errInfo := entry.Info()
		if errInfo != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to read log info for %s: %v", name, errInfo)})
			return
		}
		files = append(files, errorLog{
			Name:     name,
			Size:     info.Size(),
			Modified: info.ModTime().Unix(),
		})
	}

	sort.Slice(files, func(i, j int) bool { return files[i].Modified > files[j].Modified })

	c.JSON(http.StatusOK, gin.H{"files": files})
}

// GetRequestLogByID finds and downloads a request log file by its request ID.
// The ID is matched against the suffix of log file names (format: *-{requestID}.log).
func (h *Handler) GetRequestLogByID(c *gin.Context) {
	if h == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "handler unavailable"})
		return
	}
	if h.cfg == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "configuration unavailable"})
		return
	}

	dir := h.logDirectory()
	if strings.TrimSpace(dir) == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "log directory not configured"})
		return
	}

	requestID := strings.TrimSpace(c.Param("id"))
	if requestID == "" {
		requestID = strings.TrimSpace(c.Query("id"))
	}
	if requestID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing request ID"})
		return
	}
	if strings.ContainsAny(requestID, "/\\") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request ID"})
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "log directory not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to list log directory: %v", err)})
		return
	}

	suffix := "-" + logging.ShortRequestID(requestID) + ".log"
	var matchedFile string
	var latestModTime time.Time
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, suffix) {
			info, errInfo := entry.Info()
			if errInfo != nil {
				if matchedFile == "" {
					matchedFile = name
				}
				continue
			}
			if matchedFile == "" || logFileIsNewer(name, info.ModTime(), matchedFile, latestModTime) {
				matchedFile = name
				latestModTime = info.ModTime()
			}
		}
	}

	if matchedFile == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "log file not found for the given request ID"})
		return
	}

	dirAbs, errAbs := filepath.Abs(dir)
	if errAbs != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to resolve log directory: %v", errAbs)})
		return
	}
	fullPath := filepath.Clean(filepath.Join(dirAbs, matchedFile))
	prefix := dirAbs + string(os.PathSeparator)
	if !strings.HasPrefix(fullPath, prefix) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid log file path"})
		return
	}

	info, errStat := os.Stat(fullPath)
	if errStat != nil {
		if os.IsNotExist(errStat) {
			c.JSON(http.StatusNotFound, gin.H{"error": "log file not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to read log file: %v", errStat)})
		return
	}
	if info.IsDir() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid log file"})
		return
	}

	c.FileAttachment(fullPath, matchedFile)
}

// DownloadRequestErrorLog downloads a specific error request log file by name.
func (h *Handler) DownloadRequestErrorLog(c *gin.Context) {
	if h == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "handler unavailable"})
		return
	}
	if h.cfg == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "configuration unavailable"})
		return
	}

	dir := h.logDirectory()
	if strings.TrimSpace(dir) == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "log directory not configured"})
		return
	}

	name := strings.TrimSpace(c.Param("name"))
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid log file name"})
		return
	}
	if !strings.HasPrefix(name, "error-") || !strings.HasSuffix(name, ".log") {
		c.JSON(http.StatusNotFound, gin.H{"error": "log file not found"})
		return
	}

	dirAbs, errAbs := filepath.Abs(dir)
	if errAbs != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to resolve log directory: %v", errAbs)})
		return
	}
	fullPath := filepath.Clean(filepath.Join(dirAbs, name))
	prefix := dirAbs + string(os.PathSeparator)
	if !strings.HasPrefix(fullPath, prefix) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid log file path"})
		return
	}

	info, errStat := os.Stat(fullPath)
	if errStat != nil {
		if os.IsNotExist(errStat) {
			c.JSON(http.StatusNotFound, gin.H{"error": "log file not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to read log file: %v", errStat)})
		return
	}
	if info.IsDir() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid log file"})
		return
	}

	c.FileAttachment(fullPath, name)
}
