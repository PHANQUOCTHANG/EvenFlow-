package config

import (
	"fmt"
	"os"
)

type Config struct {
	HTTPAddr    string
	DatabaseURL string
}

func Load() *Config {
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8083" // Default for identity service
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		// Fail fast if database URL is missing
		panic("DATABASE_URL environment variable is strictly required")
	}

	return &Config{
		HTTPAddr:    addr,
		DatabaseURL: dbURL,
	}
}
