// Package config собирает настройки из переменных окружения.
//
// ⭐ Имена переменных те же, что у Java-бэкенда (BARDAK_*). Это не педантизм: во время
// миграции оба бэкенда поднимаются рядом на одной машине и читают одну базу, и разъехавшиеся
// имена означали бы, что они настроены по-разному ровно тогда, когда сравниваются.
//
// ⚠️ Значения по умолчанию тоже совпадают с Java. Отличие в умолчании — это отличие
// в поведении, которое не поймает ни один differential-тест: оба ответят «как настроено»,
// а настроены будут по-разному.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config — всё, что бэкенд знает о своём окружении.
type Config struct {
	Port int

	DatabaseURL string

	// JWTSecret — общий секрет HS256. Тот же, что у Java: в окне отката токены,
	// выданные одним бэкендом, обязан принимать другой.
	JWTSecret []byte

	InviteCodes  []string
	SeasonAdmins []string

	// AutoMove — ходить ли за игрока по истечении времени хода.
	// ⚠️ По умолчанию ВЫКЛЮЧЕНО: ход остаётся за игроком, стол просто ждёт.
	AutoMove bool

	WSOrigins        []string
	WSOriginPatterns []string

	VAPIDPublic  string
	VAPIDPrivate string
	VAPIDSubject string

	ShutdownTimeout time.Duration
}

// MinJWTSecretLen — HS256 требует ключ не короче 256 бит.
const MinJWTSecretLen = 32

// Load читает окружение. Ошибку возвращает только на том, что чинить в рантайме нельзя.
func Load() (Config, error) {
	port, err := strconv.Atoi(env("BARDAK_PORT", "8088"))
	if err != nil {
		return Config{}, fmt.Errorf("BARDAK_PORT: %w", err)
	}

	secret := env("BARDAK_JWT_SECRET", "dev-only-secret-change-me-32-bytes-minimum!!")
	if len(secret) < MinJWTSecretLen {
		return Config{}, fmt.Errorf("BARDAK_JWT_SECRET короче %d байт: HS256 такой ключ не примет", MinJWTSecretLen)
	}

	autoMove, err := strconv.ParseBool(env("BARDAK_AUTO_MOVE", "false"))
	if err != nil {
		return Config{}, fmt.Errorf("BARDAK_AUTO_MOVE: %w", err)
	}

	return Config{
		Port:             port,
		DatabaseURL:      env("BARDAK_DB_URL", "postgres://bardak:bardak@localhost:5432/bardak"),
		JWTSecret:        []byte(secret),
		InviteCodes:      list(env("BARDAK_INVITE_CODES", "bardak-2026")),
		SeasonAdmins:     list(env("BARDAK_SEASON_ADMINS", "")),
		AutoMove:         autoMove,
		WSOrigins:        list(env("BARDAK_WS_ORIGINS", "http://localhost:8088,http://localhost:5173")),
		WSOriginPatterns: list(env("BARDAK_WS_ORIGIN_PATTERNS", "http://192.168.*.*:8088,http://10.*.*.*:8088,http://172.16.*.*:8088")),
		VAPIDPublic:      env("BARDAK_VAPID_PUBLIC", ""),
		VAPIDPrivate:     env("BARDAK_VAPID_PRIVATE", ""),
		VAPIDSubject:     env("BARDAK_VAPID_SUBJECT", "mailto:admin@bardak.local"),
		ShutdownTimeout:  20 * time.Second,
	}, nil
}

func env(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// list разбирает список через запятую. Пустая строка — пустой список, а не список
// из одной пустой строки: иначе «нет ведущих сезона» превращалось бы в «ведущий с пустым логином».
func list(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// IsInviteCodeValid — регистр не важен.
//
// ⚠️ Не косметика: поле ввода на клиенте приводит код к верхнему регистру, а в настройках
// он записан строчными. Строгое сравнение однажды уже не пускало никого зарегистрироваться.
func (c Config) IsInviteCodeValid(code string) bool {
	candidate := strings.TrimSpace(code)
	for _, known := range c.InviteCodes {
		if strings.EqualFold(known, candidate) {
			return true
		}
	}
	return false
}

// IsSeasonAdmin — вправе ли логин закрыть сезон.
func (c Config) IsSeasonAdmin(username string) bool {
	for _, admin := range c.SeasonAdmins {
		if admin == username {
			return true
		}
	}
	return false
}
