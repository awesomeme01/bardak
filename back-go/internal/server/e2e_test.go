package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/awesomeme01/bardak/back-go/internal/config"
	"github.com/awesomeme01/bardak/back-go/internal/observability"
	"github.com/awesomeme01/bardak/back-go/internal/server"
	"github.com/awesomeme01/bardak/back-go/internal/testsupport"
)

// Сквозной прогон: настоящий сервер, настоящий сокет, настоящая база.
//
// ⭐ Поднимается ровно та сборка, что уходит в бой (server.Build), а не её копия: копия
// расходится с оригиналом молча, и первое расхождение в проводке нашлось бы уже на живых
// игроках. За эту работу живая игра не раз находила то, чего не нашли сотни модульных
// тестов, — этот тест и есть попытка сделать её частью прогона.

type client struct {
	t        *testing.T
	name     string
	base     string
	token    string
	userID   string
	username string
	conn     *websocket.Conn
	// pending — что уже пришло, но ещё не разобрано ждущим.
	pending []envelope
}

type envelope struct {
	V       int             `json:"v"`
	ID      *string         `json:"id,omitempty"`
	Type    string          `json:"type"`
	TableID *string         `json:"tableId,omitempty"`
	Seq     *int            `json:"seq,omitempty"`
	TS      int64           `json:"ts"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

func TestMatchIsPlayedOverARealSocket(t *testing.T) {
	pool := testsupport.Postgres(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := config.Config{
		JWTSecret:       []byte("тестовый-секрет-достаточной-длины-32+"),
		InviteCodes:     []string{"bardak-2026"},
		AutoMove:        false,
		TurnTimeout:     30 * time.Second,
		DisconnectGrace: 2 * time.Second,
		ShutdownTimeout: time.Second,
	}
	handler, shutdown := server.Build(ctx, cfg, pool, observability.NewLogger())
	defer shutdown()

	httpServer := httptest.NewServer(handler)
	defer httpServer.Close()

	host := newClient(t, httpServer.URL, "хозяин")
	guest := newClient(t, httpServer.URL, "гость")

	tableID := host.createTable()

	host.connect()
	defer host.close()
	guest.connect()
	defer guest.close()

	// Хозяин уже сидит за столом, но подписка на события заводится посадкой:
	// без неё он не увидит ни одного хода.
	host.send(envelope{Type: "TABLE_JOIN", TableID: &tableID})
	host.await("PLAYER_JOINED")
	guest.send(envelope{Type: "TABLE_JOIN", TableID: &tableID})
	guest.await("PLAYER_JOINED")
	host.await("PLAYER_JOINED") // хозяин видит, что к нему сели

	host.send(envelope{Type: "TABLE_READY", TableID: &tableID})
	guest.send(envelope{Type: "TABLE_READY", TableID: &tableID})
	// Готовность видят оба: рассылка идёт всем за столом, и непрочитанное потом мешало бы
	// ждать нужное.
	host.await("PLAYER_READY")
	host.await("PLAYER_READY")
	guest.await("PLAYER_READY")
	guest.await("PLAYER_READY")

	host.send(envelope{Type: "MATCH_START", TableID: &tableID})

	hostState := parseState(t, host.await("STATE_SYNC"))
	guestState := parseState(t, guest.await("STATE_SYNC"))

	// ⭐ Проекция ПЕРСОНАЛЬНАЯ: каждый видит свою руку и только количество чужих карт.
	if len(hostState.MyHand) == 0 || len(guestState.MyHand) == 0 {
		t.Fatalf("кому-то раздали пустую руку: %d / %d", len(hostState.MyHand), len(guestState.MyHand))
	}
	if hostState.MySeat == guestState.MySeat {
		t.Fatalf("оба игрока сидят на месте %d", hostState.MySeat)
	}
	if sameCards(hostState.MyHand, guestState.MyHand) {
		t.Fatal("игрокам раздали одинаковые руки — проекция отдаёт чужие карты")
	}

	// Ходит тот, у кого право атаки.
	mover, watcher := host, guest
	state := hostState
	if hostState.MySeat != hostState.CanAttackSeat {
		mover, watcher = guest, host
		state = guestState
	}
	if len(state.AvailableActions) == 0 {
		t.Fatal("сервер не предложил ходящему ни одного действия")
	}

	// ⭐ Играется действие, предложенное САМИМ сервером: кнопка, которую он показал,
	// обязана проходить. Иначе игрок жмёт её и получает отказ.
	action := pickAction(state.AvailableActions, "PLAY_CARD")
	mover.send(envelope{Type: action.Type, TableID: &tableID, Payload: rawOf(t, action.Payload)})

	// ⭐ Ход виден событием со сквозным номером: по нему клиент понимает, что пропустил.
	event := mover.awaitGameEvent()
	if event.Seq == nil || *event.Seq != 1 {
		t.Fatalf("первое событие матча пришло с номером %v, ждали 1", event.Seq)
	}
	after := parseState(t, mover.await("STATE_SYNC"))
	// ⚠️ Проверять «в руке стало на карту меньше» нельзя: сервер предлагает не только
	// карту. В фазе кости первым действием идёт выбор масти, и рука от него не меняется —
	// на этом тест падал примерно раз в четыре прогона. Проверяем то, что действительно
	// следует из сыгранного действия.
	if code, isCard := action.Payload["cardCode"].(string); isCard {
		if contains(after.MyHand, code) {
			t.Fatalf("сыгранная карта %s осталась в руке: %v", code, after.MyHand)
		}
	} else if fmt.Sprint(after) == fmt.Sprint(state) {
		t.Fatalf("после хода %s состояние не изменилось", action.Type)
	}
	// ⚠️ Соперник видит тот же ход: рассылка идёт всем игрокам матча, а не только ходившему.
	watcher.await("STATE_SYNC")

	// Уход из матча отменяет его целиком и возвращает стол в лобби.
	mover.send(envelope{Type: "MATCH_LEAVE", TableID: &tableID})
	aborted := watcher.await("MATCH_ABORTED")
	if aborted.TableID == nil || *aborted.TableID != tableID {
		t.Fatalf("отмена пришла не по тому столу: %v", aborted.TableID)
	}

	var status string
	err := pool.QueryRow(ctx, `select status from matches where table_id = $1`, tableID).Scan(&status)
	if err != nil {
		t.Fatalf("матч не нашёлся в базе: %v", err)
	}
	if status != "ABORTED" {
		t.Fatalf("статус матча в базе %q, ждали ABORTED", status)
	}

	var tableStatus string
	err = pool.QueryRow(ctx, `select status from game_tables where id = $1`, tableID).Scan(&tableStatus)
	if err != nil {
		t.Fatal(err)
	}
	if tableStatus != "WAITING" {
		t.Fatalf("стол остался в состоянии %q: сесть за него больше нельзя", tableStatus)
	}
}

// ── клиент ──────────────────────────────────────────────────────────────────

func newClient(t *testing.T, base, name string) *client {
	t.Helper()
	c := &client{t: t, name: name, base: base}

	login := name + "-" + uuid.NewString()[:8]
	body := map[string]any{
		"username": login, "displayName": name, "password": "пароль-достаточной-длины",
		"inviteCode": "bardak-2026",
	}
	var response struct {
		AccessToken string `json:"accessToken"`
		User        struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	c.post("/api/auth/register", body, "", &response)
	if response.AccessToken == "" {
		t.Fatalf("регистрация не выдала токен")
	}
	c.token = response.AccessToken
	c.userID = response.User.ID
	c.username = login
	return c
}

func (c *client) createTable() string {
	var table struct {
		ID string `json:"id"`
	}
	c.post("/api/tables", map[string]any{"name": "Сквозной стол", "maxPlayers": 2},
		c.token, &table)
	if table.ID == "" {
		c.t.Fatal("стол не создался")
	}
	return table.ID
}

func (c *client) connect() {
	var ticket struct {
		Ticket string `json:"ticket"`
	}
	c.post("/api/auth/ws-ticket", map[string]any{}, c.token, &ticket)
	if ticket.Ticket == "" {
		c.t.Fatal("тикет не выдан")
	}

	url := "ws" + strings.TrimPrefix(c.base, "http") + "/ws?ticket=" + ticket.Ticket
	conn, _, err := websocket.Dial(context.Background(), url, nil)
	if err != nil {
		c.t.Fatalf("сокет не открылся: %v", err)
	}
	c.conn = conn
	c.await("CONNECTED")
}

func (c *client) close() {
	if c.conn != nil {
		_ = c.conn.Close(websocket.StatusNormalClosure, "тест закончен")
	}
}

func (c *client) send(message envelope) {
	c.t.Helper()
	message.V = 1
	id := uuid.NewString()
	message.ID = &id
	raw, err := json.Marshal(message)
	if err != nil {
		c.t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.conn.Write(ctx, websocket.MessageText, raw); err != nil {
		c.t.Fatalf("сообщение не ушло: %v", err)
	}
}

// await ждёт сообщение нужного типа, пропуская остальные.
//
// ⚠️ Диагностика при неудаче обязана быть подробной: «сокет замолчал» без того, чего
// ждали и что уже пришло, превращает разбор в гадание.
func (c *client) await(messageType string) envelope {
	c.t.Helper()
	for index, message := range c.pending {
		if message.Type == messageType {
			c.pending = append(c.pending[:index], c.pending[index+1:]...)
			return message
		}
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		message := c.read(messageType)
		if message.Type == messageType {
			return message
		}
		if message.Type == "ERROR" {
			c.t.Fatalf("%s: вместо %s пришёл отказ: %s", c.name, messageType, message.Payload)
		}
		c.pending = append(c.pending, message)
	}
	c.t.Fatalf("%s не дождался %s", c.name, messageType)
	return envelope{}
}

// awaitGameEvent — первое сообщение со сквозным номером: игровое событие, а не снимок
// и не событие лобби. Тип зависит от того, какой ход предложил сервер, поэтому ждём
// по признаку, а не по имени.
func (c *client) awaitGameEvent() envelope {
	c.t.Helper()
	for index, message := range c.pending {
		if message.Seq != nil {
			c.pending = append(c.pending[:index], c.pending[index+1:]...)
			return message
		}
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		message := c.read("игровое событие")
		if message.Type == "ERROR" {
			c.t.Fatalf("ход, предложенный сервером, им же и отклонён: %s", message.Payload)
		}
		if message.Seq != nil {
			return message
		}
		c.pending = append(c.pending, message)
	}
	c.t.Fatalf("%s не дождался игрового события", c.name)
	return envelope{}
}

func (c *client) read(awaited string) envelope {
	c.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, raw, err := c.conn.Read(ctx)
	if err != nil {
		c.t.Fatalf("%s: сокет замолчал в ожидании %s (уже пришло: %s): %v",
			c.name, awaited, typesOf(c.pending), err)
	}
	var message envelope
	if err := json.Unmarshal(raw, &message); err != nil {
		c.t.Fatalf("сообщение не разобрано: %v", err)
	}
	return message
}

func (c *client) post(path string, body any, token string, into any) {
	c.t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		c.t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, c.base+path, bytes.NewReader(raw))
	if err != nil {
		c.t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		c.t.Fatalf("%s: %v", path, err)
	}
	defer response.Body.Close()

	payload, _ := io.ReadAll(response.Body)
	if response.StatusCode >= 300 {
		c.t.Fatalf("%s ответил %d: %s", path, response.StatusCode, payload)
	}
	if into != nil {
		if err := json.Unmarshal(payload, into); err != nil {
			c.t.Fatalf("%s: ответ не разобран: %v (%s)", path, err, payload)
		}
	}
}

// get читает ручку от лица клиента и валится на любом ответе, кроме 200.
func (c *client) get(path string, into any) {
	c.t.Helper()
	status, payload := c.getRaw(path)
	if status != http.StatusOK {
		c.t.Fatalf("GET %s ответил %d: %s", path, status, payload)
	}
	if into != nil {
		if err := json.Unmarshal(payload, into); err != nil {
			c.t.Fatalf("GET %s: ответ не разобран: %v (%s)", path, err, payload)
		}
	}
}

// getRaw возвращает код и тело как есть: нужен там, где проверяется ОТКАЗ.
func (c *client) getRaw(path string) (int, []byte) {
	c.t.Helper()
	request, err := http.NewRequest(http.MethodGet, c.base+path, nil)
	if err != nil {
		c.t.Fatal(err)
	}
	if c.token != "" {
		request.Header.Set("Authorization", "Bearer "+c.token)
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		c.t.Fatalf("GET %s: %v", path, err)
	}
	defer response.Body.Close()

	payload, _ := io.ReadAll(response.Body)
	return response.StatusCode, payload
}

// ── разбор состояния ────────────────────────────────────────────────────────

type stateSync struct {
	MySeat           int      `json:"mySeat"`
	CanAttackSeat    int      `json:"canAttackSeat"`
	DefenderSeat     int      `json:"defenderSeat"`
	Phase            string   `json:"phase"`
	DealNo           int      `json:"dealNo"`
	MyHand           []string `json:"myHand"`
	AvailableActions []struct {
		Type    string         `json:"type"`
		Payload map[string]any `json:"payload"`
	} `json:"availableActions"`
}

func parseState(t *testing.T, message envelope) stateSync {
	t.Helper()
	var state stateSync
	if err := json.Unmarshal(message.Payload, &state); err != nil {
		t.Fatalf("снимок состояния не разобран: %v", err)
	}
	return state
}

func pickAction(actions []struct {
	Type    string         `json:"type"`
	Payload map[string]any `json:"payload"`
}, preferred string) struct {
	Type    string         `json:"type"`
	Payload map[string]any `json:"payload"`
} {
	for _, action := range actions {
		if action.Type == preferred {
			return action
		}
	}
	return actions[0]
}

func rawOf(t *testing.T, payload map[string]any) json.RawMessage {
	t.Helper()
	if len(payload) == 0 {
		return nil
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func sameCards(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	return fmt.Sprint(left) == fmt.Sprint(right)
}

// Матч, доигранный до конца, — единственный путь, на котором пишутся итог и рейтинг.
//
// ⭐ Ходят «роботы», которые не знают правил: они играют ДЕЙСТВИЯ, предложенные самим
// сервером. Так проверяется и то, что предложенное действие всегда проходит, — иначе
// игрок жмёт кнопку, которую сервер сам же и показал, и получает отказ.
func TestMatchIsPlayedToTheEndAndCounted(t *testing.T) {
	// ⚠️ Матч играется целиком и идёт десятки секунд: раздач в нём столько, сколько
	// выпадет по картам. В коротком прогоне пропускается — но не удаляется: это
	// единственный путь, на котором пишутся итог и рейтинг.
	if testing.Short() {
		t.Skip("полный матч — долгий прогон")
	}
	pool := testsupport.Postgres(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := config.Config{
		JWTSecret:       []byte("тестовый-секрет-достаточной-длины-32+"),
		InviteCodes:     []string{"bardak-2026"},
		TurnTimeout:     30 * time.Second,
		DisconnectGrace: 10 * time.Second,
		ShutdownTimeout: time.Second,
	}
	handler, shutdown := server.Build(ctx, cfg, pool, observability.NewLogger())
	defer shutdown()

	httpServer := httptest.NewServer(handler)
	defer httpServer.Close()

	host := newClient(t, httpServer.URL, "робот-один")
	guest := newClient(t, httpServer.URL, "робот-два")
	tableID := host.createTable()

	host.connect()
	defer host.close()
	guest.connect()
	defer guest.close()

	host.send(envelope{Type: "TABLE_JOIN", TableID: &tableID})
	host.await("PLAYER_JOINED")
	// ⚠️ Гость садится, ДОЖДАВШИСЬ своей посадки: команда готовности от неподсевшего
	// отвергается, и хозяин ждал бы вторую готовность вечно.
	guest.send(envelope{Type: "TABLE_JOIN", TableID: &tableID})
	guest.await("PLAYER_JOINED")

	host.send(envelope{Type: "TABLE_READY", TableID: &tableID})
	guest.send(envelope{Type: "TABLE_READY", TableID: &tableID})

	// ⚠️ Старт — только увидев обе готовности. Команды идут с РАЗНЫХ сокетов, и порядок
	// их прихода к столу сетью не гарантирован: MATCH_START, посланный сразу вслед за
	// своим «готов», обгоняет чужой и получает TABLE_NOT_READY. Живой клиент ведёт себя
	// так же — ждёт PLAYER_READY, а не свою отправку.
	host.await("PLAYER_READY")
	host.await("PLAYER_READY")
	guest.await("PLAYER_READY")
	guest.await("PLAYER_READY")
	host.send(envelope{Type: "MATCH_START", TableID: &tableID})

	over := playUntilMatchOver(t, tableID, host, guest)

	// ── что увидел игрок ────────────────────────────────────────────────────
	players, ok := over["players"].([]any)
	if !ok || len(players) != 2 {
		t.Fatalf("в итоге матча игроков: %v", over["players"])
	}
	places := map[float64]bool{}
	deltas := 0.0
	for _, raw := range players {
		player := raw.(map[string]any)
		places[player["place"].(float64)] = true
		// ⭐ Дельта — JSON-ЧИСЛО, как BigDecimal у Java, а не строка.
		delta, ok := player["ratingDelta"].(float64)
		if !ok {
			t.Fatalf("дельта рейтинга не число: %v (%T)", player["ratingDelta"], player["ratingDelta"])
		}
		deltas += delta
	}
	if !places[1] || !places[2] {
		t.Fatalf("места в итоге: %v", places)
	}
	// ⭐ Elo — игра с нулевой суммой: при равных стартовых рейтингах сумма дельт нулевая.
	if math.Abs(deltas) > 0.001 {
		t.Fatalf("сумма изменений рейтинга %v, ждали ноль: рейтинг создаётся из воздуха", deltas)
	}

	// ── что осталось в базе ─────────────────────────────────────────────────
	var status string
	var loser *string
	err := pool.QueryRow(ctx, `select status, loser_user_id::text from matches where table_id = $1`,
		tableID).Scan(&status, &loser)
	if err != nil {
		t.Fatal(err)
	}
	if status != "FINISHED" {
		t.Fatalf("статус матча %q, ждали FINISHED", status)
	}
	if loser == nil {
		t.Fatal("матч закончился без проигравшего — а он есть всегда")
	}

	var counted int
	err = pool.QueryRow(ctx, `select count(*) from match_players p join matches m on m.id = p.match_id
	                          where m.table_id = $1 and p.place is not null`, tableID).Scan(&counted)
	if err != nil {
		t.Fatal(err)
	}
	if counted != 2 {
		t.Fatalf("мест проставлено %d, ждали 2: по пустому месту матч считается несыгранным", counted)
	}

	var histories int
	err = pool.QueryRow(ctx, `select count(*) from rating_history h join matches m on m.id = h.match_id
	                          where m.table_id = $1`, tableID).Scan(&histories)
	if err != nil {
		t.Fatal(err)
	}
	if histories != 2 {
		t.Fatalf("записей истории рейтинга %d, ждали 2", histories)
	}

	// Раздачи записаны: без них экран истории показывал бы матч без содержимого.
	var deals int
	err = pool.QueryRow(ctx, `select count(*) from deals d join matches m on m.id = d.match_id
	                          where m.table_id = $1`, tableID).Scan(&deals)
	if err != nil {
		t.Fatal(err)
	}
	if deals == 0 {
		t.Fatal("сыгранные раздачи не записаны")
	}

	var tableStatus string
	if err := pool.QueryRow(ctx, `select status from game_tables where id = $1`,
		tableID).Scan(&tableStatus); err != nil {
		t.Fatal(err)
	}
	if tableStatus != "WAITING" {
		t.Fatalf("после матча стол в состоянии %q: собраться заново нельзя", tableStatus)
	}

	// ── что отдают ручки, до которых доходит только сыгранный матч ───────────
	//
	// ⭐ Ниже проверяется то, чего не проверяет ни один другой прогон: история, разбор
	// и реплей читаются ТОЛЬКО после настоящего матча, и до сих пор их правильность
	// держалась на обработчиках с выдуманными данными. Тот же матч, что выше проверен
	// в базе, теперь читается так, как его читает экран.
	checkHistoryAfterMatch(t, ctx, pool, tableID, host, guest, httpServer.URL)
}

// checkHistoryAfterMatch читает историю, разбор, реплей и рейтинг сыгранного матча.
func checkHistoryAfterMatch(t *testing.T, ctx context.Context, pool *pgxpool.Pool,
	tableID string, host, guest *client, base string) {
	t.Helper()

	// Список матчей: заодно отсюда берётся идентификатор матча — тот, что видит экран,
	// а не тот, что достали бы запросом в базу.
	var list []struct {
		ID          string `json:"id"`
		TableID     string `json:"tableId"`
		Status      string `json:"status"`
		DealsPlayed int    `json:"dealsPlayed"`
		MyPlace     *int   `json:"myPlace"`
		MyDelta     *string
	}
	host.get("/api/matches", &list)

	matchID := ""
	for _, item := range list {
		if item.TableID == tableID {
			matchID = item.ID
			if item.Status != "FINISHED" {
				t.Fatalf("в истории матч со статусом %q", item.Status)
			}
			if item.DealsPlayed == 0 {
				t.Fatal("в истории матч без сыгранных раздач")
			}
			if item.MyPlace == nil {
				t.Fatal("в истории нет своего места: экран показал бы матч без результата")
			}
		}
	}
	if matchID == "" {
		t.Fatalf("сыгранный матч не попал в свою же историю: %d записей", len(list))
	}

	// Разбор матча: раздачи с местами и уровнями навесов.
	var details struct {
		Match struct {
			ID      string `json:"id"`
			Status  string `json:"status"`
			Players []struct {
				SeatNo int  `json:"seatNo"`
				Place  *int `json:"place"`
			} `json:"players"`
		} `json:"match"`
		Deals []struct {
			DealNo int `json:"dealNo"`
			Seats  []struct {
				SeatNo int `json:"seatNo"`
			} `json:"seats"`
		} `json:"deals"`
	}
	host.get("/api/matches/"+matchID, &details)
	if details.Match.ID != matchID || len(details.Match.Players) != 2 {
		t.Fatalf("разбор матча пуст: %+v", details.Match)
	}
	if len(details.Deals) == 0 {
		t.Fatal("разбор матча без раздач: экран показал бы пустой матч")
	}
	for _, deal := range details.Deals {
		if len(deal.Seats) != 2 {
			t.Fatalf("в раздаче %d мест %d, ждали 2", deal.DealNo, len(deal.Seats))
		}
	}

	// ⭐ Реплей: главное здесь — что чужая рука не уезжает задним числом. Проверяется
	// не «похоже на правду», а точным числом: игроку видно ВСЁ, кроме событий, приватных
	// чужому месту. Разойдись фильтр — сойдётся и число.
	hostSeat := checkReplay(t, ctx, pool, matchID, host)
	guestSeat := checkReplay(t, ctx, pool, matchID, guest)
	if hostSeat == guestSeat {
		t.Fatalf("оба игрока считают себя местом %d", hostSeat)
	}

	// Рейтинг после матча: точка истории с этим матчем.
	for _, player := range []*client{host, guest} {
		var rating struct {
			MatchesPlayed int `json:"matchesPlayed"`
			History       []struct {
				MatchID string `json:"matchId"`
				Place   int    `json:"place"`
			} `json:"history"`
		}
		player.get("/api/rating/me", &rating)
		if rating.MatchesPlayed == 0 {
			t.Fatalf("%s сыграл матч, а в рейтинге их ноль", player.name)
		}
		found := false
		for _, point := range rating.History {
			if point.MatchID == matchID {
				found = true
				if point.Place < 1 {
					t.Fatalf("%s: место в истории рейтинга %d", player.name, point.Place)
				}
			}
		}
		if !found {
			t.Fatalf("%s: матч не попал в историю рейтинга", player.name)
		}

		var stats struct {
			Matches     int `json:"matches"`
			Wins        int `json:"wins"`
			Losses      int `json:"losses"`
			DealsPlayed int `json:"dealsPlayed"`
		}
		player.get("/api/stats/me", &stats)
		if stats.Matches == 0 || stats.Wins+stats.Losses == 0 || stats.DealsPlayed == 0 {
			t.Fatalf("%s: статистика после матча пуста: %+v", player.name, stats)
		}
	}

	// ⚠️ Посторонний не читает чужую партию вовсе — ни разбор, ни реплей. Друзьям она
	// видна, но дружбы здесь нет, и ответ обязан быть отказом, а не пустым телом.
	stranger := newClient(t, base, "посторонний")
	for _, path := range []string{"/api/matches/" + matchID, "/api/matches/" + matchID + "/replay"} {
		if status, body := stranger.getRaw(path); status != http.StatusForbidden {
			t.Fatalf("посторонний получил %s с кодом %d: %s", path, status, body)
		}
	}
}

// checkReplay читает реплей глазами игрока и возвращает его место.
func checkReplay(t *testing.T, ctx context.Context, pool *pgxpool.Pool, matchID string,
	player *client) int {
	t.Helper()

	var replay struct {
		MatchID string `json:"matchId"`
		Status  string `json:"status"`
		MySeat  int    `json:"mySeat"`
		Events  []struct {
			Seq  int    `json:"seq"`
			Type string `json:"type"`
		} `json:"events"`
	}
	player.get("/api/matches/"+matchID+"/replay", &replay)

	if replay.MySeat < 0 {
		t.Fatalf("%s играл матч, а реплей считает его посторонним", player.name)
	}
	if len(replay.Events) == 0 {
		t.Fatalf("%s: реплей сыгранного матча пуст", player.name)
	}
	previous := 0
	for _, event := range replay.Events {
		if event.Seq <= previous {
			t.Fatalf("%s: реплей идёт не по порядку: %d после %d", player.name, event.Seq, previous)
		}
		previous = event.Seq
		if event.Type == "" {
			t.Fatalf("%s: событие реплея без имени (seq %d)", player.name, event.Seq)
		}
	}

	// Точное число: всё, кроме приватного ЧУЖОМУ месту.
	var expected int
	err := pool.QueryRow(ctx, `select count(*) from match_events
	                           where match_id = $1
	                             and (private_to_seat is null or private_to_seat = $2)`,
		matchID, replay.MySeat).Scan(&expected)
	if err != nil {
		t.Fatal(err)
	}
	if len(replay.Events) != expected {
		t.Fatalf("%s: в реплее %d событий, видимых ему — %d: фильтр приватности разошёлся",
			player.name, len(replay.Events), expected)
	}

	// Приватные события в матче вообще есть — иначе проверка выше ничего не значит.
	var private int
	if err := pool.QueryRow(ctx, `select count(*) from match_events
	                              where match_id = $1 and private_to_seat is not null`,
		matchID).Scan(&private); err != nil {
		t.Fatal(err)
	}
	if private == 0 {
		t.Log("⚠️ в этом матче не оказалось приватных событий — проверка фильтра прошла вхолостую")
	}
	return replay.MySeat
}

// playUntilMatchOver гоняет обоих «роботов», пока матч не кончится, и возвращает
// тело MATCH_OVER.
//
// ⭐ Робот ходит ТОЛЬКО ПО ЗАТИШЬЮ: когда сервер перестал присылать сообщения, состояние
// у обоих актуально, и в полёте нет ни одной команды.
//
// ⚠️ Затишье здесь не перестраховка. В бардаке подкидывать можно ВО ВРЕМЯ защиты, то есть
// ходить законно могут оба игрока сразу. Робот, стреляющий по каждому снимку, регулярно
// посылал команду по состоянию, которое сервер уже поменял чужим ходом, и получал
// CARD_NOT_IN_HAND на карту, которую сам же сыграл секунду назад. Живой клиент решает то
// же самое, гася кнопки до прихода ответа.
func playUntilMatchOver(t *testing.T, tableID string, players ...*client) map[string]any {
	t.Helper()

	type inbound struct {
		who     *client
		message envelope
	}
	feed := make(chan inbound, 256)
	for _, player := range players {
		go func(player *client) {
			for {
				message, err := player.readOnce()
				if err != nil {
					return
				}
				feed <- inbound{who: player, message: message}
			}
		}(player)
	}

	// ⚠️ Ограничения сверху обязательны: зациклившийся автомат раздачи иначе повесил бы
	// прогон целиком, и выглядело бы это как «тест долго идёт», а не как поломка правил.
	const quiet = 25 * time.Millisecond
	deadline := time.After(90 * time.Second)
	settled := time.NewTimer(quiet)
	defer settled.Stop()

	moves, refusals := 0, 0
	latest := map[*client]stateSync{}
	// ⚠️ Кольцо последних ходов: без него «матч не кончился» — это только число, а нужен
	// ответ на вопрос «во что уперлись».
	recent := make([]string, 0, 40)
	remember := func(line string) {
		recent = append(recent, line)
		if len(recent) > 40 {
			recent = recent[1:]
		}
	}

	for {
		select {
		case <-deadline:
			// ⚠️ Разбор обязателен: «не кончился» бывает и долгим матчем, и залипшим
			// столом, где сервер не предлагает действий никому. Второе — поломка,
			// и отличать её надо по состоянию, а не по времени.
			for _, player := range players {
				state, known := latest[player]
				if !known {
					t.Logf("%s: свежего снимка нет", player.name)
					continue
				}
				t.Logf("%s: место %d, раздача %d, фаза %s, атака у %d, защита у %d, действий %d",
					player.name, state.MySeat, state.DealNo, state.Phase, state.CanAttackSeat,
					state.DefenderSeat, len(state.AvailableActions))
			}
			t.Logf("последние ходы:\n%s", strings.Join(recent, "\n"))
			t.Fatalf("матч не кончился за отведённое время, ходов сделано %d", moves)

		case got := <-feed:
			switch got.message.Type {
			case "MATCH_OVER":
				var payload map[string]any
				if err := json.Unmarshal(got.message.Payload, &payload); err != nil {
					t.Fatalf("итог матча не разобран: %v", err)
				}
				t.Logf("матч сыгран за %d ходов, отказов %d", moves, refusals)
				return payload

			case "MATCH_ABORTED":
				t.Fatalf("матч отменился сам: %s", got.message.Payload)

			case "ERROR":
				// ⚠️ Отказы считаются: в затишье их быть не должно вовсе, и каждый —
				// повод посмотреть, кто соврал, сервер или робот.
				refusals++
				if refusals > 5 {
					t.Logf("последние ходы:\n%s", strings.Join(recent, "\n"))
					t.Fatalf("роботы буксуют на отказах, последний: %s", got.message.Payload)
				}
				t.Logf("отказ на ход: %s", got.message.Payload)
				delete(latest, got.who)
				got.who.send(envelope{Type: "STATE_REQUEST", TableID: &tableID})

			case "STATE_SYNC":
				latest[got.who] = parseState(t, got.message)
			}

			// Сервер ещё говорит — ждём, пока замолчит.
			settled.Reset(quiet)

		case <-settled.C:
			for _, player := range players {
				state, known := latest[player]
				if !known || len(state.AvailableActions) == 0 {
					continue
				}
				action := state.AvailableActions[0]
				delete(latest, player)
				remember(fmt.Sprintf("место %d: %s %v [%s, атака у %d, защита у %d, карт %d]",
					state.MySeat, action.Type, action.Payload, state.Phase,
					state.CanAttackSeat, state.DefenderSeat, len(state.MyHand)))
				player.send(envelope{Type: action.Type, TableID: &tableID,
					Payload: rawOf(t, action.Payload)})
				moves++
				break
			}
			settled.Reset(quiet)
		}
	}
}

// readOnce — чтение без t.Fatal: зовётся из отдельной goroutine, где падать нельзя.
func (c *client) readOnce() (envelope, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, raw, err := c.conn.Read(ctx)
	if err != nil {
		return envelope{}, err
	}
	var message envelope
	if err := json.Unmarshal(raw, &message); err != nil {
		return envelope{}, err
	}
	return message, nil
}

// typesOf — что уже лежит непрочитанным. Для разбора неудачного ожидания.
func typesOf(messages []envelope) string {
	types := make([]string, 0, len(messages))
	for _, message := range messages {
		types = append(types, message.Type)
	}
	return strings.Join(types, ", ")
}

func contains(cards []string, code string) bool {
	for _, card := range cards {
		if card == code {
			return true
		}
	}
	return false
}

// Готовность сразу вслед за посадкой, без ожидания ответа.
//
// ⚠️ Так делает нетерпеливый клиент: две команды подряд в один сокет. Порядок исполнения
// обязан совпадать с порядком отправки — стол исполняет команды одной очередью. Обгони
// готовность посадку, игрок получил бы NOT_AT_TABLE на ровном месте.
//
// ⭐ Проверяется своя готовность, а НЕ чужая. События, разосланные до твоей подписки,
// до тебя не доходят вовсе: подписка заводится посадкой, и всё, что случилось за столом
// раньше, клиент обязан добрать состоянием через REST. То же самое в Java — и на этом
// первая версия теста ловила «пропавшую» готовность соседа, которой никто не терял.
func TestReadyRightAfterJoinIsAccepted(t *testing.T) {
	pool := testsupport.Postgres(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := config.Config{
		JWTSecret:       []byte("тестовый-секрет-достаточной-длины-32+"),
		InviteCodes:     []string{"bardak-2026"},
		TurnTimeout:     30 * time.Second,
		DisconnectGrace: 10 * time.Second,
		ShutdownTimeout: time.Second,
	}
	handler, shutdown := server.Build(ctx, cfg, pool, observability.NewLogger())
	defer shutdown()
	httpServer := httptest.NewServer(handler)
	defer httpServer.Close()

	// Пять попыток: гонка, если она есть, воспроизводится не с первого раза.
	for attempt := 0; attempt < 5; attempt++ {
		host := newClient(t, httpServer.URL, "хозяин")
		guest := newClient(t, httpServer.URL, "гость")
		tableID := host.createTable()

		host.connect()
		guest.connect()

		host.send(envelope{Type: "TABLE_JOIN", TableID: &tableID})
		guest.send(envelope{Type: "TABLE_JOIN", TableID: &tableID})
		host.send(envelope{Type: "TABLE_READY", TableID: &tableID})
		guest.send(envelope{Type: "TABLE_READY", TableID: &tableID})

		// Каждый обязан увидеть СВОЮ готовность: значит, его посадка исполнилась раньше
		// его же готовности. Отказ пришёл бы этому же сокету и уронил бы ожидание.
		host.await("PLAYER_READY")
		guest.await("PLAYER_READY")

		host.close()
		guest.close()
	}
}

// Присутствие: друг «в сети» ровно пока открыт его сокет.
//
// ⚠️ Этот тест появился после находки, которую не увидели ШЕСТЬДЕСЯТ модульных тестов
// и вся сверка контрактов: `Presence.Attach` в Go не звал НИКТО. Реестр присутствия был,
// сценарий друзей его читал, а класть в него было некому — друзья не загорались в сети
// никогда, и приглашение за стол не доходило по сокету ни разу, молча уезжая в push.
//
// ⭐ Поймал это прогон настоящих ботов (tools/smoke/friends.mjs) — не тест. Живая связка
// снова оказалась строже: обработчик сокета вообще не имел тестов, а «ручка есть, событие
// есть» проверке присутствия не помеха.
func TestFriendIsOnlineWhileTheSocketIsOpen(t *testing.T) {
	pool := testsupport.Postgres(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := config.Config{
		JWTSecret:       []byte("тестовый-секрет-достаточной-длины-32+"),
		InviteCodes:     []string{"bardak-2026"},
		TurnTimeout:     30 * time.Second,
		DisconnectGrace: 10 * time.Second,
		ShutdownTimeout: time.Second,
	}
	handler, shutdown := server.Build(ctx, cfg, pool, observability.NewLogger())
	defer shutdown()
	httpServer := httptest.NewServer(handler)
	defer httpServer.Close()

	host := newClient(t, httpServer.URL, "хозяин")
	friend := newClient(t, httpServer.URL, "друг")
	makeFriends(t, host, friend)

	if online(t, host, friend.userID) {
		t.Fatal("друг без сокета числится в сети")
	}

	friend.connect()
	if !online(t, host, friend.userID) {
		t.Fatal("друг с открытым сокетом не в сети: присутствие не заводится")
	}

	// ⭐ И то, ради чего присутствие вообще нужно: приглашение уходит по сокету, а не в push.
	tableID := host.createTable()
	var invite struct {
		Delivered bool `json:"delivered"`
	}
	host.post("/api/friends/"+friend.userID+"/invite",
		map[string]any{"tableId": tableID}, host.token, &invite)
	if !invite.Delivered {
		t.Fatal("приглашение не доставлено по сокету, хотя друг в сети")
	}
	if got := friend.await("TABLE_INVITE"); got.Type != "TABLE_INVITE" {
		t.Fatalf("другу пришло %q вместо приглашения", got.Type)
	}

	friend.close()
	// Закрытие сокета видно не мгновенно: сервер узнаёт о нём из своего цикла чтения.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && online(t, host, friend.userID) {
		time.Sleep(20 * time.Millisecond)
	}
	if online(t, host, friend.userID) {
		t.Fatal("после закрытия сокета друг всё ещё в сети")
	}
}

// makeFriends заводит дружбу: заявка и согласие.
func makeFriends(t *testing.T, from, to *client) {
	t.Helper()
	from.post("/api/friends/requests", map[string]any{"username": to.username}, from.token, nil)
	to.post("/api/friends/"+from.userID+"/accept", map[string]any{}, to.token, nil)
}

// online — видит ли спрашивающий друга в сети.
func online(t *testing.T, asking *client, friendID string) bool {
	t.Helper()
	var friends struct {
		Friends []struct {
			UserID string `json:"userId"`
			Online bool   `json:"online"`
		} `json:"friends"`
	}
	asking.get("/api/friends", &friends)
	for _, item := range friends.Friends {
		if item.UserID == friendID {
			return item.Online
		}
	}
	t.Fatalf("%s не видит друга %s в своём списке", asking.name, friendID)
	return false
}

// Вход не должен спотыкаться о пробел в логине.
//
// ⚠️ Живая жалоба: «пароль запомнил правильно, а не пускает». Телефонная клавиатура
// и автозаполнение дописывают хвостовой пробел, и «shabdan » с верным паролем получал
// то же «неверный логин или пароль», что и злоумышленник. Регистр к тому моменту уже
// прощался (миграция 0010), а пробел — нет, и увидеть его глазами невозможно.
//
// ⭐ Пароль при этом обрезать нельзя: пробел в нём — законный символ.
func TestLoginForgivesSpacesAroundTheUsernameButNotInThePassword(t *testing.T) {
	pool := testsupport.Postgres(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := config.Config{
		JWTSecret:       []byte("тестовый-секрет-достаточной-длины-32+"),
		InviteCodes:     []string{"bardak-2026"},
		TurnTimeout:     30 * time.Second,
		DisconnectGrace: 2 * time.Second,
		ShutdownTimeout: time.Second,
	}
	handler, shutdown := server.Build(ctx, cfg, pool, observability.NewLogger())
	defer shutdown()

	httpServer := httptest.NewServer(handler)
	defer httpServer.Close()

	player := newClient(t, httpServer.URL, "пробельный")
	const password = "пароль-достаточной-длины"

	login := func(username, pass string) int {
		t.Helper()
		raw, err := json.Marshal(map[string]any{"username": username, "password": pass})
		if err != nil {
			t.Fatal(err)
		}
		response, err := http.Post(httpServer.URL+"/api/auth/login",
			"application/json", bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		return response.StatusCode
	}

	for _, probe := range []struct {
		name     string
		username string
		password string
		want     int
	}{
		{"как регистрировали", player.username, password, http.StatusOK},
		{"пробел в конце логина", player.username + " ", password, http.StatusOK},
		{"пробел в начале логина", " " + player.username, password, http.StatusOK},
		{"логин заглавными", strings.ToUpper(player.username), password, http.StatusOK},
		{"пробел в конце пароля", player.username, password + " ", http.StatusUnauthorized},
		{"неверный пароль", player.username, "совсем-другой-пароль", http.StatusUnauthorized},
	} {
		if got := login(probe.username, probe.password); got != probe.want {
			t.Errorf("%s: вход ответил %d, ждали %d", probe.name, got, probe.want)
		}
	}
}
