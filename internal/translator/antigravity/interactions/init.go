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
		Antigravity,
		nil,
		interfaces.TranslateResponse{
			Stream:    ConvertAntigravityResponseToInteractions,
			NonStream: ConvertAntigravityResponseToInteractionsNonStream,
		},
	)
	sdktranslator.Default().RegisterCheckedRequest(sdktranslator.FormatInteractions, sdktranslator.FormatAntigravity, convertInteractionsRequestToAntigravity)
}
