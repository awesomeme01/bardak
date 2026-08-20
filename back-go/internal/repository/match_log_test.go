package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Журнал матча на настоящем Postgres: половина поведения здесь принадлежит базе —
// уникальность номера события, каскады, значения по умолчанию.

func matchFixture(t *testing.T) (MatchLog, string, context.Context) {
	t.Helper()
	pool := testDB(t)
	ctx := context.Background()
	log := NewMatchLog(pool)

	// Матчу нужен стол, столу — хозяин: внешние ключи держит база.
	users := NewUsers(pool)
	hostID := uuid.NewString()
	if _, err := users.Insert(ctx, User{ID: hostID, Username: "log-" + hostID[:8],
		DisplayName: "Хозяин", PasswordHash: "hash"}); err != nil {
		t.Fatal(err)
	}
	tableID := uuid.NewString()
	if _, err := pool.Exec(ctx, `insert into game_tables
		(id, code, name, host_user_id, max_players, status, card_set_id, theme_id, rules_config, is_private)
		values ($1, $2, 'Журнал', $3, 3, 'IN_MATCH',
		        (select id from card_sets where is_default limit 1),
		        (select id from table_themes where is_default limit 1), '{}'::jsonb, false)`,
		tableID, "L"+hostID[:7], hostID); err != nil {
		t.Fatal(err)
	}

	match, err := log.StartMatch(ctx, uuid.NewString(), tableID, 3, 42, `{"dealSize":6}`)
	if err != nil {
		t.Fatal(err)
	}
	return log, match.ID, ctx
}

func TestMatchStartsInProgress(t *testing.T) {
	log, matchID, ctx := matchFixture(t)

	match, err := log.MatchByID(ctx, matchID)
	if err != nil {
		t.Fatal(err)
	}
	if match.Status != MatchInProgress {
		t.Errorf("статус %q, ждали IN_PROGRESS", match.Status)
	}
	if match.RngSeed != 42 {
		t.Errorf("seed %d — по нему воспроизводится матч", match.RngSeed)
	}
	if match.FinishedAt != nil {
		t.Error("у идущего матча не может быть времени окончания")
	}
}

// ⭐ По этому запросу матч поднимается после перезапуска сервера.
func TestActiveMatchIsFound(t *testing.T) {
	log, matchID, ctx := matchFixture(t)

	match, err := log.MatchByID(ctx, matchID)
	if err != nil {
		t.Fatal(err)
	}
	found, err := log.ActiveMatchFor(ctx, match.TableID)
	if err != nil {
		t.Fatalf("идущий матч не найден — после перезапуска стол бы не поднялся: %v", err)
	}
	if found.ID != matchID {
		t.Error("нашёлся чужой матч")
	}

	// Закрытый матч активным больше не считается.
	if err := log.Finish(ctx, matchID, MatchFinished, nil, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := log.ActiveMatchFor(ctx, match.TableID); !errors.Is(err, ErrNotFound) {
		t.Error("закрытый матч всё ещё числится идущим")
	}
}

func TestEventsAreAppendedAndRead(t *testing.T) {
	log, matchID, ctx := matchFixture(t)

	seat := 1
	events := []MatchEvent{
		{Type: "CARD_ATTACKED", ActorSeat: &seat, Payload: `{"cardCode":"A-spades"}`},
		{Type: "CARD_DEFENDED", ActorSeat: &seat, Payload: `{"cardCode":"6-clubs"}`},
	}

	last, err := log.Append(ctx, matchID, 1, 1, events)
	if err != nil {
		t.Fatal(err)
	}
	if last != 2 {
		t.Errorf("последний номер %d, ждали 2", last)
	}

	read, err := log.Since(ctx, matchID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(read) != 2 {
		t.Fatalf("прочитано %d событий, записано 2", len(read))
	}
	if read[0].Seq != 1 || read[0].Type != "CARD_ATTACKED" {
		t.Error("порядок событий нарушен — по нему воспроизводится партия")
	}

	// Чтение «с номера» отдаёт только новое: на этом держится RESYNC после обрыва.
	tail, err := log.Since(ctx, matchID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(tail) != 1 || tail[0].Seq != 2 {
		t.Error("хвост журнала прочитан неверно — переподключение получило бы лишнее")
	}
}

// ⚠️ Уникальность (match_id, seq) держит БАЗА: дыр и дублей при гонках быть не может.
func TestDuplicateSeqIsRefusedByDatabase(t *testing.T) {
	log, matchID, ctx := matchFixture(t)

	if _, err := log.Append(ctx, matchID, 1, 1,
		[]MatchEvent{{Type: "PASSED", Payload: `{}`}}); err != nil {
		t.Fatal(err)
	}
	if _, err := log.Append(ctx, matchID, 1, 1,
		[]MatchEvent{{Type: "PASSED", Payload: `{}`}}); err == nil {
		t.Error("повторный номер события принят — в журнале появился бы дубль")
	}
}

// ⚠️ Повтор номера снимка — не ошибка: он переписывается. Иначе сохранение падало бы
// после переподключения там, где состояние просто не изменилось.
func TestSnapshotIsOverwritten(t *testing.T) {
	log, matchID, ctx := matchFixture(t)

	if err := log.SaveSnapshot(ctx, matchID, 5, `{"phase":"IN_DEAL"}`); err != nil {
		t.Fatal(err)
	}
	if err := log.SaveSnapshot(ctx, matchID, 5, `{"phase":"MATCH_OVER"}`); err != nil {
		t.Fatalf("повтор номера снимка должен переписывать, а не падать: %v", err)
	}

	seq, state, err := log.LatestSnapshot(ctx, matchID)
	if err != nil {
		t.Fatal(err)
	}
	if seq != 5 || state == "" {
		t.Errorf("снимок прочитан неверно: seq=%d", seq)
	}
	if !contains(state, "MATCH_OVER") {
		t.Errorf("снимок не переписался: %s", state)
	}
}

func TestLatestSnapshotOfEmptyMatch(t *testing.T) {
	log, matchID, ctx := matchFixture(t)

	if _, _, err := log.LatestSnapshot(ctx, matchID); !errors.Is(err, ErrNotFound) {
		t.Errorf("у матча без снимков должно быть ErrNotFound, получили %v", err)
	}
}

// Отклонённая попытка — часть истории стола, хотя состояние не меняет.
func TestRejectedAttemptIsRecorded(t *testing.T) {
	log, matchID, ctx := matchFixture(t)

	if err := log.AppendRejected(ctx, matchID, 1, 1, 2, "PLAY_CARD", "NOT_YOUR_TURN"); err != nil {
		t.Fatal(err)
	}
	events, err := log.Since(ctx, matchID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Type != "ATTEMPT_REJECTED" {
		t.Fatal("отклонённая попытка не записана — разбор спорной партии был бы неполон")
	}
	if !contains(events[0].Payload, "NOT_YOUR_TURN") {
		t.Errorf("причина отказа потеряна: %s", events[0].Payload)
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
