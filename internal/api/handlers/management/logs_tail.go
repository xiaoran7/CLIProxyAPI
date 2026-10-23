package management

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

type logAccumulator struct {
	cutoff  int64
	limit   int
	lines   []string
	total   int
	latest  int64
	include bool
}

func newLogAccumulator(cutoff int64, limit int) *logAccumulator {
	capacity := 256
	if limit > 0 && limit < capacity {
		capacity = limit
	}
	return &logAccumulator{
		cutoff: cutoff,
		limit:  limit,
		lines:  make([]string, 0, capacity),
	}
}

func (acc *logAccumulator) consumeFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer func() {
		_ = file.Close()
	}()

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, logScannerInitialBuffer)
	scanner.Buffer(buf, logScannerMaxBuffer)
	for scanner.Scan() {
		acc.addLine(scanner.Text())
	}
	if errScan := scanner.Err(); errScan != nil {
		return errScan
	}
	return nil
}

func (acc *logAccumulator) addLine(raw string) {
	line := strings.TrimRight(raw, "\r")
	acc.total++
	ts := parseTimestamp(line)
	if ts > acc.latest {
		acc.latest = ts
	}
	if ts > 0 {
		acc.include = acc.cutoff == 0 || ts > acc.cutoff
		if acc.cutoff == 0 || acc.include {
			acc.append(line)
		}
		return
	}
	if acc.cutoff == 0 || acc.include {
		acc.append(line)
	}
}

func (acc *logAccumulator) append(line string) {
	acc.lines = append(acc.lines, line)
	if acc.limit > 0 && len(acc.lines) > acc.limit {
		acc.lines = acc.lines[len(acc.lines)-acc.limit:]
	}
}

func (acc *logAccumulator) result() ([]string, int, int64) {
	if acc.lines == nil {
		acc.lines = []string{}
	}
	return acc.lines, acc.total, acc.latest
}

type completeLogRead struct {
	lines     []string
	endOffset int64
	latest    int64
	hitLimit  bool
}

type logReadResult struct {
	lines      []string
	latest     int64
	nextCursor string
}

func tailLogFiles(files []string, limit int, fallbackLatest int64) (logReadResult, error) {
	result := logReadResult{
		lines:  []string{},
		latest: fallbackLatest,
	}
	for i := len(files) - 1; i >= 0; i-- {
		remaining := 0
		if limit > 0 {
			remaining = limit - len(result.lines)
			if remaining <= 0 {
				break
			}
		}
		read, errRead := readTailLogLines(files[i], remaining)
		if errRead != nil {
			if errors.Is(errRead, os.ErrNotExist) {
				continue
			}
			return logReadResult{}, errRead
		}
		if len(read.lines) == 0 {
			continue
		}
		result.lines = append(append([]string{}, read.lines...), result.lines...)
		if read.latest > result.latest {
			result.latest = read.latest
		}
	}
	nextCursor, errCursor := cursorForLatestLogFile(files, result.latest)
	if errCursor != nil {
		return logReadResult{}, errCursor
	}
	result.nextCursor = nextCursor
	return result, nil
}

func readTailLogLines(path string, limit int) (completeLogRead, error) {
	boundary, errBoundary := completeLogBoundary(path)
	if errBoundary != nil {
		return completeLogRead{}, errBoundary
	}
	if boundary == 0 {
		return completeLogRead{lines: []string{}}, nil
	}
	start, errStart := tailStartOffset(path, boundary, limit)
	if errStart != nil {
		return completeLogRead{}, errStart
	}
	return readCompleteLogLines(path, start, boundary, limit)
}

func tailStartOffset(path string, boundary int64, limit int) (int64, error) {
	if limit <= 0 {
		return 0, nil
	}
	file, errOpen := os.Open(path)
	if errOpen != nil {
		return 0, errOpen
	}
	defer func() {
		_ = file.Close()
	}()
	buf := make([]byte, 32*1024)
	pos := boundary
	lineBreaks := 0
	for pos > 0 {
		chunk := minInt64(int64(len(buf)), pos)
		pos -= chunk
		n, errRead := file.ReadAt(buf[:chunk], pos)
		if errRead != nil && errRead != io.EOF {
			return 0, errRead
		}
		if n <= 0 {
			continue
		}
		data := buf[:n]
		for len(data) > 0 {
			idx := bytes.LastIndexByte(data, '\n')
			if idx < 0 {
				break
			}
			lineBreaks++
			if lineBreaks > limit {
				return pos + int64(idx) + 1, nil
			}
			data = data[:idx]
		}
	}
	return 0, nil
}

func readCompleteLogLines(path string, offset, maxOffset int64, limit int) (completeLogRead, error) {
	if offset < 0 {
		return completeLogRead{}, fmt.Errorf("invalid log offset")
	}
	file, errOpen := os.Open(path)
	if errOpen != nil {
		return completeLogRead{}, errOpen
	}
	defer func() {
		_ = file.Close()
	}()
	info, errStat := file.Stat()
	if errStat != nil {
		return completeLogRead{}, errStat
	}
	if info.IsDir() {
		return completeLogRead{}, fmt.Errorf("invalid log file")
	}
	size := info.Size()
	if maxOffset < 0 || maxOffset > size {
		maxOffset = size
	}
	if offset > maxOffset {
		return completeLogRead{}, fmt.Errorf("invalid log offset")
	}

	reader := io.NewSectionReader(file, offset, maxOffset-offset)
	result := completeLogRead{
		lines:     []string{},
		endOffset: offset,
	}
	currentOffset := offset
	buf := make([]byte, 32*1024)
	line := make([]byte, 0, logScannerInitialBuffer)
	for {
		n, errRead := reader.Read(buf)
		if n > 0 {
			data := buf[:n]
			for len(data) > 0 {
				idx := bytes.IndexByte(data, '\n')
				if idx < 0 {
					if len(line)+len(data) > logScannerMaxBuffer {
						return completeLogRead{}, fmt.Errorf("log line exceeds %d bytes", logScannerMaxBuffer)
					}
					line = append(line, data...)
					currentOffset += int64(len(data))
					break
				}

				segment := data[:idx]
				if len(line)+len(segment) > logScannerMaxBuffer {
					return completeLogRead{}, fmt.Errorf("log line exceeds %d bytes", logScannerMaxBuffer)
				}
				line = append(line, segment...)
				currentOffset += int64(idx) + 1
				text := strings.TrimRight(string(line), "\r")
				result.lines = append(result.lines, text)
				result.endOffset = currentOffset
				if ts := parseTimestamp(text); ts > result.latest {
					result.latest = ts
				}
				line = line[:0]
				if limit > 0 && len(result.lines) >= limit {
					result.hitLimit = true
					return result, nil
				}
				data = data[idx+1:]
			}
		}
		if errRead == io.EOF {
			break
		}
		if errRead != nil {
			return completeLogRead{}, errRead
		}
	}
	return result, nil
}

func completeLogBoundary(path string) (int64, error) {
	file, errOpen := os.Open(path)
	if errOpen != nil {
		return 0, errOpen
	}
	defer func() {
		_ = file.Close()
	}()
	info, errStat := file.Stat()
	if errStat != nil {
		return 0, errStat
	}
	if info.IsDir() {
		return 0, fmt.Errorf("invalid log file")
	}
	size := info.Size()
	if size == 0 {
		return 0, nil
	}
	buf := make([]byte, 32*1024)
	pos := size
	for pos > 0 {
		chunk := minInt64(int64(len(buf)), pos)
		pos -= chunk
		n, errRead := file.ReadAt(buf[:chunk], pos)
		if errRead != nil && errRead != io.EOF {
			return 0, errRead
		}
		if n <= 0 {
			continue
		}
		if idx := bytes.LastIndexByte(buf[:n], '\n'); idx >= 0 {
			return pos + int64(idx) + 1, nil
		}
	}
	return 0, nil
}

func writeFileRange(dst io.Writer, file *os.File, start, length int64) error {
	if length <= 0 {
		return nil
	}
	buf := make([]byte, 32*1024)
	pos := start
	remaining := length
	for remaining > 0 {
		chunk := minInt64(int64(len(buf)), remaining)
		n, errRead := file.ReadAt(buf[:chunk], pos)
		if n > 0 {
			if _, errWrite := dst.Write(buf[:n]); errWrite != nil {
				return errWrite
			}
			pos += int64(n)
			remaining -= int64(n)
		}
		if errRead != nil {
			if errRead == io.EOF && remaining == 0 {
				return nil
			}
			return errRead
		}
	}
	return nil
}
