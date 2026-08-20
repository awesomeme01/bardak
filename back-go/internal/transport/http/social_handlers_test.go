package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/awesomeme01/bardak/back-go/internal/application"
)

// Обработчики друзей и push проверяются БЕЗ базы: здесь важны коды, статусы и форма
// ответа на проводе — то, что видит фронт и сверяет differential-проверка.

const socialViewer = "11111111-1111-4111-8111-111111111111"

// ⚠️ Проверка сборки на этапе компиляции: обработчики объявляют узкие интерфейсы, но
// собираются они из настоящих сценариев. Разъехавшаяся сигнатура должна ломать тесты
// здесь, а не при сборке main.
var (
	_ friendScenarios = application.FriendService{}
	_ pushScenarios   = application.PushSubscriptionService{}
)

// ⚠️ Пустой список — это [], а не null: Java всегда отдаёт массивы, а nil-слайс в Go
// сериализовался бы в null и молча сломал бы экран (MD-003).
func TestFriendListSendsEmptyArraysNotNull(t *testing.T) {
	handlers := SocialHandlers{Friends: &socialFriendsStub{}, Push: &socialPushStub{}}

	response := socialCall(t, handlers, http.MethodGet, "/api/friends", "")

	if response.Code != http.StatusOK {
		t.Fatalf("статус %d, ждали 200", response.Code)
	}
	want := `{"friends":[],"incoming":[],"outgoing":[]}`
	if got := strings.TrimSpace(response.Body.String()); got != want {
		t.Errorf("получили %s, ждали %s", got, want)
	}
}

// ⚠️ MD-003: Java вырезает только null. avatar может отсутствовать, а online, mine
// и status — нет, даже когда это false и пустая строка.
func TestFriendViewCutsOnlyTheAvatar(t *testing.T) {
	handlers := SocialHandlers{Push: &socialPushStub{}, Friends: &socialFriendsStub{
		list: application.FriendList{Friends: []application.Friend{{
			UserID: "22222222-2222-4222-8222-222222222222", Username: "sosed",
			DisplayName: "Сосед", Status: "ACCEPTED",
		}}},
	}}

	response := socialCall(t, handlers, http.MethodGet, "/api/friends", "")

	body := response.Body.String()
	if strings.Contains(body, `"avatar"`) {
		t.Errorf("пустая мордочка обязана вырезаться целиком: %s", body)
	}
	for _, field := range []string{`"online":false`, `"mine":false`, `"status":"ACCEPTED"`} {
		if !strings.Contains(body, field) {
			t.Errorf("поле %s обязано остаться в ответе: %s", field, body)
		}
	}
}

func TestAddFriendValidation(t *testing.T) {
	cases := map[string]struct {
		body  string
		field string
		want  string
	}{
		"пусто":          {`{"username":"   "}`, "username", "must not be blank"},
		"длиннее 32":     {`{"username":"` + strings.Repeat("щ", 33) + `"}`, "username", "size must be between 0 and 32"},
		"поля вовсе нет": {`{}`, "username", "must not be blank"},
	}
	for name, item := range cases {
		t.Run(name, func(t *testing.T) {
			handlers := SocialHandlers{Friends: &socialFriendsStub{}, Push: &socialPushStub{}}

			response := socialCall(t, handlers, http.MethodPost, "/api/friends/requests", item.body)

			if response.Code != http.StatusBadRequest {
				t.Fatalf("статус %d, ждали 400", response.Code)
			}
			var failure APIError
			if err := json.Unmarshal(response.Body.Bytes(), &failure); err != nil {
				t.Fatal(err)
			}
			if failure.Code != "VALIDATION_FAILED" {
				t.Errorf("код %q, ждали VALIDATION_FAILED", failure.Code)
			}
			// ⚠️ details — часть контракта, а не отладочная роскошь: экран подсвечивает
			// по нему конкретное поле.
			if failure.Details[item.field] != item.want {
				t.Errorf("details[%s] = %v, ждали %q", item.field, failure.Details[item.field], item.want)
			}
		})
	}
}

// ⚠️ Один код с ДВУМЯ статусами: NOT_FRIENDS — 404, когда пары нет вовсе, и 403, когда
// зовут за стол не друга. Одна константа на код сломала бы контракт.
func TestNotFriendsLivesWithTwoStatuses(t *testing.T) {
	handlers := SocialHandlers{Push: &socialPushStub{}, Friends: &socialFriendsStub{
		removeErr: application.ErrPairNotFound,
		inviteErr: application.ErrNotFriends,
	}}

	missing := socialCall(t, handlers, http.MethodDelete,
		"/api/friends/22222222-2222-4222-8222-222222222222", "")
	stranger := socialCall(t, handlers, http.MethodPost,
		"/api/friends/22222222-2222-4222-8222-222222222222/invite",
		`{"tableId":"33333333-3333-4333-8333-333333333333"}`)

	if missing.Code != http.StatusNotFound || socialErrorCode(t, missing.Body.Bytes()) != "NOT_FRIENDS" {
		t.Errorf("пары нет: ждали 404 NOT_FRIENDS, получили %d %s", missing.Code, missing.Body)
	}
	if stranger.Code != http.StatusForbidden || socialErrorCode(t, stranger.Body.Bytes()) != "NOT_FRIENDS" {
		t.Errorf("не друг: ждали 403 NOT_FRIENDS, получили %d %s", stranger.Code, stranger.Body)
	}
}

func TestFriendErrorsKeepTheirCodesAndStatuses(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"логина нет", application.ErrFriendLoginNotFound, http.StatusNotFound, "USER_NOT_FOUND"},
		{"пропал сам спрашивающий", application.ErrUserNotFound, http.StatusNotFound, "USER_NOT_FOUND"},
		{"сам себе", application.ErrCannotFriendSelf, http.StatusConflict, "CANNOT_FRIEND_SELF"},
		{"уже друзья", application.ErrAlreadyFriends, http.StatusConflict, "ALREADY_FRIENDS"},
		{"заявка висит", application.ErrRequestAlreadySent, http.StatusConflict, "REQUEST_ALREADY_SENT"},
		{"неизвестная поломка", errors.New("база упала"), http.StatusInternalServerError, "INTERNAL_ERROR"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			handlers := SocialHandlers{Push: &socialPushStub{},
				Friends: &socialFriendsStub{requestErr: item.err}}

			response := socialCall(t, handlers, http.MethodPost, "/api/friends/requests",
				`{"username":"sosed"}`)

			if response.Code != item.status {
				t.Errorf("статус %d, ждали %d", response.Code, item.status)
			}
			if code := socialErrorCode(t, response.Body.Bytes()); code != item.code {
				t.Errorf("код %q, ждали %q", code, item.code)
			}
		})
	}
}

func TestAcceptAnswersTwoHundredAndRemoveAnswersNoContent(t *testing.T) {
	handlers := SocialHandlers{Push: &socialPushStub{}, Friends: &socialFriendsStub{
		accepted: application.Friend{UserID: "22222222-2222-4222-8222-222222222222",
			Username: "sosed", DisplayName: "Сосед", Status: "ACCEPTED"},
	}}

	accepted := socialCall(t, handlers, http.MethodPost,
		"/api/friends/22222222-2222-4222-8222-222222222222/accept", "")
	removed := socialCall(t, handlers, http.MethodDelete,
		"/api/friends/22222222-2222-4222-8222-222222222222", "")

	if accepted.Code != http.StatusOK || !strings.Contains(accepted.Body.String(), `"status":"ACCEPTED"`) {
		t.Errorf("принятие: %d %s", accepted.Code, accepted.Body)
	}
	if removed.Code != http.StatusNoContent || removed.Body.Len() != 0 {
		t.Errorf("удаление обязано отвечать 204 без тела: %d %s", removed.Code, removed.Body)
	}
}

// ⚠️ Мусор вместо UUID в пути — 400, а не 500: в Java его ловит обработчик
// MethodArgumentTypeMismatchException.
func TestBrokenFriendUUIDInPathIsBadRequest(t *testing.T) {
	handlers := SocialHandlers{Friends: &socialFriendsStub{}, Push: &socialPushStub{}}

	response := socialCall(t, handlers, http.MethodPost, "/api/friends/не-uuid/accept", "")

	if response.Code != http.StatusBadRequest || socialErrorCode(t, response.Body.Bytes()) != "BAD_REQUEST" {
		t.Errorf("получили %d %s, ждали 400 BAD_REQUEST", response.Code, response.Body)
	}
}

// ⚠️ ПОВТОРЕНИЕ поведения Java, а не задумка: tableId объявлен строкой и разбирается
// вручную уже в контроллере, поэтому мусор проваливается в общий обработчик и даёт 500.
// Тест закрепляет расхождение, чтобы оно не «исправилось» само и не разошлось с Java.
func TestBrokenTableIDInInviteRepeatsTheJavaFiveHundred(t *testing.T) {
	handlers := SocialHandlers{Friends: &socialFriendsStub{}, Push: &socialPushStub{}}

	response := socialCall(t, handlers, http.MethodPost,
		"/api/friends/22222222-2222-4222-8222-222222222222/invite", `{"tableId":"мусор"}`)

	if response.Code != http.StatusInternalServerError {
		t.Errorf("статус %d, а Java на битом tableId отвечает 500", response.Code)
	}
}

func TestInviteTellsWhetherTheFriendHeardIt(t *testing.T) {
	stub := &socialFriendsStub{delivered: true}
	handlers := SocialHandlers{Friends: stub, Push: &socialPushStub{}}

	heard := socialCall(t, handlers, http.MethodPost,
		"/api/friends/22222222-2222-4222-8222-222222222222/invite",
		`{"tableId":"33333333-3333-4333-8333-333333333333"}`)
	stub.delivered = false
	missed := socialCall(t, handlers, http.MethodPost,
		"/api/friends/22222222-2222-4222-8222-222222222222/invite",
		`{"tableId":"33333333-3333-4333-8333-333333333333"}`)

	if got := strings.TrimSpace(heard.Body.String()); got != `{"delivered":true}` {
		t.Errorf("получили %s, ждали {\"delivered\":true}", got)
	}
	// ⚠️ delivered:false обязан остаться в теле: с omitempty он вырезался бы, и экран
	// не смог бы отличить «позвал» от «его нет в сети».
	if got := strings.TrimSpace(missed.Body.String()); got != `{"delivered":false}` {
		t.Errorf("получили %s, ждали {\"delivered\":false}", got)
	}
	if stub.invitedFriend != "22222222-2222-4222-8222-222222222222" {
		t.Errorf("до сценария не доехал друг из пути: %q", stub.invitedFriend)
	}
}

// ⚠️ Две разные формы ответа: с ключом и БЕЗ поля publicKey вовсе. enabled=false —
// не поломка: клиент по этому признаку просто не показывает кнопку подписки.
func TestPushKeyHasTwoShapes(t *testing.T) {
	off := socialCall(t, SocialHandlers{Friends: &socialFriendsStub{}, Push: &socialPushStub{}},
		http.MethodGet, "/api/push/key", "")
	on := socialCall(t, SocialHandlers{Friends: &socialFriendsStub{},
		Push: &socialPushStub{enabled: true, key: "BPublicKey"}}, http.MethodGet, "/api/push/key", "")

	if got := strings.TrimSpace(off.Body.String()); got != `{"enabled":false}` {
		t.Errorf("выключенные уведомления: получили %s", got)
	}
	if got := strings.TrimSpace(on.Body.String()); got != `{"enabled":true,"publicKey":"BPublicKey"}` {
		t.Errorf("включённые уведомления: получили %s", got)
	}
}

func TestSubscribeStoresTheDeviceAndAnswersNoContent(t *testing.T) {
	push := &socialPushStub{}
	handlers := SocialHandlers{Friends: &socialFriendsStub{}, Push: push}

	response := socialCall(t, handlers, http.MethodPost, "/api/push/subscriptions",
		`{"endpoint":"https://push.example/1","p256dh":"key","auth":"auth"}`)

	if response.Code != http.StatusNoContent || response.Body.Len() != 0 {
		t.Fatalf("получили %d %s, ждали 204 без тела", response.Code, response.Body)
	}
	if push.endpoint != "https://push.example/1" || push.userID != socialViewer {
		t.Errorf("подписка ушла не туда: %+v", push)
	}
	if push.userAgent == nil || *push.userAgent != "TestAgent/1.0" {
		t.Error("User-Agent обязан сохраняться: по нему потом видно, что за устройство")
	}
}

// ⚠️ DELETE С ТЕЛОМ — так в Java, и контракт менять нельзя. Владелец при этом обязан
// доехать до сценария: без него любой вошедший отписывал бы чужое устройство.
func TestUnsubscribeReadsTheBodyAndPassesTheOwner(t *testing.T) {
	push := &socialPushStub{}
	handlers := SocialHandlers{Friends: &socialFriendsStub{}, Push: push}

	response := socialCall(t, handlers, http.MethodDelete, "/api/push/subscriptions",
		`{"endpoint":"https://push.example/1"}`)

	if response.Code != http.StatusNoContent {
		t.Fatalf("получили %d %s, ждали 204", response.Code, response.Body)
	}
	if push.unsubscribedEndpoint != "https://push.example/1" || push.unsubscribedUser != socialViewer {
		t.Errorf("отписка ушла без владельца или без адреса: %+v", push)
	}
}

func TestPushSubscribeValidation(t *testing.T) {
	handlers := SocialHandlers{Friends: &socialFriendsStub{}, Push: &socialPushStub{}}

	response := socialCall(t, handlers, http.MethodPost, "/api/push/subscriptions",
		`{"endpoint":"","p256dh":"key","auth":""}`)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("статус %d, ждали 400", response.Code)
	}
	var failure APIError
	if err := json.Unmarshal(response.Body.Bytes(), &failure); err != nil {
		t.Fatal(err)
	}
	if failure.Details["endpoint"] != "must not be blank" || failure.Details["auth"] != "must not be blank" {
		t.Errorf("ждали разбор по обоим пустым полям, получили %v", failure.Details)
	}
	if _, extra := failure.Details["p256dh"]; extra {
		t.Error("заполненное поле не должно попадать в разбор ошибок")
	}
}

// Токен проверяет посредник, но обработчик обязан пережить и его отсутствие.
//
// ⚠️ Тело у 401 ПУСТОЕ и без формата ApiError — так отвечает Spring Security.
func TestSocialHandlersAnswerUnauthorizedWithoutAPrincipal(t *testing.T) {
	router := chi.NewRouter()
	SocialHandlers{Friends: &socialFriendsStub{}, Push: &socialPushStub{}}.Routes(router)

	request := httptest.NewRequest(http.MethodGet, "/api/friends", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("статус %d, ждали 401", response.Code)
	}
	if response.Body.Len() != 0 {
		t.Errorf("тело 401 обязано быть пустым, получили %s", response.Body)
	}
	if response.Header().Get("WWW-Authenticate") != "Bearer" {
		t.Error("у 401 обязан быть заголовок WWW-Authenticate")
	}
}

// --- вспомогательное ---

// socialCall прогоняет запрос через настоящий маршрутизатор: пути и методы — тоже часть
// контракта, и проверять их в обход роутера значит не проверять вовсе.
func socialCall(t *testing.T, handlers SocialHandlers, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	router := chi.NewRouter()
	handlers.Routes(router)

	var request *http.Request
	if body == "" {
		request = httptest.NewRequest(method, path, nil)
	} else {
		request = httptest.NewRequest(method, path, strings.NewReader(body))
	}
	request.Header.Set("User-Agent", "TestAgent/1.0")
	request = request.WithContext(context.WithValue(request.Context(), principalKey{},
		Principal{UserID: socialViewer, Username: "viewer"}))

	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func socialErrorCode(t *testing.T, body []byte) string {
	t.Helper()
	var failure APIError
	if err := json.Unmarshal(body, &failure); err != nil {
		t.Fatalf("ответ об ошибке не разобрать: %v (%s)", err, body)
	}
	return failure.Code
}

type socialFriendsStub struct {
	list          application.FriendList
	accepted      application.Friend
	delivered     bool
	invitedFriend string
	requestErr    error
	acceptErr     error
	removeErr     error
	inviteErr     error
}

func (s *socialFriendsStub) List(context.Context, string) (application.FriendList, error) {
	list := s.list
	if list.Friends == nil {
		list.Friends = []application.Friend{}
	}
	if list.Incoming == nil {
		list.Incoming = []application.Friend{}
	}
	if list.Outgoing == nil {
		list.Outgoing = []application.Friend{}
	}
	return list, nil
}

func (s *socialFriendsStub) Request(context.Context, string, string) (application.Friend, error) {
	return s.accepted, s.requestErr
}

func (s *socialFriendsStub) Accept(context.Context, string, string) (application.Friend, error) {
	return s.accepted, s.acceptErr
}

func (s *socialFriendsStub) Remove(context.Context, string, string) error { return s.removeErr }

func (s *socialFriendsStub) Invite(_ context.Context, _, friendID, _ string) (bool, error) {
	s.invitedFriend = friendID
	return s.delivered, s.inviteErr
}

type socialPushStub struct {
	enabled              bool
	key                  string
	userID               string
	endpoint             string
	userAgent            *string
	unsubscribedEndpoint string
	unsubscribedUser     string
}

func (p *socialPushStub) Enabled() bool { return p.enabled }

func (p *socialPushStub) PublicKey() string {
	if !p.enabled {
		return ""
	}
	return p.key
}

func (p *socialPushStub) Subscribe(_ context.Context, userID, endpoint, _, _ string, userAgent *string) error {
	p.userID, p.endpoint, p.userAgent = userID, endpoint, userAgent
	return nil
}

func (p *socialPushStub) Unsubscribe(_ context.Context, userID, endpoint string) error {
	p.unsubscribedUser, p.unsubscribedEndpoint = userID, endpoint
	return nil
}
