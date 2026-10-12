// Package workbuddy provides authentication and token management functionality
// for WorkBuddy and CodeBuddy services.
package workbuddy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/misc"
)

// WorkBuddyTokenStorage stores authentication token information for WorkBuddy / CodeBuddy.
type WorkBuddyTokenStorage struct {
	AccessToken  string         `json:"access_token"`
	RefreshToken string         `json:"refresh_token"`
	TokenType    string         `json:"token_type"`
	UserID       string         `json:"user_id,omitempty"`
	Email        string         `json:"email,omitempty"`
	Expired      string         `json:"expired,omitempty"`
	Type         string         `json:"type"`
	Realm        string         `json:"realm,omitempty"`
	Domain       string         `json:"domain,omitempty"`
	BaseURL      string         `json:"base_url,omitempty"`
	Metadata     map[string]any `json:"-"`
}

// SetMetadata allows external callers to inject metadata into the storage before saving.
func (ts *WorkBuddyTokenStorage) SetMetadata(meta map[string]any) {
	ts.Metadata = meta
}

// SaveTokenToFile serializes the WorkBuddy token storage to a JSON file.
func (ts *WorkBuddyTokenStorage) SaveTokenToFile(authFilePath string) error {
	misc.LogSavingCredentials(authFilePath)
	if ts.Type == "" {
		ts.Type = "workbuddy"
	}
	if ts.Realm == "" {
		ts.Realm = RealmIntl
	}
	if ts.Domain == "" {
		ts.Domain = ResolveDomain(ts.Realm)
	}
	if ts.BaseURL == "" {
		ts.BaseURL = ResolveBaseURL(ts.Realm)
	}

	if err := os.MkdirAll(filepath.Dir(authFilePath), 0o700); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	data, errMerge := misc.MergeMetadata(ts, ts.Metadata)
	if errMerge != nil {
		return fmt.Errorf("failed to merge metadata: %w", errMerge)
	}

	f, err := os.Create(authFilePath)
	if err != nil {
		return fmt.Errorf("failed to create token file: %w", err)
	}
	defer func() {
		_ = f.Close()
	}()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(data); err != nil {
		return fmt.Errorf("failed to write token file: %w", err)
	}
	return nil
}
