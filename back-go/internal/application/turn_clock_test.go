package application

import (
	"sync/atomic"
	"testing"
	"time"
)

// Таймер хода (§5.2).
//
// ⭐ Главное здесь — что пауза ОСТАНАВЛИВАЕТ отсчёт, а не сбрасывает его: игрок,
// у которого оставалось чуть-чуть, после возвращения получает своё «чуть-чуть»,
// а не полный ход заново. Перепутать эти два поведения легко, а заметит игрок.

// waitFor ждёт срабатывания не дольше срока. Возвращает, сработало ли.
func waitFor(fired *atomic.Bool, limit time.Duration) bool {
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if fired.Load() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return fired.Load()
}

func TestClockFiresWhenTheTurnRunsOut(t *testing.T) {
	clock := NewTurnClock()
	var fired atomic.Bool

	clock.Start("table-1", 100*time.Millisecond, func() { fired.Store(true) })

	if !waitFor(&fired, 2*time.Second) {
		t.Fatalf("часы не сработали: ход остался без присмотра")
	}
}

func TestClockFiresNothingWhenCancelled(t *testing.T) {
	clock := NewTurnClock()
	var fired atomic.Bool
	clock.Start("table-1", 150*time.Millisecond, func() { fired.Store(true) })

	clock.Cancel("table-1")

	if waitFor(&fired, 400*time.Millisecond) {
		t.Fatalf("отменённые часы всё равно сходили за игрока")
	}
	if _, running := clock.Remaining("table-1"); running {
		t.Fatalf("отменённые часы всё ещё показывают остаток")
	}
}

// ⭐ То самое различие между «остановить» и «перезапустить»: остаток обязан пережить паузу.
func TestClockKeepsTheRemainderAcrossPause(t *testing.T) {
	clock := NewTurnClock()
	var fired atomic.Bool
	clock.Start("table-1", 600*time.Millisecond, func() { fired.Store(true) })
	time.Sleep(400 * time.Millisecond)

	left := clock.Pause("table-1")

	if left <= 0 || left >= 400*time.Millisecond {
		t.Fatalf("остаток %v: ждали меньше 400 мс и больше нуля", left)
	}
	if waitFor(&fired, 400*time.Millisecond) {
		t.Fatalf("на паузе таймер сработал — матч ждёт вернувшегося, а не идёт")
	}
	if _, running := clock.Remaining("table-1"); running {
		t.Fatalf("на паузе часы показывают остаток: клиент рисовал бы бегущий отсчёт")
	}

	clock.Resume("table-1")

	// Остаток был около 200 мс — полного хода заново не начинается.
	if !waitFor(&fired, 500*time.Millisecond) {
		t.Fatalf("после возвращения часы не досчитали остаток")
	}
}

func TestClockReplacesThePreviousTimerWhenANewTurnStarts(t *testing.T) {
	clock := NewTurnClock()
	var first, second atomic.Bool

	clock.Start("table-1", 100*time.Millisecond, func() { first.Store(true) })
	clock.Start("table-1", 300*time.Millisecond, func() { second.Store(true) })

	if !waitFor(&second, 2*time.Second) {
		t.Fatalf("новые часы не сработали")
	}
	if first.Load() {
		t.Fatalf("старый таймер обязан быть отменён: сервер сходил бы дважды за один ход")
	}
}

// ⚠️ Часы хода и ожидание вернувшегося — РАЗНЫЕ таймеры: первый на паузе стоит,
// второй в это время идёт. Свести их в один значит либо не отменять матч никогда,
// либо отбирать ход у того, кого за столом нет.
func TestAbortTimerRunsWhileTheTurnClockIsPaused(t *testing.T) {
	clock := NewTurnClock()
	var turn, aborted atomic.Bool
	clock.Start("table-1", 200*time.Millisecond, func() { turn.Store(true) })
	clock.Pause("table-1")

	clock.ScheduleAbort("table-1", 150*time.Millisecond, func() { aborted.Store(true) })

	if !waitFor(&aborted, 2*time.Second) {
		t.Fatalf("матч не отменился: пропавшего ждали бы вечно")
	}
	if turn.Load() {
		t.Fatalf("часы хода шли на паузе")
	}
}

func TestAbortTimerStopsWhenThePlayerComesBack(t *testing.T) {
	clock := NewTurnClock()
	var aborted atomic.Bool
	clock.ScheduleAbort("table-1", 150*time.Millisecond, func() { aborted.Store(true) })

	clock.CancelAbort("table-1")

	if waitFor(&aborted, 400*time.Millisecond) {
		t.Fatalf("матч отменился, хотя игрок вернулся")
	}
}

// Пауза без идущих часов — ноль, а не отрицательное «сколько-то»: рассылка кладёт это
// число в MATCH_PAUSED, и отрицательный остаток клиент нарисовал бы как есть.
func TestPauseOfIdleClockGivesZero(t *testing.T) {
	clock := NewTurnClock()

	if left := clock.Pause("table-1"); left != 0 {
		t.Fatalf("остаток стоящих часов %v, ждали ноль", left)
	}
	clock.Resume("table-1") // и продолжать нечего — молча
}
