package ws

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/awesomeme01/bardak/back-go/internal/application"
	"github.com/awesomeme01/bardak/back-go/internal/domain/game"
	"github.com/awesomeme01/bardak/back-go/internal/repository"
	"github.com/awesomeme01/bardak/back-go/internal/transport/protocol"
)

// Игровые команды поверх сокета.
//
// ⭐ Проверяется поведение стола целиком: что записано в журнал, что ушло игрокам и
// в каком порядке, и что происходит с матчем, когда игрок уходит или пропадает.

type fakeMatches struct {
	session  *application.MatchSession
	startErr error
	finished []string
}

func (f *fakeMatches) Find(context.Context, string) (*application.MatchSession, bool) {
	return f.session, f.session != nil
}

func (f *fakeMatches) Start(context.Context, string) (*application.MatchSession, error) {
	if f.startErr != nil {
		return nil, f.startErr
	}
	return f.session, nil
}

func (f *fakeMatches) Finish(tableID string) {
	f.finished = append(f.finished, tableID)
	f.session = nil
}

type fakeGameLog struct {
	mu        sync.Mutex
	events    []repository.MatchEvent
	rejected  []repository.MatchEvent
	snapshots []int
	status    string
	reason    string
}

func (f *fakeGameLog) Append(_ context.Context, _ string, firstSeq, _ int,
	events []repository.MatchEvent) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	seq := firstSeq
	for _, event := range events {
		event.Seq = seq
		f.events = append(f.events, event)
		seq++
	}
	return seq - 1, nil
}

func (f *fakeGameLog) AppendRejected(_ context.Context, _ string, seq, _, actorSeat int,
	commandType, reason string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	seat := actorSeat
	f.rejected = append(f.rejected, repository.MatchEvent{Seq: seq, Type: "MOVE_REJECTED",
		ActorSeat: &seat, PrivateToSeat: &seat,
		Payload: `{"command":"` + commandType + `","reason":"` + reason + `"}`})
	return nil
}

func (f *fakeGameLog) SaveSnapshot(_ context.Context, _ string, seq int, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.snapshots = append(f.snapshots, seq)
	return nil
}

func (f *fakeGameLog) DealsPlayed(context.Context, string, int) error { return nil }

func (f *fakeGameLog) Since(_ context.Context, _ string, afterSeq int) ([]repository.MatchEvent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	tail := []repository.MatchEvent{}
	for _, event := range f.events {
		if event.Seq > afterSeq {
			tail = append(tail, event)
		}
	}
	return tail, nil
}

func (f *fakeGameLog) Finish(_ context.Context, _, status string, _, abortReason *string,
	_ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status = status
	if abortReason != nil {
		f.reason = *abortReason
	}
	return nil
}

func (f *fakeGameLog) snapshotCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.snapshots)
}

type fakeResults struct {
	changes []application.RatingChange
	called  int
}

func (f *fakeResults) FinishMatch(context.Context, string, []application.SeatOwner,
	game.MatchState, game.NavesScale) ([]application.RatingChange, error) {
	f.called++
	return f.changes, nil
}

type fakeDeals struct{ recorded int }

func (f *fakeDeals) RecordFinished(context.Context, string, game.MatchState, game.NavesScale,
	int) error {
	f.recorded++
	return nil
}

type fakeMatchLobby struct {
	finished  int
	left      []string
	tableName string
	nameErr   error
}

func (f *fakeMatchLobby) FinishMatch(context.Context, string) error {
	f.finished++
	return nil
}

func (f *fakeMatchLobby) Leave(_ context.Context, _, userID string) error {
	f.left = append(f.left, userID)
	return nil
}

func (f *fakeMatchLobby) ByID(_ context.Context, tableID string) (application.TableSnapshot, error) {
	if f.nameErr != nil {
		return application.TableSnapshot{}, f.nameErr
	}
	return application.TableSnapshot{
		Table: repository.GameTable{ID: tableID, Name: f.tableName},
	}, nil
}

type stubState struct{}

func (stubState) EncodeState(game.MatchState) (string, error) { return "снимок", nil }

// startedSession — реальная сессия матча на двоих.
//
// ⚠️ Seed подбирается так, чтобы раздача началась в фазе атаки. Если нижней картой колоды
// выпал джокер, раздача стартует в фазе DICE, любой ход до выбора масти отклоняется — и
// тест падал бы примерно раз в восемь прогонов, выглядя как поломка маршрутизатора.
// Та же ловушка описана в Java (SnapshotRestoreIT).
func startedSession(t *testing.T) *application.MatchSession {
	t.Helper()
	config := game.DefaultRulesConfig()
	engine := game.NewMatchEngineFor(config)

	for seed := int64(1); seed < 200; seed++ {
		state, err := engine.StartMatch(2, seed)
		if err != nil {
			continue
		}
		if state.Deal.Phase != game.PhaseAttack {
			continue
		}
		return application.NewMatchSession(tableID, "match-1", []application.SeatOwner{
			{SeatNo: 0, UserID: "user-a", DisplayName: "Аида"},
			{SeatNo: 1, UserID: "user-b", DisplayName: "Борис"},
		}, config, state)
	}
	t.Fatal("не нашёл seed, дающий раздачу в фазе атаки")
	return nil
}

func gameFixture(t *testing.T) (GameRouter, *fakeMatches, *fakeGameLog, *fakeMatchLobby, *fakeResults) {
	t.Helper()
	registry := NewTableRegistry(context.Background(), nil)
	t.Cleanup(registry.CloseAll)
	clock := application.NewTurnClock()
	t.Cleanup(clock.StopAll)

	matches := &fakeMatches{session: startedSession(t)}
	log := &fakeGameLog{}
	lobby := &fakeMatchLobby{}
	results := &fakeResults{}
	router := GameRouter{
		Matches: matches, Results: results, Deals: &fakeDeals{}, Log: log, Lobby: lobby,
		State: stubState{}, Registry: registry, Clock: clock,
		AutoMove: false, TurnTimeout: 50 * time.Millisecond,
		DisconnectGrace: 80 * time.Millisecond,
	}
	return router, matches, log, lobby, results
}

// attackCommand — законный ход обладателя права атаки: карта из его собственной руки.
func attackCommand(t *testing.T, session *application.MatchSession, id string) Envelope {
	t.Helper()
	deal := session.State().Deal
	hand := deal.MustPlayerAt(deal.AttackRightSeat).Hand
	if len(hand) == 0 {
		t.Fatal("у атакующего пустая рука")
	}
	envelope := envelopeOf("PLAY_CARD", `{"cardCode":"`+protocol.EncodeCard(hand[0])+`"}`)
	envelope.ID = &id
	return envelope
}

func userAtSeat(session *application.MatchSession, seatNo int) string {
	for _, seat := range session.Seats {
		if seat.SeatNo == seatNo {
			return seat.UserID
		}
	}
	return ""
}

func TestMatchStartIsRefusedWhenTableIsNotReady(t *testing.T) {
	router, matches, _, _, _ := gameFixture(t)
	matches.startErr = application.ErrTableNotReady
	socket := newCollector()

	router.Handle(context.Background(), envelopeOf("MATCH_START", ""), socket.client("user-a"))

	event := socket.nextDirect(t)
	var payload map[string]string
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if event.Type != "ERROR" || payload["code"] != "TABLE_NOT_READY" {
		t.Fatalf("ответ на старт в одиночку: %s / %v", event.Type, payload)
	}
}

// ⭐ Начавший матч подписывается на стол и сразу получает своё состояние: без подписки
// он не увидит ни одного последующего события.
func TestMatchStartSubscribesAndSendsState(t *testing.T) {
	router, _, _, _, _ := gameFixture(t)
	socket := newCollector()

	router.Handle(context.Background(), envelopeOf("MATCH_START", ""), socket.client("user-a"))

	event := socket.nextBroadcast(t)
	if event.Type != "STATE_SYNC" {
		t.Fatalf("после старта пришло %q, ждали STATE_SYNC", event.Type)
	}
	if event.Seq != nil {
		t.Fatalf("у снимка появился номер события: клиент отличает их именно по нему")
	}
}

// Валидного тикета мало: ходить может только тот, кто занимает место за этим столом.
func TestMoveFromAStrangerIsRefused(t *testing.T) {
	router, matches, _, _, _ := gameFixture(t)
	socket := newCollector()

	router.Handle(context.Background(), attackCommand(t, matches.session, "cmd-1"),
		socket.client("user-x"))

	event := socket.nextDirect(t)
	var payload map[string]string
	_ = json.Unmarshal(event.Payload, &payload)
	if payload["code"] != "NOT_A_PLAYER" {
		t.Fatalf("посторонний сходил за стол: %v", payload)
	}
}

func TestCommandWithoutAMatchIsRefused(t *testing.T) {
	router, matches, _, _, _ := gameFixture(t)
	matches.session = nil
	socket := newCollector()

	router.Handle(context.Background(), envelopeOf("PASS", ""), socket.client("user-a"))

	event := socket.nextDirect(t)
	var payload map[string]string
	_ = json.Unmarshal(event.Payload, &payload)
	if payload["code"] != "NO_MATCH" {
		t.Fatalf("команда без матча прошла: %v", payload)
	}
}

// ⭐ Принятый ход: сначала журнал, потом рассылка; события ПЕРЕД снимком; снимок сохранён.
func TestAppliedMoveIsLoggedThenBroadcast(t *testing.T) {
	router, matches, log, _, _ := gameFixture(t)
	session := matches.session
	attacker := newCollector()
	router.Handle(context.Background(), envelopeOf("MATCH_START", ""),
		attacker.client(userAtSeat(session, session.State().Deal.AttackRightSeat)))
	attacker.nextBroadcast(t)

	router.Handle(context.Background(),
		attackCommand(t, session, "cmd-1"),
		attacker.client(userAtSeat(session, session.State().Deal.AttackRightSeat)))

	first := attacker.nextBroadcast(t)
	if first.Type == "STATE_SYNC" {
		t.Fatalf("снимок пришёл раньше события: игрок увидел бы следствие раньше причины")
	}
	if first.Seq == nil || *first.Seq != 1 {
		t.Fatalf("номер первого события матча: %v, ждали 1", first.Seq)
	}
	second := attacker.nextBroadcast(t)
	if second.Type != "STATE_SYNC" {
		t.Fatalf("за событием пришло %q, ждали снимок", second.Type)
	}

	log.mu.Lock()
	defer log.mu.Unlock()
	if len(log.events) == 0 {
		t.Fatalf("ход не записан в журнал")
	}
	if len(log.snapshots) == 0 {
		t.Fatalf("снимок не сохранён: после рестарта матч было бы не поднять")
	}
}

// ⚠️ Отклонённый ход НЕ запоминается применённым: повтор обязан вернуть причину отказа,
// а не снимок, — иначе игрок так и не узнает, почему ход не прошёл.
func TestRejectedMoveBurnsSeqAndIsRepeatable(t *testing.T) {
	router, matches, log, _, _ := gameFixture(t)
	session := matches.session
	defender := userAtSeat(session, session.State().Deal.DefenderSeat)
	socket := newCollector()

	// Защищающийся кладёт карту, когда очередь не его.
	command := attackCommand(t, session, "cmd-1")
	router.Handle(context.Background(), command, socket.client(defender))

	first := socket.nextDirect(t)
	var payload map[string]string
	_ = json.Unmarshal(first.Payload, &payload)
	if first.Type != "ERROR" || payload["code"] == "" {
		t.Fatalf("отказ пришёл без причины: %v", payload)
	}
	if session.LastSeq() != 1 {
		t.Fatalf("отклонённая попытка не сожгла номер: lastSeq=%d", session.LastSeq())
	}
	log.mu.Lock()
	if len(log.rejected) != 1 || log.rejected[0].PrivateToSeat == nil {
		t.Fatalf("отклонённая попытка записана неверно: %+v", log.rejected)
	}
	log.mu.Unlock()

	// Повтор той же команды: снова отказ, а не снимок.
	router.Handle(context.Background(), command, socket.client(defender))
	repeated := socket.nextDirect(t)
	if repeated.Type != "ERROR" {
		t.Fatalf("повтор отклонённого хода вернул %q вместо причины отказа", repeated.Type)
	}
}

// ⭐ Клиент повторяет команду после обрыва, не зная, дошла ли она: повтор не применяется,
// но состояние отдаётся.
func TestRepeatedCommandReturnsStateInsteadOfPlayingTwice(t *testing.T) {
	router, matches, log, _, _ := gameFixture(t)
	session := matches.session
	attacker := userAtSeat(session, session.State().Deal.AttackRightSeat)
	socket := newCollector()
	router.Handle(context.Background(), envelopeOf("MATCH_START", ""), socket.client(attacker))
	socket.nextBroadcast(t)

	command := attackCommand(t, session, "cmd-1")
	router.Handle(context.Background(), command, socket.client(attacker))
	socket.nextBroadcast(t)
	socket.nextBroadcast(t)
	before := log.snapshotCount()

	router.Handle(context.Background(), command, socket.client(attacker))

	event := socket.nextDirect(t)
	if event.Type != "STATE_SYNC" {
		t.Fatalf("на повтор пришло %q, ждали снимок состояния", event.Type)
	}
	if log.snapshotCount() != before {
		t.Fatalf("повтор команды сыграл ход второй раз")
	}
}

// ⭐ Догон после обрыва фильтруется по ЗАПИСАННОЙ видимости: чужая вскрытая карта
// не должна уехать соседу.
func TestResyncSkipsEventsPrivateToSomebodyElse(t *testing.T) {
	router, matches, log, _, _ := gameFixture(t)
	session := matches.session
	other := 1
	log.events = []repository.MatchEvent{
		{Seq: 1, Type: "CARD_ATTACKED", Payload: `{"cardCode":"A-spades"}`},
		{Seq: 2, Type: "FACE_DOWN_REVEALED", Payload: `{"cardCode":"6-clubs"}`,
			PrivateToSeat: &other},
	}
	socket := newCollector()

	resync := envelopeOf("RESYNC", `{"lastSeq":0}`)
	router.Handle(context.Background(), resync, socket.client(userAtSeat(session, 0)))

	first := socket.nextDirect(t)
	if first.Type != "CARD_ATTACKED" {
		t.Fatalf("догон начался с %q", first.Type)
	}
	second := socket.nextDirect(t)
	if second.Type == "FACE_DOWN_REVEALED" {
		t.Fatalf("чужая скрытая карта уехала соседу")
	}
	if second.Type != "STATE_SYNC" {
		t.Fatalf("после догона пришло %q, ждали снимок", second.Type)
	}
	// Вернувшегося ждут остальные: им уходит MATCH_RESUMED.
	resumed := socket.nextBroadcast(t)
	if resumed.Type != "MATCH_RESUMED" {
		t.Fatalf("столу разослали %q, ждали MATCH_RESUMED", resumed.Type)
	}
}

// Уйти из матча может только тот, кто в нём играет.
func TestMatchLeaveFromAStrangerIsRefused(t *testing.T) {
	router, _, _, lobby, _ := gameFixture(t)
	socket := newCollector()

	router.Handle(context.Background(), envelopeOf("MATCH_LEAVE", ""), socket.client("user-x"))

	event := socket.nextDirect(t)
	var payload map[string]string
	_ = json.Unmarshal(event.Payload, &payload)
	if payload["code"] != "NOT_AT_TABLE" {
		t.Fatalf("посторонний отменил чужой матч: %v", payload)
	}
	if lobby.finished != 0 {
		t.Fatalf("стол освободили по команде постороннего")
	}
}

// ⭐ Ушедший отменяет матч целиком: тихо освободить своё место нельзя — движок продолжал
// бы ждать ушедшего, а на стул сел бы посторонний.
func TestMatchLeaveAbortsTheMatchAndFreesTheTable(t *testing.T) {
	router, matches, log, lobby, results := gameFixture(t)
	socket := newCollector()
	router.Handle(context.Background(), envelopeOf("MATCH_START", ""), socket.client("user-a"))
	socket.nextBroadcast(t)

	router.Handle(context.Background(), envelopeOf("MATCH_LEAVE", ""), socket.client("user-a"))

	event := socket.nextBroadcast(t)
	if event.Type != "MATCH_ABORTED" {
		t.Fatalf("столу разослали %q, ждали MATCH_ABORTED", event.Type)
	}
	if log.status != repository.MatchAborted || log.reason == "" {
		t.Fatalf("матч не помечен отменённым: %q / %q", log.status, log.reason)
	}
	if lobby.finished == 0 {
		t.Fatalf("стол остался в матче навсегда: сесть за него больше нельзя")
	}
	if len(matches.finished) == 0 {
		t.Fatalf("матч остался в памяти")
	}
	// ⚠️ Отменённый матч рейтинга не касается (§5.3).
	if results.called != 0 {
		t.Fatalf("отменённый матч посчитали в рейтинг")
	}
}

// Игрок пропал: матч на паузе, часы стоят, и всем сказано, сколько его ждут.
func TestDisconnectPausesTheMatchAndAbortsAfterTheGrace(t *testing.T) {
	router, _, log, lobby, _ := gameFixture(t)
	socket := newCollector()
	router.Handle(context.Background(), envelopeOf("MATCH_START", ""), socket.client("user-a"))
	socket.nextBroadcast(t)

	router.Disconnect(context.Background(), tableID, socket.client("user-a"))

	paused := socket.nextBroadcast(t)
	if paused.Type != "MATCH_PAUSED" {
		t.Fatalf("столу разослали %q, ждали MATCH_PAUSED", paused.Type)
	}
	var payload map[string]any
	_ = json.Unmarshal(paused.Payload, &payload)
	if payload["graceSeconds"] == nil {
		t.Fatalf("в паузе не сказано, сколько ждут: %v", payload)
	}

	aborted := socket.nextBroadcast(t)
	if aborted.Type != "MATCH_ABORTED" {
		t.Fatalf("не вернувшегося ждали вечно: пришло %q", aborted.Type)
	}
	if log.status != repository.MatchAborted {
		t.Fatalf("матч не помечен отменённым: %q", log.status)
	}
	if lobby.finished == 0 {
		t.Fatalf("стол не вернулся в лобби после отмены")
	}
}

// Экран итога матча.
//
// ⭐ Форма этого тела — контракт с экраном: по нему рисуются места, дельты рейтинга
// и приговор. Последняя атака отдаётся целиком: по восьмёркам в ней различаются
// «Королевский» и «Супер-мега-сак» (§0.3), а карты были на столе у всех на виду.
func TestMatchOverPayloadCarriesPlacesAndRatings(t *testing.T) {
	session := startedSession(t)
	level, degree := "Jk", "SUPER_MEGA_FAIL"
	outcome := game.NewDealOutcome([]game.PlayerOutcome{
		game.NewPlayerOutcome(0, 3, 4, game.NoLossDegree),
		game.NewPlayerOutcome(1, 8, game.FullNavesScale().JokerLevel(), game.LossSuperMegaFail),
	}, 1)
	outcome.LastAttackCards = []game.Card{game.NewPip(game.Eight, game.Hearts)}
	state := game.MatchState{Phase: game.MatchOver, Results: []game.DealOutcome{outcome}}

	payload := matchOverPayload(session, state, []application.RatingChange{
		{UserID: "user-a", SeatNo: 0, Place: 1, Before: "1000.00", After: "1010.00", Delta: "10.00"},
		{UserID: "user-b", SeatNo: 1, Place: 2, NavesLevel: &level, LossDegree: &degree,
			Before: "1000.00", After: "990.00", Delta: "-10.00"},
	})

	if payload["matchId"] != "match-1" || payload["dealsPlayed"] != 1 {
		t.Fatalf("шапка итога собрана неверно: %v", payload)
	}
	players, ok := payload["players"].([]map[string]any)
	if !ok || len(players) != 2 {
		t.Fatalf("игроки в итоге: %v", payload["players"])
	}
	// ⭐ Имя берётся из мест МАТЧА: к концу партии лобби живёт уже своей жизнью.
	if players[0]["displayName"] != "Аида" {
		t.Fatalf("имя игрока в итоге: %v", players[0]["displayName"])
	}
	if players[1]["lossDegree"] == nil || players[1]["ratingDelta"] != "-10.00" {
		t.Fatalf("приговор проигравшего собран неверно: %v", players[1])
	}
	if players[0]["lossDegree"] != (*string)(nil) {
		t.Fatalf("у не проигравшего появилась степень: %v", players[0]["lossDegree"])
	}
	cards, ok := payload["lastAttackCards"].([]string)
	if !ok || len(cards) != 1 {
		t.Fatalf("последняя атака в итоге: %v", payload["lastAttackCards"])
	}
}

// Зов к столу отсутствующих.
//
// ⭐ Уведомление — единственный способ вернуть человека вовремя, и единственный способ
// довести его до «отключить уведомления». Поэтому проверяется не факт отправки, а КОМУ
// оно уходит: тому, кого за столом нет, и только ему.

type fakeNotifier struct {
	mu      sync.Mutex
	turns   []notified
	paused  []notified
	present []string
}

type notified struct {
	userID    string
	tableID   string
	tableName string
	present   bool
	seconds   int64
}

func (f *fakeNotifier) TurnOf(userID, tableID string, present bool, nameOf func() string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.turns = append(f.turns, notified{userID: userID, tableID: tableID,
		tableName: nameOf(), present: present})
}

func (f *fakeNotifier) PausedFor(userID, tableID string, secondsLeft int64, nameOf func() string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.paused = append(f.paused, notified{userID: userID, tableID: tableID,
		tableName: nameOf(), seconds: secondsLeft})
}

func (f *fakeNotifier) Present(userID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.present = append(f.present, userID)
}

// lastTurn ждёт зова к столу.
//
// ⚠️ Ждёт, а не читает сразу: рассылка уходит РАНЬШЕ, чем перезапускаются часы хода,
// и всё это исполняется на goroutine стола. Проверка сразу после рассылки проходила бы
// через раз — тот самый флак, который на деле означает «тест торопится».
func (f *fakeNotifier) lastTurn(t *testing.T) notified {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		if len(f.turns) > 0 {
			last := f.turns[len(f.turns)-1]
			f.mu.Unlock()
			return last
		}
		f.mu.Unlock()
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("к столу никого не позвали")
	return notified{}
}

// awaitPaused ждёт зова пропавшего обратно.
func (f *fakeNotifier) awaitPaused(t *testing.T) notified {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		if len(f.paused) > 0 {
			call := f.paused[0]
			f.mu.Unlock()
			return call
		}
		f.mu.Unlock()
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("пропавшего не позвали обратно")
	return notified{}
}

// awaitPresent ждёт отметки о возвращении игрока.
func (f *fakeNotifier) awaitPresent(t *testing.T, userID string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		for _, seen := range f.present {
			if seen == userID {
				f.mu.Unlock()
				return
			}
		}
		f.mu.Unlock()
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("вернувшийся %s так и остался «ушедшим»", userID)
}

// onTheClockUser — кому сейчас принадлежит ход, и кто за столом второй.
func onTheClockUser(t *testing.T, session *application.MatchSession) (string, string) {
	t.Helper()
	seat, ok := application.SeatOnTheClock(session.State().Deal)
	if !ok {
		t.Fatal("сразу после раздачи ход никому не принадлежит")
	}
	onClock, _ := session.Naming(seat)
	for _, other := range session.Seats {
		if other.UserID != onClock {
			return onClock, other.UserID
		}
	}
	t.Fatal("за столом один игрок")
	return "", ""
}

func TestTurnCallsThePlayerWhoIsNotAtTheTable(t *testing.T) {
	router, matches, _, lobby, _ := gameFixture(t)
	notifier := &fakeNotifier{}
	router.Notifier = notifier
	lobby.tableName = "Вечерний"
	// Матч начинает НЕ тот, чей ход: у обладателя хода вкладка закрыта.
	onClock, starter := onTheClockUser(t, matches.session)
	socket := newCollector()

	router.Handle(context.Background(), envelopeOf("MATCH_START", ""), socket.client(starter))
	socket.nextBroadcast(t)

	call := notifier.lastTurn(t)
	if call.userID != onClock || call.present {
		t.Fatalf("позвали не отсутствующего обладателя хода: %+v", call)
	}
	if call.tableName != "Вечерний" || call.tableID != tableID {
		t.Fatalf("позвали без стола: %+v", call)
	}
}

func TestTurnStaysSilentWhenThePlayerIsSubscribedToTheTable(t *testing.T) {
	router, matches, _, _, _ := gameFixture(t)
	notifier := &fakeNotifier{}
	router.Notifier = notifier
	onClock, _ := onTheClockUser(t, matches.session)
	socket := newCollector()

	// Ход принадлежит тому, кто сам же и начал матч: он смотрит на стол.
	router.Handle(context.Background(), envelopeOf("MATCH_START", ""), socket.client(onClock))
	socket.nextBroadcast(t)

	// ⭐ Присутствие считается по ПОДПИСКЕ на события стола: маршрутизатор обязан
	// сказать зову «он здесь», а решение молчать принимает уже зов.
	call := notifier.lastTurn(t)
	if call.userID != onClock || !call.present {
		t.Fatalf("зову соврали о присутствии игрока: %+v", call)
	}
}

func TestTurnCallIsMadeEvenWhenTheTableDoesNotMoveForTheSilent(t *testing.T) {
	router, matches, _, _, _ := gameFixture(t)
	notifier := &fakeNotifier{}
	router.Notifier = notifier
	router.AutoMove = false
	onClock, starter := onTheClockUser(t, matches.session)
	socket := newCollector()

	router.Handle(context.Background(), envelopeOf("MATCH_START", ""), socket.client(starter))
	socket.nextBroadcast(t)

	// ⚠️ Часов при выключенном автодействии нет вовсе — ход ждёт хозяина сколько угодно.
	// Именно поэтому звать его надо тем более: иначе стол стоит молча и бесконечно.
	if notifier.lastTurn(t).userID != onClock {
		t.Fatalf("без автодействия зов пропал: %+v", notifier.turns)
	}
}

func TestPauseCallsTheMissingPlayerBack(t *testing.T) {
	router, _, _, lobby, _ := gameFixture(t)
	notifier := &fakeNotifier{}
	router.Notifier = notifier
	lobby.tableName = "Вечерний"
	socket := newCollector()
	router.Handle(context.Background(), envelopeOf("MATCH_START", ""), socket.client("user-a"))
	socket.nextBroadcast(t)

	router.Disconnect(context.Background(), tableID, socket.client("user-a"))
	socket.nextBroadcast(t)

	call := notifier.awaitPaused(t)
	if call.userID != "user-a" || call.tableName != "Вечерний" {
		t.Fatalf("позвали не того или не за тот стол: %+v", call)
	}
	// Человеку важно, сколько у него осталось: это окно и есть весь смысл зова.
	if call.seconds != int64(router.DisconnectGrace.Seconds()) {
		t.Fatalf("не сказали, сколько ждут: %+v", call)
	}
}

func TestReturningToTheTableMakesTheNextTurnWorthACallAgain(t *testing.T) {
	router, _, _, _, _ := gameFixture(t)
	notifier := &fakeNotifier{}
	router.Notifier = notifier
	socket := newCollector()
	router.Handle(context.Background(), envelopeOf("MATCH_START", ""), socket.client("user-a"))
	socket.nextBroadcast(t)

	router.Handle(context.Background(), envelopeOf("STATE_REQUEST", ""), socket.client("user-b"))
	socket.nextDirect(t)

	notifier.awaitPresent(t, "user-b")
}

func TestTurnCallSurvivesATableWithoutAName(t *testing.T) {
	router, matches, _, lobby, _ := gameFixture(t)
	notifier := &fakeNotifier{}
	router.Notifier = notifier
	lobby.nameErr = repository.ErrNotFound
	onClock, starter := onTheClockUser(t, matches.session)
	socket := newCollector()

	router.Handle(context.Background(), envelopeOf("MATCH_START", ""), socket.client(starter))
	socket.nextBroadcast(t)

	// ⚠️ Имя — украшение уведомления, а не его условие: у отправителя текст без имени
	// предусмотрен, и молчать из-за неудачного запроса в базу нельзя.
	call := notifier.lastTurn(t)
	if call.userID != onClock || call.tableName != "" {
		t.Fatalf("стол без имени сорвал зов: %+v", call)
	}
}

func TestTableWithoutANotifierPlaysAsUsual(t *testing.T) {
	router, _, log, _, _ := gameFixture(t)
	router.Notifier = nil
	socket := newCollector()
	router.Handle(context.Background(), envelopeOf("MATCH_START", ""), socket.client("user-a"))
	socket.nextBroadcast(t)

	// Уведомления — не условие игры: без них стол обязан работать ровно так же.
	router.Disconnect(context.Background(), tableID, socket.client("user-a"))
	if paused := socket.nextBroadcast(t); paused.Type != "MATCH_PAUSED" {
		t.Fatalf("без уведомлений стол сломался: пришло %q", paused.Type)
	}
	_ = log
}
