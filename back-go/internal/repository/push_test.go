package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// Подписки против НАСТОЯЩЕГО Postgres: здесь всё держится на UNIQUE (endpoint),
// а его на подделке не воспроизвести.

// ⭐ Ключ строки — endpoint, а не пара «пользователь + устройство»: браузер присылает
// ту же подписку при каждом запуске приложения. Без склейки игрок получал бы один и тот же
// звонок столько раз, сколько раз открывал вкладку.
func TestPushSubscriptionKeepsOneRowPerEndpoint(t *testing.T) {
	pool := testDB(t)
	subscriptions := NewPushSubscriptions(pool)
	ctx := context.Background()

	owner := newFriendUser(t, pool, "push-owner")
	endpoint := "https://push.example/" + uuid.NewString()
	agent := "Chrome"

	first, err := subscriptions.Save(ctx, PushSubscription{
		ID: uuid.NewString(), UserID: owner, Endpoint: endpoint,
		P256dh: "key-1", Auth: "auth-1", UserAgent: &agent,
	})
	if err != nil {
		t.Fatalf("первая подписка: %v", err)
	}

	second, err := subscriptions.Save(ctx, PushSubscription{
		ID: uuid.NewString(), UserID: owner, Endpoint: endpoint,
		P256dh: "key-2", Auth: "auth-2",
	})
	if err != nil {
		t.Fatalf("повторная подписка: %v", err)
	}

	if second.ID != first.ID {
		t.Errorf("строка пересоздана: id %s стал %s — это то же устройство", first.ID, second.ID)
	}
	if !second.CreatedAt.Equal(first.CreatedAt) {
		t.Error("created_at обязан сохраниться: устройство то же, обновились только ключи")
	}
	if second.P256dh != "key-2" || second.Auth != "auth-2" {
		t.Errorf("ключи не обновились: %+v", second)
	}
	if second.UserAgent != nil {
		t.Error("не присланный User-Agent обязан затирать прежний, а не оставаться старым")
	}

	rows, err := subscriptions.FindByUserID(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Errorf("у игрока %d строк вместо одной — звонок придёт дважды", len(rows))
	}
}

// ⭐ Тот же endpoint у другого игрока МЕНЯЕТ владельца строки, а не заводит вторую:
// телефон перешёл к другому человеку, и звонить на него прежнему нельзя.
func TestPushSubscriptionMovesToTheNewOwner(t *testing.T) {
	pool := testDB(t)
	subscriptions := NewPushSubscriptions(pool)
	ctx := context.Background()

	former := newFriendUser(t, pool, "push-former")
	newcomer := newFriendUser(t, pool, "push-newcomer")
	endpoint := "https://push.example/" + uuid.NewString()

	if _, err := subscriptions.Save(ctx, PushSubscription{
		ID: uuid.NewString(), UserID: former, Endpoint: endpoint, P256dh: "k", Auth: "a",
	}); err != nil {
		t.Fatal(err)
	}
	moved, err := subscriptions.Save(ctx, PushSubscription{
		ID: uuid.NewString(), UserID: newcomer, Endpoint: endpoint, P256dh: "k2", Auth: "a2",
	})
	if err != nil {
		t.Fatalf("смена владельца: %v", err)
	}

	if moved.UserID != newcomer {
		t.Errorf("владелец не сменился: %s", moved.UserID)
	}
	left, err := subscriptions.FindByUserID(ctx, former)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Error("у прежнего хозяина устройство остаться не должно: он видел бы чужие партии")
	}
}

// ⚠️ Отписка сверяет ВЛАДЕЛЬЦА. Пока условие было только по endpoint, любой вошедший,
// знающий чужой адрес подписки, отписывал чужое устройство — и хозяин не мог понять,
// почему перестали приходить уведомления о его ходе.
func TestPushUnsubscribeChecksTheOwner(t *testing.T) {
	pool := testDB(t)
	subscriptions := NewPushSubscriptions(pool)
	ctx := context.Background()

	owner := newFriendUser(t, pool, "push-mine")
	stranger := newFriendUser(t, pool, "push-stranger")
	endpoint := "https://push.example/" + uuid.NewString()

	if _, err := subscriptions.Save(ctx, PushSubscription{
		ID: uuid.NewString(), UserID: owner, Endpoint: endpoint, P256dh: "k", Auth: "a",
	}); err != nil {
		t.Fatal(err)
	}

	removed, err := subscriptions.DeleteByEndpointAndUserID(ctx, endpoint, stranger)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 0 {
		t.Error("чужую подписку отписать нельзя")
	}
	if _, err := subscriptions.FindByEndpoint(ctx, endpoint); err != nil {
		t.Fatalf("подписка обязана остаться на месте: %v", err)
	}

	removed, err = subscriptions.DeleteByEndpointAndUserID(ctx, endpoint, owner)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Errorf("владелец обязан отписать своё устройство, удалено строк: %d", removed)
	}

	// Отписка идемпотентна: повторный вызов с того же устройства — обычное дело,
	// и ошибкой он быть не должен.
	removed, err = subscriptions.DeleteByEndpointAndUserID(ctx, endpoint, owner)
	if err != nil || removed != 0 {
		t.Errorf("повторная отписка должна проходить молча: удалено %d, ошибка %v", removed, err)
	}
	if _, err := subscriptions.FindByEndpoint(ctx, endpoint); !errors.Is(err, ErrNotFound) {
		t.Errorf("после отписки строки быть не должно, получили %v", err)
	}
}
