package zcode

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/tidwall/gjson"
)

// setOriginForTest points ZCode's own API reads at a test server.
func setOriginForTest(origin string) {
	zcodeOrigin = origin
}

// timeNowForTest reads the package clock.
func timeNowForTest() time.Time { return nowFunc() }

// testJWT mints an unsigned JWT whose expiry is offset seconds from now.
func testJWT(t *testing.T, offset int64) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	claims, errMarshal := json.Marshal(map[string]any{
		"exp": time.Now().Add(time.Duration(offset) * time.Second).Unix(),
		"sub": "test",
	})
	if errMarshal != nil {
		t.Fatalf("marshal claims: %v", errMarshal)
	}
	payload := base64.RawURLEncoding.EncodeToString(claims)
	return fmt.Sprintf("%s.%s.signature", header, payload)
}

// mustResult parses JSON the test wrote itself.
func mustResult(t *testing.T, raw string) gjson.Result {
	t.Helper()
	if !gjson.Valid(raw) {
		t.Fatalf("test wrote invalid JSON: %s", raw)
	}
	return gjson.Parse(raw)
}
