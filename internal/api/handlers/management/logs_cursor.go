package management

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type logCursor struct {
	Version         int    `json:"v"`
	File            string `json:"file"`
	Offset          int64  `json:"offset"`
	Size            int64  `json:"size"`
	ModTime         int64  `json:"modTime"`
	ModTimeUnixNano int64  `json:"modTimeUnixNano,omitempty"`
	LatestTimestamp int64  `json:"latestTimestamp"`
	Fingerprint     string `json:"fingerprint"`
}

func writeLogsResponse(c *gin.Context, lines []string, lineCount int, latest int64, nextCursor string, cursorReset bool) {
	if lines == nil {
		lines = []string{}
	}
	payload := gin.H{
		"lines":            lines,
		"line-count":       lineCount,
		"latest-timestamp": latest,
		"next-cursor":      nextCursor,
	}
	if cursorReset {
		payload["cursor-reset"] = true
	}
	c.JSON(http.StatusOK, payload)
}

func cursorForLatestLogFile(files []string, latest int64) (string, error) {
	for i := len(files) - 1; i >= 0; i-- {
		boundary, errBoundary := completeLogBoundary(files[i])
		if errBoundary != nil {
			if errors.Is(errBoundary, os.ErrNotExist) {
				continue
			}
			return "", errBoundary
		}
		cursor, errCursor := newLogCursor(files[i], boundary, latest)
		if errCursor != nil {
			if errors.Is(errCursor, os.ErrNotExist) {
				continue
			}
			return "", errCursor
		}
		return cursor, nil
	}
	return "", nil
}

func readLogFilesFromCursor(logDir string, files []string, raw string, limit int) (logReadResult, bool, error) {
	cursor, errDecode := decodeLogCursor(raw)
	if errDecode != nil {
		return logReadResult{lines: []string{}}, true, nil
	}
	result := logReadResult{
		lines:      []string{},
		latest:     cursor.LatestTimestamp,
		nextCursor: raw,
	}
	if _, errPath := safeLogFilePath(logDir, cursor.File); errPath != nil {
		return result, true, nil
	}
	startIndex, found, errLocate := locateLogCursorFile(files, cursor)
	if errLocate != nil {
		return result, false, errLocate
	}
	if !found {
		return result, true, nil
	}

	currentCursorPath := files[startIndex]
	currentCursorOffset := cursor.Offset
	advanced := false
	for i := startIndex; i < len(files); i++ {
		remaining := 0
		if limit > 0 {
			remaining = limit - len(result.lines)
			if remaining <= 0 {
				break
			}
		}
		offset := int64(0)
		if i == startIndex {
			offset = cursor.Offset
		}
		read, errRead := readCompleteLogLines(files[i], offset, -1, remaining)
		if errRead != nil {
			if errors.Is(errRead, os.ErrNotExist) {
				return result, true, nil
			}
			return result, false, errRead
		}
		if len(read.lines) > 0 {
			result.lines = append(result.lines, read.lines...)
			if read.latest > result.latest {
				result.latest = read.latest
			}
			currentCursorPath = files[i]
			currentCursorOffset = read.endOffset
			advanced = true
		}
		if read.hitLimit {
			break
		}
	}
	if !advanced {
		return result, false, nil
	}

	nextCursor, errCursor := newLogCursor(currentCursorPath, currentCursorOffset, result.latest)
	if errCursor != nil {
		if errors.Is(errCursor, os.ErrNotExist) {
			return result, true, nil
		}
		return result, false, errCursor
	}
	result.nextCursor = nextCursor
	return result, false, nil
}

func locateLogCursorFile(files []string, cursor logCursor) (int, bool, error) {
	nameToIndex := make(map[string]int, len(files))
	for i := range files {
		nameToIndex[filepath.Base(files[i])] = i
	}
	deferEmptyMainMatch := false
	if index, ok := nameToIndex[cursor.File]; ok {
		matches, truncated, errMatch := logFileMatchesCursor(files[index], cursor)
		if errMatch != nil {
			if errors.Is(errMatch, os.ErrNotExist) {
				return 0, false, nil
			}
			return 0, false, errMatch
		}
		if matches && !truncated {
			if shouldDeferEmptyMainCursorToRotated(files, cursor) {
				deferEmptyMainMatch = true
			} else if shouldResetAmbiguousEmptyMainCursor(files, index, cursor) {
				return 0, false, nil
			} else {
				return index, true, nil
			}
		}
	}

	if cursor.File != defaultLogFileName || (cursor.Offset == 0 && cursor.Size == 0 && !deferEmptyMainMatch) {
		return 0, false, nil
	}
	if cursor.Offset == 0 && cursor.Size == 0 {
		for i := range files {
			if filepath.Base(files[i]) == defaultLogFileName {
				continue
			}
			if !logFileChangedAfterCursor(files[i], cursor) {
				continue
			}
			matches, truncated, errMatch := logFileMatchesCursor(files[i], cursor)
			if errMatch != nil {
				if errors.Is(errMatch, os.ErrNotExist) {
					continue
				}
				return 0, false, errMatch
			}
			if truncated {
				continue
			}
			if matches {
				return i, true, nil
			}
		}
		return 0, false, nil
	}
	for i := len(files) - 1; i >= 0; i-- {
		if filepath.Base(files[i]) == defaultLogFileName {
			continue
		}
		matches, truncated, errMatch := logFileMatchesCursor(files[i], cursor)
		if errMatch != nil {
			if errors.Is(errMatch, os.ErrNotExist) {
				continue
			}
			return 0, false, errMatch
		}
		if truncated {
			continue
		}
		if matches {
			return i, true, nil
		}
	}
	return 0, false, nil
}

func shouldDeferEmptyMainCursorToRotated(files []string, cursor logCursor) bool {
	if cursor.File != defaultLogFileName || cursor.Offset != 0 || cursor.Size != 0 {
		return false
	}
	for i := range files {
		if filepath.Base(files[i]) == defaultLogFileName {
			continue
		}
		if logFileChangedAfterCursor(files[i], cursor) {
			return true
		}
	}
	return false
}

func shouldResetAmbiguousEmptyMainCursor(files []string, mainIndex int, cursor logCursor) bool {
	if cursor.File != defaultLogFileName || cursor.Offset != 0 || cursor.Size != 0 {
		return false
	}
	info, errStat := os.Stat(files[mainIndex])
	if errStat != nil || info.IsDir() {
		return false
	}
	if info.Size() == cursor.Size && info.ModTime().UnixNano() == cursorModTimeUnixNano(cursor) {
		return false
	}
	for i := range files {
		if i == mainIndex || filepath.Base(files[i]) == defaultLogFileName {
			continue
		}
		rotatedInfo, errRotated := os.Stat(files[i])
		if errRotated != nil || rotatedInfo.IsDir() || rotatedInfo.Size() == 0 {
			continue
		}
		if !logFileChangedAfterCursor(files[i], cursor) {
			return true
		}
	}
	return false
}

func logFileChangedAfterCursor(path string, cursor logCursor) bool {
	info, errStat := os.Stat(path)
	if errStat != nil || info.IsDir() || info.Size() == 0 {
		return false
	}
	return info.ModTime().UnixNano() > cursorModTimeUnixNano(cursor)
}

func logFileMatchesCursor(path string, cursor logCursor) (bool, bool, error) {
	info, errStat := os.Stat(path)
	if errStat != nil {
		return false, false, errStat
	}
	if info.IsDir() {
		return false, false, fmt.Errorf("invalid log file")
	}
	if info.Size() < cursor.Offset {
		return false, true, nil
	}
	boundary := cursorFingerprintBoundary(cursor)
	if info.Size() < boundary {
		return false, true, nil
	}
	fingerprint, errFingerprint := logFileFingerprint(path, boundary)
	if errFingerprint != nil {
		return false, false, errFingerprint
	}
	return fingerprint == cursor.Fingerprint, false, nil
}

func encodeLogCursor(cursor logCursor) (string, error) {
	raw, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func decodeLogCursor(raw string) (logCursor, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return logCursor{}, fmt.Errorf("empty cursor")
	}
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		data, err = base64.URLEncoding.DecodeString(value)
	}
	if err != nil {
		return logCursor{}, fmt.Errorf("invalid cursor encoding")
	}
	var cursor logCursor
	if errUnmarshal := json.Unmarshal(data, &cursor); errUnmarshal != nil {
		return logCursor{}, fmt.Errorf("invalid cursor payload")
	}
	if errValidate := validateLogCursor(cursor); errValidate != nil {
		return logCursor{}, errValidate
	}
	return cursor, nil
}

func validateLogCursor(cursor logCursor) error {
	if cursor.Version != logCursorVersion {
		return fmt.Errorf("unsupported cursor version")
	}
	if !isAllowedLogCursorFile(cursor.File) {
		return fmt.Errorf("invalid cursor file")
	}
	if cursor.Offset < 0 || cursor.Size < 0 || cursor.ModTime < 0 || cursor.LatestTimestamp < 0 {
		return fmt.Errorf("invalid cursor position")
	}
	if strings.TrimSpace(cursor.Fingerprint) == "" {
		return fmt.Errorf("invalid cursor fingerprint")
	}
	return nil
}

func isAllowedLogCursorFile(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	if strings.ContainsAny(name, `/\`) {
		return false
	}
	if filepath.Base(name) != name {
		return false
	}
	return name == defaultLogFileName || isRotatedLogFile(name)
}

func safeLogFilePath(logDir, name string) (string, error) {
	if !isAllowedLogCursorFile(name) {
		return "", fmt.Errorf("invalid log file")
	}
	dirAbs, errAbs := filepath.Abs(logDir)
	if errAbs != nil {
		return "", fmt.Errorf("resolve log directory: %w", errAbs)
	}
	dirAbs = filepath.Clean(dirAbs)
	fullPath := filepath.Clean(filepath.Join(dirAbs, name))
	rel, errRel := filepath.Rel(dirAbs, fullPath)
	if errRel != nil {
		return "", fmt.Errorf("resolve log file: %w", errRel)
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || rel == ".." || filepath.IsAbs(rel) {
		return "", fmt.Errorf("invalid log file")
	}
	return fullPath, nil
}

func newLogCursor(path string, offset, latest int64) (string, error) {
	info, errStat := os.Stat(path)
	if errStat != nil {
		return "", errStat
	}
	if info.IsDir() {
		return "", fmt.Errorf("invalid log file")
	}
	if offset < 0 || offset > info.Size() {
		return "", fmt.Errorf("invalid cursor offset")
	}
	fingerprintCursor := logCursor{
		Offset: offset,
		Size:   info.Size(),
	}
	fingerprint, errFingerprint := logFileFingerprint(path, cursorFingerprintBoundary(fingerprintCursor))
	if errFingerprint != nil {
		return "", errFingerprint
	}
	return encodeLogCursor(logCursor{
		Version:         logCursorVersion,
		File:            filepath.Base(path),
		Offset:          offset,
		Size:            info.Size(),
		ModTime:         info.ModTime().Unix(),
		ModTimeUnixNano: info.ModTime().UnixNano(),
		LatestTimestamp: latest,
		Fingerprint:     fingerprint,
	})
}

func cursorFingerprintBoundary(cursor logCursor) int64 {
	if cursor.Offset == 0 && cursor.Size > 0 {
		return cursor.Size
	}
	return cursor.Offset
}

func cursorModTimeUnixNano(cursor logCursor) int64 {
	if cursor.ModTimeUnixNano > 0 {
		return cursor.ModTimeUnixNano
	}
	return cursor.ModTime * int64(time.Second)
}

func logFileFingerprint(path string, boundary int64) (string, error) {
	if boundary < 0 {
		return "", fmt.Errorf("invalid fingerprint boundary")
	}
	file, errOpen := os.Open(path)
	if errOpen != nil {
		return "", errOpen
	}
	defer func() {
		_ = file.Close()
	}()
	info, errStat := file.Stat()
	if errStat != nil {
		return "", errStat
	}
	if info.IsDir() {
		return "", fmt.Errorf("invalid log file")
	}
	if boundary > info.Size() {
		return "", fmt.Errorf("invalid fingerprint boundary")
	}

	hash := sha256.New()
	_, _ = fmt.Fprintf(hash, "log-cursor-v1:%d:", boundary)
	firstLen := minInt64(boundary, logCursorFingerprintMax)
	if errRead := writeFileRange(hash, file, 0, firstLen); errRead != nil {
		return "", errRead
	}
	tailLen := minInt64(boundary, logCursorFingerprintMax)
	tailStart := boundary - tailLen
	_, _ = fmt.Fprintf(hash, ":%d:", tailStart)
	if errRead := writeFileRange(hash, file, tailStart, tailLen); errRead != nil {
		return "", errRead
	}
	sum := hash.Sum(nil)
	return base64.RawURLEncoding.EncodeToString(sum[:12]), nil
}
