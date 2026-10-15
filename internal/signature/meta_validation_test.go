package signature

import (
	"encoding/base64"
	"strings"
	"testing"
)

// testMetaReasoningSignature builds a synthetic Meta Muse envelope with the
// observed fixed header and a power-of-two payload bucket. The body bytes are
// deterministic filler; recognition is a transport-shape check, so no real
// captured value is needed (or permitted) here.
func testMetaReasoningSignature(payloadLen int) string {
	return base64.RawURLEncoding.EncodeToString(testMetaReasoningSignatureBytes(payloadLen, nil))
}

func testMetaReasoningSignatureBytes(payloadLen int, mutate func([]byte)) []byte {
	decoded := make([]byte, metaReasoningSignatureOverheadLen+payloadLen)
	copy(decoded, metaReasoningSignatureHeader)
	for i := len(metaReasoningSignatureHeader); i < len(decoded); i++ {
		decoded[i] = byte(i*31 + 7)
	}
	if mutate != nil {
		mutate(decoded)
	}
	return decoded
}

func TestInspectMetaReasoningSignature_ValidBuckets(t *testing.T) {
	for _, tc := range []struct {
		payload  int
		exponent int
	}{
		{1024, 0},
		{2048, 1},
		{4096, 2},
		{32768, 5},
	} {
		sig := testMetaReasoningSignature(tc.payload)
		info, err := InspectMetaReasoningSignature(sig)
		if err != nil {
			t.Fatalf("payload %d: InspectMetaReasoningSignature error = %v", tc.payload, err)
		}
		if info.DecodedLen != tc.payload+metaReasoningSignatureOverheadLen || info.PayloadLen != tc.payload || info.BucketExponent != tc.exponent {
			t.Fatalf("payload %d: info = %+v", tc.payload, info)
		}
		if !IsValidMetaReasoningSignature(sig) {
			t.Fatalf("payload %d: IsValidMetaReasoningSignature = false", tc.payload)
		}
		if got := DetectSignatureProvider(sig); got != SignatureProviderMeta {
			t.Fatalf("payload %d: DetectSignatureProvider = %q, want %q", tc.payload, got, SignatureProviderMeta)
		}
	}
}

func TestInspectMetaReasoningSignature_RejectsMalformed(t *testing.T) {
	valid := testMetaReasoningSignature(1024)
	encode := func(payloadLen int, mutate func([]byte)) string {
		return base64.RawURLEncoding.EncodeToString(testMetaReasoningSignatureBytes(payloadLen, mutate))
	}
	nonCanonical := func() string {
		const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
		last := strings.IndexByte(alphabet, valid[len(valid)-1])
		for i := 0; i < len(alphabet); i++ {
			if i != last && i&0x30 == last&0x30 {
				return valid[:len(valid)-1] + string(alphabet[i])
			}
		}
		return valid
	}()
	cases := map[string]string{
		"empty":               "",
		"mock prefix only":    "Q-PaDg-mock-muse-spark-encrypted-reasoning-payload",
		"wrong literal start": "R" + valid[1:],
		"header byte flipped": encode(1024, func(b []byte) { b[2] ^= 0xff }),
		"version byte zero":   encode(1024, func(b []byte) { b[4] = 0x00 }),
		"payload below min":   encode(512, nil),
		"payload not power2":  encode(1025, nil),
		"payload off by one":  encode(2047, nil),
		"padded encoding":     valid + "=",
		"standard base64 +":   valid[:100] + "+" + valid[101:],
		"embedded whitespace": valid[:100] + " " + valid[100:],
		"length mod4 is 1":    valid[:len(valid)-1],
		"non-canonical bits":  nonCanonical,
	}
	for name, sig := range cases {
		if _, err := InspectMetaReasoningSignature(sig); err == nil {
			t.Errorf("%s: InspectMetaReasoningSignature unexpectedly succeeded", name)
		}
		if got := DetectSignatureProvider(sig); got == SignatureProviderMeta {
			t.Errorf("%s: DetectSignatureProvider = %q, must not claim Meta", name, got)
		}
	}
}

func TestMetaSignatureCompatibility(t *testing.T) {
	sig := testMetaReasoningSignature(2048)

	decision := DecideSignatureCompatibility(SignatureProviderMeta, sig, SignatureBlockKindUnknown)
	if !decision.Compatible || decision.Action != SignatureActionPreserve || decision.NormalizedSignature != sig {
		t.Fatalf("Meta target decision = %+v, want preserve exact signature", decision)
	}
	if got, ok := CompatibleSignatureForProvider(SignatureProviderMeta, sig); !ok || got != sig {
		t.Fatalf("CompatibleSignatureForProvider(Meta) = (%q..., %v), want exact signature", got[:16], ok)
	}

	gptDecision := DecideSignatureCompatibility(SignatureProviderGPT, sig, SignatureBlockKindUnknown)
	if gptDecision.Compatible || gptDecision.Action != SignatureActionDropBlock {
		t.Fatalf("GPT target decision = %+v, want drop_block", gptDecision)
	}

	foreign := testGPTReasoningSignature()
	metaDecision := DecideSignatureCompatibility(SignatureProviderMeta, foreign, SignatureBlockKindUnknown)
	if metaDecision.Compatible || metaDecision.DetectedProvider != SignatureProviderGPT || metaDecision.Action != SignatureActionDropBlock {
		t.Fatalf("Meta target with GPT signature decision = %+v, want detected GPT + drop_block", metaDecision)
	}
}

func TestSignatureProviderFromModelName_Meta(t *testing.T) {
	for _, model := range []string{"muse-wonder-full-1.0-contributor", "muse-spark-1.3", "Muse-Wonder-1.0", "meta/muse-spark-1.2"} {
		if got := SignatureProviderFromModelName(model); got != SignatureProviderMeta {
			t.Errorf("SignatureProviderFromModelName(%q) = %q, want %q", model, got, SignatureProviderMeta)
		}
	}
	// The match is anchored to the "muse-" model-id boundary so unrelated
	// names containing the substring "muse" stay unmapped.
	for _, model := range []string{"museum-1", "amuse-model", "meta-llama-4"} {
		if got := SignatureProviderFromModelName(model); got != SignatureProviderUnknown {
			t.Errorf("SignatureProviderFromModelName(%q) = %q, want %q", model, got, SignatureProviderUnknown)
		}
	}
}

func TestMetaSignatureCompatibility_OtherTargets(t *testing.T) {
	sig := testMetaReasoningSignature(1024)
	for _, tc := range []struct {
		target SignatureProvider
		action SignatureCompatibilityAction
	}{
		{SignatureProviderClaude, SignatureActionDropBlock},
		{SignatureProviderGemini, SignatureActionReplaceWithGeminiBypass},
		{SignatureProviderKimi, SignatureActionDropSignature},
		{SignatureProviderGrok, SignatureActionDropBlock},
	} {
		decision := DecideSignatureCompatibility(tc.target, sig, SignatureBlockKindUnknown)
		if decision.Compatible || decision.DetectedProvider != SignatureProviderMeta || decision.Action != tc.action {
			t.Errorf("target %q: decision = %+v, want detected Meta + action %q", tc.target, decision, tc.action)
		}
	}
}

func TestMetaCachePrefixUnsupported(t *testing.T) {
	// No producer emits provider-prefixed Meta signatures, so unlike GPT or
	// Claude there is deliberately no cache-prefix alias for Meta yet.
	sig := testMetaReasoningSignature(1024)
	for _, prefixed := range []string{"meta#" + sig, "muse#" + sig} {
		if got := DetectSignatureProvider(prefixed); got != SignatureProviderUnknown {
			t.Errorf("DetectSignatureProvider(%q...) = %q, want %q", prefixed[:12], got, SignatureProviderUnknown)
		}
		if _, ok := CompatibleSignatureForProvider(SignatureProviderMeta, prefixed); ok {
			t.Errorf("CompatibleSignatureForProvider(Meta, %q...) unexpectedly compatible", prefixed[:12])
		}
	}
}
