package common

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

func testOAuthState(
	t *testing.T,
	provider string,
	userID int,
	expiresAt int64,
) string {
	t.Helper()

	payload, err := json.Marshal(OAuthState{
		Provider:  provider,
		UserID:    userID,
		ExpiresAt: expiresAt,
		Nonce:     "test-nonce",
	})
	if err != nil {
		t.Fatal(err)
	}

	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	signedValue := "v1." + encodedPayload
	mac := hmac.New(sha256.New, []byte("secret"))
	mac.Write([]byte(signedValue))

	return signedValue + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func TestValidateOAuthState(t *testing.T) {
	rawState := testOAuthState(t, "discord", 123, 200)

	userID, err := ValidateOAuthState(rawState, "discord", "secret", time.Unix(100, 0))
	if err != nil {
		t.Fatal(err)
	}

	if userID != 123 {
		t.Fatalf("expected user id 123, got %d", userID)
	}
}

func TestValidateOAuthStateRejectsWrongProvider(t *testing.T) {
	rawState := testOAuthState(t, "discord", 123, 200)

	_, err := ValidateOAuthState(rawState, "twitch", "secret", time.Unix(100, 0))
	if err != ErrInvalidOAuthState {
		t.Fatalf("expected ErrInvalidOAuthState, got %v", err)
	}
}

func TestValidateOAuthStateRejectsExpiredState(t *testing.T) {
	rawState := testOAuthState(t, "discord", 123, 200)

	_, err := ValidateOAuthState(rawState, "discord", "secret", time.Unix(201, 0))
	if err != ErrExpiredOAuthState {
		t.Fatalf("expected ErrExpiredOAuthState, got %v", err)
	}
}
