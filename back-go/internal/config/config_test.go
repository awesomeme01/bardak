package config

import (
	"os"
	"testing"
	"time"
)

// ⭐ Регистр кода приглашения — не мелочь: поле ввода на клиенте приводит его к верхнему
// регистру, а в настройках он записан строчными. Строгое сравнение однажды уже не пускало
// зарегистрироваться никого.
func TestInviteCodeIgnoresCase(t *testing.T) {
	cfg := Config{InviteCodes: []string{"bardak-2026"}}

	cases := map[string]bool{
		"bardak-2026":  true,
		"BARDAK-2026":  true,
		"Bardak-2026":  true,
		" bardak-2026": true, // пробелы из буфера обмена
		"bardak-2027":  false,
		"":             false,
	}

	for code, want := range cases {
		if got := cfg.IsInviteCodeValid(code); got != want {
			t.Errorf("код %q: получили %v, ждали %v", code, got, want)
		}
	}
}

// ⚠️ Логин ведущего сезона сравнивается СТРОГО, в отличие от кода приглашения: это право,
// а не удобство ввода, и расширять его регистром нельзя.
func TestSeasonAdminIsExactMatch(t *testing.T) {
	cfg := Config{SeasonAdmins: []string{"shabdan"}}

	if !cfg.IsSeasonAdmin("shabdan") {
		t.Error("свой логин должен проходить")
	}
	if cfg.IsSeasonAdmin("SHABDAN") {
		t.Error("другой регистр правом не является")
	}
	if cfg.IsSeasonAdmin("") {
		t.Error("пустой логин не должен давать право")
	}
}

// Пустая строка настройки — это «никого», а не «один с пустым именем».
func TestEmptyListIsNobody(t *testing.T) {
	if got := list(""); got != nil {
		t.Errorf("пустая строка дала %#v, ждали nil", got)
	}
	if got := list("  "); got != nil {
		t.Errorf("пробелы дали %#v, ждали nil", got)
	}
	if got := list("a, b ,c"); len(got) != 3 || got[1] != "b" {
		t.Errorf("разбор списка сломан: %#v", got)
	}

	// Без этого «нет ведущих» превращалось бы в «есть ведущий с пустым логином»,
	// и пустой claim в токене открывал бы закрытие сезона кому угодно.
	cfg := Config{SeasonAdmins: list("")}
	if cfg.IsSeasonAdmin("") {
		t.Error("пустой список ведущих не должен давать право пустому логину")
	}
}

// HS256 не примет ключ короче 256 бит — падать надо на старте, а не на первом входе.
func TestShortSecretIsRefused(t *testing.T) {
	t.Setenv("BARDAK_JWT_SECRET", "коротко")

	if _, err := Load(); err == nil {
		t.Fatal("короткий секрет должен ронять запуск")
	}
}

// Автоход по таймауту по умолчанию ВЫКЛЮЧЕН: ход остаётся за игроком, стол ждёт.
func TestAutoMoveIsOffByDefault(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("умолчания должны грузиться без ошибки: %v", err)
	}
	if cfg.AutoMove {
		t.Error("автоход обязан быть выключен по умолчанию")
	}
	if cfg.Port != 8088 {
		t.Errorf("порт по умолчанию %d, ждали 8088", cfg.Port)
	}
}

// ⚠️ Умолчания времён стола обязаны совпадать с Java (30 с и 60 с): различие в умолчании
// не поймает ни один differential — оба бэкенда ответят «как настроено», а настроены
// будут по-разному.
func TestGameTimingsDefaultToJavaValues(t *testing.T) {
	// t.Setenv, а затем снятие: так значение вернётся после теста, даже если оно было.
	t.Setenv("BARDAK_TURN_TIMEOUT", "")
	t.Setenv("BARDAK_DISCONNECT_GRACE", "")
	os.Unsetenv("BARDAK_TURN_TIMEOUT")
	os.Unsetenv("BARDAK_DISCONNECT_GRACE")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("конфигурация не собралась: %v", err)
	}

	if cfg.TurnTimeout != 30*time.Second {
		t.Errorf("ход длится %v, в Java 30s", cfg.TurnTimeout)
	}
	if cfg.DisconnectGrace != 60*time.Second {
		t.Errorf("пропавшего ждут %v, в Java 60s", cfg.DisconnectGrace)
	}
}

func TestGameTimingsComeFromTheEnvironment(t *testing.T) {
	t.Setenv("BARDAK_TURN_TIMEOUT", "1s")
	t.Setenv("BARDAK_DISCONNECT_GRACE", "2500ms")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("конфигурация не собралась: %v", err)
	}

	if cfg.TurnTimeout != time.Second || cfg.DisconnectGrace != 2500*time.Millisecond {
		t.Errorf("времена стола прочитаны неверно: %v / %v", cfg.TurnTimeout, cfg.DisconnectGrace)
	}
}

// Нулевой таймаут хода означал бы, что сервер ходит за игрока мгновенно: лучше отказ
// на старте, чем стол, играющий сам с собой.
func TestZeroTurnTimeoutIsRefused(t *testing.T) {
	t.Setenv("BARDAK_TURN_TIMEOUT", "0s")

	if _, err := Load(); err == nil {
		t.Fatal("нулевой таймаут хода принят")
	}
}

// ⚠️ Окно тишины уведомлений тоже обязано совпадать с Java (2 минуты): различие здесь
// не увидит никто, кроме игрока, которому звонят вдвое чаще, чем задумано.
func TestPushQuietWindowDefaultsToJavaValue(t *testing.T) {
	t.Setenv("BARDAK_PUSH_QUIET_FOR", "")
	os.Unsetenv("BARDAK_PUSH_QUIET_FOR")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("конфигурация не собралась: %v", err)
	}

	if cfg.PushQuietFor != 2*time.Minute {
		t.Errorf("окно тишины %v, в Java 2m", cfg.PushQuietFor)
	}
}

func TestPushQuietWindowComesFromTheEnvironment(t *testing.T) {
	t.Setenv("BARDAK_PUSH_QUIET_FOR", "45s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("конфигурация не собралась: %v", err)
	}

	if cfg.PushQuietFor != 45*time.Second {
		t.Errorf("окно тишины прочитано неверно: %v", cfg.PushQuietFor)
	}
}

// Прод с незаданными переменными — сервер с ИЗВЕСТНЫМ секретом и известным кодом
// приглашения.
//
// ⚠️ Проверяется отказ, а не предупреждение: сервер, поднявшийся с секретом из
// репозитория, выглядит здоровым, и узнают об этом по чужим токенам.
func TestProductionRefusesTheDevelopmentSecret(t *testing.T) {
	t.Setenv("BARDAK_ENV", "prod")
	t.Setenv("BARDAK_JWT_SECRET", "")
	os.Unsetenv("BARDAK_JWT_SECRET")

	if _, err := Load(); err == nil {
		t.Fatal("прод поднялся с секретом для разработки")
	}
}

func TestProductionRefusesTheDefaultInviteCode(t *testing.T) {
	t.Setenv("BARDAK_ENV", "prod")
	t.Setenv("BARDAK_JWT_SECRET", "секрет-достаточной-длины-для-HS256-подписи")
	t.Setenv("BARDAK_INVITE_CODES", "")
	os.Unsetenv("BARDAK_INVITE_CODES")

	if _, err := Load(); err == nil {
		t.Fatal("прод поднялся с кодом приглашения из README")
	}
}

func TestProductionStartsWithItsOwnSecrets(t *testing.T) {
	t.Setenv("BARDAK_ENV", "prod")
	t.Setenv("BARDAK_JWT_SECRET", "секрет-достаточной-длины-для-HS256-подписи")
	t.Setenv("BARDAK_INVITE_CODES", "свой-код-приглашения")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("прод со своими секретами не поднялся: %v", err)
	}
	if !cfg.Production {
		t.Fatal("BARDAK_ENV=prod не распознан")
	}
}

// ⚠️ Обратная сторона: локальный запуск обязан остаться запуском в одну команду.
// Требовать секреты на машине разработчика — верный способ получить их в git.
func TestDevelopmentStartsWithoutAnyEnvironment(t *testing.T) {
	for _, name := range []string{"BARDAK_ENV", "BARDAK_JWT_SECRET", "BARDAK_INVITE_CODES"} {
		t.Setenv(name, "")
		os.Unsetenv(name)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("локальный запуск потребовал настройки: %v", err)
	}
	if cfg.Production {
		t.Fatal("сервер без BARDAK_ENV считает себя продом")
	}
}
