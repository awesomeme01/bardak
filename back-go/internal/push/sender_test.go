package push

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
)

// Тексты уведомлений и поведение без настроенных ключей.
//
// ⭐ Тексты проверяются дословно, потому что их показывает service worker игрока:
// «улучшенная» формулировка — это заметное изменение поведения, которого никто не просил.

func payloadText(payload map[string]string) string {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func TestSenderNamesTheTableWhenTheTableHasAName(t *testing.T) {
	text := payloadText(turnPayload("Вечерний", "table-1"))

	for _, want := range []string{"Твой ход", "Вечерний", "table-1"} {
		if !strings.Contains(text, want) {
			t.Fatalf("в уведомлении нет %q: %s", want, text)
		}
	}
}

func TestSenderFallsBackToAGenericTextWhenTheTableHasNoName(t *testing.T) {
	payload := turnPayload("  ", "")

	// ⭐ Пустое название лучше заменить фразой, чем показать «Стол «» ждёт».
	if payload["body"] != "За столом ждут тебя" {
		t.Fatalf("текст без имени стола: %q", payload["body"])
	}
	// ⚠️ Имя из одних пробелов — это ОТСУТСТВИЕ имени: Java здесь зовёт isBlank,
	// и проверка на пустую строку разошлась бы с ней ровно на таком столе.
	if _, has := payload["tableId"]; has {
		t.Fatalf("неизвестный стол попал в уведомление: %+v", payload)
	}
}

func TestSenderSaysHowLongIsLeftWhenTheMatchIsPaused(t *testing.T) {
	text := payloadText(pausedPayload("Вечерний", "table-1", 60))

	// Человеку важно не «матч на паузе», а сколько у него есть, чтобы вернуться.
	for _, want := range []string{"Тебя ждут", "Вечерний", "60"} {
		if !strings.Contains(text, want) {
			t.Fatalf("в уведомлении о паузе нет %q: %s", want, text)
		}
	}
}

func TestSenderNamesTheCallerWhenAFriendInvites(t *testing.T) {
	text := payloadText(invitePayload("Аскар", "Вечерний", "table-1"))

	for _, want := range []string{"Аскар зовёт за стол", "Вечерний", "table-1"} {
		if !strings.Contains(text, want) {
			t.Fatalf("в приглашении нет %q: %s", want, text)
		}
	}
}

func TestSenderStaysDisabledWhenVapidKeysAreNotConfigured(t *testing.T) {
	sender := NewSender(nil, Options{}, time.Now, nil)
	defer sender.Stop()

	// Отсутствие ключей — не поломка: локально играют с открытой вкладкой.
	if sender.Enabled() {
		t.Fatalf("отправитель включился без ключей VAPID")
	}
}

func TestSenderSendsNothingWhenDisabled(t *testing.T) {
	sender := NewSender(nil, Options{}, time.Now, nil)
	defer sender.Stop()

	// ⚠️ Подписки читает база: выключенный отправитель, дошедший до неё, упал бы здесь
	// на nil-хранилище. Молчать он обязан РАНЬШЕ, чем куда-то пойдёт.
	sender.NotifyTurn("user-1", "Вечерний", "table-1")
	sender.NotifyPaused("user-1", "Вечерний", "table-1", 60)
	sender.NotifyInvite("user-1", "Аскар", "Вечерний", "table-1")
}

func TestSenderStopIsSafeToCallTwice(t *testing.T) {
	sender := NewSender(nil, Options{}, time.Now, nil)

	sender.Stop()
	sender.Stop()
}

func TestSenderChecksKeyShapeAtStartup(t *testing.T) {
	// ⚠️ Порядок возврата у библиотеки — СНАЧАЛА ЗАКРЫТЫЙ ключ. Перепутать местами
	// легко, а сервер с переставленной парой поднимется молча и не отправит ничего.
	private, public, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		t.Fatalf("не сгенерировал пару VAPID: %v", err)
	}

	if err := CheckKeys(Options{PublicKey: public, PrivateKey: private}); err != nil {
		t.Fatalf("настоящая пара VAPID не принята: %v", err)
	}
	if err := CheckKeys(Options{}); err != nil {
		t.Fatalf("пустые ключи — это выключенные уведомления, а не поломка: %v", err)
	}
	// ⭐ Кривые ключи — поломка, и узнать о ней надо ПРИ СТАРТЕ: иначе сервер поднимется
	// здоровым, а уведомления окажутся мёртвыми ровно тогда, когда понадобятся.
	if err := CheckKeys(Options{PublicKey: "not-base64!", PrivateKey: private}); err == nil {
		t.Fatalf("кривой открытый ключ принят")
	}
	if err := CheckKeys(Options{PublicKey: public, PrivateKey: "not-base64!"}); err == nil {
		t.Fatalf("кривой закрытый ключ принят")
	}
}
