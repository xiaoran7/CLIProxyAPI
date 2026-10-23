package api

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/engine"
)

var (
	cstZone          = time.FixedZone("CST", 8*3600)
	effortModelRegex = regexp.MustCompile(`[-_:](high|medium|low|max)$`)
)

func resolveReasoningEffort(effort, model string) string {
	e := strings.TrimSpace(effort)
	if e != "" {
		return e
	}
	m := strings.ToLower(strings.TrimSpace(model))
	match := effortModelRegex.FindStringSubmatch(m)
	if len(match) > 1 {
		return match[1]
	}
	return ""
}

func resolveTimeRange(timeRange, sinceStr, untilStr string) (since *int64, until *int64) {
	nowSec := time.Now().Unix()
	if sinceStr != "" {
		if s, err := strconv.ParseInt(sinceStr, 10, 64); err == nil {
			if s > 1e11 {
				s = s / 1000
			}
			since = &s
		}
	}
	if untilStr != "" {
		if u, err := strconv.ParseInt(untilStr, 10, 64); err == nil {
			if u > 1e11 {
				u = u / 1000
			}
			until = &u
		}
	}

	if since == nil && timeRange != "" && timeRange != "all" {
		switch timeRange {
		case "1h":
			s := nowSec - 3600
			since = &s
		case "today":
			todayStr := engine.ShanghaiDate(time.Now())
			t, err := time.ParseInLocation("2006-01-02", todayStr, cstZone)
			if err == nil {
				s := t.Unix()
				since = &s
			}
		case "24h":
			s := nowSec - 86400
			since = &s
		case "3d":
			s := nowSec - 3*86400
			since = &s
		case "7d":
			s := nowSec - 7*86400
			since = &s
		case "30d":
			s := nowSec - 30*86400
			since = &s
		}
	}
	return since, until
}
