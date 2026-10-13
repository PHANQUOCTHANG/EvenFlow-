package domain_test

import (
	"testing"
	"time"

	"github.com/eventflow/eventflow/services/identity/internal/domain"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func TestJWTTokenGenerator_GenerateToken(t *testing.T) {
	secretKey := "test-secret-key-12345"
	generator := domain.NewJWTTokenGenerator(secretKey)

	uid := uuid.New()
	userEmail := "test@example.com"
	identity := &domain.Identity{
		ID:        uid,
		Email:     &userEmail,
		Role:      domain.RoleBuyer,
		CreatedAt: time.Now(),
	}

	tokenStr, err := generator.GenerateToken(identity)
	if err != nil {
		t.Fatalf("unexpected error generating token: %v", err)
	}
	if tokenStr == "" {
		t.Fatal("expected non-empty token string")
	}

	// Parse and validate claims
	parsedToken, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		return []byte(secretKey), nil
	})
	if err != nil {
		t.Fatalf("failed to parse generated token: %v", err)
	}

	claims, ok := parsedToken.Claims.(jwt.MapClaims)
	if !ok || !parsedToken.Valid {
		t.Fatal("invalid token claims")
	}

	if claims["sub"] != uid.String() {
		t.Errorf("expected sub %s, got %v", uid.String(), claims["sub"])
	}
	if claims["role"] != string(domain.RoleBuyer) {
		t.Errorf("expected role %s, got %v", domain.RoleBuyer, claims["role"])
	}
}
