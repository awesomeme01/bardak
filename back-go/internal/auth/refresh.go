package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// RefreshTokenBytes — 32 случайных байта, как в Java.
const RefreshTokenBytes = 32

// NewRefreshToken выпускает refresh-токен.
//
// ⭐ Это НЕ JWT: подпись не нужна, потому что токен всё равно проверяется по базе —
// его надо уметь отзывать. 32 случайных байта в url-safe Base64 без паддинга дают
// строку в 43 символа.
func NewRefreshToken() (string, error) {
	buf := make([]byte, RefreshTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("случайность для refresh-токена: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// HashRefreshToken — то, что кладётся в колонку token_hash.
//
// ⚠️ ЛОВУШКА, которую легко не заметить: сам токен кодируется url-safe БЕЗ паддинга,
// а его хеш — СТАНДАРТНЫМ Base64 С паддингом. Два разных кодирования в одном месте.
// Перепутать — и токены, выданные Java, перестанут находиться в базе, то есть после
// переключения бэкендов всех выкинет из игры.
func HashRefreshToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.StdEncoding.EncodeToString(sum[:])
}
