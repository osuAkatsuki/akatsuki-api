package common

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type OAuthState struct {
	Provider  string `json:"provider"`
	UserID    int    `json:"user_id"`
	ExpiresAt int64  `json:"expires_at"`
	Nonce     string `json:"nonce"`
}

var (
	ErrInvalidOAuthState = errors.New("invalid oauth state")
	ErrExpiredOAuthState = errors.New("expired oauth state")
)

func ValidateOAuthState(
	rawState string,
	expectedProvider string,
	secret string,
	now time.Time,
) (int, error) {
	parts := strings.Split(rawState, ".")
	if len(parts) != 3 || parts[0] != "v1" {
		return 0, ErrInvalidOAuthState
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return 0, ErrInvalidOAuthState
	}

	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return 0, ErrInvalidOAuthState
	}

	if !hmac.Equal(signature, signOAuthState(parts[0]+"."+parts[1], secret)) {
		return 0, ErrInvalidOAuthState
	}

	var state OAuthState
	if err := json.Unmarshal(payloadBytes, &state); err != nil {
		return 0, ErrInvalidOAuthState
	}

	if state.Provider != expectedProvider || state.UserID <= 0 || state.Nonce == "" {
		return 0, ErrInvalidOAuthState
	}

	if now.Unix() > state.ExpiresAt {
		return 0, ErrExpiredOAuthState
	}

	return state.UserID, nil
}

func signOAuthState(value string, secret string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(value))
	return mac.Sum(nil)
}

func OAuthStateValidationMessage(err error) string {
	if errors.Is(err, ErrExpiredOAuthState) {
		return "OAuth state has expired. Please try linking your account again."
	}

	return "Invalid OAuth state. Please try linking your account again."
}
