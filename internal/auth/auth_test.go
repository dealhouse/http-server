package auth

import (
	"testing"
	"time"

	"net/http"

	"github.com/google/uuid"
)

func TestMakeAndValidateJWT(t *testing.T) {
	userID := uuid.New()
	tokenSecret := "secret"
	expiresIn := time.Minute
	token, err := MakeJWT(userID, tokenSecret, expiresIn)
	if err != nil {
		t.Errorf("Error making JWT: %s", err)
	}
	returnedUserID, err := ValidateJWT(token, tokenSecret)
	if err != nil {
		t.Errorf("Error validating JWT: %s", err)
	}
	if returnedUserID != userID {
		t.Errorf("Expected user ID %s, got %s", userID, returnedUserID)
	}
}

func TestSecretValidation(t *testing.T) {
	userID := uuid.New()
	tokenSecret := "secret"
	expiresIn := time.Minute
	token, err := MakeJWT(userID, tokenSecret, expiresIn)
	if err != nil {
		t.Errorf("Error making JWT: %s", err)
	}
	_, err = ValidateJWT(token, "wrong secret")
	if err == nil {
		t.Errorf("Expected error validating JWT with wrong secret, got nil")
	}
}

func TestExpiredValidation(t *testing.T) {
	userID := uuid.New()
	tokenSecret := "secret"
	expiresIn := -time.Minute
	token, err := MakeJWT(userID, tokenSecret, expiresIn)
	if err != nil {
		t.Errorf("Error making JWT: %s", err)
	}
	_, err = ValidateJWT(token, tokenSecret)
	if err == nil {
		t.Errorf("Expected error validating JWT with expired token, got nil")
	}
}

func TestGetBearerToken(t *testing.T) {
	headers := http.Header{}
	headers.Set("Authorization", "Bearer token")
	token, err := GetBearerToken(headers)
	if err != nil {
		t.Fatalf("Error getting bearer token: %s", err)
	}
	if token != "token" {
		t.Errorf("Expected token 'token', got '%s'", token)
	}
}

func TestGetBearerTokenError(t *testing.T) {
	headers := http.Header{}
	token, err := GetBearerToken(headers)
	if err == nil {
		t.Errorf("Expected error getting bearer token, got nil")
	}
	if token != "" {
		t.Errorf("Expected token '', got '%s'", token)
	}
}

func TestWhiteSpaceBearerToken(t *testing.T) {
	headers := http.Header{}
	headers.Set("Authorization", "Bearer   token")
	token, err := GetBearerToken(headers)
	if err != nil {
		t.Fatalf("Error getting bearer token: %s", err)
	}
	if token != "token" {
		t.Errorf("Expected token 'token', got '%s'", token)
	}
}
