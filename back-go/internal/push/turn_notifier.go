package push

import (
	"log/slog"
	"sync"
	"time"
)

// defaultQuietFor — окно тишины по умолчанию, как в Java (`bardak.push.quiet-for: 2m`).
const defaultQuietFor = 2 * time.Minute

// Notifications — что нужно зову к столу от отправителя.
//
// ⭐ Интерфейс на стороне потребителя: зову хватает трёх методов, и подставить в тесте
// счётчик вместо настоящего push-сервиса можно без единого сетевого вызова.
type Notifications interface {
	Enabled() bool
	NotifyTurn(userID, tableName, tableID string)
	NotifyPaused(userID, tableName, tableID string, secondsLeft int64)
}

// ⭐ Проверка сборкой: отправитель подходит зову.
var _ Notifications = (*Sender)(nil)

// TurnNotifier — кому и когда звонить «твой ход».
//
// ⭐ Уведомление уходит ТОЛЬКО тому, кого нет за столом. Игроку с открытой вкладкой
// звонить не нужно — он и так видит ход; звонок в этом случае не помогает, а раздражает
// и быстро приводит к тому, что уведомления отключают целиком.
//
// ⭐ Второе ограничение — тишина после звонка. Ход может вернуться к игроку через
// несколько секунд (отбился, подкинули, снова его очередь), и без паузы партия
// превратилась бы в очередь звонков. Один звонок за окно тишины на игрока.
type TurnNotifier struct {
	sender   Notifications
	quietFor time.Duration
	now      func() time.Time
	log      *slog.Logger

	// mu защищает lastNotified: зовут с goroutine РАЗНЫХ столов, а карта одна на узел.
	mu sync.Mutex
	// lastNotified — когда последний раз звонили. Ключ — игрок: окно тишины персональное.
	lastNotified map[string]time.Time
}

// NewTurnNotifier собирает зов к столу.
func NewTurnNotifier(sender Notifications, quietFor time.Duration, now func() time.Time,
	log *slog.Logger) *TurnNotifier {
	if quietFor <= 0 {
		quietFor = defaultQuietFor
	}
	if now == nil {
		now = time.Now
	}
	return &TurnNotifier{
		sender: sender, quietFor: quietFor, now: now, log: log,
		lastNotified: map[string]time.Time{},
	}
}

// TurnOf — ход перешёл к игроку.
//
// present — есть ли игрок за столом прямо сейчас.
//
// ⭐ Имя стола берётся ФУНКЦИЕЙ, а не значением: в Java оно вычисляется всегда, то есть
// запросом в базу на каждый переход хода — даже когда звонить некому. Здесь тот же
// запрос делается только перед настоящей отправкой. Поведение то же, лишнего похода
// в базу на горячем пути нет.
func (n *TurnNotifier) TurnOf(userID, tableID string, present bool, nameOf func() string) {
	if n == nil || !n.sender.Enabled() || present {
		return
	}
	if !n.rememberIfQuiet(userID) {
		return
	}
	if n.log != nil {
		n.log.Debug("зову игрока к столу", "user", userID, "table", tableID)
	}
	n.sender.NotifyTurn(userID, nameOrEmpty(nameOf), tableID)
}

// PausedFor — игрок пропал, и матч встал из-за него на паузу.
//
// ⭐ Окно тишины здесь НЕ применяется: пауза случается редко, а цена молчания —
// отменённый матч у всех за столом.
func (n *TurnNotifier) PausedFor(userID, tableID string, secondsLeft int64,
	nameOf func() string) {
	if n == nil || !n.sender.Enabled() {
		return
	}
	n.remember(userID)
	if n.log != nil {
		n.log.Debug("зову игрока обратно к столу", "user", userID, "table", tableID)
	}
	n.sender.NotifyPaused(userID, nameOrEmpty(nameOf), tableID, secondsLeft)
}

// Present — игрок вернулся за стол: следующий его ход снова достоин звонка.
func (n *TurnNotifier) Present(userID string) {
	if n == nil {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	delete(n.lastNotified, userID)
}

// rememberIfQuiet отмечает звонок, если окно тишины прошло.
func (n *TurnNotifier) rememberIfQuiet(userID string) bool {
	now := n.now()

	n.mu.Lock()
	defer n.mu.Unlock()

	if last, called := n.lastNotified[userID]; called && last.Add(n.quietFor).After(now) {
		return false
	}
	n.pruneStale(now)
	n.lastNotified[userID] = now
	return true
}

func (n *TurnNotifier) remember(userID string) {
	now := n.now()

	n.mu.Lock()
	defer n.mu.Unlock()

	n.pruneStale(now)
	n.lastNotified[userID] = now
}

// pruneStale выбрасывает отметки, которые уже никого не заглушают.
//
// ⚠️ В Java эта карта не чистится никогда: отметка остаётся навсегда, если игрок
// не вернулся за стол. Разница невидима снаружи — просроченная отметка и так никого
// не заглушает, — зато карта не растёт по одной записи на каждого игрока за всё время
// жизни узла. Вызывается под уже взятым замком.
func (n *TurnNotifier) pruneStale(now time.Time) {
	for userID, last := range n.lastNotified {
		if !last.Add(n.quietFor).After(now) {
			delete(n.lastNotified, userID)
		}
	}
}

// nameOrEmpty — имя стола, если его вообще удалось узнать.
//
// ⭐ Неизвестное имя не повод молчать: текст без имени стола хуже, чем с именем,
// но несравнимо лучше, чем несостоявшийся звонок (см. turnPayload).
func nameOrEmpty(nameOf func() string) string {
	if nameOf == nil {
		return ""
	}
	return nameOf()
}
