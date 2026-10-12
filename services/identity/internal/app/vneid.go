package app

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/eventflow/eventflow/services/identity/internal/domain"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type VNeIDCallbackInput struct {
	SessionID  string `json:"session_id"`
	CCCDNumber string `json:"cccd_number"`
	FullName   string `json:"full_name"`
	Dob        string `json:"dob"`
}

type VNeIDUseCase interface {
	HandleCallback(ctx context.Context, input VNeIDCallbackInput) error
}

type vneidUseCase struct {
	repo        domain.IdentityRepository
	redisClient *redis.Client
	tokenGen    domain.TokenGenerator
}

func NewVNeIDUseCase(repo domain.IdentityRepository, redisClient *redis.Client, tokenGen domain.TokenGenerator) VNeIDUseCase {
	return &vneidUseCase{
		repo:        repo,
		redisClient: redisClient,
		tokenGen:    tokenGen,
	}
}

func (uc *vneidUseCase) HandleCallback(ctx context.Context, input VNeIDCallbackInput) error {
	if input.SessionID == "" || input.CCCDNumber == "" {
		return errors.New("invalid vneid callback payload")
	}

	// 1. (Mock) Tìm hoặc tạo mới user dựa trên CCCD
	// Trong thực tế, VNeID trả về thông tin đáng tin cậy. Nếu chưa có, ta tạo account.
	// Để đơn giản, ta tìm qua Repo. Ghi chú: identity_repo hiện chưa có cột CCCD.
	// Nên ta sẽ mượn Email để lưu (vd: 0123456789@vneid.mock) để tránh rườm rà thay đổi schema DB hiện tại.
	
	mockEmail := input.CCCDNumber + "@vneid.mock"
	identity, err := uc.repo.FindByEmailOrPhone(ctx, mockEmail)
	
	if err != nil {
		if errors.Is(err, domain.ErrIdentityNotFound) {
			// Tạo mới
			identity = &domain.Identity{
				ID:           uuid.New(),
				Email:        &mockEmail,
				Role:         domain.RoleBuyer,
				PasswordHash: "$argon2id$v=19$m=65536,t=1,p=2$mock$mock", // Fake password
			}
			if saveErr := uc.repo.Save(ctx, identity); saveErr != nil {
				return saveErr
			}
		} else {
			return err
		}
	}

	// 2. Tạo JWT Token cho user này
	token, err := uc.tokenGen.GenerateToken(identity)
	if err != nil {
		return err
	}

	// 3. Pub message qua Redis Pub/Sub để báo Frontend biết
	payload := map[string]interface{}{
		"status": "success",
		"token":  token,
		"user": map[string]string{
			"full_name": input.FullName,
			"cccd":      input.CCCDNumber,
		},
	}
	
	payloadBytes, _ := json.Marshal(payload)
	
	// Pub/Sub channel theo SessionID
	channelName := "vneid_session:" + input.SessionID
	return uc.redisClient.Publish(ctx, channelName, string(payloadBytes)).Err()
}
