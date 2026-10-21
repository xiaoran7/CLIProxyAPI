package console

import (
	"context"
	"net/http"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/engine"
	sdkaccess "github.com/router-for-me/CLIProxyAPI/v8/sdk/access"
)

// AccessProviderTypeConsole defines the provider identifier registered in sdk/access.
const AccessProviderTypeConsole = "console"

// ConsoleAccessProvider integrates Console multi-tenant API keys into CPA's native sdk/access manager.
type ConsoleAccessProvider struct {
	eng *engine.Engine
}

// NewConsoleAccessProvider constructs an access provider bound to the Console engine.
func NewConsoleAccessProvider(eng *engine.Engine) *ConsoleAccessProvider {
	return &ConsoleAccessProvider{eng: eng}
}

// Identifier satisfies sdkaccess.Provider.
func (p *ConsoleAccessProvider) Identifier() string {
	return AccessProviderTypeConsole
}

// Authenticate extracts API keys across standard penetration headers and query parameters
// and validates them against the Console engine's in-memory key registry.
func (p *ConsoleAccessProvider) Authenticate(ctx context.Context, r *http.Request) (*sdkaccess.Result, *sdkaccess.AuthError) {
	if p == nil || p.eng == nil {
		return nil, sdkaccess.NewNotHandledError()
	}

	rawKey := ExtractRawKeyFromRequest(r)
	if rawKey == "" {
		return nil, sdkaccess.NewNoCredentialsError()
	}

	keyID, ok := p.eng.ResolveKeyID(rawKey)
	if !ok || keyID <= 0 {
		return nil, sdkaccess.NewInvalidCredentialError()
	}

	return &sdkaccess.Result{
		Provider:  p.Identifier(),
		Principal: rawKey,
		Metadata: map[string]string{
			"source": "console",
		},
	}, nil
}

// ExtractRawKeyFromRequest extracts the plaintext API key from an incoming HTTP request across:
// 1. Authorization: Bearer <key>
// 2. x-api-key: <key> (Anthropic)
// 3. api-key: <key> (Azure OpenAI)
// 4. x-goog-api-key: <key> (Google Gemini)
// 5. Query parameter ?key=<key> or ?auth_token=<key>
func ExtractRawKeyFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}

	// 1. Authorization: Bearer <key>
	if auth := r.Header.Get("Authorization"); auth != "" {
		trimmed := strings.TrimSpace(auth)
		if strings.HasPrefix(strings.ToLower(trimmed), "bearer ") {
			return strings.TrimSpace(trimmed[7:])
		}
		return trimmed
	}

	// 2. Anthropic x-api-key: <key>
	if k := r.Header.Get("x-api-key"); k != "" {
		return strings.TrimSpace(k)
	}

	// 3. Azure OpenAI api-key: <key>
	if k := r.Header.Get("api-key"); k != "" {
		return strings.TrimSpace(k)
	}

	// 4. Google Gemini x-goog-api-key: <key>
	if k := r.Header.Get("x-goog-api-key"); k != "" {
		return strings.TrimSpace(k)
	}

	// 5. URL query parameters ?key= or ?auth_token=
	if r.URL != nil {
		if k := r.URL.Query().Get("key"); k != "" {
			return strings.TrimSpace(k)
		}
		if k := r.URL.Query().Get("auth_token"); k != "" {
			return strings.TrimSpace(k)
		}
	}

	return ""
}
