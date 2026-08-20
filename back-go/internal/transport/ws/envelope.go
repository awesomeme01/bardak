// Package ws — WebSocket: конверт протокола, жизненный цикл соединений и рассылка.
package ws

import (
	"encoding/json"
	"time"
)

// ProtocolVersion — текущая версия протокола. Ломающее изменение → +1.
const ProtocolVersion = 1

// Envelope — конверт сообщения, ОДИНАКОВЫЙ в обе стороны.
//
// ⚠️ Правила пропуска полей повторяют Java и различаются по типам:
//   - `v` и `ts` у сервера есть всегда — обычные значения;
//   - `id`, `tableId`, `seq` бывают пустыми и выпадают из JSON целиком, а не приходят
//     как null — поэтому указатели с omitempty;
//   - `payload` — сырой JSON: у PONG его нет вовсе, а у ECHO он бывает явным null.
//
// ⚠️ Числа 0 и false НЕ выпадают: в Java стоит NON_NULL, а не NON_DEFAULT. Поставить
// omitempty на int означало бы потерять `mySeat: 0` и `passed: false`.
type Envelope struct {
	V       int             `json:"v"`
	ID      *string         `json:"id,omitempty"`
	Type    string          `json:"type"`
	TableID *string         `json:"tableId,omitempty"`
	Seq     *int            `json:"seq,omitempty"`
	TS      int64           `json:"ts"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// Event собирает событие сервера.
//
// `commandID` пуст у всех событий, кроме ERROR, PONG и ECHO, где он равен идентификатору
// вызвавшей команды.
func Event(eventType string, commandID, tableID *string, payload any) Envelope {
	envelope := Envelope{
		V:       ProtocolVersion,
		ID:      commandID,
		Type:    eventType,
		TableID: tableID,
		TS:      time.Now().UnixMilli(),
	}
	if payload != nil {
		if raw, err := json.Marshal(payload); err == nil {
			envelope.Payload = raw
		}
	}
	return envelope
}

// GameEvent — событие матча со сквозным номером.
//
// ⭐ `seq` есть ТОЛЬКО у игровых событий. У снимка состояния, у событий лобби и у всех
// служебных его нет — клиент отличает их именно по этому.
func GameEvent(eventType string, tableID string, seq int, payload any) Envelope {
	envelope := Event(eventType, nil, &tableID, payload)
	envelope.Seq = &seq
	return envelope
}

// ErrorEvent — отказ в ответ на команду.
func ErrorEvent(commandID, tableID *string, code, message string) Envelope {
	return Event("ERROR", commandID, tableID, map[string]string{
		"code":    code,
		"message": message,
	})
}

// ParseEnvelope разбирает входящее сообщение.
//
// ⚠️ Неизвестные поля молча игнорируются — как у Jackson с FAIL_ON_UNKNOWN_PROPERTIES=false.
// Клиент вправе прислать лишнее, и это не повод рвать соединение.
func ParseEnvelope(raw []byte) (Envelope, error) {
	var envelope Envelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return Envelope{}, err
	}
	return envelope, nil
}

// StringPtr — короткая обёртка: указатели на строки нужны здесь на каждом шагу.
func StringPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
