package pkce

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"strings"
)

// PKCECodes contains the verifier and challenge codes for PKCE flow
type PKCECodes struct {
	CodeVerifier  string // Random 128-char string (96 bytes base64url encoded)
	CodeChallenge string // SHA256(verifier), base64url encoded (43 chars)
}

// GeneratePKCECodes generates PKCE codes for OAuth flow
// Returns a new PKCECodes struct with verifier and challenge
func GeneratePKCECodes() (*PKCECodes, error) {
	// Generate 96 random bytes
	verifierBytes := make([]byte, 96)
	if _, err := rand.Read(verifierBytes); err != nil {
		return nil, err
	}

	// Base64URL encode without padding (results in 128 characters)
	verifier := base64URLEncode(verifierBytes)

	// Calculate challenge: SHA256(verifier), base64url encoded
	challenge := calculateChallenge(verifier)

	return &PKCECodes{
		CodeVerifier:  verifier,
		CodeChallenge: challenge,
	}, nil
}

// ValidatePKCE validates that the code_verifier matches the challenge
// Returns true if verifier is valid for the given challenge
func ValidatePKCE(verifier, challenge string) bool {
	calculatedChallenge := calculateChallenge(verifier)
	return calculatedChallenge == challenge
}

// calculateChallenge calculates the code challenge from verifier
// challenge = BASE64URL(SHA256(ASCII(verifier)))
func calculateChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64URLEncode(h[:])
}

// base64URLEncode encodes bytes to base64url without padding
func base64URLEncode(data []byte) string {
	return strings.TrimRight(base64.URLEncoding.EncodeToString(data), "=")
}