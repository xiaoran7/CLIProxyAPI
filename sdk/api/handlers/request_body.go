package handlers

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"sync"

	"github.com/andybalholm/brotli"
	"github.com/gin-gonic/gin"
	"github.com/klauspost/compress/zstd"
)

var (
	gzipReaderPool   sync.Pool
	brotliReaderPool sync.Pool
	zstdDecoderPool  sync.Pool
)

func getGzipReader(r io.Reader) (*gzip.Reader, error) {
	if v := gzipReaderPool.Get(); v != nil {
		zr := v.(*gzip.Reader)
		if err := zr.Reset(r); err != nil {
			return nil, err
		}
		return zr, nil
	}
	return gzip.NewReader(r)
}

func putGzipReader(zr *gzip.Reader) {
	if zr != nil {
		_ = zr.Close()
		gzipReaderPool.Put(zr)
	}
}

func getBrotliReader(r io.Reader) (*brotli.Reader, error) {
	if v := brotliReaderPool.Get(); v != nil {
		br := v.(*brotli.Reader)
		if err := br.Reset(r); err != nil {
			return nil, err
		}
		return br, nil
	}
	return brotli.NewReader(r), nil
}

func putBrotliReader(br *brotli.Reader) {
	if br != nil {
		brotliReaderPool.Put(br)
	}
}

func getZstdDecoder(r io.Reader) (*zstd.Decoder, error) {
	if v := zstdDecoderPool.Get(); v != nil {
		d := v.(*zstd.Decoder)
		if err := d.Reset(r); err != nil {
			return nil, err
		}
		return d, nil
	}
	return zstd.NewReader(r, zstd.WithDecoderConcurrency(1))
}

func putZstdDecoder(d *zstd.Decoder) {
	if d != nil {
		zstdDecoderPool.Put(d)
	}
}

// MaxRequestBodyBytesContextKey stores the configured request body size limit (in bytes) on the Gin context.
const MaxRequestBodyBytesContextKey = "maxRequestBodyBytes"

// ReadRequestBody reads the incoming request body and decodes supported
// Content-Encoding values before handlers inspect JSON fields.
func ReadRequestBody(c *gin.Context) ([]byte, error) {
	maxBytes := int64(0)
	if c != nil {
		maxBytes = c.GetInt64(MaxRequestBodyBytesContextKey)
	}
	raw, err := c.GetRawData()
	if err != nil {
		return nil, err
	}

	encoding := ""
	if c != nil && c.Request != nil {
		encoding = strings.TrimSpace(c.Request.Header.Get("Content-Encoding"))
	}
	if encoding == "" || strings.EqualFold(encoding, "identity") {
		return raw, nil
	}

	decoded, err := decodeRequestBody(raw, encoding, maxBytes)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return nil, err
		}
		if json.Valid(raw) {
			return raw, nil
		}
		return nil, err
	}
	return decoded, nil
}

func decodeRequestBody(raw []byte, encoding string, maxBytes int64) ([]byte, error) {
	parts := strings.Split(encoding, ",")
	body := raw
	for i := len(parts) - 1; i >= 0; i-- {
		enc := strings.ToLower(strings.TrimSpace(parts[i]))
		switch enc {
		case "", "identity":
			continue
		case "gzip", "x-gzip":
			decoded, err := decodeGzipRequestBody(body, maxBytes)
			if err != nil {
				return nil, err
			}
			body = decoded
		case "br":
			decoded, err := decodeBrotliRequestBody(body, maxBytes)
			if err != nil {
				return nil, err
			}
			body = decoded
		case "deflate":
			decoded, err := decodeDeflateRequestBody(body, maxBytes)
			if err != nil {
				return nil, err
			}
			body = decoded
		case "zstd":
			decoded, err := decodeZstdRequestBody(body, maxBytes)
			if err != nil {
				return nil, err
			}
			body = decoded
		default:
			return nil, fmt.Errorf("unsupported request content encoding: %s", enc)
		}
	}
	return body, nil
}

func decodeGzipRequestBody(raw []byte, maxBytes int64) ([]byte, error) {
	reader, err := getGzipReader(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip request decoder: %w", err)
	}
	defer putGzipReader(reader)

	var r io.Reader = reader
	if maxBytes > 0 && maxBytes < math.MaxInt64 {
		r = io.LimitReader(reader, maxBytes+1)
	}
	decoded, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to decode gzip request body: %w", err)
	}
	if maxBytes > 0 && int64(len(decoded)) > maxBytes {
		return nil, &http.MaxBytesError{Limit: maxBytes}
	}
	return decoded, nil
}

func decodeBrotliRequestBody(raw []byte, maxBytes int64) ([]byte, error) {
	reader, err := getBrotliReader(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("failed to create brotli request decoder: %w", err)
	}
	defer putBrotliReader(reader)

	var r io.Reader = reader
	if maxBytes > 0 && maxBytes < math.MaxInt64 {
		r = io.LimitReader(reader, maxBytes+1)
	}
	decoded, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to decode brotli request body: %w", err)
	}
	if maxBytes > 0 && int64(len(decoded)) > maxBytes {
		return nil, &http.MaxBytesError{Limit: maxBytes}
	}
	return decoded, nil
}

func decodeDeflateRequestBody(raw []byte, maxBytes int64) ([]byte, error) {
	zlibReader, err := zlib.NewReader(bytes.NewReader(raw))
	if err != nil {
		flateReader := flate.NewReader(bytes.NewReader(raw))
		defer func() { _ = flateReader.Close() }()
		var r io.Reader = flateReader
		if maxBytes > 0 && maxBytes < math.MaxInt64 {
			r = io.LimitReader(flateReader, maxBytes+1)
		}
		decoded, readErr := io.ReadAll(r)
		if readErr != nil {
			return nil, fmt.Errorf("failed to decode deflate request body: %w", err)
		}
		if maxBytes > 0 && int64(len(decoded)) > maxBytes {
			return nil, &http.MaxBytesError{Limit: maxBytes}
		}
		return decoded, nil
	}
	defer func() { _ = zlibReader.Close() }()

	var r io.Reader = zlibReader
	if maxBytes > 0 && maxBytes < math.MaxInt64 {
		r = io.LimitReader(zlibReader, maxBytes+1)
	}
	decoded, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to decode deflate request body: %w", err)
	}
	if maxBytes > 0 && int64(len(decoded)) > maxBytes {
		return nil, &http.MaxBytesError{Limit: maxBytes}
	}
	return decoded, nil
}

func decodeZstdRequestBody(raw []byte, maxBytes int64) ([]byte, error) {
	decoder, err := getZstdDecoder(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("failed to create zstd request decoder: %w", err)
	}
	defer putZstdDecoder(decoder)

	var reader io.Reader = decoder
	if maxBytes > 0 && maxBytes < math.MaxInt64 {
		reader = io.LimitReader(decoder, maxBytes+1)
	}
	decoded, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to decode zstd request body: %w", err)
	}
	if maxBytes > 0 && int64(len(decoded)) > maxBytes {
		return nil, &http.MaxBytesError{Limit: maxBytes}
	}
	return decoded, nil
}
