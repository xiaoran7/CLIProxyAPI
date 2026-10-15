package signature

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"strings"
	"unicode/utf8"
)

// MaxMetaReasoningSignatureLen bounds the raw base64url text accepted by
// InspectMetaReasoningSignature. It mirrors the GPT bound; the bucket-length
// rule below is far more restrictive in practice.
const MaxMetaReasoningSignatureLen = 32 * 1024 * 1024

const (
	metaReasoningSignatureOverheadLen = 45
	metaReasoningSignatureMinPayload  = 1024
)

// metaReasoningSignatureHeader is the fixed five-byte prefix observed on every
// captured Meta Muse reasoning encrypted_content value (api.meta.ai,
// 2026-10-15 corpus: 82 values across nine muse-* models). Its semantics are
// unresolved - it may be a format magic, a key/context fingerprint, or a
// versioned identifier ending in 0x01 - and every capture came from one
// account on one day, so a future rotation would fall back to Unknown rather
// than being claimed here on weaker evidence. Cache-prefixed forms
// ("meta#...") are intentionally not recognized: no producer emits them, so
// there is no alias in SignatureProviderFromCachePrefix.
var metaReasoningSignatureHeader = []byte{0x43, 0xe3, 0xda, 0x0e, 0x01}

// MetaReasoningSignatureInfo describes the transport shape of a Meta Muse
// reasoning encrypted_content envelope.
type MetaReasoningSignatureInfo struct {
	DecodedLen int
	// PayloadLen is DecodedLen minus the fixed 45-byte envelope overhead. It
	// is the power-of-two padded region, not a proven plaintext length.
	PayloadLen     int
	BucketExponent int
}

func IsValidMetaReasoningSignature(rawSignature string) bool {
	_, err := InspectMetaReasoningSignature(rawSignature)
	return err == nil
}

// InspectMetaReasoningSignature validates the outer envelope shape observed
// for Meta Muse reasoning encrypted_content: canonical unpadded base64url,
// the fixed five-byte header above, and a decoded length of exactly
// 45 + 1024*2^k bytes. This is only a transport-shape check; it does not
// prove decryptability or upstream replay acceptance.
func InspectMetaReasoningSignature(rawSignature string) (*MetaReasoningSignatureInfo, error) {
	sig := strings.TrimSpace(rawSignature)
	if sig == "" {
		return nil, fmt.Errorf("empty Meta reasoning signature")
	}
	if len(sig) > MaxMetaReasoningSignatureLen {
		return nil, fmt.Errorf("Meta reasoning signature exceeds maximum length (%d bytes)", MaxMetaReasoningSignatureLen)
	}
	// The literal prefix is the cheapest discriminator: every observed value
	// starts with "Q-PaDg", and probing this validator is on the hot path for
	// signatures of every provider.
	if !strings.HasPrefix(sig, "Q-PaDg") {
		return nil, fmt.Errorf("invalid Meta reasoning signature: expected Q-PaDg prefix")
	}
	if index, r, ok := firstInvalidMetaReasoningSignatureChar(sig); ok {
		return nil, fmt.Errorf("invalid Meta reasoning signature: contains non-base64url character U+%04X at byte %d", r, index)
	}
	if len(sig)%4 == 1 {
		return nil, fmt.Errorf("invalid Meta reasoning signature: base64url length %d cannot decode", len(sig))
	}
	// Strict decoding rejects non-canonical trailing bits, so no re-encode
	// comparison is needed to prove canonical form.
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(sig)
	if err != nil {
		return nil, fmt.Errorf("invalid Meta reasoning signature: base64url decode failed")
	}
	if len(decoded) < metaReasoningSignatureOverheadLen+metaReasoningSignatureMinPayload {
		return nil, fmt.Errorf("invalid Meta reasoning signature: decoded payload too short")
	}
	if !bytes.Equal(decoded[:len(metaReasoningSignatureHeader)], metaReasoningSignatureHeader) {
		return nil, fmt.Errorf("invalid Meta reasoning signature: unexpected envelope header %x", decoded[:len(metaReasoningSignatureHeader)])
	}
	payloadLen := len(decoded) - metaReasoningSignatureOverheadLen
	if payloadLen&(payloadLen-1) != 0 {
		return nil, fmt.Errorf("invalid Meta reasoning signature: payload length %d is not a power-of-two bucket", payloadLen)
	}
	exponent := 0
	for p := payloadLen; p > metaReasoningSignatureMinPayload; p >>= 1 {
		exponent++
	}
	return &MetaReasoningSignatureInfo{
		DecodedLen:     len(decoded),
		PayloadLen:     payloadLen,
		BucketExponent: exponent,
	}, nil
}

// metaReasoningSignatureCharSet is the unpadded base64url alphabet. Observed
// Meta values never carry '=' padding, so padding is rejected rather than
// normalized away.
var metaReasoningSignatureCharSet = base64AlphabetSet("-_")

func firstInvalidMetaReasoningSignatureChar(sig string) (int, rune, bool) {
	for index := 0; index < len(sig); index++ {
		if !metaReasoningSignatureCharSet[sig[index]] {
			r, _ := utf8.DecodeRuneInString(sig[index:])
			return index, r, true
		}
	}
	return 0, 0, false
}
