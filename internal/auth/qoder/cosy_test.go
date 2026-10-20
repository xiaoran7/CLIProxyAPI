package qoder

import (
	"net/http"
	"strings"
	"testing"
	"time"

	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
)

func TestGetCosyRSAPublicKey(t *testing.T) {
	key, err := getCosyRSAPublicKey()
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if key == nil {
		t.Fatal("expected non-nil RSA public key")
	}
	if key.N.BitLen() < 1024 {
		t.Fatalf("expected key bit length >= 1024, got %d", key.N.BitLen())
	}
}

func TestPKCS7Pad(t *testing.T) {
	data := []byte("hello")
	padded := pkcs7Pad(data, 16)
	if len(padded) != 16 {
		t.Fatalf("expected length 16, got %d", len(padded))
	}
	// Padding should be 11 bytes of value 11
	expectedPad := byte(11)
	for i := 5; i < 16; i++ {
		if padded[i] != expectedPad {
			t.Fatalf("byte at %d was %d, expected %d", i, padded[i], expectedPad)
		}
	}
}

func TestEncryptAES128CBC(t *testing.T) {
	key := []byte("0123456789abcdef")
	iv := key
	plaintext := []byte(`{"sub":"user_123","token":"jt-abc"}`)

	ciphertext, err := encryptAES128CBC(key, iv, plaintext)
	if err != nil {
		t.Fatalf("encryption failed: %v", err)
	}
	if len(ciphertext)%16 != 0 {
		t.Fatalf("expected multiple of 16, got %d", len(ciphertext))
	}
}

func TestComputeCosySignature(t *testing.T) {
	uid := "user_test_999"
	jobToken := "jt-test-token"
	path := "/algo/api/v2/service/pro/sse/agent_chat_generation"
	body := `{"model":"qwen3.8-flash","messages":[]}`
	nowSec := time.Now().Unix()

	sig, err := ComputeCosySignature(uid, jobToken, path, body, nowSec)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if sig == nil {
		t.Fatal("expected non-nil signature")
	}
	if !strings.HasPrefix(sig.Authorization, "Bearer COSY.") {
		t.Fatalf("expected 'Bearer COSY.' prefix, got: %s", sig.Authorization)
	}
	if sig.CosyKey == "" {
		t.Fatal("expected non-empty CosyKey")
	}
	if sig.Signature == "" || len(sig.Signature) != 32 {
		t.Fatalf("expected 32-char hex md5 signature, got: %s", sig.Signature)
	}
	if sig.CosyUser != uid {
		t.Fatalf("expected CosyUser %s, got: %s", uid, sig.CosyUser)
	}
}

func TestSignRequest(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "https://gateway.qoder.com.cn/algo/api/v2/model/list", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("create request failed: %v", err)
	}

	auth := &cliproxyauth.Auth{
		ID:       "test-qoder-cn",
		Provider: "qoder-cn",
		Metadata: map[string]any{
			"user_id":   "uid_12345",
			"realm":     "cn",
			"job_token": "jt-mock-token",
		},
	}

	err = SignRequest(req, auth, "jt-mock-token", "{}")
	if err != nil {
		t.Fatalf("SignRequest failed: %v", err)
	}

	if !strings.HasPrefix(req.Header.Get("Authorization"), "Bearer COSY.") {
		t.Errorf("Authorization header missing Bearer COSY prefix: %s", req.Header.Get("Authorization"))
	}
	if req.Header.Get("Cosy-Key") == "" {
		t.Error("Cosy-Key header is empty")
	}
	if req.Header.Get("Cosy-User") != "uid_12345" {
		t.Errorf("Cosy-User expected uid_12345, got %s", req.Header.Get("Cosy-User"))
	}
	if req.Header.Get("Cosy-Machinetype") != "5" {
		t.Errorf("Cosy-Machinetype expected 5, got %s", req.Header.Get("Cosy-Machinetype"))
	}
	if req.Header.Get("Cosy-Version") != "0.1.43" {
		t.Errorf("Cosy-Version expected 0.1.43, got %s", req.Header.Get("Cosy-Version"))
	}
}
