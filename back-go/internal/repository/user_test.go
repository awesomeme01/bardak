package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// Репозиторий пользователей против НАСТОЯЩЕГО Postgres со схемой Java.
//
// ⚠️ Проверять такое на подделке бессмысленно: половина поведения здесь принадлежит базе —
// уникальные индексы, регистр, значения по умолчанию.

func TestUserRoundTrip(t *testing.T) {
	pool := testDB(t)
	users := NewUsers(pool)
	ctx := context.Background()

	id := uuid.NewString()
	saved, err := users.Insert(ctx, User{
		ID: id, Username: "Shabdan-" + id[:6], DisplayName: "Шабдан",
		PasswordHash: "hash",
	})
	if err != nil {
		t.Fatalf("вставка: %v", err)
	}

	if saved.Status != "ACTIVE" {
		t.Errorf("статус по умолчанию %q, ждали ACTIVE — его ставит база", saved.Status)
	}
	if saved.CreatedAt.IsZero() {
		t.Error("created_at ставит база, он не должен быть пустым")
	}

	found, err := users.FindByID(ctx, id)
	if err != nil {
		t.Fatalf("чтение по id: %v", err)
	}
	if found.Username != saved.Username {
		t.Errorf("логин разошёлся: %q против %q", found.Username, saved.Username)
	}
}

// ⭐ Вход по логину в другом регистре обязан работать: в базе стоит уникальный индекс
// по lower(username), и строгое сравнение однажды уже не пускало войти с верными данными.
func TestFindByUsernameIgnoresCase(t *testing.T) {
	pool := testDB(t)
	users := NewUsers(pool)
	ctx := context.Background()

	id := uuid.NewString()
	login := "MixedCase-" + id[:6]
	if _, err := users.Insert(ctx, User{ID: id, Username: login,
		DisplayName: "Игрок", PasswordHash: "hash"}); err != nil {
		t.Fatal(err)
	}

	for _, variant := range []string{login, upper(login), lower(login)} {
		found, err := users.FindByUsernameIgnoreCase(ctx, variant)
		if err != nil {
			t.Errorf("вариант %q не найден: %v", variant, err)
			continue
		}
		if found.ID != id {
			t.Errorf("вариант %q нашёл чужого", variant)
		}
	}

	if _, err := users.FindByUsernameIgnoreCase(ctx, "нет-такого-логина"); !errors.Is(err, ErrNotFound) {
		t.Errorf("отсутствующий логин должен давать ErrNotFound, получили %v", err)
	}
}

// ⚠️ Правду про занятость держит БАЗА, а не предварительная проверка: проверка и вставка
// не атомарны. Именно эта гонка давала 500 вместо готового кода в Java.
func TestDuplicateLoginIsConflict(t *testing.T) {
	pool := testDB(t)
	users := NewUsers(pool)
	ctx := context.Background()

	login := "dup-" + uuid.NewString()[:8]
	if _, err := users.Insert(ctx, User{ID: uuid.NewString(), Username: login,
		DisplayName: "Первый", PasswordHash: "hash"}); err != nil {
		t.Fatal(err)
	}

	// Тот же логин в ДРУГОМ регистре — тоже занят: индекс построен по lower(username).
	_, err := users.Insert(ctx, User{ID: uuid.NewString(), Username: upper(login),
		DisplayName: "Второй", PasswordHash: "hash"})
	if !errors.Is(err, ErrConflict) {
		t.Errorf("повтор логина должен давать ErrConflict, получили %v", err)
	}

	exists, err := users.ExistsByUsernameIgnoreCase(ctx, upper(login))
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Error("занятость логина не видна без учёта регистра")
	}
}

func TestUpdateProfileKeepsLogin(t *testing.T) {
	pool := testDB(t)
	users := NewUsers(pool)
	ctx := context.Background()

	id := uuid.NewString()
	login := "profile-" + id[:6]
	if _, err := users.Insert(ctx, User{ID: id, Username: login,
		DisplayName: "Старое", PasswordHash: "hash"}); err != nil {
		t.Fatal(err)
	}

	avatar := "🦊"
	updated, err := users.UpdateProfile(ctx, id, "Новое", &avatar)
	if err != nil {
		t.Fatalf("обновление профиля: %v", err)
	}

	if updated.DisplayName != "Новое" {
		t.Errorf("имя не поменялось: %q", updated.DisplayName)
	}
	if updated.Avatar == nil || *updated.Avatar != avatar {
		t.Error("мордочка не сохранилась")
	}
	// ⭐ Логин не меняется никогда: по нему входят.
	if updated.Username != login {
		t.Errorf("логин изменился на %q — по нему входят, он обязан остаться", updated.Username)
	}
}

func TestUpdatePasswordOfMissingUser(t *testing.T) {
	pool := testDB(t)
	users := NewUsers(pool)

	err := users.UpdatePasswordHash(context.Background(), uuid.NewString(), "новый-хеш")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("смена пароля несуществующему должна давать ErrNotFound, получили %v", err)
	}
}

func upper(s string) string { return mapCase(s, true) }
func lower(s string) string { return mapCase(s, false) }

func mapCase(s string, up bool) string {
	out := []rune(s)
	for i, r := range out {
		if up && r >= 'a' && r <= 'z' {
			out[i] = r - 32
		}
		if !up && r >= 'A' && r <= 'Z' {
			out[i] = r + 32
		}
	}
	return string(out)
}
