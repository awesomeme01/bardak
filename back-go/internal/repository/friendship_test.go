package repository

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Дружбы против НАСТОЯЩЕГО Postgres со схемой Java.
//
// ⚠️ Половина поведения здесь принадлежит базе: первичный ключ пары, проверка
// friendships_ordered и внешние ключи на users. На подделке это не проверить.

// ⚠️ Главная ловушка переноса: порядок пары считается ПО СТРОКЕ, как у Postgres,
// а не как у Java UUID.compareTo (два знаковых long). На идентификаторах со старшим
// единичным битом эти порядки противоположны.
func TestComparePairOrderMatchesPostgresAndNotSignedLongs(t *testing.T) {
	// У первого старший бит выставлен: Java-сравнение считает его ОТРИЦАТЕЛЬНЫМ
	// и ставит перед вторым, Postgres же — после.
	high := "ffffffff-ffff-4fff-bfff-ffffffffffff"
	low := "0fffffff-ffff-4fff-8fff-ffffffffffff"

	if ComparePairOrder(low, high) >= 0 {
		t.Fatalf("по строке %q обязан идти раньше %q — так же сравнивает Postgres", low, high)
	}
	gotLow, gotHigh := OrderPair(high, low)
	if gotLow != low || gotHigh != high {
		t.Errorf("порядок пары перевёрнут: (%s, %s)", gotLow, gotHigh)
	}

	// Идентификатор мог прийти из пути запроса в верхнем регистре, а Postgres печатает
	// uuid только строчными: без приведения порядок разъехался бы на ровном месте.
	upperLow, upperHigh := OrderPair(strings.ToUpper(high), strings.ToUpper(low))
	if upperLow != low || upperHigh != high {
		t.Errorf("верхний регистр ломает порядок: (%s, %s)", upperLow, upperHigh)
	}
}

// ⭐ Одна строка на пару: заявка ложится в базу в любом порядке аргументов и находится
// с обеих сторон. Перепутанный порядок упёрся бы в проверку friendships_ordered — ровно
// в половине случаев, поэтому берём пару, на которой знаковое сравнение ошибается.
func TestFriendshipIsOneRowForBothSides(t *testing.T) {
	pool := testDB(t)
	friendships := NewFriendships(pool)
	ctx := context.Background()

	asker, target := newFriendUser(t, pool, "fr-asker"), newFriendUser(t, pool, "fr-target")

	created, err := friendships.Insert(ctx, asker, target)
	if err != nil {
		t.Fatalf("заявка не завелась: %v", err)
	}
	if created.Status != FriendshipPending {
		t.Errorf("новая заявка обязана быть PENDING, а не %q", created.Status)
	}
	if created.RequestedBy != asker {
		t.Errorf("заявку отправил %s, а записан %s", asker, created.RequestedBy)
	}
	if ComparePairOrder(created.LowUserID, created.HighUserID) >= 0 {
		t.Errorf("в базе нарушен порядок пары: %s не меньше %s", created.LowUserID, created.HighUserID)
	}

	// Пара находится с обеих сторон — это и есть «одна строка на двоих».
	for _, pairOrder := range [][2]string{{asker, target}, {target, asker}} {
		found, err := friendships.FindPair(ctx, pairOrder[0], pairOrder[1])
		if err != nil {
			t.Fatalf("пара (%s, %s) не найдена: %v", pairOrder[0], pairOrder[1], err)
		}
		if !found.Involves(asker) || !found.Involves(target) {
			t.Error("найденная пара не содержит обоих участников")
		}
	}

	// Встречная заявка второй строкой лечь не может: правду держит первичный ключ.
	if _, err := friendships.Insert(ctx, target, asker); !errors.Is(err, ErrConflict) {
		t.Errorf("вторая строка на ту же пару должна давать ErrConflict, получили %v", err)
	}
}

// Принять заявку может только тот, кто её не отправлял, и после принятия дружба
// симметрична: асимметрия означала бы «он у меня в друзьях, а я у него нет».
func TestFriendshipAcceptMakesItMutual(t *testing.T) {
	pool := testDB(t)
	friendships := NewFriendships(pool)
	ctx := context.Background()

	asker, target := newFriendUser(t, pool, "fr-acc-a"), newFriendUser(t, pool, "fr-acc-b")
	pair, err := friendships.Insert(ctx, asker, target)
	if err != nil {
		t.Fatal(err)
	}
	if pair.CanBeAcceptedBy(asker) {
		t.Error("автор заявки не может принимать её сам")
	}
	if !pair.CanBeAcceptedBy(target) {
		t.Error("адресат обязан иметь право принять заявку")
	}

	accepted, err := friendships.Accept(ctx, target, asker, time.Now().UTC())
	if err != nil {
		t.Fatalf("принятие: %v", err)
	}
	if !accepted.IsAccepted() {
		t.Errorf("статус после принятия %q", accepted.Status)
	}
	if accepted.DecidedAt == nil {
		t.Error("decided_at обязан проставиться: по нему видно, когда дружба состоялась")
	}

	for _, order := range [][2]string{{asker, target}, {target, asker}} {
		friends, err := friendships.IsFriend(ctx, order[0], order[1])
		if err != nil {
			t.Fatal(err)
		}
		if !friends {
			t.Errorf("дружба обязана быть видна в обе стороны, а (%s, %s) её не видит",
				order[0], order[1])
		}
	}
}

// ⭐ Убрать из друзей и отклонить заявку — одна операция: строки просто больше нет.
func TestFriendshipDeleteDropsThePairForBoth(t *testing.T) {
	pool := testDB(t)
	friendships := NewFriendships(pool)
	ctx := context.Background()

	one, two := newFriendUser(t, pool, "fr-del-a"), newFriendUser(t, pool, "fr-del-b")
	if _, err := friendships.Insert(ctx, one, two); err != nil {
		t.Fatal(err)
	}

	// Удаляет второй участник, а не автор заявки: для сервера это одно и то же действие.
	if err := friendships.Delete(ctx, two, one); err != nil {
		t.Fatalf("удаление: %v", err)
	}
	if _, err := friendships.FindPair(ctx, one, two); !errors.Is(err, ErrNotFound) {
		t.Errorf("после удаления пары быть не должно, получили %v", err)
	}
	if err := friendships.Delete(ctx, one, two); !errors.Is(err, ErrNotFound) {
		t.Errorf("повторное удаление обязано отвечать ErrNotFound, получили %v", err)
	}
	friends, err := friendships.IsFriend(ctx, one, two)
	if err != nil {
		t.Fatal(err)
	}
	if friends {
		t.Error("после удаления дружбы быть не может")
	}
}

// Висящая заявка дружбой НЕ считается: согласия ещё не было, а вместе с ним нет ни права
// звать за стол, ни доступа к присутствию.
func TestPendingRequestIsNotFriendship(t *testing.T) {
	pool := testDB(t)
	friendships := NewFriendships(pool)
	ctx := context.Background()

	one, two := newFriendUser(t, pool, "fr-pend-a"), newFriendUser(t, pool, "fr-pend-b")
	if _, err := friendships.Insert(ctx, one, two); err != nil {
		t.Fatal(err)
	}

	friends, err := friendships.IsFriend(ctx, one, two)
	if err != nil {
		t.Fatal(err)
	}
	if friends {
		t.Error("PENDING не должен считаться дружбой")
	}
}

// Список читается одним запросом вместе с профилями, и в нём видны обе стороны пары.
func TestFindAllInvolvingReturnsBothSidesWithProfiles(t *testing.T) {
	pool := testDB(t)
	friendships := NewFriendships(pool)
	ctx := context.Background()

	me := newFriendUser(t, pool, "fr-list-me")
	asked := newFriendUser(t, pool, "fr-list-out")
	asker := newFriendUser(t, pool, "fr-list-in")

	if _, err := friendships.Insert(ctx, me, asked); err != nil {
		t.Fatal(err)
	}
	if _, err := friendships.Insert(ctx, asker, me); err != nil {
		t.Fatal(err)
	}

	pairs, err := friendships.FindAllInvolving(ctx, me)
	if err != nil {
		t.Fatalf("список: %v", err)
	}
	if len(pairs) != 2 {
		t.Fatalf("ждали две пары, получили %d", len(pairs))
	}

	seen := map[string]string{}
	for _, item := range pairs {
		if item.Other.ID == me {
			t.Error("в списке не должно быть меня самого: показывается второй участник")
		}
		if item.Other.Username == "" || item.Other.DisplayName == "" {
			t.Errorf("профиль друга пуст: %+v", item.Other)
		}
		seen[item.Other.ID] = item.Pair.RequestedBy
	}
	if seen[asked] != me {
		t.Error("исходящая заявка обязана помнить, что её отправил я")
	}
	if seen[asker] != asker {
		t.Error("входящая заявка обязана помнить, что её отправил другой")
	}
}

// newFriendUser заводит игрока: без строки в users пара не ляжет — на всех трёх
// колонках стоят внешние ключи.
func newFriendUser(t *testing.T, pool *pgxpool.Pool, prefix string) string {
	t.Helper()
	id := uuid.NewString()
	_, err := NewUsers(pool).Insert(context.Background(), User{
		ID: id, Username: prefix + "-" + id[:8], DisplayName: prefix, PasswordHash: "hash",
	})
	if err != nil {
		t.Fatalf("не удалось завести игрока %s: %v", prefix, err)
	}
	return id
}
