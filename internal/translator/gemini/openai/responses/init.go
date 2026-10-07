package responses

import (
	. "github.com/router-for-me/CLIProxyAPI/v8/internal/constant"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/interfaces"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/translator/translator"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v8/sdk/translator"
)

func init() {
	translator.Register(
		OpenaiResponse,
		Gemini,
		nil,
		interfaces.TranslateResponse{
			Stream:    ConvertGeminiResponseToOpenAIResponses,
			NonStream: ConvertGeminiResponseToOpenAIResponsesNonStream,
		},
	)
	sdktranslator.Default().RegisterCheckedRequest(sdktranslator.FormatOpenAIResponse, sdktranslator.FormatGemini, convertOpenAIResponsesRequestToGemini)
}
