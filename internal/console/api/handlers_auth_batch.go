package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// HandlePutUpstreamAuthBatch processes batch actions on upstream credentials.
func HandlePutUpstreamAuthBatch(cpa *CpaClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		var payload struct {
			Names    []string    `json:"names"`
			Action   string      `json:"action"`
			Value    interface{} `json:"value"`
			Priority *int        `json:"priority"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
			return
		}

		if len(payload.Names) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Please select at least one auth file"})
			return
		}
		if len(payload.Names) > 200 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot batch process more than 200 files at once"})
			return
		}

		validActions := map[string]bool{
			"enable": true, "disable": true, "priority": true, "reset-cooldown": true, "delete": true,
		}
		if !validActions[payload.Action] {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Unsupported batch action: %s", payload.Action)})
			return
		}

		ctx := c.Request.Context()
		type BatchResultItem struct {
			Name  string `json:"name"`
			OK    bool   `json:"ok"`
			Error string `json:"error,omitempty"`
		}

		var results []BatchResultItem
		succeeded := 0

		authIndexMap := make(map[string]string)
		if payload.Action == "reset-cooldown" {
			var authFilesResp struct {
				Files []struct {
					Name      string `json:"name"`
					AuthIndex string `json:"auth_index"`
				} `json:"files"`
			}
			if err := cpa.RequestJSON(ctx, http.MethodGet, "/auth-files", nil, "v0", &authFilesResp); err == nil {
				for _, f := range authFilesResp.Files {
					if f.Name != "" && f.AuthIndex != "" {
						authIndexMap[f.Name] = f.AuthIndex
					}
				}
			}
		}

		for _, name := range payload.Names {
			if !isSafeAuthFilename(name) {
				results = append(results, BatchResultItem{Name: name, OK: false, Error: "Invalid auth filename"})
				continue
			}

			var err error
			var resp *CpaResponse
			switch payload.Action {
			case "enable":
				b, _ := json.Marshal(map[string]interface{}{"name": name, "disabled": false})
				resp, err = cpa.Request(ctx, http.MethodPatch, "/auth-files/status", b, "application/json", "v0")
			case "disable":
				b, _ := json.Marshal(map[string]interface{}{"name": name, "disabled": true})
				resp, err = cpa.Request(ctx, http.MethodPatch, "/auth-files/status", b, "application/json", "v0")
			case "priority":
				prio := 0
				if payload.Priority != nil {
					prio = *payload.Priority
				} else if payload.Value != nil {
					switch v := payload.Value.(type) {
					case float64:
						prio = int(v)
					case string:
						prio, _ = strconv.Atoi(v)
					}
				}
				b, _ := json.Marshal(map[string]interface{}{"name": name, "priority": prio})
				resp, err = cpa.Request(ctx, http.MethodPatch, "/auth-files/fields", b, "application/json", "v0")
			case "reset-cooldown":
				authIdx := authIndexMap[name]
				if authIdx == "" {
					authIdx = name
				}
				b, _ := json.Marshal(map[string]string{"auth_index": authIdx})
				resp, err = cpa.Request(ctx, http.MethodPost, "/reset-quota", b, "application/json", "v0")
			case "delete":
				targetURL := fmt.Sprintf("/auth-files?name=%s", name)
				resp, err = cpa.Request(ctx, http.MethodDelete, targetURL, nil, "application/json", "v0")
			}

			if err != nil {
				results = append(results, BatchResultItem{Name: name, OK: false, Error: err.Error()})
			} else if resp != nil && (resp.StatusCode < 200 || resp.StatusCode >= 300) {
				errMsg := fmt.Sprintf("status %d", resp.StatusCode)
				var errResp map[string]interface{}
				if json.Unmarshal(resp.Bytes, &errResp) == nil && errResp["error"] != nil {
					errMsg = fmt.Sprintf("%v", errResp["error"])
				} else if len(resp.Bytes) > 0 {
					errMsg = string(resp.Bytes)
				}
				results = append(results, BatchResultItem{Name: name, OK: false, Error: errMsg})
			} else {
				results = append(results, BatchResultItem{Name: name, OK: true})
				succeeded++
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"results":   results,
			"succeeded": succeeded,
			"total":     len(payload.Names),
		})
	}
}
