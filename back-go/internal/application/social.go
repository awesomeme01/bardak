package application

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/awesomeme01/bardak/back-go/internal/repository"
)

// Друзья и подписки на уведомления.
//
// ⭐ Дружба взаимна и потому требует согласия. Односторонний «подписался» здесь не
// подходит: друг видит, что ты в сети, и зовёт за стол — это доступ к присутствию,
// а его не выдают без спроса.

// Отказы сценариев друзей. Транспорт превращает их в коды и статусы.
//
// ⚠️ ErrPairNotFound и ErrNotFriends — РАЗНЫЕ ошибки с одним кодом NOT_FRIENDS:
// «пары нет вовсе» отвечает 404, «звать можно только друга» — 403. Статус берётся
// по месту, поэтому и ошибки две.
var (
	// ErrFriendLoginNotFound — логина, по которому зовут в друзья, нет.
	//
	// ⚠️ Отдельно от ErrUserNotFound: код у них один (USER_NOT_FOUND), а сообщение
	// в Java разное — «Игрока с таким логином нет» при поиске по логину и «Игрок
	// не найден», когда пропал сам спрашивающий. Текст уходит клиенту, поэтому
	// склеивать их нельзя.
	ErrFriendLoginNotFound = errors.New("игрока с таким логином нет")

	ErrCannotFriendSelf    = errors.New("с самим собой дружить не получится")
	ErrAlreadyFriends      = errors.New("уже друзья")
	ErrRequestAlreadySent  = errors.New("заявка уже отправлена")
	ErrNotYourRequest      = errors.New("эту заявку принимаешь не ты")
	ErrPairNotFound        = errors.New("такой пары нет")
	ErrNotFriends          = errors.New("звать за стол можно только друзей")
	ErrInviteTableNotFound = errors.New("стол не найден")
)

// Friend — друг или заявка так, как их видит спрашивающий.
type Friend struct {
	UserID      string
	Username    string
	DisplayName string
	Avatar      *string
	// Online — в сети прямо сейчас, по живому сокету, а не по «был недавно».
	Online bool
	Status string
	// Mine — заявку отправил спрашивающий. По этому флагу экран решает, звать или отвечать.
	Mine bool
}

// FriendList — список, разложенный по смыслу: с кем дружим, кто ждёт ответа от нас
// и кто ждёт ответа нашего.
type FriendList struct {
	Friends  []Friend
	Incoming []Friend
	Outgoing []Friend
}

// InviteTable — стол, за который зовут: только то, что уходит в приглашение.
type InviteTable struct {
	ID   string
	Name string
	Code string
}

// FriendPresence — кто сейчас в сети.
//
// ⭐ Присутствие считается по живым сокетам, а не по «последней активности»: игра идёт
// через WebSocket, и открытое соединение — это и есть присутствие. Отметка времени врала бы
// в обе стороны: закрывший вкладку числился бы онлайн ещё минуту, а задумавшийся над ходом
// успел бы «уйти».
type FriendPresence interface {
	IsOnline(userID string) bool
}

// InviteDelivery — доставка оклика за стол.
//
// Возвращает, дошло ли приглашение ПРЯМО СЕЙЧАС по живому сокету: приглашение нигде
// не хранится, и экрану надо честно показать «позвал» или «его нет — позвали уведомлением».
type InviteDelivery interface {
	SendTableInvite(ctx context.Context, friendID, fromName string, table InviteTable) bool
}

// TableLookup — «найди стол по идентификатору».
//
// ⚠️ Друзья не знают про лобби и не должны: им нужны три поля стола, а не его состояние.
// Реализация обязана вернуть ErrInviteTableNotFound (или repository.ErrNotFound), если
// стола нет, — иначе «стола не существует» уедет в 500 вместо 404.
type TableLookup interface {
	InviteTableByID(ctx context.Context, tableID string) (InviteTable, error)
}

// friendshipStore — то, что сценарию нужно от таблицы пар. Интерфейс, а не конкретный
// репозиторий, чтобы правила дружбы проверялись без базы: они здесь главное.
type friendshipStore interface {
	FindPair(ctx context.Context, one, two string) (repository.Friendship, error)
	FindAllInvolving(ctx context.Context, userID string) ([]repository.FriendPair, error)
	IsFriend(ctx context.Context, one, two string) (bool, error)
	Insert(ctx context.Context, from, to string) (repository.Friendship, error)
	Accept(ctx context.Context, one, two string, at time.Time) (repository.Friendship, error)
	Delete(ctx context.Context, one, two string) error
}

// userLookup — поиск игрока по логину и по идентификатору.
type userLookup interface {
	FindByUsernameIgnoreCase(ctx context.Context, username string) (repository.User, error)
	FindByID(ctx context.Context, id string) (repository.User, error)
}

// FriendService — заявки, список друзей и приглашение за стол.
type FriendService struct {
	friendships friendshipStore
	users       userLookup
	presence    FriendPresence
	invites     InviteDelivery
	tables      TableLookup
	now         func() time.Time
}

// NewFriendService собирает сценарии друзей.
//
// presence, invites и tables могут быть нулевыми: без сокета все считаются офлайн,
// а приглашение без доставки не уходит никуда. Так собранный сервис годен для списка
// и заявок — то есть для всего, что не про живое соединение.
func NewFriendService(friendships friendshipStore, users userLookup, presence FriendPresence,
	invites InviteDelivery, tables TableLookup, now func() time.Time) FriendService {
	if now == nil {
		now = time.Now
	}
	return FriendService{
		friendships: friendships, users: users, presence: presence,
		invites: invites, tables: tables, now: now,
	}
}

// Request — позвать в друзья по логину.
//
// ⚠️ Заявка тому, кто уже позвал тебя сам, — это СОГЛАСИЕ, а не вторая заявка. Иначе двое,
// нажавшие «добавить» одновременно, остались бы с двумя висящими приглашениями и без
// дружбы, и выбраться из этого состояния было бы нечем.
func (s FriendService) Request(ctx context.Context, userID, username string) (Friend, error) {
	// Логин ищется БЕЗ учёта регистра и с обрезкой пробелов — его вводят руками
	// с мобильной клавиатуры, которая заглавит первую букву сама.
	target, err := s.users.FindByUsernameIgnoreCase(ctx, strings.TrimSpace(username))
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return Friend{}, ErrFriendLoginNotFound
		}
		return Friend{}, err
	}
	if strings.EqualFold(target.ID, userID) {
		return Friend{}, ErrCannotFriendSelf
	}

	existing, err := s.friendships.FindPair(ctx, userID, target.ID)
	switch {
	case err == nil:
		if existing.IsAccepted() {
			return Friend{}, ErrAlreadyFriends
		}
		if !existing.CanBeAcceptedBy(userID) {
			return Friend{}, ErrRequestAlreadySent
		}
		accepted, err := s.friendships.Accept(ctx, userID, target.ID, s.now())
		if err != nil {
			return Friend{}, err
		}
		return s.toFriend(accepted, userID, target), nil
	case errors.Is(err, repository.ErrNotFound):
		created, err := s.friendships.Insert(ctx, userID, target.ID)
		if err != nil {
			// Пара успела появиться между проверкой и вставкой: значит, встречная заявка
			// пришла в ту же секунду. Для игрока это то же самое, что «уже отправлена».
			if errors.Is(err, repository.ErrConflict) {
				return Friend{}, ErrRequestAlreadySent
			}
			return Friend{}, err
		}
		return s.toFriend(created, userID, target), nil
	default:
		return Friend{}, err
	}
}

// Accept принимает заявку. Принимает только тот, кому её прислали.
//
// ⚠️ Повторное принятие уже принятой пары — не ошибка: экран мог не успеть обновиться,
// и второй ответ обязан быть таким же, как первый.
func (s FriendService) Accept(ctx context.Context, userID, friendID string) (Friend, error) {
	pair, err := s.pairOrFail(ctx, userID, friendID)
	if err != nil {
		return Friend{}, err
	}
	if pair.IsAccepted() {
		other, err := s.userOrFail(ctx, friendID)
		if err != nil {
			return Friend{}, err
		}
		return s.toFriend(pair, userID, other), nil
	}
	if !pair.CanBeAcceptedBy(userID) {
		return Friend{}, ErrNotYourRequest
	}
	other, err := s.userOrFail(ctx, friendID)
	if err != nil {
		return Friend{}, err
	}

	accepted, err := s.friendships.Accept(ctx, userID, friendID, s.now())
	if err != nil {
		return Friend{}, err
	}
	return s.toFriend(accepted, userID, other), nil
}

// Remove убирает из друзей или отклоняет заявку — для сервера это одно и то же.
//
// Отдельного «отклонить» нет намеренно: отказ и разрыв означают одно — пары больше нет.
func (s FriendService) Remove(ctx context.Context, userID, friendID string) error {
	if _, err := s.pairOrFail(ctx, userID, friendID); err != nil {
		return err
	}
	if err := s.friendships.Delete(ctx, userID, friendID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrPairNotFound
		}
		return err
	}
	return nil
}

// List — друзья и заявки, разложенные по смыслу.
func (s FriendService) List(ctx context.Context, userID string) (FriendList, error) {
	pairs, err := s.friendships.FindAllInvolving(ctx, userID)
	if err != nil {
		return FriendList{}, err
	}

	// ⚠️ Списки пустые, но не nil: наверху они уходят в JSON, а nil стал бы null,
	// тогда как Java отдаёт [] (MD-003).
	list := FriendList{Friends: []Friend{}, Incoming: []Friend{}, Outgoing: []Friend{}}
	for _, item := range pairs {
		friend := s.toFriendProfile(item.Pair, userID, item.Other)
		switch {
		case item.Pair.IsAccepted():
			list.Friends = append(list.Friends, friend)
		case friend.Mine:
			list.Outgoing = append(list.Outgoing, friend)
		default:
			list.Incoming = append(list.Incoming, friend)
		}
	}

	// Онлайн — наверх: за стол зовут тех, кто сейчас здесь. Внутри — по имени без учёта
	// регистра. Сортировка устойчивая: у Java она тоже устойчивая, и порядок равных имён
	// не должен прыгать между ответами.
	//
	// ⚠️ incoming и outgoing НЕ сортируются — как в Java. Их порядок задаёт выборка.
	sort.SliceStable(list.Friends, func(i, j int) bool {
		left, right := list.Friends[i], list.Friends[j]
		if left.Online != right.Online {
			return left.Online
		}
		return caseInsensitiveLess(left.DisplayName, right.DisplayName)
	})
	return list, nil
}

// IsFriend — состоят ли двое в принятой дружбе.
//
// Спрашивают перед приглашением за стол и перед показом чужой истории матчей: висящая
// заявка правом не считается.
func (s FriendService) IsFriend(ctx context.Context, one, two string) (bool, error) {
	return s.friendships.IsFriend(ctx, one, two)
}

// Invite зовёт друга за стол.
//
// ⚠️ Звать можно только друга — иначе приглашение становится способом писать незнакомым
// людям. Проверка здесь, а не на экране: экран показывает лишь друзей, но запрос приходит
// из сети, а не с экрана.
//
// Порядок проверок повторяет Java: сначала стол, потом дружба. Поэтому на несуществующий
// стол отвечает 404, даже если позвали не друга.
//
// Возвращает, дошло ли приглашение прямо сейчас; нет — друг не в сети, и его позвали
// уведомлением (если оно настроено).
func (s FriendService) Invite(ctx context.Context, userID, friendID, tableID string) (bool, error) {
	table, err := s.inviteTable(ctx, tableID)
	if err != nil {
		return false, err
	}

	friends, err := s.friendships.IsFriend(ctx, userID, friendID)
	if err != nil {
		return false, err
	}
	if !friends {
		return false, ErrNotFriends
	}

	me, err := s.userOrFail(ctx, userID)
	if err != nil {
		return false, err
	}
	if s.invites == nil {
		// Доставки нет — звать некуда. Это не отказ: приглашение просто никого не застало.
		return false, nil
	}
	return s.invites.SendTableInvite(ctx, friendID, me.DisplayName, table), nil
}

func (s FriendService) inviteTable(ctx context.Context, tableID string) (InviteTable, error) {
	if s.tables == nil {
		return InviteTable{}, ErrInviteTableNotFound
	}
	table, err := s.tables.InviteTableByID(ctx, tableID)
	if err != nil {
		// repository.ErrNotFound принимается наравне со «своей» ошибкой: адаптер лобби
		// чаще всего просто пробрасывает ошибку репозитория, и терять из-за этого 404 глупо.
		if errors.Is(err, ErrInviteTableNotFound) || errors.Is(err, repository.ErrNotFound) {
			return InviteTable{}, ErrInviteTableNotFound
		}
		return InviteTable{}, err
	}
	return table, nil
}

// pairOrFail — пара двоих или отказ «такой пары нет» (404 NOT_FRIENDS).
func (s FriendService) pairOrFail(ctx context.Context, userID, friendID string) (repository.Friendship, error) {
	pair, err := s.friendships.FindPair(ctx, userID, friendID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return repository.Friendship{}, ErrPairNotFound
		}
		return repository.Friendship{}, err
	}
	if !pair.Involves(userID) {
		return repository.Friendship{}, ErrPairNotFound
	}
	return pair, nil
}

func (s FriendService) userOrFail(ctx context.Context, userID string) (repository.User, error) {
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return repository.User{}, ErrUserNotFound
		}
		return repository.User{}, err
	}
	return user, nil
}

func (s FriendService) toFriend(pair repository.Friendship, viewer string, other repository.User) Friend {
	return s.toFriendProfile(pair, viewer, repository.FriendProfile{
		ID: other.ID, Username: other.Username,
		DisplayName: other.DisplayName, Avatar: other.Avatar,
	})
}

func (s FriendService) toFriendProfile(pair repository.Friendship, viewer string,
	other repository.FriendProfile) Friend {
	return Friend{
		UserID:      other.ID,
		Username:    other.Username,
		DisplayName: other.DisplayName,
		Avatar:      other.Avatar,
		Online:      s.isOnline(other.ID),
		Status:      pair.Status,
		Mine:        strings.EqualFold(pair.RequestedBy, viewer),
	}
}

func (s FriendService) isOnline(userID string) bool {
	return s.presence != nil && s.presence.IsOnline(userID)
}

// caseInsensitiveLess повторяет java.lang.String.CASE_INSENSITIVE_ORDER: Java сравнивает
// посимвольно, приводя каждый символ сначала к верхнему, потом к нижнему регистру.
func caseInsensitiveLess(one, two string) bool {
	return strings.ToLower(strings.ToUpper(one)) < strings.ToLower(strings.ToUpper(two))
}

// pushStore — то, что сценарию подписок нужно от таблицы устройств.
type pushStore interface {
	Save(ctx context.Context, sub repository.PushSubscription) (repository.PushSubscription, error)
	DeleteByEndpointAndUserID(ctx context.Context, endpoint, userID string) (int64, error)
}

// PushSubscriptionService — подписки устройств на уведомления «твой ход».
//
// Название длиннее очевидного PushService намеренно: так же зовётся отправитель, и два
// одинаковых имени в одном слое читались бы как одно и то же.
type PushSubscriptionService struct {
	subscriptions pushStore
	publicKey     string
	privateKey    string
}

// NewPushSubscriptionService собирает сценарии подписки.
//
// Ключи VAPID передаются сюда, а не читаются из конфигурации на месте: «включено ли»
// решается наличием обоих ключей, и это решение должно быть одно на весь сервис.
func NewPushSubscriptionService(subscriptions pushStore, publicKey, privateKey string) PushSubscriptionService {
	return PushSubscriptionService{
		subscriptions: subscriptions,
		publicKey:     strings.TrimSpace(publicKey),
		privateKey:    strings.TrimSpace(privateKey),
	}
}

// Enabled — уведомления настроены.
//
// ⭐ Без ключей отправлять нечем, и это НЕ поломка: локально играют с открытой вкладкой,
// и заводить ключи ради этого незачем. Клиент по этому признаку просто не показывает
// кнопку подписки.
func (s PushSubscriptionService) Enabled() bool {
	return s.publicKey != "" && s.privateKey != ""
}

// PublicKey — открытый ключ для браузера. Пусто — подписываться не на что.
func (s PushSubscriptionService) PublicKey() string {
	if !s.Enabled() {
		return ""
	}
	return s.publicKey
}

// Subscribe запоминает устройство.
func (s PushSubscriptionService) Subscribe(ctx context.Context, userID, endpoint, p256dh,
	auth string, userAgent *string) error {
	// ⚠️ Новый id уходит в запрос всегда, но применяется только при вставке: при
	// совпадении endpoint строка обновляется и сохраняет свой прежний id и created_at.
	_, err := s.subscriptions.Save(ctx, repository.PushSubscription{
		ID: uuid.NewString(), UserID: userID, Endpoint: endpoint,
		P256dh: p256dh, Auth: auth, UserAgent: userAgent,
	})
	return err
}

// Unsubscribe отписывает устройство ВЛАДЕЛЬЦА.
//
// ⚠️ Владелец обязателен: без него отписка была доступна любому вошедшему, кто знал чужой
// endpoint. Отсутствие подписки ошибкой не считается — отписка идемпотентна.
func (s PushSubscriptionService) Unsubscribe(ctx context.Context, userID, endpoint string) error {
	_, err := s.subscriptions.DeleteByEndpointAndUserID(ctx, endpoint, userID)
	return err
}
