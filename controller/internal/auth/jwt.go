package auth

import (
	"errors"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrExpiredToken = errors.New("expired token")
)

type Claims struct {
	ItemID    uint `json:"item_id"`
	SessionID uint `json:"session_id"`
	IsAgent   bool `json:"is_agent"`
	jwt.RegisteredClaims
}

func signingKey() []byte {
	k := os.Getenv("KEY")
	return []byte(k)
}

func IssueJWT(itemID, sessionID uint, isAgent bool, ttl time.Duration) (string, error) {
	now := time.Now()
	c := &Claims{
		ItemID:    itemID,
		SessionID: sessionID,
		IsAgent:   isAgent,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	return tok.SignedString(signingKey())
}

func ParseJWT(tokenString string) (*Claims, error) {
	tok, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return signingKey(), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := tok.Claims.(*Claims)
	if !ok || !tok.Valid {
		return nil, ErrInvalidToken
	}
	if claims.ExpiresAt != nil && time.Now().After(claims.ExpiresAt.Time) {
		return nil, ErrExpiredToken
	}
	return claims, nil
}
