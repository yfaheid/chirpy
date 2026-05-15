package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/alexedwards/argon2id"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func HashPassword(password string) (string, error) {
	return argon2id.CreateHash(password, argon2id.DefaultParams)
}

func CheckPasswordHash(password, hash string) (bool, error) {
	return argon2id.ComparePasswordAndHash(password, hash)
}

func MakeJWT(userID uuid.UUID, tokenSecret string, expiresIn time.Duration) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{Issuer: "chirpy-access", IssuedAt: jwt.NewNumericDate(time.Now().UTC()), ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(expiresIn)), Subject: userID.String()})
	return token.SignedString([]byte(tokenSecret))
}

func ValidateJWT(tokenString, tokenSecret string) (uuid.UUID, error) {
	claims := &jwt.RegisteredClaims{}

	token, err := jwt.ParseWithClaims(
		tokenString,
		claims,
		func(token *jwt.Token) (interface{}, error) {
			return []byte(tokenSecret), nil
		},
	)
	if err != nil {
		return uuid.UUID{}, err
	}
	if !token.Valid {
		return uuid.UUID{}, errors.New("invalid token")
	}
	idString, err := claims.GetSubject()
	if err != nil {
		return uuid.UUID{}, err
	}
	return uuid.Parse(idString)
}

func GetBearerToken(headers http.Header) (string, error) {
	authorization := headers.Get("Authorization")
	if authorization == "" {
		return "", fmt.Errorf("no authorization header")
	}
	token := strings.TrimPrefix(authorization, "Bearer ")
	if authorization == token {
		return "", fmt.Errorf("invalid authorization header")
	}
	return strings.TrimSpace(token), nil
}

func MakeRefreshToken() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

func GetAPIKey(headers http.Header) (string, error) {
	authorization := headers.Get("Authorization")
	if authorization == "" {
		return "", fmt.Errorf("no authorization header")
	}
	key := strings.TrimPrefix(authorization, "ApiKey ")
	if authorization == key {
		return "", fmt.Errorf("invalid authorization header")
	}
	return strings.TrimSpace(key), nil
}
