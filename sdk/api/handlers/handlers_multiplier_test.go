package handlers

import (
	"net/http"
	"testing"
)

func TestParseModelMultiplier(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantClean   string
		wantBase    string
		wantMult    float64
		wantHasMult bool
		wantRawMult string
	}{
		{
			name:        "plain model without multiplier",
			input:       "glm-5.3-flash",
			wantClean:   "glm-5.3-flash",
			wantBase:    "glm-5.3-flash",
			wantHasMult: false,
		},
		{
			name:        "model with suffix multiplier ending in x",
			input:       "glm-5.3-flash:0.06x",
			wantClean:   "glm-5.3-flash",
			wantBase:    "glm-5.3-flash",
			wantMult:    0.06,
			wantHasMult: true,
			wantRawMult: "0.06x",
		},
		{
			name:        "model with suffix multiplier pure decimal",
			input:       "glm-5.3-flash:0.06",
			wantClean:   "glm-5.3-flash",
			wantBase:    "glm-5.3-flash",
			wantMult:    0.06,
			wantHasMult: true,
			wantRawMult: "0.06",
		},
		{
			name:        "free model with 0x multiplier",
			input:       "hy3:0x",
			wantClean:   "hy3",
			wantBase:    "hy3",
			wantMult:    0.0,
			wantHasMult: true,
			wantRawMult: "0x",
		},
		{
			name:        "free model with 0 multiplier",
			input:       "hy3:0",
			wantClean:   "hy3",
			wantBase:    "hy3",
			wantMult:    0.0,
			wantHasMult: true,
			wantRawMult: "0",
		},
		{
			name:        "model with thinking suffix then multiplier",
			input:       "glm-5.3-flash(16384):0.06x",
			wantClean:   "glm-5.3-flash(16384)",
			wantBase:    "glm-5.3-flash",
			wantMult:    0.06,
			wantHasMult: true,
			wantRawMult: "0.06x",
		},
		{
			name:        "model with multiplier then outer thinking suffix",
			input:       "glm-5.3-flash:0.06x(16384)",
			wantClean:   "glm-5.3-flash(16384)",
			wantBase:    "glm-5.3-flash",
			wantMult:    0.06,
			wantHasMult: true,
			wantRawMult: "0.06x",
		},
		{
			name:        "docker style tag that is not multiplier",
			input:       "llama3:latest",
			wantClean:   "llama3:latest",
			wantBase:    "llama3:latest",
			wantHasMult: false,
		},
		{
			name:        "workbuddy namespace prefix with multiplier",
			input:       "workbuddy/claude-opus-5-5:3.33x",
			wantClean:   "claude-opus-5-5",
			wantBase:    "claude-opus-5-5",
			wantMult:    3.33,
			wantHasMult: true,
			wantRawMult: "3.33x",
		},
		{
			name:        "workbuddy-cn namespace prefix with free multiplier",
			input:       "workbuddy-cn/hy3:0x",
			wantClean:   "hy3",
			wantBase:    "hy3",
			wantMult:    0.0,
			wantHasMult: true,
			wantRawMult: "0x",
		},
		{
			name:        "workbuddy namespace prefix without multiplier",
			input:       "workbuddy/claude-opus-5-5",
			wantClean:   "claude-opus-5-5",
			wantBase:    "claude-opus-5-5",
			wantHasMult: false,
		},
		{
			name:        "workbuddy namespace prefix with multiplier and thinking suffix",
			input:       "workbuddy/claude-opus-5-5:3.33x(16384)",
			wantClean:   "claude-opus-5-5(16384)",
			wantBase:    "claude-opus-5-5",
			wantMult:    3.33,
			wantHasMult: true,
			wantRawMult: "3.33x",
		},
		{
			name:        "opencode namespace prefix without multiplier",
			input:       "opencode/step-5-preview-free",
			wantClean:   "step-5-preview-free",
			wantBase:    "step-5-preview-free",
			wantHasMult: false,
		},
		{
			name:        "antigravity namespace prefix without multiplier",
			input:       "antigravity/gemini-3.8-flash-high",
			wantClean:   "gemini-3.8-flash-high",
			wantBase:    "gemini-3.8-flash-high",
			wantHasMult: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := ParseModelMultiplier(tc.input)
			if res.CleanModel != tc.wantClean {
				t.Errorf("CleanModel = %q, want %q", res.CleanModel, tc.wantClean)
			}
			if res.BaseModel != tc.wantBase {
				t.Errorf("BaseModel = %q, want %q", res.BaseModel, tc.wantBase)
			}
			if res.HasMultiplier != tc.wantHasMult {
				t.Errorf("HasMultiplier = %v, want %v", res.HasMultiplier, tc.wantHasMult)
			}
			if tc.wantHasMult && res.Multiplier != tc.wantMult {
				t.Errorf("Multiplier = %v, want %v", res.Multiplier, tc.wantMult)
			}
			if tc.wantHasMult && res.RawMultiplier != tc.wantRawMult {
				t.Errorf("RawMultiplier = %q, want %q", res.RawMultiplier, tc.wantRawMult)
			}
		})
	}
}

func TestExtractAndValidateModelMultiplier(t *testing.T) {
	tests := []struct {
		name          string
		modelName     string
		wantClean     string
		expectErr     bool
		wantErrStatus int
	}{
		{
			name:      "correct multiplier matches static catalog",
			modelName: "glm-5.3-flash:0.06x",
			wantClean: "glm-5.3-flash",
			expectErr: false,
		},
		{
			name:      "correct multiplier without x suffix",
			modelName: "glm-5.3-flash:0.06",
			wantClean: "glm-5.3-flash",
			expectErr: false,
		},
		{
			name:      "free model 0x matches catalog",
			modelName: "hy3:0x",
			wantClean: "hy3",
			expectErr: false,
		},
		{
			name:      "free alias model -free matches 0x",
			modelName: "glm-5.3-flash-free:0x",
			wantClean: "glm-5.3-flash-free",
			expectErr: false,
		},
		{
			name:      "high multiplier model matches",
			modelName: "claude-opus-5-5:3.33x",
			wantClean: "claude-opus-5-5",
			expectErr: false,
		},
		{
			name:      "thinking suffix with correct multiplier",
			modelName: "glm-5.3-flash(32768):0.06x",
			wantClean: "glm-5.3-flash(32768)",
			expectErr: false,
		},
		{
			name:      "no multiplier provided passes through unmodified",
			modelName: "glm-5.3-flash",
			wantClean: "glm-5.3-flash",
			expectErr: false,
		},
		{
			name:      "workbuddy namespace prefix with correct multiplier",
			modelName: "workbuddy/claude-opus-5-5:3.33x",
			wantClean: "claude-opus-5-5",
			expectErr: false,
		},
		{
			name:      "workbuddy-cn namespace prefix with free multiplier",
			modelName: "workbuddy-cn/hy3:0x",
			wantClean: "hy3",
			expectErr: false,
		},
		{
			name:      "workbuddy namespace prefix without multiplier",
			modelName: "workbuddy/claude-opus-5-5",
			wantClean: "claude-opus-5-5",
			expectErr: false,
		},
		{
			name:          "workbuddy namespace prefix with wrong multiplier rejected",
			modelName:     "workbuddy/claude-opus-5-5:1.0x",
			expectErr:     true,
			wantErrStatus: http.StatusBadRequest,
		},
		{
			name:          "wrong multiplier is rejected with 400 Bad Request",
			modelName:     "glm-5.3-flash:1.5x",
			expectErr:     true,
			wantErrStatus: http.StatusBadRequest,
		},
		{
			name:          "wrong multiplier on free model is rejected",
			modelName:     "hy3:0.5x",
			expectErr:     true,
			wantErrStatus: http.StatusBadRequest,
		},
		{
			name:          "model not publishing credits with multiplier is rejected",
			modelName:     "nonexistent-unregistered-model:0.5x",
			expectErr:     true,
			wantErrStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clean, err := ExtractAndValidateModelMultiplier(tc.modelName)
			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected error for model %s, got nil", tc.modelName)
				}
				if err.StatusCode != tc.wantErrStatus {
					t.Errorf("expected StatusCode %d, got %d", tc.wantErrStatus, err.StatusCode)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error for model %s: %v", tc.modelName, err.Error)
				}
				if clean != tc.wantClean {
					t.Errorf("clean model = %q, want %q", clean, tc.wantClean)
				}
			}
		})
	}
}

func TestDualRealmWorkBuddyMultiplierValidation(t *testing.T) {
	// 1. CN realm with correct 0.11x rate for deepseek-v4.1-flash
	details, err := ExtractAndValidateModelMultiplierDetails("workbuddy-cn/deepseek-v4.1-flash:0.11x")
	if err != nil {
		t.Fatalf("unexpected error for workbuddy-cn valid rate: %v", err.Error)
	}
	if details.CleanModel != "deepseek-v4.1-flash" {
		t.Errorf("CleanModel = %q, want deepseek-v4.1-flash", details.CleanModel)
	}
	if details.TargetProvider != "workbuddy-cn" {
		t.Errorf("TargetProvider = %q, want workbuddy-cn", details.TargetProvider)
	}
	if details.TargetRealm != "cn" {
		t.Errorf("TargetRealm = %q, want cn", details.TargetRealm)
	}
	if details.Multiplier != 0.11 {
		t.Errorf("Multiplier = %v, want 0.11", details.Multiplier)
	}

	// 2. Intl realm with correct 0x rate for deepseek-v4.1-flash
	detailsIntl, errIntl := ExtractAndValidateModelMultiplierDetails("workbuddy/deepseek-v4.1-flash:0x")
	if errIntl != nil {
		t.Fatalf("unexpected error for workbuddy intl valid rate: %v", errIntl.Error)
	}
	if detailsIntl.TargetProvider != "workbuddy" {
		t.Errorf("TargetProvider = %q, want workbuddy", detailsIntl.TargetProvider)
	}
	if detailsIntl.TargetRealm != "intl" {
		t.Errorf("TargetRealm = %q, want intl", detailsIntl.TargetRealm)
	}

	// 3. CN realm does NOT have a free tier: 0x must be rejected
	_, errCNFree := ExtractAndValidateModelMultiplierDetails("workbuddy-cn/deepseek-v4.1-flash:0x")
	if errCNFree == nil {
		t.Fatal("expected error when requesting 0x for CN realm deepseek-v4.1-flash, got nil")
	}
	if errCNFree.StatusCode != http.StatusBadRequest {
		t.Errorf("expected StatusCode %d, got %d", http.StatusBadRequest, errCNFree.StatusCode)
	}

	// 4. CodeBuddy alias prefix (codebuddy/) also resolves to TargetProvider workbuddy-cn and realm cn
	detailsCodeBuddy, errCodeBuddy := ExtractAndValidateModelMultiplierDetails("codebuddy/deepseek-v4.1-flash:0.11x")
	if errCodeBuddy != nil {
		t.Fatalf("unexpected error for codebuddy prefix: %v", errCodeBuddy.Error)
	}
	if detailsCodeBuddy.TargetProvider != "workbuddy-cn" {
		t.Errorf("TargetProvider = %q, want workbuddy-cn", detailsCodeBuddy.TargetProvider)
	}
}
