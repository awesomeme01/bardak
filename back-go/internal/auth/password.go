package auth

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// HashPassword — BCrypt со стоимостью 10.
//
// ⚠️ Стоимость 10 — не произвольный выбор, а совпадение с Java: `BCryptPasswordEncoder`
// без аргументов использует именно её. Возьми другую — Java всё равно проверит пароль
// (стоимость записана в самом хеше), но новые хеши станут отличаться по цене проверки,
// и нагрузка на вход поедет незаметно.
const bcryptCost = 10

// HashPassword хеширует пароль.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("хеширование пароля: %w", err)
	}
	return string(hash), nil
}

// CheckPassword — подходит ли пароль к хешу.
//
// ⭐ Формат BCrypt один на все реализации, поэтому хеш, записанный Java, проверяется здесь
// без всякой подготовки. Это и есть условие отката: игрок, заведённый на Java, входит
// на Go и обратно.
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
