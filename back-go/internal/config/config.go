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

	// TurnTimeout — сколько отведено на ход, DisconnectGrace — сколько ждут пропавшего
	// со связи, прежде чем отменить матч (§5.1–5.2).
	//
	// ⚠️ В Java обе величины зашиты в application.yml без переменной окружения. Здесь
	// они читаются из окружения, но УМОЛЧАНИЯ те же — 30 с и 60 с: иначе два бэкенда,
	// поднятые рядом, вели бы себя по-разному ровно там, где это никем не сравнивается.
	// Переменные нужны стендам и тестам, где ждать полминуты нечем.
	TurnTimeout     time.Duration
	DisconnectGrace time.Duration

	WSOrigins        []string
	WSOriginPatterns []string

	// FrontendPath и AssetsPath — собранный фронт и картинки карт. Пусто — не раздаём:
	// в проде перед сервером может стоять Caddy, и тогда файлы отдаёт он.
	FrontendPath string
	AssetsPath   string

	VAPIDPublic  string
	VAPIDPrivate string
	VAPIDSubject string

	// PushQuietFor — сколько молчать после звонка одному игроку. Ход может вернуться
	// к нему через несколько секунд, и без паузы партия превратилась бы в очередь звонков.
	PushQuietFor time.Duration

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

	turnTimeout, err := duration(env("BARDAK_TURN_TIMEOUT", "30s"))
	if err != nil {
		return Config{}, fmt.Errorf("BARDAK_TURN_TIMEOUT: %w", err)
	}
	disconnectGrace, err := duration(env("BARDAK_DISCONNECT_GRACE", "60s"))
	if err != nil {
		return Config{}, fmt.Errorf("BARDAK_DISCONNECT_GRACE: %w", err)
	}
	pushQuietFor, err := duration(env("BARDAK_PUSH_QUIET_FOR", "2m"))
	if err != nil {
		return Config{}, fmt.Errorf("BARDAK_PUSH_QUIET_FOR: %w", err)
	}

	return Config{
		Port:             port,
		DatabaseURL:      env("BARDAK_DB_URL", "postgres://bardak:bardak@localhost:5432/bardak"),
		JWTSecret:        []byte(secret),
		InviteCodes:      list(env("BARDAK_INVITE_CODES", "bardak-2026")),
		SeasonAdmins:     list(env("BARDAK_SEASON_ADMINS", "")),
		AutoMove:         autoMove,
		TurnTimeout:      turnTimeout,
		DisconnectGrace:  disconnectGrace,
		WSOrigins:        list(env("BARDAK_WS_ORIGINS", "http://localhost:8088,http://localhost:5173")),
		WSOriginPatterns: list(env("BARDAK_WS_ORIGIN_PATTERNS", "http://192.168.*.*:8088,http://10.*.*.*:8088,http://172.16.*.*:8088")),
		FrontendPath:     env("BARDAK_FRONTEND_PATH", "../front-bardak/dist"),
		AssetsPath:       env("BARDAK_ASSETS_PATH", "../assets"),
		VAPIDPublic:      env("BARDAK_VAPID_PUBLIC", ""),
		VAPIDPrivate:     env("BARDAK_VAPID_PRIVATE", ""),
		VAPIDSubject:     env("BARDAK_VAPID_SUBJECT", "mailto:admin@bardak.local"),
		PushQuietFor:     pushQuietFor,
		ShutdownTimeout:  20 * time.Second,
	}, nil
}

// duration разбирает срок в записи Go («30s»). Ноль и отрицательные не принимаются:
// нулевой таймаут хода означал бы, что сервер ходит за игрока мгновенно.
func duration(raw string) (time.Duration, error) {
	value, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil {
		return 0, err
	}
	if value <= 0 {
		return 0, fmt.Errorf("срок %q должен быть положительным", raw)
	}
	return value, nil
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
