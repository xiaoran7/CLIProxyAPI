package api

import (
	"context"
	"math"
	"regexp"
	"strings"
	"sync"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/db"
)

// SeedPrice represents an initial default model pricing entry.
type SeedPrice struct {
	Pattern         string
	PromptPrice     float64
	CompletionPrice float64
	CacheReadPrice  float64
}

// InitialSeedPrices contains standard baseline prices per 1M tokens (USD).
var InitialSeedPrices = []SeedPrice{
	// Gemini series
	{Pattern: "gemini-3.8-flash-high", PromptPrice: 0.75, CompletionPrice: 3.75, CacheReadPrice: 0.075},
	{Pattern: "gemini-3.1-flash-lite", PromptPrice: 0.25, CompletionPrice: 1.50, CacheReadPrice: 0.025},
	{Pattern: "gemini-3.1-flash-image", PromptPrice: 0.50, CompletionPrice: 60.00, CacheReadPrice: 0.05},
	{Pattern: "gemini-3-flash", PromptPrice: 0.50, CompletionPrice: 3.00, CacheReadPrice: 0.05},
	{Pattern: "gemini-2.5-flash", PromptPrice: 0.10, CompletionPrice: 0.40, CacheReadPrice: 0.025},
	{Pattern: "gemini-2.0-flash", PromptPrice: 0.10, CompletionPrice: 0.40, CacheReadPrice: 0.025},
	{Pattern: "gemini-1.5-flash", PromptPrice: 0.075, CompletionPrice: 0.30, CacheReadPrice: 0.01875},
	{Pattern: "gemini-2.5-pro", PromptPrice: 1.25, CompletionPrice: 5.00, CacheReadPrice: 0.3125},
	{Pattern: "gemini-2.0-pro", PromptPrice: 1.25, CompletionPrice: 5.00, CacheReadPrice: 0.3125},
	{Pattern: "gemini-1.5-pro", PromptPrice: 1.25, CompletionPrice: 5.00, CacheReadPrice: 0.3125},
	{Pattern: "gemini", PromptPrice: 0.15, CompletionPrice: 0.60, CacheReadPrice: 0.0375},

	// Claude series
	{Pattern: "claude-opus-4-6-thinking", PromptPrice: 5.00, CompletionPrice: 25.00, CacheReadPrice: 0.50},
	{Pattern: "claude-sonnet-4-6", PromptPrice: 3.00, CompletionPrice: 15.00, CacheReadPrice: 0.30},
	{Pattern: "claude-3-7-sonnet", PromptPrice: 3.00, CompletionPrice: 15.00, CacheReadPrice: 0.30},
	{Pattern: "claude-3-5-sonnet", PromptPrice: 3.00, CompletionPrice: 15.00, CacheReadPrice: 0.30},
	{Pattern: "claude-3-5-haiku", PromptPrice: 0.80, CompletionPrice: 4.00, CacheReadPrice: 0.08},
	{Pattern: "claude-3-opus", PromptPrice: 15.00, CompletionPrice: 75.00, CacheReadPrice: 1.50},
	{Pattern: "claude", PromptPrice: 3.00, CompletionPrice: 15.00, CacheReadPrice: 0.30},

	// OpenAI series
	{Pattern: "gpt-5.6-sol", PromptPrice: 4.00, CompletionPrice: 20.00, CacheReadPrice: 0.40},
	{Pattern: "gpt-6-astra", PromptPrice: 10.00, CompletionPrice: 50.00, CacheReadPrice: 1.00},
	{Pattern: "gpt-4o-mini", PromptPrice: 0.15, CompletionPrice: 0.60, CacheReadPrice: 0.075},
	{Pattern: "gpt-4o", PromptPrice: 2.50, CompletionPrice: 10.00, CacheReadPrice: 1.25},
	{Pattern: "o1-mini", PromptPrice: 1.10, CompletionPrice: 4.40, CacheReadPrice: 0.55},
	{Pattern: "o1", PromptPrice: 15.00, CompletionPrice: 60.00, CacheReadPrice: 7.50},
	{Pattern: "o3-mini", PromptPrice: 1.10, CompletionPrice: 4.40, CacheReadPrice: 0.55},
	{Pattern: "gpt-4", PromptPrice: 10.00, CompletionPrice: 30.00, CacheReadPrice: 5.00},

	// DeepSeek series
	{Pattern: "deepseek-reasoner", PromptPrice: 0.55, CompletionPrice: 2.19, CacheReadPrice: 0.14},
	{Pattern: "deepseek-r1", PromptPrice: 0.55, CompletionPrice: 2.19, CacheReadPrice: 0.14},
	{Pattern: "deepseek-chat", PromptPrice: 0.14, CompletionPrice: 0.28, CacheReadPrice: 0.014},
	{Pattern: "deepseek-v3", PromptPrice: 0.14, CompletionPrice: 0.28, CacheReadPrice: 0.014},
	{Pattern: "deepseek", PromptPrice: 0.14, CompletionPrice: 0.28, CacheReadPrice: 0.014},

	// Qwen series
	{Pattern: "qwen-max", PromptPrice: 1.60, CompletionPrice: 6.40, CacheReadPrice: 0.40},
	{Pattern: "qwen-plus", PromptPrice: 0.40, CompletionPrice: 1.20, CacheReadPrice: 0.10},
	{Pattern: "qwen-turbo", PromptPrice: 0.05, CompletionPrice: 0.20, CacheReadPrice: 0.01},
	{Pattern: "qwen", PromptPrice: 0.20, CompletionPrice: 0.60, CacheReadPrice: 0.05},

	// GLM / ZCode series
	{Pattern: "glm-5.3-flash", PromptPrice: 0.10, CompletionPrice: 0.10, CacheReadPrice: 0.02},
	{Pattern: "glm-5.3", PromptPrice: 1.00, CompletionPrice: 1.00, CacheReadPrice: 0.20},
	{Pattern: "glm-5.2", PromptPrice: 0.70, CompletionPrice: 0.70, CacheReadPrice: 0.14},
	{Pattern: "glm-5-turbo", PromptPrice: 0.50, CompletionPrice: 0.50, CacheReadPrice: 0.10},
	{Pattern: "glm", PromptPrice: 0.50, CompletionPrice: 0.50, CacheReadPrice: 0.10},
}

// CompiledPricingRule holds an in-memory compiled model pricing rule.
type CompiledPricingRule struct {
	ID              int64
	RawPattern      string
	Regex           *regexp.Regexp
	PromptPrice     float64
	CompletionPrice float64
	CacheReadPrice  float64
	Multiplier      float64
}

// PricingRates details unit pricing per 1M tokens.
type PricingRates struct {
	PromptPer1M     float64 `json:"prompt_per_1m"`
	CompletionPer1M float64 `json:"completion_per_1m"`
	CachedPer1M     float64 `json:"cached_per_1m"`
	Multiplier      float64 `json:"multiplier"`
}

// ModelCostResult represents the financial calculation for a model request.
type ModelCostResult struct {
	CostUSD    float64      `json:"cost_usd"`
	SavingsUSD float64      `json:"savings_usd"`
	Rates      PricingRates `json:"rates"`
}

// PricingEngine manages in-memory pricing rules and cost calculations.
type PricingEngine struct {
	mu    sync.RWMutex
	rules []*CompiledPricingRule
	db    *db.DB
}

// NewPricingEngine initializes the pricing manager and seeds initial pricing data if empty.
func NewPricingEngine(database *db.DB) (*PricingEngine, error) {
	pe := &PricingEngine{
		db: database,
	}
	if err := pe.InitAndReload(context.Background()); err != nil {
		return nil, err
	}
	return pe, nil
}

// InitAndReload seeds default pricing if empty and compiles all rules in memory.
func (pe *PricingEngine) InitAndReload(ctx context.Context) error {
	existing, err := pe.db.SelectAllModelPrices(ctx)
	if err != nil {
		return err
	}

	if len(existing) == 0 {
		for _, seed := range InitialSeedPrices {
			_ = pe.db.UpsertModelPrice(ctx, seed.Pattern, seed.PromptPrice, seed.CompletionPrice, seed.CacheReadPrice, 1.0)
		}
		existing, err = pe.db.SelectAllModelPrices(ctx)
		if err != nil {
			return err
		}
	}

	var compiled []*CompiledPricingRule
	for _, row := range existing {
		re, err := regexp.Compile("(?i)" + regexp.QuoteMeta(row.Pattern))
		if err != nil {
			// Fallback: try raw regex if quotation fails
			re, err = regexp.Compile("(?i)" + row.Pattern)
			if err != nil {
				continue
			}
		}

		mult := row.Multiplier
		if mult <= 0 {
			mult = 1.0
		}

		compiled = append(compiled, &CompiledPricingRule{
			ID:              row.ID,
			RawPattern:      row.Pattern,
			Regex:           re,
			PromptPrice:     row.PromptPrice,
			CompletionPrice: row.CompletionPrice,
			CacheReadPrice:  row.CacheReadPrice,
			Multiplier:      mult,
		})
	}

	pe.mu.Lock()
	pe.rules = compiled
	pe.mu.Unlock()

	return nil
}

// CalculateModelCost computes the cost and cache savings in USD for the given token distribution.
func (pe *PricingEngine) CalculateModelCost(model string, promptTokens, completionTokens, cachedTokens int64) ModelCostResult {
	m := strings.TrimSpace(model)

	pRate := 0.50
	cRate := 1.50
	crRate := 0.10
	mult := 1.0

	pe.mu.RLock()
	for _, r := range pe.rules {
		if r.Regex.MatchString(m) {
			pRate = r.PromptPrice
			cRate = r.CompletionPrice
			crRate = r.CacheReadPrice
			mult = r.Multiplier
			break
		}
	}
	pe.mu.RUnlock()

	normalPrompt := promptTokens - cachedTokens
	if normalPrompt < 0 {
		normalPrompt = 0
	}

	costPrompt := (float64(normalPrompt) / 1000000.0) * pRate * mult
	costCached := (float64(cachedTokens) / 1000000.0) * crRate * mult
	costCompletion := (float64(completionTokens) / 1000000.0) * cRate * mult
	costTotal := costPrompt + costCached + costCompletion

	savings := 0.0
	if cachedTokens > 0 && pRate > crRate {
		savings = (float64(cachedTokens) / 1000000.0) * (pRate - crRate) * mult
	}

	return ModelCostResult{
		CostUSD:    math.Round(costTotal*1000000.0) / 1000000.0,
		SavingsUSD: math.Round(savings*1000000.0) / 1000000.0,
		Rates: PricingRates{
			PromptPer1M:     pRate,
			CompletionPer1M: cRate,
			CachedPer1M:     crRate,
			Multiplier:      mult,
		},
	}
}
