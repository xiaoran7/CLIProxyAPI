package chat_completions

import (
	. "github.com/router-for-me/CLIProxyAPI/v8/internal/constant"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/interfaces"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/translator/translator"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v8/sdk/translator"
)

func init() {
	translator.Register(
		OpenAI,
		Claude,
		nil,
		interfaces.TranslateResponse{
			Stream:    ConvertClaudeResponseToOpenAI,
			NonStream: ConvertClaudeResponseToOpenAINonStream,
		},
	)
	sdktranslator.Default().RegisterCheckedRequest(sdktranslator.FormatOpenAI, sdktranslator.FormatClaude, func(model string, body []byte, stream bool) ([]byte, error) {
		return convertOpenAIRequestToClaude(model, body, stream, false)
	})
}
