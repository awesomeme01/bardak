package ws

import (
	"context"
	"testing"
	"time"
)

// Реестр столов: стол поднимается по первому подключению и выгружается, когда за ним
// никого не осталось.

func TestRegistryKeepsOneRuntimePerTable(t *testing.T) {
	registry := NewTableRegistry(context.Background(), nil)
	defer registry.CloseAll()

	first := registry.RuntimeFor("table-1")
	second := registry.RuntimeFor("table-1")
	other := registry.RuntimeFor("table-2")

	if first != second {
		t.Fatalf("на один стол подняты два рантайма: команды пошли бы двумя очередями")
	}
	if other == first {
		t.Fatalf("два стола получили один рантайм")
	}
	if registry.Size() != 2 {
		t.Fatalf("реестр держит %d столов, ждали 2", registry.Size())
	}
}

func TestRegistryUnloadsTheTableWhenLastPlayerLeaves(t *testing.T) {
	registry := NewTableRegistry(context.Background(), nil)
	defer registry.CloseAll()
	runtime := registry.RuntimeFor("table-1")
	registry.Subscribe(runtime, silentClient("user-a"))
	registry.Subscribe(runtime, silentClient("user-b"))

	registry.Unsubscribe("table-1", "user-a")

	if registry.Size() != 1 {
		t.Fatalf("стол выгружен, пока за ним ещё сидят")
	}

	registry.Unsubscribe("table-1", "user-b")

	if registry.Size() != 0 {
		t.Fatalf("пустой стол остался в памяти: goroutine и очередь висят ни на ком")
	}
}

// Отписка неизвестного стола — не поломка: обрыв связи приходит и от того, кто ни за
// каким столом не сидел.
func TestRegistryIgnoresUnsubscribeOfUnknownTable(t *testing.T) {
	registry := NewTableRegistry(context.Background(), nil)
	defer registry.CloseAll()

	registry.Unsubscribe("table-1", "user-a")

	if registry.Size() != 0 {
		t.Fatalf("отписка подняла стол, которого не было")
	}
}

// ⭐ Подписка заводится вместе с качалкой: без читателя очередь наполнялась бы молча,
// и игрок терял бы соединение из-за того, что его никто не слушает.
func TestRegistrySubscriptionPumpsMessagesToTheSocket(t *testing.T) {
	registry := NewTableRegistry(context.Background(), nil)
	defer registry.CloseAll()
	runtime := registry.RuntimeFor("table-1")
	received := make(chan []byte, 4)
	registry.Subscribe(runtime, Client{
		UserID:  "user-a",
		Send:    func(Envelope) {},
		SendRaw: func(raw []byte) { received <- raw },
	})

	runtime.Broadcast([]byte(`{"type":"PLAYER_JOINED"}`))

	select {
	case message := <-received:
		if string(message) != `{"type":"PLAYER_JOINED"}` {
			t.Fatalf("до сокета доехало не то: %s", message)
		}
	case <-time.After(time.Second):
		t.Fatal("рассылка стола не доехала до сокета")
	}
}

func silentClient(userID string) Client {
	return Client{UserID: userID, Send: func(Envelope) {}, SendRaw: func([]byte) {}}
}
