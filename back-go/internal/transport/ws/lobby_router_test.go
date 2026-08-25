package ws

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/awesomeme01/bardak/back-go/internal/application"
	"github.com/awesomeme01/bardak/back-go/internal/repository"
)

// Команды лобби поверх сокета.
//
// ⭐ Проверяется не «пришло событие», а КОМУ и В КАКОМ ПОРЯДКЕ: уходящий обязан увидеть
// собственный уход, а сообщать о нём после отписки — значит не сообщить вовсе.

type fakeLobby struct {
	seat     repository.TablePlayer
	joinErr  error
	leaveErr error
	readyErr error

	lastReady bool
	left      int
}

func (f *fakeLobby) Join(context.Context, string, string) (repository.TablePlayer, error) {
	return f.seat, f.joinErr
}

func (f *fakeLobby) Leave(context.Context, string, string) error {
	f.left++
	return f.leaveErr
}

func (f *fakeLobby) SetReady(_ context.Context, _, _ string, ready bool) (repository.TablePlayer, error) {
	f.lastReady = ready
	seat := f.seat
	if ready {
		seat.State = repository.SeatReady
	}
	return seat, f.readyErr
}

type fakeNames map[string]string

func (f fakeNames) DisplayNamesOf(_ context.Context, ids []string) (map[string]string, error) {
	names := map[string]string{}
	for _, id := range ids {
		if name, ok := f[id]; ok {
			names[id] = name
		}
	}
	return names, nil
}

// collector — сокет одного игрока: и прямые ответы, и рассылка стола.
type collector struct {
	direct chan Envelope
	raw    chan Envelope
}

func newCollector() *collector {
	return &collector{direct: make(chan Envelope, 16), raw: make(chan Envelope, 16)}
}

func (c *collector) client(userID string) Client {
	return Client{
		UserID: userID,
		Send:   func(envelope Envelope) { c.direct <- envelope },
		SendRaw: func(message []byte) {
			var envelope Envelope
			if err := json.Unmarshal(message, &envelope); err == nil {
				c.raw <- envelope
			}
		},
	}
}

func (c *collector) nextBroadcast(t *testing.T) Envelope {
	t.Helper()
	select {
	case envelope := <-c.raw:
		return envelope
	case <-time.After(2 * time.Second):
		t.Fatal("рассылка стола не пришла")
		return Envelope{}
	}
}

func (c *collector) nextDirect(t *testing.T) Envelope {
	t.Helper()
	select {
	case envelope := <-c.direct:
		return envelope
	case <-time.After(2 * time.Second):
		t.Fatal("прямой ответ не пришёл")
		return Envelope{}
	}
}

func lobbyFixture(t *testing.T) (LobbyRouter, *fakeLobby, *TableRegistry) {
	t.Helper()
	lobby := &fakeLobby{seat: repository.TablePlayer{TableID: tableID, UserID: "user-a",
		SeatNo: 2, State: repository.SeatJoined}}
	registry := NewTableRegistry(context.Background(), nil)
	t.Cleanup(registry.CloseAll)
	router := LobbyRouter{Lobby: lobby, Names: fakeNames{"user-a": "Аида"}, Registry: registry}
	return router, lobby, registry
}

const tableID = "11111111-1111-1111-1111-111111111111"

func envelopeOf(commandType string, payload string) Envelope {
	id, table := "cmd-1", tableID
	envelope := Envelope{V: ProtocolVersion, ID: &id, Type: commandType, TableID: &table}
	if payload != "" {
		envelope.Payload = json.RawMessage(payload)
	}
	return envelope
}

func TestJoinSubscribesAndTellsTheTable(t *testing.T) {
	router, _, registry := lobbyFixture(t)
	socket := newCollector()

	router.Handle(context.Background(), envelopeOf("TABLE_JOIN", ""), socket.client("user-a"))

	event := socket.nextBroadcast(t)
	if event.Type != "PLAYER_JOINED" {
		t.Fatalf("разослали %q, ждали PLAYER_JOINED", event.Type)
	}
	var payload map[string]any
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["userId"] != "user-a" || payload["displayName"] != "Аида" {
		t.Fatalf("тело события неполное: %v", payload)
	}
	// ⚠️ seatNo и ready есть только там, где место известно.
	if payload["seatNo"] != float64(2) || payload["ready"] != false {
		t.Fatalf("место в событии не то: %v", payload)
	}
	if registry.Size() != 1 {
		t.Fatalf("стол не поднят: %d", registry.Size())
	}
}

// ⭐ Уходящий получает СВОЁ событие: рассылка идёт до отписки. Наоборот — и он встал бы,
// не увидев подтверждения.
func TestLeaveTellsTheTableBeforeUnsubscribing(t *testing.T) {
	router, _, registry := lobbyFixture(t)
	socket := newCollector()
	router.Handle(context.Background(), envelopeOf("TABLE_JOIN", ""), socket.client("user-a"))
	socket.nextBroadcast(t)

	router.Handle(context.Background(), envelopeOf("TABLE_LEAVE", ""), socket.client("user-a"))

	event := socket.nextBroadcast(t)
	if event.Type != "PLAYER_LEFT" {
		t.Fatalf("разослали %q, ждали PLAYER_LEFT", event.Type)
	}
	var payload map[string]any
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if _, hasSeat := payload["seatNo"]; hasSeat {
		t.Fatalf("у PLAYER_LEFT появилось место: %v", payload)
	}
	waitFor(t, func() bool { return registry.Size() == 0 }, "стол не выгрузился после ухода последнего")
}

func TestReadyDefaultsToTrueWhenPayloadIsMissing(t *testing.T) {
	router, lobby, _ := lobbyFixture(t)
	socket := newCollector()
	router.Handle(context.Background(), envelopeOf("TABLE_JOIN", ""), socket.client("user-a"))
	socket.nextBroadcast(t)

	router.Handle(context.Background(), envelopeOf("TABLE_READY", ""), socket.client("user-a"))

	event := socket.nextBroadcast(t)
	if event.Type != "PLAYER_READY" {
		t.Fatalf("разослали %q, ждали PLAYER_READY", event.Type)
	}
	if !lobby.lastReady {
		t.Fatalf("команда без тела прочитана как отказ от готовности")
	}
}

func TestReadyReadsTheFlagWhenItIsGiven(t *testing.T) {
	router, lobby, _ := lobbyFixture(t)
	socket := newCollector()
	router.Handle(context.Background(), envelopeOf("TABLE_JOIN", ""), socket.client("user-a"))
	socket.nextBroadcast(t)

	router.Handle(context.Background(), envelopeOf("TABLE_READY", `{"ready":false}`),
		socket.client("user-a"))

	socket.nextBroadcast(t)
	if lobby.lastReady {
		t.Fatalf("явное «не готов» прочитано как готовность")
	}
}

// Отказ сценария приходит ЛИЧНО отправителю и с тем же кодом, что у REST: одно и то же
// «стол полон» не должно называться по-разному в зависимости от пути.
func TestRefusalGoesToTheSenderWithTheRestCode(t *testing.T) {
	router, lobby, _ := lobbyFixture(t)
	lobby.joinErr = application.ErrTableFull
	socket := newCollector()

	router.Handle(context.Background(), envelopeOf("TABLE_JOIN", ""), socket.client("user-a"))

	event := socket.nextDirect(t)
	if event.Type != "ERROR" {
		t.Fatalf("ответили %q, ждали ERROR", event.Type)
	}
	var payload map[string]string
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["code"] != "TABLE_FULL" {
		t.Fatalf("код отказа %q, ждали TABLE_FULL", payload["code"])
	}
	if event.ID == nil || *event.ID != "cmd-1" {
		t.Fatalf("отказ пришёл без идентификатора команды: клиент не поймёт, на что он")
	}
}

// Обрыв связи: подписка снимается, остальным за столом сообщают, что игрока нет.
func TestDisconnectTellsTheTableThePlayerIsOffline(t *testing.T) {
	router, _, registry := lobbyFixture(t)
	leaving, staying := newCollector(), newCollector()
	router.Handle(context.Background(), envelopeOf("TABLE_JOIN", ""), leaving.client("user-a"))
	leaving.nextBroadcast(t)
	registry.Subscribe(registry.RuntimeFor(tableID), staying.client("user-b"))

	router.Disconnect(context.Background(), tableID, leaving.client("user-a"))

	event := staying.nextBroadcast(t)
	if event.Type != "PLAYER_OFFLINE" {
		t.Fatalf("оставшимся пришло %q, ждали PLAYER_OFFLINE", event.Type)
	}
}

func waitFor(t *testing.T, condition func() bool, message string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal(message)
}
