package claude

import (
	. "github.com/router-for-me/CLIProxyAPI/v8/internal/constant"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/interfaces"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/translator/translator"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v8/sdk/translator"
)

func init() {
	translator.Register(
		Claude,
		Interactions,
		nil,
		interfaces.TranslateResponse{
			Stream:    ConvertInteractionsResponseToClaude,
			NonStream: ConvertInteractionsResponseToClaudeNonStream,
		},
	)
	sdktranslator.Default().RegisterCheckedRequest(sdktranslator.FormatClaude, sdktranslator.FormatInteractions, func(model string, body []byte, stream bool) ([]byte, error) {
		return convertClaudeRequestToInteractions(model, body, stream, false)
	})
}
