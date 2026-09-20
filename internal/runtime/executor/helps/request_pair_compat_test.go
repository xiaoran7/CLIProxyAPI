package helps

import (
	"bytes"
	"context"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"
)

func TestCompatibilityRequestPair(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{}
	from, to := sdktranslator.FormatOpenAIResponse, sdktranslator.FormatOpenAI
	for _, compat := range []bool{false, true} {
		for _, stream := range []bool{false, true} {
			for _, distinct := range []bool{false, true} {
				original := []byte(`{"model":"test","input":"hello"}`)
				request := original
				if distinct {
					request = []byte(`{"model":"test","input":"changed"}`)
				}
				wantOriginal := TranslateRequestWithAPIKeyModelCompatibility(ctx, nil, cfg, from, to, "test", original, stream, compat)
				wantWorking := TranslateRequestWithAPIKeyModelCompatibility(ctx, nil, cfg, from, to, "test", request, stream, compat)
				base, work := TranslateRequestPairWithAPIKeyModelCompatibility(ctx, nil, cfg, from, to, "test", original, request, stream, compat)
				if !bytes.Equal(base, wantOriginal) || !bytes.Equal(work, wantWorking) {
					t.Fatalf("pair changed translation: compat=%v stream=%v distinct=%v", compat, stream, distinct)
				}
				work[0] = '!'
				if !bytes.Equal(base, wantOriginal) || original[0] != '{' {
					t.Fatal("working buffer aliases baseline or input")
				}
			}
		}
	}
}

func TestCompatibilityRequestPairPreservesHooks(t *testing.T) {
	hooks := &pairRequestPluginHooks{}
	sdktranslator.SetPluginHooks(hooks)
	t.Cleanup(func() { sdktranslator.SetPluginHooks(nil) })
	request := []byte(`{"model":"test","input":"hello"}`)
	base, work := TranslateRequestPairWithAPIKeyModelCompatibility(context.Background(), nil, &config.Config{}, sdktranslator.FormatOpenAIResponse, sdktranslator.FormatOpenAI, "test", request, request, true, true)
	if hooks.calls != 2 || bytes.Equal(base, work) {
		t.Fatalf("stateful plugin calls must remain independent, calls=%d", hooks.calls)
	}
}
