package interactions

import (
	. "github.com/router-for-me/CLIProxyAPI/v8/internal/constant"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/interfaces"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/translator/translator"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v8/sdk/translator"
)

func init() {
	translator.Register(
		Interactions,
		Codex,
		nil,
		interfaces.TranslateResponse{
			Stream:    ConvertCodexResponseToInteractions,
			NonStream: ConvertCodexResponseToInteractionsNonStream,
		},
	)
	sdktranslator.Default().RegisterCheckedRequest(sdktranslator.FormatInteractions, sdktranslator.FormatCodex, convertInteractionsRequestToCodex)
}
