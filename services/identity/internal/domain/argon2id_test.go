package domain_test

import (
	"strings"
	"testing"

	"github.com/eventflow/eventflow/services/identity/internal/domain"
)

func TestArgon2idHasher_HashAndVerify(t *testing.T) {
	hasher := domain.NewArgon2idHasher()

	password := "SecurePassword123!"
	encodedHash, err := hasher.Hash(password)
	if err != nil {
		t.Fatalf("unexpected hash error: %v", err)
	}

	if !strings.HasPrefix(encodedHash, "$argon2id$") {
		t.Errorf("expected hash to start with $argon2id$, got %s", encodedHash)
	}

	// Verify correct password
	match, err := hasher.Verify(password, encodedHash)
	if err != nil {
		t.Fatalf("unexpected verify error: %v", err)
	}
	if !match {
		t.Errorf("expected password to match hash")
	}

	// Verify wrong password
	match, err = hasher.Verify("WrongPassword456!", encodedHash)
	if err != nil {
		t.Fatalf("unexpected verify error: %v", err)
	}
	if match {
		t.Errorf("expected wrong password to not match hash")
	}
}

func TestArgon2idHasher_VerifyInvalidFormats(t *testing.T) {
	hasher := domain.NewArgon2idHasher()

	// Invalid number of parts
	_, err := hasher.Verify("test", "invalid_hash_string")
	if err == nil {
		t.Errorf("expected error for malformed hash")
	}

	// Wrong algorithm name
	_, err = hasher.Verify("test", "$argon2i$v=19$m=65536,t=1,p=4$salt$hash")
	if err == nil {
		t.Errorf("expected error for wrong algorithm")
	}

	// Bad version format
	_, err = hasher.Verify("test", "$argon2id$v=invalid$m=65536,t=1,p=4$salt$hash")
	if err == nil {
		t.Errorf("expected error for bad version format")
	}

	// Incompatible version
	_, err = hasher.Verify("test", "$argon2id$v=999$m=65536,t=1,p=4$salt$hash")
	if err == nil {
		t.Errorf("expected error for incompatible version")
	}

	// Bad memory/time/thread params
	_, err = hasher.Verify("test", "$argon2id$v=19$bad_params$salt$hash")
	if err == nil {
		t.Errorf("expected error for bad params")
	}

	// Bad base64 salt
	_, err = hasher.Verify("test", "$argon2id$v=19$m=65536,t=1,p=4$!invalid_b64!$dGVzdA")
	if err == nil {
		t.Errorf("expected error for bad salt base64")
	}

	// Bad base64 hash
	_, err = hasher.Verify("test", "$argon2id$v=19$m=65536,t=1,p=4$dGVzdA$!invalid_b64!")
	if err == nil {
		t.Errorf("expected error for bad hash base64")
	}
}
