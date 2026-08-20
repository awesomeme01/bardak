package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/awesomeme01/bardak/back-go/internal/repository"
)

// Правила дружбы проверяются БЕЗ базы: здесь главное — кто кому что может, а не то,
// как строка ложится в Postgres (это проверяет репозиторий на настоящем сервере).

func TestFriendRequestFindsTheLoginIgnoringCase(t *testing.T) {
	friends, store, _ := newSocialFixture()
	store.users["fr-target"] = repository.User{ID: "22222222-2222-4222-8222-222222222222",
		Username: "fr-Target", DisplayName: "Цель"}

	friend, err := friends.Request(context.Background(), socialMe, "  FR-TARGET ")
	if err != nil {
		t.Fatalf("заявка не прошла: %v", err)
	}

	// Логин вводят руками с мобильной клавиатуры, которая заглавит первую букву сама;
	// в ответ уходит оригинальное написание, а не то, что набрали.
	if friend.Username != "fr-Target" {
		t.Errorf("логин в ответе %q, ждали оригинальное написание", friend.Username)
	}
	if friend.Status != repository.FriendshipPending || !friend.Mine {
		t.Errorf("новая заявка обязана быть моей и висящей: %+v", friend)
	}
}

// ⚠️ Двое нажали «добавить» одновременно. Встречная заявка — это СОГЛАСИЕ, а не вторая
// заявка: иначе оба остались бы с висящими приглашениями и без дружбы, и выбраться
// из этого состояния было бы нечем.
func TestCounterRequestIsTakenAsConsent(t *testing.T) {
	friends, store, _ := newSocialFixture()
	other := store.addUser("fr-mutual", "Друг")
	if _, err := friends.Request(context.Background(), other, "fr-me"); err != nil {
		t.Fatal(err)
	}

	friend, err := friends.Request(context.Background(), socialMe, "fr-mutual")
	if err != nil {
		t.Fatalf("встречная заявка отбита: %v", err)
	}

	if friend.Status != repository.FriendshipAccepted {
		t.Errorf("встречная заявка обязана давать дружбу, а не %q", friend.Status)
	}
	if store.pairs[socialPairKey(socialMe, other)].DecidedAt == nil {
		t.Error("decided_at обязан проставиться в момент согласия")
	}
	mutual, err := friends.IsFriend(context.Background(), other, socialMe)
	if err != nil || !mutual {
		t.Errorf("дружба обязана быть видна в обе стороны: %v, %v", mutual, err)
	}
}

func TestFriendRequestRefusals(t *testing.T) {
	ctx := context.Background()

	t.Run("сам себе", func(t *testing.T) {
		friends, store, _ := newSocialFixture()
		store.users["fr-me"] = repository.User{ID: socialMe, Username: "fr-me", DisplayName: "Я"}

		// Пара «сам с собой» сломала бы «второй участник» — он вернул бы меня же —
		// и раздвоила бы игрока в собственном списке.
		if _, err := friends.Request(ctx, socialMe, "fr-me"); !errors.Is(err, ErrCannotFriendSelf) {
			t.Errorf("ждали ErrCannotFriendSelf, получили %v", err)
		}
	})

	t.Run("логина нет", func(t *testing.T) {
		friends, _, _ := newSocialFixture()
		if _, err := friends.Request(ctx, socialMe, "нет-такого"); !errors.Is(err, ErrFriendLoginNotFound) {
			t.Errorf("ждали ErrFriendLoginNotFound, получили %v", err)
		}
	})

	t.Run("заявка уже висит", func(t *testing.T) {
		friends, store, _ := newSocialFixture()
		store.addUser("fr-twice", "Друг")
		if _, err := friends.Request(ctx, socialMe, "fr-twice"); err != nil {
			t.Fatal(err)
		}
		if _, err := friends.Request(ctx, socialMe, "fr-twice"); !errors.Is(err, ErrRequestAlreadySent) {
			t.Errorf("ждали ErrRequestAlreadySent, получили %v", err)
		}
	})

	t.Run("уже друзья", func(t *testing.T) {
		friends, store, _ := newSocialFixture()
		other := store.addUser("fr-already", "Друг")
		store.accept(socialMe, other)
		if _, err := friends.Request(ctx, socialMe, "fr-already"); !errors.Is(err, ErrAlreadyFriends) {
			t.Errorf("ждали ErrAlreadyFriends, получили %v", err)
		}
	})
}

// Принять заявку может только адресат: иначе дружба выдаётся в одностороннем порядке,
// а вместе с ней — доступ к присутствию и право звать за стол.
func TestAcceptOnlyByTheAddressee(t *testing.T) {
	ctx := context.Background()
	friends, store, _ := newSocialFixture()
	other := store.addUser("fr-acc", "Друг")
	if _, err := friends.Request(ctx, socialMe, "fr-acc"); err != nil {
		t.Fatal(err)
	}

	if _, err := friends.Accept(ctx, socialMe, other); !errors.Is(err, ErrNotYourRequest) {
		t.Errorf("автор заявки не принимает её сам: %v", err)
	}

	friend, err := friends.Accept(ctx, other, socialMe)
	if err != nil {
		t.Fatalf("адресат обязан принять заявку: %v", err)
	}
	if friend.Status != repository.FriendshipAccepted {
		t.Errorf("после принятия статус %q", friend.Status)
	}

	// Экран мог не успеть обновиться: повторное принятие — не ошибка, а тот же ответ.
	again, err := friends.Accept(ctx, other, socialMe)
	if err != nil || again.Status != repository.FriendshipAccepted {
		t.Errorf("повторное принятие обязано быть идемпотентным: %+v, %v", again, err)
	}
}

func TestAcceptAndRemoveWithoutAPairAnswerNotFound(t *testing.T) {
	ctx := context.Background()
	friends, store, _ := newSocialFixture()
	stranger := store.addUser("fr-stranger", "Незнакомец")

	if _, err := friends.Accept(ctx, socialMe, stranger); !errors.Is(err, ErrPairNotFound) {
		t.Errorf("принятие без пары: ждали ErrPairNotFound, получили %v", err)
	}
	if err := friends.Remove(ctx, socialMe, stranger); !errors.Is(err, ErrPairNotFound) {
		t.Errorf("удаление без пары: ждали ErrPairNotFound, получили %v", err)
	}
}

// ⭐ Убрать из друзей и отклонить заявку — одна операция: строки больше нет ни у кого.
// Односторонний разрыв оставил бы удалённому доступ к присутствию и право приглашать.
func TestRemoveDropsThePairForBoth(t *testing.T) {
	ctx := context.Background()
	friends, store, _ := newSocialFixture()
	other := store.addUser("fr-drop", "Друг")
	store.accept(socialMe, other)

	if err := friends.Remove(ctx, other, socialMe); err != nil {
		t.Fatalf("удаление: %v", err)
	}

	mine, err := friends.List(ctx, socialMe)
	if err != nil {
		t.Fatal(err)
	}
	his, err := friends.List(ctx, other)
	if err != nil {
		t.Fatal(err)
	}
	if len(mine.Friends) != 0 || len(his.Friends) != 0 {
		t.Error("дружба обязана исчезнуть сразу у обоих")
	}
}

// Список раскладывается по смыслу: одна и та же строка у отправителя попадает в outgoing,
// у получателя — в incoming, и ни у кого в friends, пока согласия нет.
func TestListSplitsPairsByMeaningAndPutsOnlineFirst(t *testing.T) {
	ctx := context.Background()
	friends, store, presence := newSocialFixture()

	online := store.addUser("fr-online", "Яна")
	offline := store.addUser("fr-offline", "антон")
	incoming := store.addUser("fr-incoming", "Проситель")
	outgoing := store.addUser("fr-outgoing", "Приглашённый")

	store.accept(socialMe, online)
	store.accept(socialMe, offline)
	store.request(incoming, socialMe)
	store.request(socialMe, outgoing)
	presence.online[online] = true

	list, err := friends.List(ctx, socialMe)
	if err != nil {
		t.Fatal(err)
	}

	if len(list.Incoming) != 1 || list.Incoming[0].UserID != incoming || list.Incoming[0].Mine {
		t.Errorf("входящая заявка разложена неверно: %+v", list.Incoming)
	}
	if len(list.Outgoing) != 1 || list.Outgoing[0].UserID != outgoing || !list.Outgoing[0].Mine {
		t.Errorf("исходящая заявка разложена неверно: %+v", list.Outgoing)
	}
	if len(list.Friends) != 2 {
		t.Fatalf("друзей должно быть двое, получили %d", len(list.Friends))
	}
	// Онлайн — наверх: за стол зовут тех, кто сейчас здесь. Внутри — по имени без учёта
	// регистра, иначе «антон» уехал бы за «Яну».
	if list.Friends[0].UserID != online || !list.Friends[0].Online {
		t.Errorf("наверху обязан быть тот, кто в сети: %+v", list.Friends)
	}
	if list.Friends[1].Online {
		t.Error("присутствие считается по живому сокету, а не назначается всем подряд")
	}
}

func TestListSortsFriendsByNameIgnoringCase(t *testing.T) {
	friends, store, _ := newSocialFixture()
	for _, name := range []string{"яна", "Антон", "борис"} {
		store.accept(socialMe, store.addUser("fr-sort-"+name, name))
	}

	list, err := friends.List(context.Background(), socialMe)
	if err != nil {
		t.Fatal(err)
	}

	got := []string{list.Friends[0].DisplayName, list.Friends[1].DisplayName, list.Friends[2].DisplayName}
	want := []string{"Антон", "борис", "яна"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("порядок имён %v, ждали %v — сравнение без учёта регистра", got, want)
		}
	}
}

// ⚠️ Звать за стол можно только друга: иначе приглашение становится способом писать
// незнакомым людям. Проверка в сценарии, а не на экране — запрос приходит из сети.
func TestInviteRequiresAnAcceptedFriendship(t *testing.T) {
	ctx := context.Background()
	friends, store, _ := newSocialFixture()
	stranger := store.addUser("fr-invite-stranger", "Незнакомец")
	store.request(socialMe, stranger) // висящая заявка правом звать не считается

	if _, err := friends.Invite(ctx, socialMe, stranger, socialTableID); !errors.Is(err, ErrNotFriends) {
		t.Errorf("ждали ErrNotFriends, получили %v", err)
	}
}

func TestInviteTellsWhetherItWasHeard(t *testing.T) {
	ctx := context.Background()
	friends, store, _ := newSocialFixture()
	friend := store.addUser("fr-invite", "Друг")
	store.accept(socialMe, friend)

	delivered, err := friends.Invite(ctx, socialMe, friend, socialTableID)
	if err != nil {
		t.Fatalf("приглашение: %v", err)
	}
	if !delivered {
		t.Error("друг в сети — приглашение обязано считаться доставленным")
	}
	if store.invites.lastFrom != "Я" || store.invites.lastTable.Code != "ABC123" {
		t.Errorf("в приглашение уходит имя зовущего и код стола: %+v", store.invites)
	}

	// Не дошло по сокету — это не ошибка: друга просто нет в сети, и его позовут push-ом.
	store.invites.deliver = false
	delivered, err = friends.Invite(ctx, socialMe, friend, socialTableID)
	if err != nil || delivered {
		t.Errorf("недоставленное приглашение — не ошибка: %v, %v", delivered, err)
	}
}

func TestInviteToAMissingTableIsNotFound(t *testing.T) {
	friends, store, _ := newSocialFixture()
	friend := store.addUser("fr-invite-404", "Друг")
	store.accept(socialMe, friend)

	_, err := friends.Invite(context.Background(), socialMe, friend, "77777777-7777-4777-8777-777777777777")
	if !errors.Is(err, ErrInviteTableNotFound) {
		t.Errorf("ждали ErrInviteTableNotFound, получили %v", err)
	}
}

// ⭐ Без ключей VAPID уведомления просто выключены — это штатный режим, а не поломка:
// локально играют с открытой вкладкой.
func TestPushIsDisabledWithoutBothKeys(t *testing.T) {
	cases := map[string]PushSubscriptionService{
		"нет обоих":     NewPushSubscriptionService(&socialPushFake{}, "", ""),
		"нет закрытого": NewPushSubscriptionService(&socialPushFake{}, "public", "  "),
		"нет открытого": NewPushSubscriptionService(&socialPushFake{}, "", "private"),
	}
	for name, service := range cases {
		if service.Enabled() || service.PublicKey() != "" {
			t.Errorf("%s: уведомления обязаны быть выключены", name)
		}
	}

	enabled := NewPushSubscriptionService(&socialPushFake{}, "public", "private")
	if !enabled.Enabled() || enabled.PublicKey() != "public" {
		t.Error("с обоими ключами уведомления обязаны включаться")
	}
}

func TestPushUnsubscribePassesTheOwner(t *testing.T) {
	store := &socialPushFake{}
	service := NewPushSubscriptionService(store, "public", "private")
	ctx := context.Background()

	if err := service.Subscribe(ctx, socialMe, "https://push/1", "p", "a", nil); err != nil {
		t.Fatal(err)
	}
	if store.saved.ID == "" {
		t.Error("подписке обязан назначаться идентификатор — он нужен при вставке")
	}

	if err := service.Unsubscribe(ctx, socialMe, "https://push/1"); err != nil {
		t.Fatal(err)
	}
	// ⚠️ Владелец обязан доехать до запроса: без него чужой endpoint отписывал бы кто угодно.
	if store.deletedEndpoint != "https://push/1" || store.deletedUser != socialMe {
		t.Errorf("отписка ушла без владельца: %+v", store)
	}
}

// --- подделки ---

const (
	socialMe      = "11111111-1111-4111-8111-111111111111"
	socialTableID = "33333333-3333-4333-8333-333333333333"
)

func newSocialFixture() (FriendService, *socialStoreFake, *socialPresenceFake) {
	store := &socialStoreFake{
		pairs: map[string]repository.Friendship{},
		users: map[string]repository.User{
			"fr-me": {ID: socialMe, Username: "fr-me", DisplayName: "Я"},
		},
		invites: &socialInvitesFake{deliver: true},
	}
	presence := &socialPresenceFake{online: map[string]bool{}}
	service := NewFriendService(store, store, presence, store.invites, store,
		func() time.Time { return time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC) })
	return service, store, presence
}

func socialPairKey(one, two string) string {
	low, high := repository.OrderPair(one, two)
	return low + "|" + high
}

// socialStoreFake разом изображает пары, людей и лобби: сценарию нужны все трое, а
// правило у них одно — «одна строка на пару».
type socialStoreFake struct {
	pairs   map[string]repository.Friendship
	users   map[string]repository.User
	invites *socialInvitesFake
}

func (s *socialStoreFake) addUser(username, displayName string) string {
	id := uuid.NewString()
	s.users[username] = repository.User{ID: id, Username: username, DisplayName: displayName}
	return id
}

func (s *socialStoreFake) request(from, to string) {
	low, high := repository.OrderPair(from, to)
	s.pairs[socialPairKey(from, to)] = repository.Friendship{
		LowUserID: low, HighUserID: high, RequestedBy: from,
		Status: repository.FriendshipPending, CreatedAt: time.Now(),
	}
}

func (s *socialStoreFake) accept(one, two string) {
	s.request(one, two)
	decided := time.Now()
	pair := s.pairs[socialPairKey(one, two)]
	pair.Status = repository.FriendshipAccepted
	pair.DecidedAt = &decided
	s.pairs[socialPairKey(one, two)] = pair
}

func (s *socialStoreFake) FindPair(_ context.Context, one, two string) (repository.Friendship, error) {
	pair, ok := s.pairs[socialPairKey(one, two)]
	if !ok {
		return repository.Friendship{}, repository.ErrNotFound
	}
	return pair, nil
}

func (s *socialStoreFake) FindAllInvolving(_ context.Context, userID string) ([]repository.FriendPair, error) {
	pairs := []repository.FriendPair{}
	for _, pair := range s.pairs {
		if !pair.Involves(userID) {
			continue
		}
		other := pair.OtherThan(userID)
		for _, user := range s.users {
			if user.ID == other {
				pairs = append(pairs, repository.FriendPair{Pair: pair, Other: repository.FriendProfile{
					ID: user.ID, Username: user.Username, DisplayName: user.DisplayName,
				}})
			}
		}
	}
	return pairs, nil
}

func (s *socialStoreFake) IsFriend(_ context.Context, one, two string) (bool, error) {
	pair, ok := s.pairs[socialPairKey(one, two)]
	return ok && pair.IsAccepted(), nil
}

func (s *socialStoreFake) Insert(_ context.Context, from, to string) (repository.Friendship, error) {
	if _, ok := s.pairs[socialPairKey(from, to)]; ok {
		return repository.Friendship{}, repository.ErrConflict
	}
	s.request(from, to)
	return s.pairs[socialPairKey(from, to)], nil
}

func (s *socialStoreFake) Accept(_ context.Context, one, two string, at time.Time) (repository.Friendship, error) {
	pair, ok := s.pairs[socialPairKey(one, two)]
	if !ok {
		return repository.Friendship{}, repository.ErrNotFound
	}
	pair.Status = repository.FriendshipAccepted
	pair.DecidedAt = &at
	s.pairs[socialPairKey(one, two)] = pair
	return pair, nil
}

func (s *socialStoreFake) Delete(_ context.Context, one, two string) error {
	key := socialPairKey(one, two)
	if _, ok := s.pairs[key]; !ok {
		return repository.ErrNotFound
	}
	delete(s.pairs, key)
	return nil
}

func (s *socialStoreFake) FindByUsernameIgnoreCase(_ context.Context, username string) (repository.User, error) {
	for login, user := range s.users {
		if strings.EqualFold(strings.TrimSpace(login), strings.TrimSpace(username)) {
			return user, nil
		}
	}
	return repository.User{}, repository.ErrNotFound
}

func (s *socialStoreFake) FindByID(_ context.Context, id string) (repository.User, error) {
	for _, user := range s.users {
		if user.ID == id {
			return user, nil
		}
	}
	return repository.User{}, repository.ErrNotFound
}

func (s *socialStoreFake) InviteTableByID(_ context.Context, tableID string) (InviteTable, error) {
	if tableID != socialTableID {
		return InviteTable{}, ErrInviteTableNotFound
	}
	return InviteTable{ID: socialTableID, Name: "Вечерний стол", Code: "ABC123"}, nil
}

type socialPresenceFake struct{ online map[string]bool }

func (p *socialPresenceFake) IsOnline(userID string) bool { return p.online[userID] }

type socialInvitesFake struct {
	deliver   bool
	lastFrom  string
	lastTable InviteTable
}

func (i *socialInvitesFake) SendTableInvite(_ context.Context, _, fromName string, table InviteTable) bool {
	i.lastFrom = fromName
	i.lastTable = table
	return i.deliver
}

type socialPushFake struct {
	saved           repository.PushSubscription
	deletedEndpoint string
	deletedUser     string
}

func (p *socialPushFake) Save(_ context.Context, sub repository.PushSubscription) (repository.PushSubscription, error) {
	p.saved = sub
	return sub, nil
}

func (p *socialPushFake) DeleteByEndpointAndUserID(_ context.Context, endpoint, userID string) (int64, error) {
	p.deletedEndpoint, p.deletedUser = endpoint, userID
	return 1, nil
}
