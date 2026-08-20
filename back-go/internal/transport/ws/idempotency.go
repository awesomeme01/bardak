package ws

import "sync"

// rememberedCommands — сколько идентификаторов команд помнит стол.
//
// ⚠️ Окно конечное: помнить всё за матч — это память, которая растёт вместе с партией.
// Двухсот хватает с запасом: клиент переотправляет последнюю команду, а не двухсотую.
const rememberedCommands = 200

// CommandMemory — окно применённых команд для идемпотентности.
//
// ⭐ Клиент переотправляет команду после обрыва: он не знает, дошла ли она. Без памяти
// повтор применялся бы дважды — то есть карта уходила бы со стола дважды.
//
// ⚠️ Отклонённая команда сюда НЕ попадает. Иначе повтор отклонённого хода возвращал бы
// снимок состояния вместо причины отказа, и игрок так и не узнал бы, почему ход не прошёл.
type CommandMemory struct {
	mu    sync.Mutex
	seen  map[string]struct{}
	order []string
}

// NewCommandMemory заводит окно.
func NewCommandMemory() *CommandMemory {
	return &CommandMemory{seen: make(map[string]struct{}, rememberedCommands)}
}

// AlreadyApplied — команда с таким идентификатором уже применялась.
func (m *CommandMemory) AlreadyApplied(id string) bool {
	if id == "" {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.seen[id]
	return ok
}

// Remember отмечает команду применённой.
func (m *CommandMemory) Remember(id string) {
	if id == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.seen[id]; ok {
		return
	}
	m.seen[id] = struct{}{}
	m.order = append(m.order, id)

	// Окно скользит: самый старый забывается.
	if len(m.order) > rememberedCommands {
		oldest := m.order[0]
		m.order = m.order[1:]
		delete(m.seen, oldest)
	}
}
