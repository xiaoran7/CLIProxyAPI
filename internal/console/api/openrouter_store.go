package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

var openRouterMutex sync.RWMutex

// maskSecret masks sensitive key material or tokens for display.
func maskSecret(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if len(raw) <= 10 {
		return "******"
	}
	prefixLen := 4
	if len(raw) > 16 {
		prefixLen = 6
	}
	return raw[:prefixLen] + "..." + raw[len(raw)-4:]
}

// OpenRouterAccount represents an OpenRouter account entry in storage.
type OpenRouterAccount struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type,omitempty"`
	Note      string `json:"note"`
	Enabled   bool   `json:"enabled"`
	Priority  int    `json:"priority"`
	Weight    int    `json:"weight"`
	APIKey    string `json:"apiKey"`
	MgmtKey   string `json:"mgmtKey"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// OpenRouterAccountItem represents the public sanitized response item.
type OpenRouterAccountItem struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Note          string  `json:"note"`
	Enabled       bool    `json:"enabled"`
	Priority      int     `json:"priority"`
	Weight        int     `json:"weight"`
	HasAPIKey     bool    `json:"hasApiKey"`
	APIKeyMasked  string  `json:"apiKeyMasked"`
	HasMgmtKey    bool    `json:"hasMgmtKey"`
	MgmtKeyMasked string  `json:"mgmtKeyMasked"`
	TodayRequests *int    `json:"today_requests"`
	RawError      *string `json:"raw_error"`
	UpdatedAt     *string `json:"updated_at"`
	CreatedAt     string  `json:"createdAt"`
}

type openRouterConfigFile struct {
	Version  int                 `json:"version,omitempty"`
	Seq      int                 `json:"seq,omitempty"`
	Accounts []OpenRouterAccount `json:"accounts"`
}

func getOpenRouterAccountsPath() string {
	if p := os.Getenv("GATE_OPENROUTER_ACCOUNTS_PATH"); p != "" {
		return p
	}
	if p := os.Getenv("OPENROUTER_ACCOUNTS_PATH"); p != "" {
		return p
	}

	candidates := []string{
		"./data/openrouter-accounts.json",
		"../gate/data/openrouter-accounts.json",
		"../../gate/data/openrouter-accounts.json",
		"D:/Claudespace/Project/gate/data/openrouter-accounts.json",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return "./data/openrouter-accounts.json"
}

func loadOpenRouterConfig() (openRouterConfigFile, string, error) {
	filePath := getOpenRouterAccountsPath()
	var cfg openRouterConfigFile

	b, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, filePath, nil
		}
		return cfg, filePath, err
	}

	if err := json.Unmarshal(b, &cfg); err != nil {
		return cfg, filePath, err
	}
	return cfg, filePath, nil
}

func saveOpenRouterConfig(cfg openRouterConfigFile, filePath string) error {
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	tempPath := fmt.Sprintf("%s.tmp.%d", filePath, time.Now().UnixNano())
	if err := os.WriteFile(tempPath, b, 0644); err != nil {
		return err
	}
	return os.Rename(tempPath, filePath)
}

func getOpenRouterAccountsList() []OpenRouterAccountItem {
	openRouterMutex.RLock()
	defer openRouterMutex.RUnlock()

	cfg, _, _ := loadOpenRouterConfig()
	var list []OpenRouterAccountItem

	for _, a := range cfg.Accounts {
		accID := a.ID
		if accID == "" {
			accID = a.Name
		}
		name := a.Name
		if name == "" {
			name = "OpenRouter #" + accID
		}

		list = append(list, OpenRouterAccountItem{
			ID:            accID,
			Name:          name,
			Note:          a.Note,
			Enabled:       a.Enabled,
			Priority:      a.Priority,
			Weight:        a.Weight,
			HasAPIKey:     strings.TrimSpace(a.APIKey) != "",
			APIKeyMasked:  maskSecret(a.APIKey),
			HasMgmtKey:    strings.TrimSpace(a.MgmtKey) != "",
			MgmtKeyMasked: maskSecret(a.MgmtKey),
			CreatedAt:     a.CreatedAt,
		})
	}

	if list == nil {
		list = []OpenRouterAccountItem{}
	}
	return list
}

// HandleGetOpenRouterAccounts returns all configured OpenRouter accounts.
func HandleGetOpenRouterAccounts(cpa *CpaClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		accounts := getOpenRouterAccountsList()
		c.JSON(http.StatusOK, gin.H{"accounts": accounts})
	}
}

// HandleCreateOpenRouterAccount creates a new OpenRouter account.
func HandleCreateOpenRouterAccount(cpa *CpaClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		var payload struct {
			Name     string `json:"name"`
			Note     string `json:"note"`
			APIKey   string `json:"apiKey"`
			MgmtKey  string `json:"mgmtKey"`
			Priority *int   `json:"priority"`
			Weight   *int   `json:"weight"`
			Enabled  *bool  `json:"enabled"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
			return
		}

		cleanName := strings.TrimSpace(payload.Name)
		if cleanName == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Account name cannot be empty"})
			return
		}

		openRouterMutex.Lock()
		defer openRouterMutex.Unlock()

		cfg, filePath, err := loadOpenRouterConfig()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to load accounts: %v", err)})
			return
		}

		for _, a := range cfg.Accounts {
			if strings.EqualFold(a.Name, cleanName) {
				c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("Account name [%s] already exists", cleanName)})
				return
			}
		}

		maxID := 0
		for _, a := range cfg.Accounts {
			if n, err := strconv.Atoi(a.ID); err == nil && n > maxID {
				maxID = n
			}
		}
		newID := strconv.Itoa(maxID + 1)

		prio := 0
		if payload.Priority != nil {
			prio = *payload.Priority
		}
		weight := 100
		if payload.Weight != nil {
			weight = *payload.Weight
		}
		enabled := true
		if payload.Enabled != nil {
			enabled = *payload.Enabled
		}

		nowISO := time.Now().UTC().Format(time.RFC3339)
		newAcc := OpenRouterAccount{
			ID:        newID,
			Name:      cleanName,
			Note:      strings.TrimSpace(payload.Note),
			Enabled:   enabled,
			Priority:  prio,
			Weight:    weight,
			APIKey:    strings.TrimSpace(payload.APIKey),
			MgmtKey:   strings.TrimSpace(payload.MgmtKey),
			CreatedAt: nowISO,
			UpdatedAt: nowISO,
		}

		cfg.Accounts = append(cfg.Accounts, newAcc)
		cfg.Seq++
		if err := saveOpenRouterConfig(cfg, filePath); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to save account: %v", err)})
			return
		}

		InvalidateUpstreamQuotaCache()

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"account": OpenRouterAccountItem{
				ID:            newAcc.ID,
				Name:          newAcc.Name,
				Note:          newAcc.Note,
				Enabled:       newAcc.Enabled,
				Priority:      newAcc.Priority,
				Weight:        newAcc.Weight,
				HasAPIKey:     newAcc.APIKey != "",
				APIKeyMasked:  maskSecret(newAcc.APIKey),
				HasMgmtKey:    newAcc.MgmtKey != "",
				MgmtKeyMasked: maskSecret(newAcc.MgmtKey),
				CreatedAt:     newAcc.CreatedAt,
			},
		})
	}
}

// HandleUpdateOpenRouterAccount updates an existing OpenRouter account.
func HandleUpdateOpenRouterAccount(cpa *CpaClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := strings.TrimSpace(c.Param("id"))
		if id == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Account id is required"})
			return
		}

		var payload struct {
			Name     *string `json:"name"`
			Note     *string `json:"note"`
			APIKey   *string `json:"apiKey"`
			MgmtKey  *string `json:"mgmtKey"`
			Priority *int    `json:"priority"`
			Weight   *int    `json:"weight"`
			Enabled  *bool   `json:"enabled"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
			return
		}

		openRouterMutex.Lock()
		defer openRouterMutex.Unlock()

		cfg, filePath, err := loadOpenRouterConfig()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to load accounts: %v", err)})
			return
		}

		idx := -1
		for i, a := range cfg.Accounts {
			if a.ID == id || a.Name == id {
				idx = i
				break
			}
		}
		if idx == -1 {
			c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("OpenRouter account with id [%s] not found", id)})
			return
		}

		current := &cfg.Accounts[idx]
		if payload.Name != nil && strings.TrimSpace(*payload.Name) != "" {
			cleanName := strings.TrimSpace(*payload.Name)
			for i, a := range cfg.Accounts {
				if i != idx && strings.EqualFold(a.Name, cleanName) {
					c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("Account name [%s] already exists", cleanName)})
					return
				}
			}
			current.Name = cleanName
		}
		if payload.Note != nil {
			current.Note = strings.TrimSpace(*payload.Note)
		}
		if payload.Enabled != nil {
			current.Enabled = *payload.Enabled
		}
		if payload.Priority != nil {
			current.Priority = *payload.Priority
		}
		if payload.Weight != nil {
			current.Weight = *payload.Weight
		}
		if payload.APIKey != nil && strings.TrimSpace(*payload.APIKey) != "" {
			current.APIKey = strings.TrimSpace(*payload.APIKey)
		}
		if payload.MgmtKey != nil && strings.TrimSpace(*payload.MgmtKey) != "" {
			current.MgmtKey = strings.TrimSpace(*payload.MgmtKey)
		}
		current.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

		if err := saveOpenRouterConfig(cfg, filePath); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to save account: %v", err)})
			return
		}

		InvalidateUpstreamQuotaCache()

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"account": OpenRouterAccountItem{
				ID:            current.ID,
				Name:          current.Name,
				Note:          current.Note,
				Enabled:       current.Enabled,
				Priority:      current.Priority,
				Weight:        current.Weight,
				HasAPIKey:     current.APIKey != "",
				APIKeyMasked:  maskSecret(current.APIKey),
				HasMgmtKey:    current.MgmtKey != "",
				MgmtKeyMasked: maskSecret(current.MgmtKey),
				CreatedAt:     current.CreatedAt,
			},
		})
	}
}

// HandleDeleteOpenRouterAccount removes an OpenRouter account.
func HandleDeleteOpenRouterAccount(cpa *CpaClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := strings.TrimSpace(c.Param("id"))
		if id == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Account id is required"})
			return
		}

		openRouterMutex.Lock()
		defer openRouterMutex.Unlock()

		cfg, filePath, err := loadOpenRouterConfig()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to load accounts: %v", err)})
			return
		}

		idx := -1
		for i, a := range cfg.Accounts {
			if a.ID == id || a.Name == id {
				idx = i
				break
			}
		}
		if idx == -1 {
			c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("OpenRouter account with id [%s] not found", id)})
			return
		}

		cfg.Accounts = append(cfg.Accounts[:idx], cfg.Accounts[idx+1:]...)
		if err := saveOpenRouterConfig(cfg, filePath); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to delete account: %v", err)})
			return
		}

		InvalidateUpstreamQuotaCache()

		c.JSON(http.StatusOK, gin.H{"success": true})
	}
}
