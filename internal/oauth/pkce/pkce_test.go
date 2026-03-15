package pkce

import (
	"testing"
)

func TestGeneratePKCECodes(t *testing.T) {
	codes, err := GeneratePKCECodes()
	if err != nil {
		t.Fatalf("GeneratePKCECodes() error = %v", err)
	}

	// Verify verifier is 128 characters (96 bytes base64url encoded)
	if len(codes.CodeVerifier) != 128 {
		t.Errorf("CodeVerifier length = %d, want 128", len(codes.CodeVerifier))
	}

	// Verify verifier is base64url encoded (no padding, only alphanumeric, -, _)
	for _, c := range codes.CodeVerifier {
		if !isBase64URLRune(c) {
			t.Errorf("CodeVerifier contains invalid character: %c", c)
		}
	}

	// Verify challenge is 43 characters (32 bytes SHA256 base64url encoded)
	if len(codes.CodeChallenge) != 43 {
		t.Errorf("CodeChallenge length = %d, want 43", len(codes.CodeChallenge))
	}

	// Verify challenge is valid base64url
	for _, c := range codes.CodeChallenge {
		if !isBase64URLRune(c) {
			t.Errorf("CodeChallenge contains invalid character: %c", c)
		}
	}
}

func TestGeneratePKCECodesUniqueness(t *testing.T) {
	codes1, _ := GeneratePKCECodes()
	codes2, _ := GeneratePKCECodes()

	if codes1.CodeVerifier == codes2.CodeVerifier {
		t.Error("Two generated verifiers should be different")
	}
	if codes1.CodeChallenge == codes2.CodeChallenge {
		t.Error("Two generated challenges should be different")
	}
}

func TestValidatePKCE(t *testing.T) {
	codes, err := GeneratePKCECodes()
	if err != nil {
		t.Fatalf("GeneratePKCECodes() error = %v", err)
	}

	// Valid pair
	if !ValidatePKCE(codes.CodeVerifier, codes.CodeChallenge) {
		t.Error("ValidatePKCE() returned false for valid pair")
	}

	// Invalid verifier
	if ValidatePKCE("invalid_verifier", codes.CodeChallenge) {
		t.Error("ValidatePKCE() returned true for invalid verifier")
	}

	// Invalid challenge
	if ValidatePKCE(codes.CodeVerifier, "invalid_challenge") {
		t.Error("ValidatePKCE() returned true for invalid challenge")
	}
}

func TestPKCEChallengeCalculation(t *testing.T) {
	// Test with known values from RFC 7636
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	expectedChallenge := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"

	challenge := calculateChallenge(verifier)
	if challenge != expectedChallenge {
		t.Errorf("calculateChallenge() = %s, want %s", challenge, expectedChallenge)
	}
}

func isBase64URLRune(r rune) bool {
	return (r >= 'A' && r <= 'Z') ||
		(r >= 'a' && r <= 'z') ||
		(r >= '0' && r <= '9') ||
		r == '-' || r == '_'
}