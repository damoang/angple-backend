package config

import (
	"os"

	"github.com/joho/godotenv"
)

// LoadDotEnv loads .env files with priority: .env.local > .env
// godotenv.Load does NOT overwrite already-set env vars,
// so OS env vars always win, .env.local wins over .env.
// Returns list of files actually loaded.
func LoadDotEnv() []string {
	candidates := []string{".env.local", ".env"}
	var loaded []string
	for _, f := range candidates {
		if _, err := os.Stat(f); err == nil {
			loaded = append(loaded, f)
		}
	}
	if len(loaded) > 0 {
		_ = godotenv.Load(loaded...)
	}
	return loaded
}
