package zcode

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/misc"
)

// TokenStorage is the credential file a ZCode sign-in writes.
type TokenStorage struct {
	// Type is the credential's provider key.
	Type string `json:"type"`
	// APIKey is the minted `<id>.<secret>` coding plan key.
	APIKey string `json:"api_key,omitempty"`
	// StartToken is ZCode's session JWT, which spends the Start Plan.
	StartToken string `json:"zcode_token,omitempty"`
	// SiteToken is the Z.ai or BigModel account token, kept for team key minting.
	SiteToken string `json:"site_token,omitempty"`
	// Site is the realm the account signed in to.
	Site string `json:"site"`
	// BaseURL is the Messages base URL the key is served from.
	BaseURL string `json:"base_url,omitempty"`
	// Plan is the plan the account holds.
	Plan string `json:"plan,omitempty"`
	// Team reports a seat on a team's plan.
	Team bool `json:"team,omitempty"`
	// TeamOrg and TeamProject identify the team project holding the seat.
	TeamOrg     string `json:"team_org,omitempty"`
	TeamProject string `json:"team_project,omitempty"`
	// DeviceID is the machine id the Start Plan dressing names.
	DeviceID string `json:"device_id,omitempty"`
	// TrialModels lists the models the Start Plan served at sign-in.
	TrialModels []string `json:"trial_models,omitempty"`
	// UserID, Email, and Nickname identify the account.
	UserID   string `json:"user_id,omitempty"`
	Email    string `json:"email,omitempty"`
	Nickname string `json:"nickname,omitempty"`
	// Expired is when the Start Plan token runs out, in RFC 3339.
	Expired string `json:"expired,omitempty"`
	// Metadata carries extra fields merged into the file.
	Metadata map[string]any `json:"-"`
}

// SetMetadata injects metadata merged into the saved file.
func (ts *TokenStorage) SetMetadata(meta map[string]any) {
	ts.Metadata = meta
}

// SaveTokenToFile writes the credential file.
func (ts *TokenStorage) SaveTokenToFile(authFilePath string) error {
	misc.LogSavingCredentials(authFilePath)
	if ts.Type == "" {
		ts.Type = "zcode"
	}
	if ts.Site == "" {
		ts.Site = SiteZAI
	}
	if ts.BaseURL == "" {
		ts.BaseURL = SiteBaseURL(ts.Site)
	}
	if errDir := os.MkdirAll(filepath.Dir(authFilePath), 0o700); errDir != nil {
		return fmt.Errorf("failed to create directory: %w", errDir)
	}
	data, errMerge := misc.MergeMetadata(ts, ts.Metadata)
	if errMerge != nil {
		return fmt.Errorf("failed to merge metadata: %w", errMerge)
	}
	file, errCreate := os.Create(authFilePath)
	if errCreate != nil {
		return fmt.Errorf("failed to create token file: %w", errCreate)
	}
	defer func() {
		_ = file.Close()
	}()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if errEncode := encoder.Encode(data); errEncode != nil {
		return fmt.Errorf("failed to write token file: %w", errEncode)
	}
	return nil
}
