// Package qoder provides authentication, COSY request signing, and token management
// for Qoder Global and Qoder Mainland China APIs.
package qoder

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
)

const defaultCosyRSAPublicKeyPEM = `-----BEGIN PUBLIC KEY-----
MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDA8iMH5c02LilrsERw9t6Pv5Nc
4k6Pz1EaDicBMpdpxKduSZu5OANqUq8er4GM95omAGIOPOh+Nx0spthYA2BqGz+l
6HRkPJ7S236FZz73In/KVuLnwI8JJ2CbuJap8kvheCCZpmAWpb/cPx/3Vr/J6I17
XcW+ML9FoCI6AOvOzwIDAQAB
-----END PUBLIC KEY-----`

var (
	parsedRSAPublicKey *rsa.PublicKey
	parseRSAOnce       sync.Once
	parseRSAErr        error
)

// getCosyRSAPublicKey parses and returns the singleton COSY RSA public key.
func getCosyRSAPublicKey() (*rsa.PublicKey, error) {
	parseRSAOnce.Do(func() {
		block, _ := pem.Decode([]byte(defaultCosyRSAPublicKeyPEM))
		if block == nil {
			parseRSAErr = errors.New("qoder cosy: failed to decode PEM block for RSA public key")
			return
		}
		pub, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			parseRSAErr = fmt.Errorf("qoder cosy: failed to parse RSA public key: %w", err)
			return
		}
		rsaPub, ok := pub.(*rsa.PublicKey)
		if !ok {
			parseRSAErr = errors.New("qoder cosy: key is not an RSA public key")
			return
		}
		parsedRSAPublicKey = rsaPub
	})
	return parsedRSAPublicKey, parseRSAErr
}

// pkcs7Pad pads plain buffer to block size with PKCS#7 standard.
func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - (len(data) % blockSize)
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(data, padtext...)
}

// encryptAES128CBC encrypts plaintext using AES-128-CBC with the given 16-byte key and IV.
func encryptAES128CBC(key, iv, plaintext []byte) ([]byte, error) {
	block, errBlock := aes.NewCipher(key)
	if errBlock != nil {
		return nil, fmt.Errorf("qoder cosy: new aes cipher: %w", errBlock)
	}
	padded := pkcs7Pad(plaintext, aes.BlockSize)
	ciphertext := make([]byte, len(padded))
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ciphertext, padded)
	return ciphertext, nil
}

// CosySignature holds components of a computed COSY authentication header and companion values.
type CosySignature struct {
	Authorization string
	CosyKey       string
	CosyDate      string
	CosyUser      string
	PayloadB64    string
	Signature     string
}

// ComputeCosySignature computes the COSY Authorization header and companion Cosy-Key.
// signingPath is the URL path (e.g. "/algo/api/v2/model/list").
// requestBody is the raw body string of the request.
func ComputeCosySignature(uid, jobToken, signingPath, requestBody string, nowSec int64) (*CosySignature, error) {
	return ComputeCosySignatureWithIdentity(uid, uid, "personal_professional_trial", jobToken, "", signingPath, requestBody, nowSec)
}

// ComputeCosySignatureWithIdentity computes the COSY Authorization header using the canonical 9-field identity schema.
func ComputeCosySignatureWithIdentity(uid, nickname, userType, token, refreshToken, signingPath, requestBody string, nowSec int64) (*CosySignature, error) {
	rsaPub, errPub := getCosyRSAPublicKey()
	if errPub != nil {
		return nil, errPub
	}

	rawUUID := uuid.New().String()
	tempKeyHex := strings.ReplaceAll(rawUUID, "-", "")[:16]
	tempKeyBytes := []byte(tempKeyHex)

	// 1. RSA encrypt tempKey (PKCS#1 v1.5)
	encryptedKey, errRSA := rsa.EncryptPKCS1v15(rand.Reader, rsaPub, tempKeyBytes)
	if errRSA != nil {
		return nil, fmt.Errorf("qoder cosy: rsa encrypt temp key failed: %w", errRSA)
	}
	cosyKey := base64.StdEncoding.EncodeToString(encryptedKey)

	// 2. AES-128-CBC encrypt canonical identity
	if userType == "" {
		userType = "personal_professional_trial"
	}
	if nickname == "" {
		nickname = uid
	}
	identity := map[string]string{
		"name":                 nickname,
		"aid":                  uid,
		"uid":                  uid,
		"yx_uid":               "",
		"organization_id":      "",
		"organization_name":    "",
		"user_type":            userType,
		"security_oauth_token": token,
		"refresh_token":        refreshToken,
	}
	infoJSON, errJSON := json.Marshal(identity)
	if errJSON != nil {
		return nil, fmt.Errorf("qoder cosy: marshal identity json failed: %w", errJSON)
	}

	iv := tempKeyBytes
	encryptedInfo, errAES := encryptAES128CBC(tempKeyBytes, iv, infoJSON)
	if errAES != nil {
		return nil, fmt.Errorf("qoder cosy: aes encrypt info failed: %w", errAES)
	}
	encryptedInfoB64 := base64.StdEncoding.EncodeToString(encryptedInfo)

	// 3. Construct payload JSON
	payloadObj := map[string]string{
		"cosyVersion": CosyVersion,
		"ideVersion":  "",
		"info":        encryptedInfoB64,
		"requestId":   uuid.New().String(),
		"version":     "v1",
	}
	payloadBytes, errPayload := json.Marshal(payloadObj)
	if errPayload != nil {
		return nil, fmt.Errorf("qoder cosy: marshal payload json failed: %w", errPayload)
	}
	payloadB64 := base64.StdEncoding.EncodeToString(payloadBytes)

	// 4. Compute signature hash
	// Normalize path by stripping /algo prefix if present
	normPath := signingPath
	if strings.HasPrefix(normPath, "/algo") {
		normPath = strings.TrimPrefix(normPath, "/algo")
	}
	if normPath == "" {
		normPath = "/"
	}

	dateStr := fmt.Sprintf("%d", nowSec)
	signingString := fmt.Sprintf("%s\n%s\n%s\n%s\n%s", payloadB64, cosyKey, dateStr, requestBody, normPath)
	hash := md5.Sum([]byte(signingString))
	sigHex := hex.EncodeToString(hash[:])

	authHeader := fmt.Sprintf("Bearer COSY.%s.%s", payloadB64, sigHex)

	return &CosySignature{
		Authorization: authHeader,
		CosyKey:       cosyKey,
		CosyDate:      dateStr,
		CosyUser:      uid,
		PayloadB64:    payloadB64,
		Signature:     sigHex,
	}, nil
}

// SignRequest injects standard COSY headers, machine identifiers, and Bearer token into an HTTP request.
func SignRequest(req *http.Request, auth *cliproxyauth.Auth, jobToken, requestBody string) error {
	if req == nil {
		return errors.New("qoder cosy: request is nil")
	}
	uid := ResolveUserID(auth)
	nickname := ResolveNickname(auth)
	machineID := ResolveMachineID(auth)
	token := jobToken
	if token == "" {
		token = AccessToken(auth)
	}
	refreshToken := ResolveRefreshToken(auth)
	userType := "personal_professional_trial"
	if auth != nil && auth.Metadata != nil {
		if ut, ok := auth.Metadata["user_type"].(string); ok && strings.TrimSpace(ut) != "" {
			userType = strings.TrimSpace(ut)
		}
	}
	nowSec := time.Now().Unix()

	path := req.URL.Path
	if path == "" {
		path = "/"
	}

	sig, errSig := ComputeCosySignatureWithIdentity(uid, nickname, userType, token, refreshToken, path, requestBody, nowSec)
	if errSig != nil {
		return errSig
	}

	req.Header.Set("Authorization", sig.Authorization)
	req.Header.Set("Cosy-Key", sig.CosyKey)
	req.Header.Set("Cosy-Date", sig.CosyDate)
	req.Header.Set("Cosy-User", sig.CosyUser)
	req.Header.Set("Cosy-Machineid", machineID)
	req.Header.Set("Cosy-Machinetoken", machineID)
	req.Header.Set("Cosy-Machinetype", CosyMachineType)
	req.Header.Set("Cosy-Machineos", "x86_64_windows")
	req.Header.Set("Cosy-Clienttype", CosyClientType)
	req.Header.Set("Cosy-Version", CosyVersion)
	req.Header.Set("Cosy-Data-Policy", "AGREE")
	req.Header.Set("User-Agent", DefaultUserAgent)
	if req.Header.Get("x-model-key") == "" {
		req.Header.Set("x-model-key", "auto")
	}
	return nil
}
