package domain

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type TokenGenerator interface {
	GenerateToken(identity *Identity) (string, error)
}

type jwtTokenGenerator struct {
	secretKey string
}

func NewJWTTokenGenerator(secretKey string) TokenGenerator {
	return &jwtTokenGenerator{
		secretKey: secretKey,
	}
}

func (g *jwtTokenGenerator) GenerateToken(identity *Identity) (string, error) {
	// Custom Claims
	claims := jwt.MapClaims{
		"sub":  identity.ID.String(),
		"role": string(identity.Role),
		"iat":  time.Now().Unix(),
		"exp":  time.Now().Add(24 * time.Hour).Unix(), // Token expires in 24 hours
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Sign the token
	signedToken, err := token.SignedString([]byte(g.secretKey))
	if err != nil {
		return "", err
	}

	return signedToken, nil
}
