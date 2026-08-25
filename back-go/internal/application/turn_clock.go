package application

import (
	"sync"
	"time"
)

// TurnClock — таймеры хода и ожидания вернувшегося игрока.
//
// ⭐ Таймер при отключении ОСТАНАВЛИВАЕТСЯ, а не перезапускается (§5.2): игрок, у которого
// оставалось три секунды, после возвращения получает свои три секунды, а не полные
// тридцать. Приостанавливаемый таймер — не то же самое, что перезапускаемый, и правила
// требуют именно первое.
//
// ⚠️ Само срабатывание НИЧЕГО игрового не делает: оно кладёт работу в очередь стола
// (ADR-007). Иначе автодействие пришло бы с чужой goroutine и могло бы пересечься
// с настоящим ходом игрока, успевшего в последнюю секунду.
type TurnClock struct {
	mu      sync.Mutex
	pending map[string]*pendingTimer
	paused  map[string]pausedTimer
	aborts  map[string]*time.Timer
}

type pendingTimer struct {
	timer    *time.Timer
	deadline time.Time
	onExpiry func()
}

type pausedTimer struct {
	remaining time.Duration
	onExpiry  func()
}

// NewTurnClock собирает часы.
func NewTurnClock() *TurnClock {
	return &TurnClock{
		pending: map[string]*pendingTimer{},
		paused:  map[string]pausedTimer{},
		aborts:  map[string]*time.Timer{},
	}
}

// Start запускает отсчёт заново. Предыдущий таймер стола отменяется.
func (c *TurnClock) Start(tableID string, timeout time.Duration, onExpiry func()) {
	c.Cancel(tableID)
	c.schedule(tableID, timeout, onExpiry)
}

// Pause останавливает отсчёт и запоминает остаток. Возвращает, сколько оставалось.
func (c *TurnClock) Pause(tableID string) time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()

	current, ok := c.pending[tableID]
	if !ok {
		return 0
	}
	delete(c.pending, tableID)
	current.timer.Stop()

	left := time.Until(current.deadline)
	if left < 0 {
		left = 0
	}
	c.paused[tableID] = pausedTimer{remaining: left, onExpiry: current.onExpiry}
	return left
}

// Remaining — сколько осталось на ход. Второе значение false — часы не идут: либо ждать
// некого, либо матч на паузе.
//
// ⭐ Остаток снимается с самого задания, а не считается по своей копии времени: два
// счётчика одного и того же неизбежно разъезжаются, и клиент увидел бы не то, по чему
// сервер на самом деле сходит за игрока.
func (c *TurnClock) Remaining(tableID string) (time.Duration, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	current, ok := c.pending[tableID]
	if !ok {
		return 0, false
	}
	left := time.Until(current.deadline)
	if left < 0 {
		left = 0
	}
	return left, true
}

// Resume продолжает с остатка. Паузы не было — ничего не делает.
func (c *TurnClock) Resume(tableID string) {
	c.mu.Lock()
	stopped, ok := c.paused[tableID]
	delete(c.paused, tableID)
	c.mu.Unlock()

	if !ok {
		return
	}
	c.schedule(tableID, stopped.remaining, stopped.onExpiry)
}

// Cancel снимает часы стола вместе с запомненной паузой.
func (c *TurnClock) Cancel(tableID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if current, ok := c.pending[tableID]; ok {
		current.timer.Stop()
		delete(c.pending, tableID)
	}
	delete(c.paused, tableID)
}

// ScheduleAbort — отдельный таймер: сколько ждём вернувшегося, прежде чем отменить матч (§5.3).
//
// ⚠️ Он ОТДЕЛЬНЫЙ от часов хода не для удобства: часы хода на паузе стоят, а этот идёт —
// именно он и отмеряет паузу. Свести их в один значило бы либо не отменять матч никогда,
// либо продолжать отбирать ход у того, кого нет за столом.
func (c *TurnClock) ScheduleAbort(tableID string, grace time.Duration, onExpiry func()) {
	c.CancelAbort(tableID)

	c.mu.Lock()
	defer c.mu.Unlock()
	c.aborts[tableID] = time.AfterFunc(grace, func() {
		c.mu.Lock()
		delete(c.aborts, tableID)
		c.mu.Unlock()
		onExpiry()
	})
}

// CancelAbort — игрок вернулся, ждать больше нечего.
func (c *TurnClock) CancelAbort(tableID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if timer, ok := c.aborts[tableID]; ok {
		timer.Stop()
		delete(c.aborts, tableID)
	}
}

// StopAll гасит все таймеры: сервер завершается, и досчитывать некому.
func (c *TurnClock) StopAll() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for tableID, current := range c.pending {
		current.timer.Stop()
		delete(c.pending, tableID)
	}
	for tableID, timer := range c.aborts {
		timer.Stop()
		delete(c.aborts, tableID)
	}
	c.paused = map[string]pausedTimer{}
}

func (c *TurnClock) schedule(tableID string, timeout time.Duration, onExpiry func()) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry := &pendingTimer{deadline: time.Now().Add(timeout), onExpiry: onExpiry}
	entry.timer = time.AfterFunc(timeout, func() {
		// ⚠️ Сработавший таймер убирает СЕБЯ, а не «таймер этого стола»: за время
		// ожидания стол мог получить новые часы, и снятие чужих оставило бы ход без
		// присмотра навсегда.
		c.mu.Lock()
		if current, ok := c.pending[tableID]; ok && current == entry {
			delete(c.pending, tableID)
		}
		c.mu.Unlock()
		onExpiry()
	})
	c.pending[tableID] = entry
}
