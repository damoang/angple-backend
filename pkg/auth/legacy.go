package auth

import (
	"crypto/sha1" //nolint:gosec // G505: Gnuboard 레거시 호환을 위해 SHA1 필요
	"fmt"
	"strings"
)

// VerifyGnuboardPassword verifies password against Gnuboard hash
// Supports multiple hashing methods used in different Gnuboard versions
func VerifyGnuboardPassword(plainPassword, hashedPassword string) bool {
	// 1. MySQL PASSWORD() function (old versions with * prefix)
	if strings.HasPrefix(hashedPassword, "*") && len(hashedPassword) == 41 {
		return verifyMySQLPassword(plainPassword, hashedPassword)
	}

	// 2. SHA1 hash (40 hex characters)
	if len(hashedPassword) == 40 && !strings.HasPrefix(hashedPassword, "*") {
		return verifySHA1(plainPassword, hashedPassword)
	}

	// 3. Plain text (very old versions - not recommended)
	if plainPassword == hashedPassword {
		return true
	}

	// 4. Empty password check
	if hashedPassword == "" && plainPassword == "" {
		return true
	}

	return false
}

// verifyMySQLPassword verifies against MySQL PASSWORD() hash
// Format: *<SHA1(SHA1(password))>
//
//nolint:gosec // G401: Gnuboard 레거시 호환을 위해 SHA1 필요
func verifyMySQLPassword(plain, hashed string) bool {
	// First SHA1
	sha1Once := sha1.Sum([]byte(plain))

	// Second SHA1
	sha1Twice := sha1.Sum(sha1Once[:])

	// Format as *HEX
	generated := "*" + strings.ToUpper(fmt.Sprintf("%x", sha1Twice))

	return generated == hashed
}

// verifySHA1 verifies against simple SHA1 hash
//
//nolint:gosec // G401: Gnuboard 레거시 호환을 위해 SHA1 필요
func verifySHA1(plain, hashed string) bool {
	sha := sha1.Sum([]byte(plain))
	generated := fmt.Sprintf("%x", sha)

	return strings.EqualFold(generated, hashed)
}
