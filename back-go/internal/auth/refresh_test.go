package auth

import (
	"encoding/base64"
	"os"
	"strings"
	"testing"
)

func TestRefreshTokenShape(t *testing.T) {
	token, err := NewRefreshToken()
	if err != nil {
		t.Fatal(err)
	}

	// 32 байта в Base64 без паддинга — ровно 43 символа, как в Java.
	if len(token) != 43 {
		t.Errorf("длина токена %d, ждали 43", len(token))
	}
	if strings.ContainsAny(token, "+/=") {
		t.Errorf("токен обязан быть url-safe и без паддинга, получили %q", token)
	}

	second, _ := NewRefreshToken()
	if token == second {
		t.Error("два подряд выпущенных токена совпали — случайность сломана")
	}
}

// ⚠️ Хеш — СТАНДАРТНЫЙ Base64 с паддингом, в отличие от самого токена. Это не описка
// в Java, а факт, который надо повторить: иначе выданные ею токены не найдутся в базе.
func TestRefreshHashUsesStandardBase64(t *testing.T) {
	hash := HashRefreshToken("любая-строка")

	if len(hash) != 44 || !strings.HasSuffix(hash, "=") {
		t.Errorf("хеш %q не похож на стандартный Base64 от SHA-256 (44 символа с паддингом)", hash)
	}
	raw, err := base64.StdEncoding.DecodeString(hash)
	if err != nil {
		t.Fatalf("хеш не разбирается стандартным Base64: %v", err)
	}
	if len(raw) != 32 {
		t.Errorf("под хешем %d байт, а SHA-256 даёт 32", len(raw))
	}

	if HashRefreshToken("а") == HashRefreshToken("б") {
		t.Error("разные токены дали один хеш")
	}
}

// ⭐ Условие отката: хеш, посчитанный Go, обязан совпасть с тем, что Java уже положила
// в базу для того же токена.
func TestHashMatchesJava(t *testing.T) {
	token := os.Getenv("BARDAK_JAVA_REFRESH")
	stored := os.Getenv("BARDAK_JAVA_REFRESH_HASH")
	if token == "" || stored == "" {
		t.Skip("BARDAK_JAVA_REFRESH/HASH не заданы — проверка совместимости пропущена")
	}
	if got := HashRefreshToken(token); got != stored {
		t.Fatalf("хеш разошёлся с Java:\n  Go:   %s\n  Java: %s", got, stored)
	}
	t.Log("хеш совпал с тем, что записала Java")
}
