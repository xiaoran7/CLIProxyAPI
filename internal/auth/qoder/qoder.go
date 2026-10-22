// Package qoder provides authentication, COSY request signing, and token management
// for Qoder Global and Qoder Mainland China APIs.
package qoder

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
)

const (
	// RealmIntl identifies the international/global realm (qoder.com / api3.qoder.sh).
	RealmIntl = "intl"
	// RealmCN identifies the mainland China realm (qoder.cn / gateway.qoder.com.cn).
	RealmCN = "cn"

	// BaseURLIntl is the chat inference gateway for Qoder Global.
	BaseURLIntl = "https://api3.qoder.sh"
	// BaseURLCN is the chat inference gateway for Qoder Mainland China.
	BaseURLCN = "https://gateway.qoder.com.cn"

	// OpenAPIBaseURLIntl is the OpenAPI / Auth origin for Qoder Global.
	OpenAPIBaseURLIntl = "https://openapi.qoder.sh"
	// OpenAPIBaseURLCN is the OpenAPI / Auth origin for Qoder Mainland China.
	OpenAPIBaseURLCN = "https://openapi.qoder.com.cn"

	// AuthWebURLIntl is the web authentication portal for Qoder Global.
	AuthWebURLIntl = "https://qoder.com"
	// AuthWebURLCN is the web authentication portal for Qoder Mainland China.
	AuthWebURLCN = "https://qoder.cn"

	// DefaultClientIDCN is the official Qoder CN desktop/CLI client identifier.
	DefaultClientIDCN = "e883ade2-e6e3-4d6d-adf7-f92ceff5fdcb"
	// DefaultClientIDIntl is the official Qoder Global client identifier.
	DefaultClientIDIntl = "732aef47-9cf2-46a2-95fe-4cebb5d0d1fa"

	// ChatCompletionsPath is the SSE agent chat completion endpoint on the gateway.
	ChatCompletionsPath = "/algo/api/v2/service/pro/sse/agent_chat_generation"
	// ModelListPath is the dynamic model list endpoint on the gateway.
	ModelListPath = "/algo/api/v2/model/list"
	// OpenAPIModelListPath is the fallback model list endpoint on OpenAPI.
	OpenAPIModelListPath = "/api/v1/cloud/models"

	// DefaultUserAgent is the client User-Agent matching the Qoder desktop/CLI client.
	DefaultUserAgent = "Qoder/0.4.3 (x86_64_windows) CLI/0.4.3"

	// CosyVersion is the standard client protocol version.
	CosyVersion = "0.1.43"
	// CosyMachineType indicates desktop/CLI client environment.
	CosyMachineType = "5"
	// CosyClientType indicates CLI/IDE integration type.
	CosyClientType = "5"
)

// ResolveRealm determines whether the auth belongs to international or mainland China realm.
func ResolveRealm(auth *cliproxyauth.Auth) string {
	if auth == nil {
		return RealmIntl
	}
	if auth.Attributes != nil {
		if r, ok := auth.Attributes["realm"]; ok && strings.TrimSpace(r) != "" {
			if strings.EqualFold(r, RealmCN) {
				return RealmCN
			}
			return RealmIntl
		}
		bu := strings.ToLower(auth.Attributes["base_url"])
		if strings.Contains(bu, "qoder.com.cn") || strings.Contains(bu, "qoder.cn") {
			return RealmCN
		}
	}
	if auth.Metadata != nil {
		if r, ok := auth.Metadata["realm"].(string); ok && strings.TrimSpace(r) != "" {
			if strings.EqualFold(r, RealmCN) {
				return RealmCN
			}
			return RealmIntl
		}
	}
	p := strings.ToLower(strings.TrimSpace(auth.Provider))
	if p == "qoder-cn" || p == "qoder_cn" {
		return RealmCN
	}
	return RealmIntl
}

// ResolveBaseURL returns the inference gateway base URL for the given realm.
// Can be overridden via QODER_CN_GATEWAY or QODER_INTL_GATEWAY environment variables.
func ResolveBaseURL(realm string) string {
	if strings.EqualFold(realm, RealmCN) {
		if override := strings.TrimSpace(os.Getenv("QODER_CN_GATEWAY")); override != "" {
			if strings.HasPrefix(override, "http://") || strings.HasPrefix(override, "https://") {
				return strings.TrimRight(override, "/")
			}
		}
		return BaseURLCN
	}
	if override := strings.TrimSpace(os.Getenv("QODER_INTL_GATEWAY")); override != "" {
		if strings.HasPrefix(override, "http://") || strings.HasPrefix(override, "https://") {
			return strings.TrimRight(override, "/")
		}
	}
	return BaseURLIntl
}

// ResolveOpenAPIBaseURL returns the OpenAPI authentication and management base URL for the given realm.
func ResolveOpenAPIBaseURL(realm string) string {
	if strings.EqualFold(realm, RealmCN) {
		if override := strings.TrimSpace(os.Getenv("QODER_CN_OPENAPI")); override != "" {
			if strings.HasPrefix(override, "http://") || strings.HasPrefix(override, "https://") {
				return strings.TrimRight(override, "/")
			}
		}
		return OpenAPIBaseURLCN
	}
	if override := strings.TrimSpace(os.Getenv("QODER_INTL_OPENAPI")); override != "" {
		if strings.HasPrefix(override, "http://") || strings.HasPrefix(override, "https://") {
			return strings.TrimRight(override, "/")
		}
	}
	return OpenAPIBaseURLIntl
}

// ResolveAuthWebURL returns the web login portal origin for the given realm.
func ResolveAuthWebURL(realm string) string {
	if strings.EqualFold(realm, RealmCN) {
		return AuthWebURLCN
	}
	return AuthWebURLIntl
}

// ResolveClientID returns the OAuth client identifier for the given realm.
func ResolveClientID(realm string) string {
	if strings.EqualFold(realm, RealmCN) {
		return DefaultClientIDCN
	}
	return DefaultClientIDIntl
}

// AccessToken returns the primary bearer or personal token held by the auth object.
func AccessToken(auth *cliproxyauth.Auth) string {
	if auth == nil {
		return ""
	}
	keys := []string{"access_token", "job_token", "device_token", "token", "personal_token", "api_key"}
	if auth.Metadata != nil {
		for _, k := range keys {
			if v, ok := auth.Metadata[k]; ok {
				if t, ok := v.(string); ok && strings.TrimSpace(t) != "" {
					return strings.TrimSpace(t)
				}
			}
		}
	}
	if auth.Attributes != nil {
		for _, k := range keys {
			if t := strings.TrimSpace(auth.Attributes[k]); t != "" {
				return t
			}
		}
	}
	return ""
}

// PersonalAccessToken returns the raw personal access token (pt-...) if present.
func PersonalAccessToken(auth *cliproxyauth.Auth) string {
	if auth == nil {
		return ""
	}
	if auth.Metadata != nil {
		if t, ok := auth.Metadata["personal_token"].(string); ok && strings.HasPrefix(strings.TrimSpace(t), "pt-") {
			return strings.TrimSpace(t)
		}
	}
	if auth.Attributes != nil {
		if t := strings.TrimSpace(auth.Attributes["personal_token"]); strings.HasPrefix(t, "pt-") {
			return t
		}
		if t := strings.TrimSpace(auth.Attributes["api_key"]); strings.HasPrefix(t, "pt-") {
			return t
		}
	}
	return ""
}

// ResolveUserID returns the user account identifier for the given auth object.
func ResolveUserID(auth *cliproxyauth.Auth) string {
	if auth == nil {
		return "anonymous"
	}
	if auth.Metadata != nil {
		if u, ok := auth.Metadata["user_id"].(string); ok && strings.TrimSpace(u) != "" {
			return strings.TrimSpace(u)
		}
		if u, ok := auth.Metadata["uid"].(string); ok && strings.TrimSpace(u) != "" {
			return strings.TrimSpace(u)
		}
	}
	if auth.Attributes != nil {
		if u := strings.TrimSpace(auth.Attributes["user_id"]); u != "" {
			return u
		}
	}
	if auth.ID != "" {
		return auth.ID
	}
	return "anonymous"
}

// ResolveMachineID derives or returns a deterministic machine identifier.
func ResolveMachineID(auth *cliproxyauth.Auth) string {
	if auth != nil {
		if auth.Metadata != nil {
			if m, ok := auth.Metadata["machine_id"].(string); ok && strings.TrimSpace(m) != "" {
				return strings.TrimSpace(m)
			}
		}
		if auth.Attributes != nil {
			if m := strings.TrimSpace(auth.Attributes["machine_id"]); m != "" {
				return m
			}
		}
		if auth.ID != "" {
			return DeriveUUID("qoder-machine", auth.ID)
		}
	}
	return uuid.New().String()
}

// DeriveUUID computes a deterministic UUIDv4-like string from salt and uid.
func DeriveUUID(salt, uid string) string {
	sum := md5.Sum([]byte(salt + ":" + uid))
	h := hex.EncodeToString(sum[:])
	h = h[:12] + "4" + h[13:16] + "a" + h[17:]
	return fmt.Sprintf("%s-%s-%s-%s-%s", h[0:8], h[8:12], h[12:16], h[16:20], h[20:32])
}

// NewRandomBytes returns n cryptographically secure random bytes.
func NewRandomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		sum := md5.Sum([]byte(fmt.Sprintf("%016x", time.Now().UnixNano())))
		return sum[:n]
	}
	return b
}
