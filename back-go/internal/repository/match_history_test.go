package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// История матчей против НАСТОЯЩЕГО Postgres со схемой Java.
//
// ⚠️ Проверять это на подделке бессмысленно: половина поведения здесь принадлежит базе —
// jsonb-массивы, numeric с масштабом, nullable-итоги отменённого матча и сравнение
// состояния без учёта регистра.

// historyFixture — стол, матч и всё, что к нему привязано.
type historyFixture struct {
	MatchID string
	TableID string
	HostID  string
	GuestID string
	DealID  string
}

func TestHistoryMatchesOfReadsSummaryAndPlayers(t *testing.T) {
	pool := testDB(t)
	history := NewMatchHistory(pool)
	ctx := context.Background()

	fixture := historySeedFinishedMatch(t, pool)

	matches, err := history.MatchesOf(ctx, fixture.HostID, "")
	if err != nil {
		t.Fatalf("список матчей: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("ждали один матч, получили %d", len(matches))
	}
	match := matches[0]
	if match.Status != "FINISHED" {
		t.Errorf("состояние %q, ждали FINISHED", match.Status)
	}
	if match.FinishedAt == nil {
		t.Error("у завершённого матча обязано быть время окончания")
	}
	if match.AbortReason != nil {
		t.Errorf("у завершённого матча нет причины отмены, а пришла %q", *match.AbortReason)
	}

	players, err := history.PlayersOf(ctx, []string{fixture.MatchID})
	if err != nil {
		t.Fatalf("итоги игроков: %v", err)
	}
	seats := players[fixture.MatchID]
	if len(seats) != 2 {
		t.Fatalf("ждали два места, получили %d", len(seats))
	}
	if seats[0].DisplayName != "Хозяин" {
		t.Errorf("имя за столом %q, ждали «Хозяин» — оно приходит JOIN'ом с users", seats[0].DisplayName)
	}
	// ⚠️ Масштаб numeric(8,2) обязан дожить до ответа: через float64 «1012.50»
	// превратилось бы в «1012.5», и побайтовое сравнение с Java развалилось бы.
	if seats[0].RatingDelta == nil || *seats[0].RatingDelta != "12.50" {
		t.Errorf("дельта рейтинга %v, ждали строку 12.50 с масштабом из базы", seats[0].RatingDelta)
	}
	if seats[1].LossType == nil || *seats[1].LossType != "SUPER_MEGA_FAIL" {
		t.Errorf("степень проигрыша %v, ждали SUPER_MEGA_FAIL", seats[1].LossType)
	}
}

// ⚠️ Фильтр по состоянию сравнивается БЕЗ учёта регистра, а неизвестное значение даёт
// пустой список, а не ошибку — так в Java, и экран на это опирается.
func TestHistoryStatusFilterIgnoresCase(t *testing.T) {
	pool := testDB(t)
	history := NewMatchHistory(pool)
	ctx := context.Background()

	fixture := historySeedFinishedMatch(t, pool)

	for _, status := range []string{"FINISHED", "finished", "FiNiShEd"} {
		matches, err := history.MatchesOf(ctx, fixture.HostID, status)
		if err != nil {
			t.Fatalf("фильтр %q: %v", status, err)
		}
		if len(matches) != 1 {
			t.Errorf("фильтр %q дал %d матчей, ждали один", status, len(matches))
		}
	}

	for _, status := range []string{"ABORTED", "НЕТ-ТАКОГО"} {
		matches, err := history.MatchesOf(ctx, fixture.HostID, status)
		if err != nil {
			t.Fatalf("фильтр %q должен давать пустой список, а не ошибку: %v", status, err)
		}
		if len(matches) != 0 {
			t.Errorf("фильтр %q дал %d матчей, ждали ноль", status, len(matches))
		}
	}
}

func TestHistoryDealsAndSeatsUnpackJsonb(t *testing.T) {
	pool := testDB(t)
	history := NewMatchHistory(pool)
	ctx := context.Background()

	fixture := historySeedFinishedMatch(t, pool)

	deals, err := history.DealsOf(ctx, fixture.MatchID)
	if err != nil {
		t.Fatalf("раздачи: %v", err)
	}
	if len(deals) != 1 {
		t.Fatalf("ждали одну раздачу, получили %d", len(deals))
	}
	if deals[0].TrumpSuit == nil || *deals[0].TrumpSuit != "SPADES" {
		t.Errorf("козырь %v, ждали имя масти движка SPADES, а не значок", deals[0].TrumpSuit)
	}
	if len(deals[0].LastAttackCards) != 1 || deals[0].LastAttackCards[0] != "8-hearts" {
		t.Errorf("последняя атака %v, ждали [8-hearts]", deals[0].LastAttackCards)
	}

	seats, err := history.DealSeatsOf(ctx, fixture.MatchID)
	if err != nil {
		t.Fatalf("итоги раздачи: %v", err)
	}
	inDeal := seats[fixture.DealID]
	if len(inDeal) != 2 {
		t.Fatalf("ждали два места в раздаче, получили %d", len(inDeal))
	}
	// Пустой jsonb-массив обязан остаться пустым списком: nil ушёл бы в ответ как null,
	// а Java отдаёт здесь [].
	if inDeal[0].HungCards == nil || len(inDeal[0].HungCards) != 0 {
		t.Errorf("навесы места 0 = %v, ждали пустой, но не nil список", inDeal[0].HungCards)
	}
	if len(inDeal[1].LevelChanges) != 1 || inDeal[1].LevelChanges[0].Reason != "LOST_DEAL" {
		t.Errorf("изменения уровня места 1 = %v, ждали одну причину LOST_DEAL", inDeal[1].LevelChanges)
	}
	if inDeal[1].LevelChanges[0].Amount != 1 {
		t.Errorf("величина изменения %d, ждали 1", inDeal[1].LevelChanges[0].Amount)
	}
}

// ⭐ Лог читается вместе с колонкой видимости: без неё фильтровать было бы нечем,
// и чужая вскрытая карта всплыла бы в реплее задним числом.
func TestHistoryEventsCarryVisibility(t *testing.T) {
	pool := testDB(t)
	history := NewMatchHistory(pool)
	ctx := context.Background()

	fixture := historySeedFinishedMatch(t, pool)

	events, err := history.EventsOf(ctx, fixture.MatchID)
	if err != nil {
		t.Fatalf("лог матча: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("ждали два события, получили %d", len(events))
	}
	if events[0].Seq != 1 || events[1].Seq != 2 {
		t.Errorf("порядок событий сбит: %d, %d", events[0].Seq, events[1].Seq)
	}
	if events[0].PrivateToSeat != nil {
		t.Errorf("первое событие публичное, а видимость = %v", events[0].PrivateToSeat)
	}
	if events[1].PrivateToSeat == nil || *events[1].PrivateToSeat != 1 {
		t.Errorf("второе событие приватно месту 1, а видимость = %v", events[1].PrivateToSeat)
	}
	if string(events[1].Payload) == "" {
		t.Error("payload обязан доехать сырым JSON'ом, а приехал пустым")
	}
}

func TestHistoryFindMatchTellsMissingApart(t *testing.T) {
	pool := testDB(t)
	history := NewMatchHistory(pool)
	ctx := context.Background()

	fixture := historySeedFinishedMatch(t, pool)

	if _, err := history.FindMatch(ctx, fixture.MatchID); err != nil {
		t.Fatalf("существующий матч не прочитан: %v", err)
	}
	if _, err := history.FindMatch(ctx, uuid.NewString()); !errors.Is(err, ErrNotFound) {
		t.Errorf("несуществующий матч должен давать ErrNotFound, получили %v", err)
	}
}

// ⚠️ У отменённого матча итог участников пуст, а причина отмены — на месте.
// Без этого нулевая дельта выглядела бы как ничья.
func TestHistoryAbortedMatchKeepsReasonAndEmptyOutcome(t *testing.T) {
	pool := testDB(t)
	history := NewMatchHistory(pool)
	ctx := context.Background()

	fixture := historySeedFinishedMatch(t, pool)
	aborted := historyInsertMatch(t, pool, fixture.TableID, "ABORTED")
	historyInsertPlayer(t, pool, aborted, fixture.HostID, 0, nil, nil, nil)

	matches, err := history.MatchesOf(ctx, fixture.HostID, "aborted")
	if err != nil {
		t.Fatalf("список отменённых: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("ждали один отменённый матч, получили %d", len(matches))
	}
	if matches[0].AbortReason == nil || *matches[0].AbortReason != "Игрок вышел из матча" {
		t.Errorf("причина отмены %v, ждали текст из GameCommandHandler", matches[0].AbortReason)
	}

	players, err := history.PlayersOf(ctx, []string{aborted})
	if err != nil {
		t.Fatalf("итоги игроков: %v", err)
	}
	if got := players[aborted]; len(got) != 1 || got[0].Place != nil || got[0].RatingDelta != nil {
		t.Errorf("у отменённого матча итог обязан быть пустым, получили %+v", got)
	}
}

func TestHistoryParticipantsListSeats(t *testing.T) {
	pool := testDB(t)
	history := NewMatchHistory(pool)
	ctx := context.Background()

	fixture := historySeedFinishedMatch(t, pool)

	participants, err := history.ParticipantsOf(ctx, fixture.MatchID)
	if err != nil {
		t.Fatalf("участники: %v", err)
	}
	if len(participants) != 2 || participants[0].UserID != fixture.HostID || participants[0].SeatNo != 0 {
		t.Fatalf("участники прочитаны неверно: %+v", participants)
	}

	// У несуществующего матча участников нет — на этом держится проверка видимости.
	empty, err := history.ParticipantsOf(ctx, uuid.NewString())
	if err != nil {
		t.Fatalf("участники несуществующего матча: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("ждали пустой список, получили %d", len(empty))
	}
}

// historySeedFinishedMatch — законченный матч из одной раздачи: место 0 вышло первым,
// место 1 добралось до джокера. Повторяет фикстуру MatchHistoryApiIT из Java.
func historySeedFinishedMatch(t *testing.T, pool *pgxpool.Pool) historyFixture {
	t.Helper()
	ctx := context.Background()

	host := historyInsertUser(t, pool, "Хозяин")
	guest := historyInsertUser(t, pool, "Гость")
	tableID := historyInsertTable(t, pool, host)
	matchID := historyInsertMatch(t, pool, tableID, "FINISHED")

	firstPlace, secondPlace := 1, 2
	loss := "SUPER_MEGA_FAIL"
	historyInsertPlayer(t, pool, matchID, host, 0, &firstPlace, nil, ptrOfHistoryString("12.50"))
	historyInsertPlayer(t, pool, matchID, guest, 1, &secondPlace, &loss, ptrOfHistoryString("-12.50"))

	dealID := uuid.NewString()
	_, err := pool.Exec(ctx, `insert into deals (id, match_id, deal_no, trump_suit, finished_at,
	                          loser_seat, last_attack_cards)
	                          values ($1, $2, 1, 'SPADES', now(), 1, '["8-hearts"]'::jsonb)`,
		dealID, matchID)
	if err != nil {
		t.Fatalf("раздача: %v", err)
	}

	_, err = pool.Exec(ctx, `insert into deal_results (deal_id, seat_no, place, hung_cards,
	                         naves_level_before, naves_level_after, level_changes)
	                         values ($1, 0, 1, '[]'::jsonb, '7', '6', '[]'::jsonb),
	                                ($1, 1, 2, '["Joker-1"]'::jsonb, '7', '8',
	                                 '[{"reason":"LOST_DEAL","amount":1}]'::jsonb)`, dealID)
	if err != nil {
		t.Fatalf("итоги раздачи: %v", err)
	}

	// Второе событие видит только место 1: вскрытая закрытая карта — чужая тайна.
	_, err = pool.Exec(ctx, `insert into match_events (match_id, seq, deal_no, type, actor_seat,
	                         payload, private_to_seat)
	                         values ($1, 1, 1, 'DEAL_STARTED', null, '{"dealNo":1}'::jsonb, null),
	                                ($1, 2, 1, 'FACE_DOWN_REVEALED', 1,
	                                 '{"card":"A-clubs"}'::jsonb, 1)`, matchID)
	if err != nil {
		t.Fatalf("события матча: %v", err)
	}

	return historyFixture{MatchID: matchID, TableID: tableID, HostID: host,
		GuestID: guest, DealID: dealID}
}

func historyInsertUser(t *testing.T, pool *pgxpool.Pool, displayName string) string {
	t.Helper()
	id := uuid.NewString()
	_, err := pool.Exec(context.Background(),
		`insert into users (id, username, display_name, password_hash)
		 values ($1, $2, $3, 'hash')`, id, "hist-"+id[:8], displayName)
	if err != nil {
		t.Fatalf("пользователь: %v", err)
	}
	return id
}

func historyInsertTable(t *testing.T, pool *pgxpool.Pool, hostID string) string {
	t.Helper()
	id := uuid.NewString()
	// Набор карт и тема — засеянные миграцией V2: свои заводить незачем.
	_, err := pool.Exec(context.Background(),
		`insert into game_tables (id, code, name, host_user_id, max_players, status,
		                          card_set_id, theme_id)
		 values ($1, $2, 'История', $3, 2, 'CLOSED',
		         '11111111-1111-1111-1111-111111111111',
		         '22222222-2222-2222-2222-222222222222')`,
		id, "H"+id[:5], hostID)
	if err != nil {
		t.Fatalf("стол: %v", err)
	}
	return id
}

func historyInsertMatch(t *testing.T, pool *pgxpool.Pool, tableID, status string) string {
	t.Helper()
	id := uuid.NewString()
	var abortReason *string
	var finishedAt any
	if status == "ABORTED" {
		abortReason = ptrOfHistoryString("Игрок вышел из матча")
	}
	if status != "IN_PROGRESS" {
		finishedAt = "now()"
	}
	_, err := pool.Exec(context.Background(),
		`insert into matches (id, table_id, status, players_count, deals_played, rng_seed,
		                      rules_snapshot, finished_at, abort_reason)
		 values ($1, $2, $3, 2, 1, 11, '{}'::jsonb,
		         case when $4::text is null then null else now() end, $5)`,
		id, tableID, status, finishedAt, abortReason)
	if err != nil {
		t.Fatalf("матч: %v", err)
	}
	return id
}

func historyInsertPlayer(t *testing.T, pool *pgxpool.Pool, matchID, userID string, seatNo int,
	place *int, lossType, ratingDelta *string) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`insert into match_players (match_id, user_id, seat_no, naves_level, loss_type, place,
		                            rating_before, rating_after, rating_delta)
		 values ($1, $2, $3, '7', $4, $5,
		         case when $6::numeric is null then null else 1000.00 end,
		         case when $6::numeric is null then null else 1000.00 + $6::numeric end,
		         $6::numeric)`,
		matchID, userID, seatNo, lossType, place, ratingDelta)
	if err != nil {
		t.Fatalf("участник матча: %v", err)
	}
}

func ptrOfHistoryString(value string) *string { return &value }
