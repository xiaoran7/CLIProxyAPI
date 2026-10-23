package zcode

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

// APIError is a failed ZCode, Z.ai, or BigModel JSON call.
type APIError struct {
	// Status is the HTTP status the endpoint answered with.
	Status int
	// Code is the envelope's code, when the body carried one.
	Code string
	// Message is the endpoint's own wording, or the status text.
	Message string
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	if e.Code != "" {
		return fmt.Sprintf("%s (HTTP %d, code %s)", e.Message, e.Status, e.Code)
	}
	return fmt.Sprintf("%s (HTTP %d)", e.Message, e.Status)
}

// IsClientError reports whether the endpoint rejected the request itself, as
// opposed to a transient upstream failure worth retrying.
func (e *APIError) IsClientError() bool {
	if e == nil {
		return false
	}
	return e.Status >= 400 && e.Status < 500 && e.Status != http.StatusRequestTimeout && e.Status != http.StatusTooManyRequests
}

// callOptions carries the optional parts of one JSON call.
type callOptions struct {
	auth    string
	body    any
	headers map[string]string
	device  string
}

// envelope is the {code,msg,data} shape Z.ai and zcode.z.ai wrap their replies in.
type envelope struct {
	Code any             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

// callJSON asks one JSON endpoint and returns its data member.
//
// A code of 0 or 200 is a success; anything else is an APIError carrying the
// endpoint's own message. zcode.z.ai is told the machine id, as the ZCode app
// tells it.
func callJSON(ctx context.Context, client *http.Client, method, url string, opts callOptions) (gjson.Result, error) {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	var reader io.Reader
	if opts.body != nil {
		payload, errMarshal := json.Marshal(opts.body)
		if errMarshal != nil {
			return gjson.Result{}, fmt.Errorf("zcode: encode request body: %w", errMarshal)
		}
		reader = bytes.NewReader(payload)
	}
	req, errReq := http.NewRequestWithContext(ctx, method, url, reader)
	if errReq != nil {
		return gjson.Result{}, fmt.Errorf("zcode: build request: %w", errReq)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", UserAgent)
	if opts.auth != "" {
		req.Header.Set("Authorization", opts.auth)
	}
	if strings.HasPrefix(url, zcodeOrigin+"/") {
		device := strings.TrimSpace(opts.device)
		if device == "" {
			device = uuid.NewString()
		}
		req.Header.Set("X-Device-Mid", device)
	}
	for k, v := range opts.headers {
		req.Header.Set(k, v)
	}

	resp, errDo := client.Do(req)
	if errDo != nil {
		return gjson.Result{}, fmt.Errorf("zcode: request failed: %w", errDo)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, errRead := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if errRead != nil {
		return gjson.Result{}, fmt.Errorf("zcode: read response: %w", errRead)
	}

	var env envelope
	decodeErr := json.Unmarshal(body, &env)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		apiErr := &APIError{Status: resp.StatusCode}
		if decodeErr == nil {
			apiErr.Code = codeString(env.Code)
			apiErr.Message = strings.TrimSpace(env.Msg)
		}
		if apiErr.Message == "" {
			apiErr.Message = http.StatusText(resp.StatusCode)
		}
		if apiErr.Message == "" {
			apiErr.Message = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		return gjson.Result{}, apiErr
	}
	if decodeErr != nil {
		return gjson.Result{}, &APIError{Status: resp.StatusCode, Message: "not JSON"}
	}
	if code := codeString(env.Code); code != "" && code != "0" && code != "200" {
		message := strings.TrimSpace(env.Msg)
		if message == "" {
			message = "error " + code
		}
		return gjson.Result{}, &APIError{Status: resp.StatusCode, Code: code, Message: message}
	}
	return gjson.ParseBytes(env.Data), nil
}

func codeString(v any) string {
	switch typed := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case float64:
		if typed == float64(int64(typed)) {
			return fmt.Sprintf("%d", int64(typed))
		}
		return fmt.Sprintf("%v", typed)
	case bool:
		return fmt.Sprintf("%t", typed)
	default:
		return fmt.Sprintf("%v", typed)
	}
}

// errNotSignedIn reports a call that needs ZCode's session token without one.
var errNotSignedIn = errors.New("zcode: not signed in to ZCode")

// jwtExpiryUnix reads a JWT's exp claim in Unix seconds, or 0 when absent.
func jwtExpiryUnix(jwt string) int64 {
	parts := strings.Split(strings.TrimSpace(jwt), ".")
	if len(parts) != 3 {
		return 0
	}
	payload, errDecode := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if errDecode != nil {
		return 0
	}
	exp := gjson.GetBytes(payload, "exp").Int()
	if exp <= 0 {
		return 0
	}
	return exp
}

// JWTExpired reports whether a Start Plan token is past its expiry.
func JWTExpired(jwt string, now time.Time) bool {
	exp := jwtExpiryUnix(jwt)
	if exp <= 0 {
		return false
	}
	return now.Unix() > exp
}
