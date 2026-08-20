package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync"
	"time"
)

// TicketTTL — сколько живёт одноразовый тикет.
const TicketTTL = 30 * time.Second

// Tickets — одноразовые тикеты для рукопожатия WebSocket.
//
// ⭐ Браузерный WebSocket не умеет слать заголовок Authorization. Варианты были: токен
// в query (утекает в логи прокси и живёт часами), subprotocol (костыльно) и одноразовый
// тикет. Выбран тикет: он живёт полминуты и сгорает при использовании, поэтому даже попав
// в лог, бесполезен.
//
// ⚠️ Хранилище в памяти процесса — то же, что в Java. Со вторым узлом рукопожатие
// сломалось бы, но второй узел отменён решением (ADR-061), и это осознанная плата.
type Tickets struct {
	mu     sync.Mutex
	issued map[string]ticket
	ttl    time.Duration
	now    func() time.Time
}

type ticket struct {
	userID    string
	expiresAt time.Time
}

// NewTickets собирает хранилище.
func NewTickets(ttl time.Duration, now func() time.Time) *Tickets {
	if now == nil {
		now = time.Now
	}
	if ttl <= 0 {
		ttl = TicketTTL
	}
	return &Tickets{issued: map[string]ticket{}, ttl: ttl, now: now}
}

// Issue выдаёт тикет игроку.
func (t *Tickets) Issue(userID string) (string, time.Duration, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", 0, fmt.Errorf("случайность для тикета: %w", err)
	}
	value := base64.RawURLEncoding.EncodeToString(buf)

	t.mu.Lock()
	defer t.mu.Unlock()
	t.sweep()
	t.issued[value] = ticket{userID: userID, expiresAt: t.now().Add(t.ttl)}
	return value, t.ttl, nil
}

// Redeem гасит тикет и возвращает владельца.
//
// ⚠️ Гашение АТОМАРНО с проверкой: иначе двое, предъявившие один тикет одновременно,
// оба прошли бы рукопожатие. Повторное предъявление невозможно по определению —
// в этом весь смысл одноразовости.
func (t *Tickets) Redeem(value string) (string, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	found, ok := t.issued[value]
	if !ok {
		return "", false
	}
	delete(t.issued, value)
	if !found.expiresAt.After(t.now()) {
		return "", false
	}
	return found.userID, true
}

// sweep выбрасывает протухшие. Вызывается под замком при выдаче: отдельный сборщик
// ради полуминутных записей — лишняя goroutine.
func (t *Tickets) sweep() {
	now := t.now()
	for value, issued := range t.issued {
		if !issued.expiresAt.After(now) {
			delete(t.issued, value)
		}
	}
}
