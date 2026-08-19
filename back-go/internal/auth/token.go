// Package auth — токены и пароли.
//
// ⚠️ Формат access-токена обязан совпадать с Java ДОСЛОВНО: в окне отката оба бэкенда
// работают рядом, и токен, выданный одним, должен приниматься другим. Разъехавшийся
// claim выкинет игрока из игры ровно в момент переключения.
package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Issuer — значение claim `iss`. Совпадает с Java.
const Issuer = "bardak"

// Claims — ровно те поля, что кладёт Java, и никаких других.
//
// ⚠️ Ролей нет; `jti`, `aud`, `nbf` не выставляются. Лишний claim — это уже другой
// токен, и различие всплывёт не здесь, а при переключении бэкендов.
type Claims struct {
	UserID      string
	Username    string
	DisplayName string
	IssuedAt    time.Time
	ExpiresAt   time.Time
}

// TokenService выпускает и проверяет access-токены.
type TokenService struct {
	secret    []byte
	accessTTL time.Duration
	now       func() time.Time
}

// NewTokenService собирает сервис.
//
// ⭐ Часы передаются параметром: «сейчас» в тестах обязано быть управляемым, иначе
// проверку истечения токена не написать без ожидания в пятнадцать минут.
func NewTokenService(secret []byte, accessTTL time.Duration, now func() time.Time) TokenService {
	if now == nil {
		now = time.Now
	}
	return TokenService{secret: secret, accessTTL: accessTTL, now: now}
}

// Issue выпускает access-токен.
func (s TokenService) Issue(userID, username, displayName string) (string, time.Duration, error) {
	issued := s.now()
	claims := jwt.MapClaims{
		"iss":         Issuer,
		"iat":         issued.Unix(),
		"exp":         issued.Add(s.accessTTL).Unix(),
		"sub":         userID,
		"username":    username,
		"displayName": displayName,
	}
	// ⚠️ Алгоритм задаётся явно. В Java это тоже сделано руками: библиотека по умолчанию
	// искала бы ключ под RS256, и подпись молча уехала бы не туда.
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
	if err != nil {
		return "", 0, fmt.Errorf("подпись токена: %w", err)
	}
	return signed, s.accessTTL, nil
}

// Parse проверяет токен и достаёт claims.
//
// ⚠️ Принимается ТОЛЬКО HS256. Без этой проверки токен с алгоритмом `none` или с чужим
// алгоритмом подписи прошёл бы как валидный — классическая дыра библиотек JWT.
func (s TokenService) Parse(token string) (Claims, error) {
	parsed, err := jwt.Parse(token, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("алгоритм подписи %v не принимается", t.Header["alg"])
		}
		return s.secret, nil
	}, jwt.WithValidMethods([]string{"HS256"}),
		jwt.WithIssuer(Issuer),
		jwt.WithTimeFunc(s.now))
	if err != nil {
		return Claims{}, fmt.Errorf("токен не принят: %w", err)
	}

	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return Claims{}, fmt.Errorf("неожиданная форма claims")
	}

	out := Claims{
		UserID:      stringClaim(claims, "sub"),
		Username:    stringClaim(claims, "username"),
		DisplayName: stringClaim(claims, "displayName"),
		IssuedAt:    timeClaim(claims, "iat"),
		ExpiresAt:   timeClaim(claims, "exp"),
	}
	if out.UserID == "" {
		return Claims{}, fmt.Errorf("в токене нет владельца")
	}
	return out, nil
}

func stringClaim(claims jwt.MapClaims, name string) string {
	if value, ok := claims[name].(string); ok {
		return value
	}
	return ""
}

func timeClaim(claims jwt.MapClaims, name string) time.Time {
	switch value := claims[name].(type) {
	case float64:
		return time.Unix(int64(value), 0).UTC()
	case int64:
		return time.Unix(value, 0).UTC()
	}
	return time.Time{}
}
