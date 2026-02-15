package i18n

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Locale represents a supported language
type Locale string

const (
	LocaleKo Locale = "ko"
	LocaleEn Locale = "en"
	LocaleJa Locale = "ja"
)

var defaultLocale = LocaleKo

// Bundle holds all translations for all locales
type Bundle struct {
	mu           sync.RWMutex
	translations map[Locale]map[string]string
	fallback     Locale
}

// NewBundle creates a new i18n bundle with the given fallback locale
func NewBundle(fallback Locale) *Bundle {
	return &Bundle{
		translations: make(map[Locale]map[string]string),
		fallback:     fallback,
	}
}

// LoadDir loads all JSON translation files from a directory.
// Files should be named like: ko.json, en.json, ja.json
func (b *Bundle) LoadDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read i18n dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		locale := Locale(strings.TrimSuffix(entry.Name(), ".json"))
		path := filepath.Join(dir, entry.Name())

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}

		var msgs map[string]string
		if err := json.Unmarshal(data, &msgs); err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}

		b.mu.Lock()
		b.translations[locale] = msgs
		b.mu.Unlock()
	}

	return nil
}

// LoadMessages loads translations for a specific locale from a map
func (b *Bundle) LoadMessages(locale Locale, messages map[string]string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if existing, ok := b.translations[locale]; ok {
		for k, v := range messages {
			existing[k] = v
		}
	} else {
		b.translations[locale] = messages
	}
}

// T translates a message key for the given locale.
// Falls back to the bundle's fallback locale, then returns the key itself.
func (b *Bundle) T(locale Locale, key string, args ...interface{}) string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	// Try requested locale
	if msgs, ok := b.translations[locale]; ok {
		if msg, ok := msgs[key]; ok {
			if len(args) > 0 {
				return fmt.Sprintf(msg, args...)
			}
			return msg
		}
	}

	// Try fallback locale
	if locale != b.fallback {
		if msgs, ok := b.translations[b.fallback]; ok {
			if msg, ok := msgs[key]; ok {
				if len(args) > 0 {
					return fmt.Sprintf(msg, args...)
				}
				return msg
			}
		}
	}

	// Return key as-is
	return key
}

// ParseAcceptLanguage parses the Accept-Language header and returns the best matching locale
func ParseAcceptLanguage(header string) Locale {
	if header == "" {
		return defaultLocale
	}

	// Simple parsing: take the first language tag
	parts := strings.Split(header, ",")
	for _, part := range parts {
		lang := strings.TrimSpace(strings.SplitN(part, ";", 2)[0])
		lang = strings.ToLower(lang)

		// Exact match
		switch {
		case strings.HasPrefix(lang, "ko"):
			return LocaleKo
		case strings.HasPrefix(lang, "en"):
			return LocaleEn
		case strings.HasPrefix(lang, "ja"):
			return LocaleJa
		}
	}

	return defaultLocale
}

// SupportedLocales returns all locales that have translations loaded
func (b *Bundle) SupportedLocales() []Locale {
	b.mu.RLock()
	defer b.mu.RUnlock()

	locales := make([]Locale, 0, len(b.translations))
	for l := range b.translations {
		locales = append(locales, l)
	}
	return locales
}
