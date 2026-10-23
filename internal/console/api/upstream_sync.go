package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/db"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/engine"
)

// SyncAuthFilesToBaselines synchronizes auth files and openai-compatibility credentials
// into the ProviderRegistryEngine in-memory registry and persists them to upstream_auth_baselines in SQLite.
func SyncAuthFilesToBaselines(ctx context.Context, eng *engine.Engine, providers *ProviderRegistryEngine, authFiles []CpaAuthFile, compat []CpaOpenAICompatProvider) {
	if providers == nil {
		return
	}

	var baselines []db.UpstreamAuthBaselineRow

	for _, f := range authFiles {
		authIndex := strings.TrimSpace(f.AuthIndex)
		if authIndex == "" {
			continue
		}
		prov := strings.ToLower(strings.TrimSpace(f.Provider))
		if prov == "" {
			prov = strings.ToLower(strings.TrimSpace(f.Type))
		}
		dispName := resolveDisplayName(f)

		providers.RegisterAuthAccount(authIndex, prov, dispName)

		pCopy := prov
		nCopy := dispName
		baselines = append(baselines, db.UpstreamAuthBaselineRow{
			AuthIndex: authIndex,
			Name:      &nCopy,
			Provider:  &pCopy,
		})
	}

	for _, comp := range compat {
		compName := strings.TrimSpace(comp.Name)
		if compName == "" {
			compName = "OpenAI Compatible"
		}
		provName := strings.ToLower(compName)
		channelKey := fmt.Sprintf("compat:%s", strings.ToLower(strings.ReplaceAll(compName, " ", "_")))
		providers.RegisterChannel(provName, compName)
		providers.RegisterChannel(channelKey, compName)

		if len(comp.APIKeyEntries) == 0 {
			// Keyless or group-level compat provider entry
			authIdx := strings.TrimSpace(comp.AuthIndex)
			if authIdx == "" {
				authIdx = fmt.Sprintf("compat_%s", strings.ToLower(strings.ReplaceAll(compName, " ", "_")))
			}
			dispName := compName
			providers.RegisterAuthAccount(authIdx, provName, dispName)

			pCopy := provName
			nCopy := dispName
			baselines = append(baselines, db.UpstreamAuthBaselineRow{
				AuthIndex: authIdx,
				Name:      &nCopy,
				Provider:  &pCopy,
			})
		} else {
			for idx, keyEntry := range comp.APIKeyEntries {
				authIdx := strings.TrimSpace(keyEntry.AuthIndex)
				if authIdx == "" {
					authIdx = fmt.Sprintf("%s_%d", provName, idx)
				}
				dispName := strings.TrimSpace(keyEntry.Note)
				if dispName == "" {
					if len(comp.APIKeyEntries) == 1 {
						dispName = compName
					} else {
						prefixLen := 8
						if len(authIdx) < prefixLen {
							prefixLen = len(authIdx)
						}
						dispName = fmt.Sprintf("%s %s", compName, authIdx[:prefixLen])
					}
				}

				providers.RegisterAuthAccount(authIdx, provName, dispName)

				pCopy := provName
				nCopy := dispName
				baselines = append(baselines, db.UpstreamAuthBaselineRow{
					AuthIndex: authIdx,
					Name:      &nCopy,
					Provider:  &pCopy,
				})
			}
		}
	}

	if len(baselines) > 0 && eng != nil && eng.DB() != nil {
		if err := eng.DB().UpsertBaselines(ctx, baselines); err == nil {
			_ = eng.DB().BackfillUpstreamDailyStatsProviders(ctx)
		}
	}
}

// SyncOpenAICompatConfig proactively registers OpenAICompatibility channels and accounts at startup.
func SyncOpenAICompatConfig(ctx context.Context, eng *engine.Engine, providers *ProviderRegistryEngine, compatList []CpaOpenAICompatProvider) {
	if providers == nil || len(compatList) == 0 {
		return
	}
	SyncAuthFilesToBaselines(ctx, eng, providers, nil, compatList)
}
