package push

import (
	"strings"
	"sync"
	"testing"
	"time"
)

// Кого зовут к столу и как часто.
//
// ⭐ Проверяется не «уведомление отправилось», а ДВА ограничения, ради которых зов
// вообще существует: игроку с открытой вкладкой не звонят, и звонок не повторяется,
// пока идёт окно тишины. Без первого уведомления раздражают, без второго быстрая
// партия превращается в очередь звонков — и в обоих случаях их отключают целиком.

// fakeSender — счётчик вместо push-сервиса.
type fakeSender struct {
	mu      sync.Mutex
	enabled bool
	turns   []call
	paused  []call
}

type call struct {
	userID    string
	tableName string
	tableID   string
	seconds   int64
}

func (f *fakeSender) Enabled() bool { return f.enabled }

func (f *fakeSender) NotifyTurn(userID, tableName, tableID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.turns = append(f.turns, call{userID: userID, tableName: tableName, tableID: tableID})
}

func (f *fakeSender) NotifyPaused(userID, tableName, tableID string, secondsLeft int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.paused = append(f.paused, call{userID: userID, tableName: tableName,
		tableID: tableID, seconds: secondsLeft})
}

func (f *fakeSender) turnCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.turns)
}

// movingClock — время, которым распоряжается тест.
type movingClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *movingClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *movingClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func notifierFor(sender *fakeSender) (*TurnNotifier, *movingClock) {
	clock := &movingClock{now: time.Date(2026, 8, 25, 20, 0, 0, 0, time.UTC)}
	return NewTurnNotifier(sender, 2*time.Minute, clock.Now, nil), clock
}

func nameIs(name string) func() string { return func() string { return name } }

func TestNotifierCallsThePlayerWhenTheirTurnCameAndTheyAreAway(t *testing.T) {
	sender := &fakeSender{enabled: true}
	notifier, _ := notifierFor(sender)

	notifier.TurnOf("user-1", "table-1", false, nameIs("Вечерний"))

	if sender.turnCount() != 1 {
		t.Fatalf("не позвал отсутствующего игрока: звонков %d", sender.turnCount())
	}
	if sender.turns[0].tableName != "Вечерний" || sender.turns[0].tableID != "table-1" {
		t.Fatalf("позвал не за тот стол: %+v", sender.turns[0])
	}
}

func TestNotifierStaysSilentWhenThePlayerIsAtTheTable(t *testing.T) {
	sender := &fakeSender{enabled: true}
	notifier, _ := notifierFor(sender)

	notifier.TurnOf("user-1", "table-1", true, nameIs("Вечерний"))

	if sender.turnCount() != 0 {
		t.Fatalf("позвонил тому, кто и так смотрит на стол")
	}
}

func TestNotifierAsksForTheTableNameOnlyWhenItCalls(t *testing.T) {
	sender := &fakeSender{enabled: true}
	notifier, _ := notifierFor(sender)
	asked := 0

	notifier.TurnOf("user-1", "table-1", true, func() string { asked++; return "Вечерний" })

	// ⭐ Имя стола — запрос в базу на goroutine стола. Тот, кто и так за столом,
	// не должен стоить этого запроса на каждом переходе хода.
	if asked != 0 {
		t.Fatalf("сходил за именем стола ради несостоявшегося звонка")
	}
}

func TestNotifierCallsOnlyOnceWhenTheTurnComesBackWithinTheQuietWindow(t *testing.T) {
	sender := &fakeSender{enabled: true}
	notifier, clock := notifierFor(sender)

	notifier.TurnOf("user-1", "table-1", false, nameIs("Вечерний"))
	clock.advance(30 * time.Second)
	notifier.TurnOf("user-1", "table-1", false, nameIs("Вечерний"))

	if sender.turnCount() != 1 {
		t.Fatalf("звонков %d вместо одного: окно тишины не держит", sender.turnCount())
	}
}

func TestNotifierCallsAgainWhenTheQuietWindowIsOver(t *testing.T) {
	sender := &fakeSender{enabled: true}
	notifier, clock := notifierFor(sender)

	notifier.TurnOf("user-1", "table-1", false, nameIs("Вечерний"))
	clock.advance(2*time.Minute + time.Second)
	notifier.TurnOf("user-1", "table-1", false, nameIs("Вечерний"))

	if sender.turnCount() != 2 {
		t.Fatalf("звонков %d вместо двух: окно тишины не кончилось", sender.turnCount())
	}
}

func TestNotifierCallsRightAwayWhenThePlayerCameBackAndLeftAgain(t *testing.T) {
	sender := &fakeSender{enabled: true}
	notifier, clock := notifierFor(sender)
	notifier.TurnOf("user-1", "table-1", false, nameIs("Вечерний"))

	notifier.Present("user-1")
	clock.advance(5 * time.Second)
	notifier.TurnOf("user-1", "table-1", false, nameIs("Вечерний"))

	// Вернулся и снова ушёл — это новый случай, а не продолжение старого.
	if sender.turnCount() != 2 {
		t.Fatalf("звонков %d вместо двух: возвращение не сбросило окно", sender.turnCount())
	}
}

func TestNotifierCallsThePlayerBackWhenTheMatchPausedBecauseOfThem(t *testing.T) {
	sender := &fakeSender{enabled: true}
	notifier, _ := notifierFor(sender)

	notifier.PausedFor("user-1", "table-1", 60, nameIs("Вечерний"))

	if len(sender.paused) != 1 || sender.paused[0].seconds != 60 {
		t.Fatalf("не позвал пропавшего обратно: %+v", sender.paused)
	}
}

func TestNotifierIgnoresTheQuietWindowWhenTheMatchIsOnPause(t *testing.T) {
	sender := &fakeSender{enabled: true}
	notifier, clock := notifierFor(sender)
	notifier.TurnOf("user-1", "table-1", false, nameIs("Вечерний"))

	clock.advance(time.Second)
	notifier.PausedFor("user-1", "table-1", 60, nameIs("Вечерний"))

	// ⭐ Цена молчания здесь — отменённый матч у всех за столом, а не лишний звонок.
	if len(sender.paused) != 1 {
		t.Fatalf("окно тишины заглушило зов с паузы: %+v", sender.paused)
	}
}

func TestNotifierSilencesTheNextTurnAfterCallingAboutThePause(t *testing.T) {
	sender := &fakeSender{enabled: true}
	notifier, clock := notifierFor(sender)

	notifier.PausedFor("user-1", "table-1", 60, nameIs("Вечерний"))
	clock.advance(time.Second)
	notifier.TurnOf("user-1", "table-1", false, nameIs("Вечерний"))

	// Пауза случается ровно тогда, когда ход и так у пропавшего: два звонка подряд
	// об одном и том же — верный способ попасть в «отключить уведомления».
	if sender.turnCount() != 0 {
		t.Fatalf("позвонил вторым звонком сразу после зова с паузы")
	}
}

func TestNotifierDoesNothingWhenPushIsNotConfigured(t *testing.T) {
	sender := &fakeSender{enabled: false}
	notifier, _ := notifierFor(sender)

	notifier.TurnOf("user-1", "table-1", false, nameIs("Вечерний"))
	notifier.PausedFor("user-1", "table-1", 60, nameIs("Вечерний"))

	// Отсутствие ключей — не поломка: локально играют с открытой вкладкой.
	if sender.turnCount() != 0 || len(sender.paused) != 0 {
		t.Fatalf("выключенный отправитель всё-таки звонил")
	}
}

func TestNotifierForgetsStaleMarksSoTheMapDoesNotGrow(t *testing.T) {
	sender := &fakeSender{enabled: true}
	notifier, clock := notifierFor(sender)
	for _, userID := range []string{"user-1", "user-2", "user-3"} {
		notifier.TurnOf(userID, "table-1", false, nameIs("Вечерний"))
	}

	clock.advance(2*time.Minute + time.Second)
	notifier.TurnOf("user-4", "table-1", false, nameIs("Вечерний"))

	// ⚠️ Отметка живёт ровно окно тишины: в Java она остаётся навсегда, если игрок
	// не вернулся за стол, и карта растёт по записи на каждого игрока за всё время.
	notifier.mu.Lock()
	defer notifier.mu.Unlock()
	if len(notifier.lastNotified) != 1 {
		t.Fatalf("просроченных отметок осталось %d", len(notifier.lastNotified))
	}
}

func TestNotifierSurvivesConcurrentTablesCallingAtOnce(t *testing.T) {
	sender := &fakeSender{enabled: true}
	notifier, _ := notifierFor(sender)
	var waiting sync.WaitGroup

	// Столы — независимые goroutine, а карта отметок одна на узел.
	for i := 0; i < 50; i++ {
		waiting.Add(1)
		go func(i int) {
			defer waiting.Done()
			notifier.TurnOf(strings.Repeat("u", i%7+1), "table-1", false, nameIs("Вечерний"))
			notifier.Present(strings.Repeat("u", i%7+1))
		}(i)
	}
	waiting.Wait()
}
