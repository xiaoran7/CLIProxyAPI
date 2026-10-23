package zcode

import (
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

// beijingLocation is the timezone Z.ai and BigModel state plan times in.
var beijingLocation = time.FixedZone("CST", 8*3600)

// beijingDatePattern matches the dates a plan's validity text names.
var beijingDatePattern = regexp.MustCompile(`\d{4}-\d{2}-\d{2}(?:[ T]\d{2}:\d{2}:\d{2})?`)

// parseBeijing reads a plan time stated in Beijing time.
func parseBeijing(text string) time.Time {
	cleaned := strings.TrimSpace(strings.Replace(text, " ", "T", 1))
	if cleaned == "" {
		return time.Time{}
	}
	layout := "2006-01-02T15:04:05"
	if len(cleaned) == len("2006-01-02") {
		layout = "2006-01-02"
	}
	parsed, errParse := time.ParseInLocation(layout, cleaned, beijingLocation)
	if errParse != nil {
		return time.Time{}
	}
	return parsed
}

// parsePlanTime reads a plan time given as Beijing text, a Unix second count,
// or a Unix millisecond count.
func parsePlanTime(value gjson.Result) time.Time {
	if value.Type == gjson.String {
		if parsed := parseBeijing(value.String()); !parsed.IsZero() {
			return parsed
		}
	}
	number := startNumber(value)
	switch {
	case number <= 0:
		return time.Time{}
	case number > 1e12:
		return time.UnixMilli(int64(number))
	default:
		return time.Unix(int64(number), 0)
	}
}

func sortStrings(values []string) {
	sort.Strings(values)
}
