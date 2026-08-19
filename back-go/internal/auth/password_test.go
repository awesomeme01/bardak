package auth

import (
	"os"
	"testing"
)

func TestPasswordRoundTrip(t *testing.T) {
	hash, err := HashPassword("very-secret-password")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(hash, "very-secret-password") {
		t.Error("свой же пароль не подошёл")
	}
	if CheckPassword(hash, "другой-пароль") {
		t.Error("чужой пароль подошёл")
	}
	if CheckPassword("не-хеш-вовсе", "very-secret-password") {
		t.Error("мусор вместо хеша не должен подходить")
	}
}

// ⭐ Условие отката: пароль, записанный ЖИВОЙ Java, обязан проверяться Go-версией.
// Иначе после переключения никто не войдёт.
func TestJavaHashIsAccepted(t *testing.T) {
	hash := os.Getenv("BARDAK_JAVA_HASH")
	password := os.Getenv("BARDAK_JAVA_PASSWORD")
	if hash == "" || password == "" {
		t.Skip("BARDAK_JAVA_HASH/PASSWORD не заданы — проверка совместимости пропущена")
	}
	if !CheckPassword(hash, password) {
		t.Fatal("хеш, записанный Java, не принят Go — вход после переключения сломается")
	}
	t.Log("хеш Java принят Go-версией")
}
