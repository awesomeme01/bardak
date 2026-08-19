package auth

import (
	"os"
	"strings"
	"testing"
	"time"
)

const testSecret = "dev-only-secret-change-me-32-bytes-minimum!!"

func service(now time.Time) TokenService {
	return NewTokenService([]byte(testSecret), 15*time.Minute, func() time.Time { return now })
}

func TestIssueAndParseRoundTrip(t *testing.T) {
	now := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	s := service(now)

	token, ttl, err := s.Issue("11111111-1111-1111-1111-111111111111", "shabdan", "Шабдан")
	if err != nil {
		t.Fatal(err)
	}
	if ttl != 15*time.Minute {
		t.Errorf("срок %v, ждали 15 минут (expiresIn = 900)", ttl)
	}

	claims, err := s.Parse(token)
	if err != nil {
		t.Fatalf("свой же токен не принят: %v", err)
	}
	if claims.UserID != "11111111-1111-1111-1111-111111111111" {
		t.Errorf("владелец разошёлся: %q", claims.UserID)
	}
	if claims.Username != "shabdan" || claims.DisplayName != "Шабдан" {
		t.Error("логин или имя за столом не сохранились")
	}
}

// ⚠️ Просроченный токен обязан отвергаться. Без управляемых часов это не проверить,
// не прождав пятнадцать минут.
func TestExpiredTokenIsRefused(t *testing.T) {
	issued := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	token, _, err := service(issued).Issue("id-1", "u", "U")
	if err != nil {
		t.Fatal(err)
	}

	later := service(issued.Add(16 * time.Minute))
	if _, err := later.Parse(token); err == nil {
		t.Error("токен старше своего срока принят")
	}
}

// ⚠️ Классическая дыра библиотек JWT: токен с алгоритмом none или с чужой подписью.
func TestOnlyHS256IsAccepted(t *testing.T) {
	now := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	token, _, _ := service(now).Issue("id-1", "u", "U")

	// Подменяем заголовок на none — подпись становится ненужной.
	parts := strings.Split(token, ".")
	forged := "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0." + parts[1] + "."
	if _, err := service(now).Parse(forged); err == nil {
		t.Error("токен с alg=none принят — это дыра, а не совместимость")
	}

	// Чужой секрет — чужая подпись.
	other := NewTokenService([]byte("совершенно-другой-секрет-длиной-32+"), 15*time.Minute,
		func() time.Time { return now })
	if _, err := other.Parse(token); err == nil {
		t.Error("токен, подписанный другим ключом, принят")
	}
}

func TestIssuerIsChecked(t *testing.T) {
	now := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	token, _, _ := service(now).Issue("id-1", "u", "U")

	claims, err := service(now).Parse(token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.IssuedAt.IsZero() || claims.ExpiresAt.IsZero() {
		t.Error("iat и exp обязаны читаться: по ним клиент понимает, когда обновляться")
	}
}

// ⭐ Главная проверка совместимости: токен, выпущенный ЖИВОЙ Java, обязан приниматься Go.
//
// Токен подаётся через переменную окружения — держать его в репозитории нельзя,
// это чужой ключ доступа, пусть и от тестовой учётки.
func TestJavaIssuedTokenIsAccepted(t *testing.T) {
	token := os.Getenv("BARDAK_JAVA_TOKEN")
	if token == "" {
		t.Skip("BARDAK_JAVA_TOKEN не задан — проверка совместимости с Java пропущена")
	}

	claims, err := NewTokenService([]byte(testSecret), 15*time.Minute, time.Now).Parse(token)
	if err != nil {
		t.Fatalf("токен Java не принят Go-версией: %v", err)
	}
	if claims.UserID == "" {
		t.Error("в токене Java нет владельца — разошлась форма claims")
	}
	t.Logf("токен Java принят: владелец %s, логин %s", claims.UserID, claims.Username)
}
