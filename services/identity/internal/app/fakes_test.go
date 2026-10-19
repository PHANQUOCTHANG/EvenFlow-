package app_test

import (
	"context"
	"time"

	"github.com/eventflow/eventflow/services/identity/internal/domain"
	"github.com/google/uuid"
)

// fakeIdentityRepo implements domain.IdentityRepository
type fakeIdentityRepo struct {
	identities map[string]*domain.Identity
	saveErr    error
	findErr    error
	updateErr  error
}

func newFakeRepo() *fakeIdentityRepo {
	return &fakeIdentityRepo{
		identities: make(map[string]*domain.Identity),
	}
}

func (r *fakeIdentityRepo) Save(ctx context.Context, identity *domain.Identity) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	if identity.Email != nil {
		r.identities[*identity.Email] = identity
	}
	if identity.PhoneHash != nil {
		r.identities[*identity.PhoneHash] = identity
	}
	if identity.Phone != nil {
		r.identities[*identity.Phone] = identity
	}
	return nil
}

func (r *fakeIdentityRepo) FindByEmailOrPhone(ctx context.Context, identifier string) (*domain.Identity, error) {
	if r.findErr != nil {
		return nil, r.findErr
	}
	id, ok := r.identities[identifier]
	if !ok {
		return nil, domain.ErrIdentityNotFound
	}
	return id, nil
}

func (r *fakeIdentityRepo) FindByID(ctx context.Context, id uuid.UUID) (*domain.Identity, error) {
	if r.findErr != nil {
		return nil, r.findErr
	}
	for _, ident := range r.identities {
		if ident.ID == id {
			return ident, nil
		}
	}
	return nil, domain.ErrIdentityNotFound
}

func (r *fakeIdentityRepo) UpdatePassword(ctx context.Context, id uuid.UUID, newHash string) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	for _, ident := range r.identities {
		if ident.ID == id {
			ident.PasswordHash = newHash
			return nil
		}
	}
	return nil
}

// fakePasswordHasher implements domain.PasswordHasher
type fakePasswordHasher struct {
	match    bool
	hashErr  error
	verErr   error
}

func (h *fakePasswordHasher) Hash(password string) (string, error) {
	if h.hashErr != nil {
		return "", h.hashErr
	}
	return "hashed:" + password, nil
}

func (h *fakePasswordHasher) Verify(password, encodedHash string) (bool, error) {
	if h.verErr != nil {
		return false, h.verErr
	}
	return h.match, nil
}

// fakeTokenGen implements domain.TokenGenerator
type fakeTokenGen struct {
	token string
	err   error
}

func (t *fakeTokenGen) GenerateTokens(ctx context.Context, identity *domain.Identity) (*domain.TokenPair, error) {
	if t.err != nil {
		return nil, t.err
	}
	return &domain.TokenPair{AccessToken: t.token, RefreshToken: "refresh-" + t.token}, nil
}

// fakeRedisClient implements redis.Client
type fakeRedisClient struct {
	otpStore    map[string]string
	rateLimits  map[string]int64
	setOTPErr   error
	getOTPErr   error
	delOTPErr   error
	rateErr     error
}

func newFakeRedis() *fakeRedisClient {
	return &fakeRedisClient{
		otpStore:   make(map[string]string),
		rateLimits: make(map[string]int64),
	}
}

func (c *fakeRedisClient) SetOTP(ctx context.Context, identifier string, otp string, ttl time.Duration) error {
	if c.setOTPErr != nil {
		return c.setOTPErr
	}
	c.otpStore[identifier] = otp
	return nil
}

func (c *fakeRedisClient) GetOTP(ctx context.Context, identifier string) (string, error) {
	if c.getOTPErr != nil {
		return "", c.getOTPErr
	}
	otp, ok := c.otpStore[identifier]
	if !ok {
		return "", domain.ErrIdentityNotFound
	}
	return otp, nil
}

func (c *fakeRedisClient) DeleteOTP(ctx context.Context, identifier string) error {
	if c.delOTPErr != nil {
		return c.delOTPErr
	}
	delete(c.otpStore, identifier)
	return nil
}

func (c *fakeRedisClient) IncrementRateLimit(ctx context.Context, key string, window time.Duration) (int64, error) {
	if c.rateErr != nil {
		return 0, c.rateErr
	}
	c.rateLimits[key]++
	return c.rateLimits[key], nil
}

// fakeNotifier implements domain.OTPNotifier
type fakeNotifier struct {
	sentTo  string
	sentOTP string
	err     error
}

func (n *fakeNotifier) SendOTP(ctx context.Context, to string, otp string) error {
	if n.err != nil {
		return n.err
	}
	n.sentTo = to
	n.sentOTP = otp
	return nil
}
