package ws

import (
	"encoding/json"
	"testing"
)

// Правила пропуска полей — самая молчаливая часть контракта: ошибка тут не роняет
// ничего, а клиент просто перестаёт понимать сервер.

func fields(t *testing.T, envelope Envelope) map[string]json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// ⚠️ PONG не несёт payload — ключа нет ВОВСЕ, а не null.
func TestPongHasNoPayloadKey(t *testing.T) {
	got := fields(t, Event("PONG", StringPtr("c-7"), StringPtr("t-1"), nil))

	if _, exists := got["payload"]; exists {
		t.Error("у PONG не должно быть ключа payload вообще")
	}
	for _, must := range []string{"v", "id", "type", "tableId", "ts"} {
		if _, exists := got[must]; !exists {
			t.Errorf("у PONG пропало поле %s", must)
		}
	}
}

// ⚠️ У событий сервера id пуст — и тогда ключа нет, а не null.
func TestEmptyOptionalFieldsDisappear(t *testing.T) {
	got := fields(t, Event("CONNECTED", nil, nil, map[string]string{"userId": "u"}))

	for _, gone := range []string{"id", "tableId", "seq"} {
		if _, exists := got[gone]; exists {
			t.Errorf("пустое поле %s должно ИСЧЕЗАТЬ, а не приходить как null", gone)
		}
	}
	if _, exists := got["v"]; !exists {
		t.Error("версия протокола обязана быть всегда")
	}
}

// ⭐ seq есть только у игровых событий — по нему клиент их и отличает.
func TestSeqOnlyOnGameEvents(t *testing.T) {
	game := fields(t, GameEvent("CARD_ATTACKED", "t-1", 42, map[string]int{"seatNo": 0}))
	if _, exists := game["seq"]; !exists {
		t.Error("у игрового события обязан быть seq")
	}

	// ⚠️ Ноль — законный номер, и он НЕ должен исчезнуть: в Java стоит NON_NULL,
	// а не NON_DEFAULT.
	zero := fields(t, GameEvent("DEAL_STARTED", "t-1", 0, nil))
	if _, exists := zero["seq"]; !exists {
		t.Error("seq = 0 исчез — это NON_NULL, а не NON_DEFAULT")
	}

	plain := fields(t, Event("STATE_SYNC", nil, StringPtr("t-1"), map[string]int{"mySeat": 0}))
	if _, exists := plain["seq"]; exists {
		t.Error("у снимка состояния seq быть не должно")
	}
}

// ⚠️ Ноль и false внутри payload тоже обязаны сохраняться: mySeat: 0 — законное место.
func TestZeroValuesSurviveInPayload(t *testing.T) {
	type view struct {
		MySeat int  `json:"mySeat"`
		Passed bool `json:"passed"`
	}
	got := fields(t, Event("STATE_SYNC", nil, StringPtr("t-1"), view{MySeat: 0, Passed: false}))

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(got["payload"], &payload); err != nil {
		t.Fatal(err)
	}
	if _, exists := payload["mySeat"]; !exists {
		t.Error("mySeat = 0 пропал — нулевое место законно, и оно первое за столом")
	}
	if _, exists := payload["passed"]; !exists {
		t.Error("passed = false пропал")
	}
}

// Неизвестные поля входящего конверта игнорируются молча: клиент вправе прислать лишнее.
func TestUnknownIncomingFieldsAreIgnored(t *testing.T) {
	raw := []byte(`{"v":1,"id":"c-1","type":"PASS","tableId":"t-1","будущее":"поле","x":42}`)

	envelope, err := ParseEnvelope(raw)
	if err != nil {
		t.Fatalf("лишнее поле не должно ломать разбор: %v", err)
	}
	if envelope.Type != "PASS" || envelope.ID == nil || *envelope.ID != "c-1" {
		t.Error("конверт разобран неверно")
	}
}

func TestBrokenEnvelopeIsRejected(t *testing.T) {
	if _, err := ParseEnvelope([]byte("{это не json")); err == nil {
		t.Error("битый конверт должен отвергаться")
	}
}

func TestErrorEventCarriesCodeAndCommandID(t *testing.T) {
	got := fields(t, ErrorEvent(StringPtr("c-9"), StringPtr("t-1"), "NOT_YOUR_TURN", "Не твой ход"))

	if string(got["id"]) != `"c-9"` {
		t.Error("ошибка обязана нести идентификатор вызвавшей команды — по нему клиент её сопоставит")
	}
	var payload map[string]string
	if err := json.Unmarshal(got["payload"], &payload); err != nil {
		t.Fatal(err)
	}
	if payload["code"] != "NOT_YOUR_TURN" {
		t.Errorf("код отказа %q — по нему экран объясняет игроку причину", payload["code"])
	}
}
