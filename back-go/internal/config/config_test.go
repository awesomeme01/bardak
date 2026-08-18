package config

import "testing"

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
