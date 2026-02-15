package i18n

import "testing"

func TestParseAcceptLanguage(t *testing.T) {
	tests := []struct {
		header string
		want   Locale
	}{
		{"", LocaleKo},
		{"ko", LocaleKo},
		{"ko-KR,ko;q=0.9,en-US;q=0.8", LocaleKo},
		{"en-US,en;q=0.9", LocaleEn},
		{"ja,en-US;q=0.7", LocaleJa},
		{"fr-FR,fr;q=0.9", LocaleKo}, // unsupported → fallback
		{"en", LocaleEn},
		{"ja-JP", LocaleJa},
	}

	for _, tt := range tests {
		got := ParseAcceptLanguage(tt.header)
		if got != tt.want {
			t.Errorf("ParseAcceptLanguage(%q) = %q, want %q", tt.header, got, tt.want)
		}
	}
}

func TestBundleTranslation(t *testing.T) {
	b := NewBundle(LocaleKo)
	for locale, msgs := range DefaultMessages() {
		b.LoadMessages(locale, msgs)
	}

	// Korean
	if got := b.T(LocaleKo, "auth.login_success"); got != "로그인 되었습니다" {
		t.Errorf("ko login_success = %q", got)
	}

	// English
	if got := b.T(LocaleEn, "auth.login_success"); got != "Successfully logged in" {
		t.Errorf("en login_success = %q", got)
	}

	// Japanese
	if got := b.T(LocaleJa, "auth.login_success"); got != "ログインしました" {
		t.Errorf("ja login_success = %q", got)
	}

	// Fallback to Korean for unknown key
	if got := b.T(LocaleEn, "unknown.key"); got != "unknown.key" {
		t.Errorf("unknown key = %q, want key itself", got)
	}

	// Format args
	if got := b.T(LocaleKo, "rate_limit.exceeded", 30); got != "요청 제한을 초과했습니다. 30초 후 다시 시도해주세요" {
		t.Errorf("rate_limit with args = %q", got)
	}
}
