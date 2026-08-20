package application

import (
	"context"
	"errors"
	"sort"

	"github.com/awesomeme01/bardak/back-go/internal/repository"
)

// История матчей: список, детали с разбивкой по раздачам и реплей.
//
// ⭐ Реплей отдаётся С ТОЧКИ ЗРЕНИЯ СПРАШИВАЮЩЕГО, а не целиком. Лог хранит и то, что
// видел один игрок; отдать его сырым — значит показать чужие карты задним числом, а по
// ним стиль игры соперника читается не хуже, чем подглядыванием вживую.

// Ошибки сценария истории. Транспорт превращает их в коды и статусы.
//
// ⚠️ Имена с префиксом History намеренно: тот же код NOT_FRIENDS живёт в друзьях и с
// другим статусом, и общая переменная на код рано или поздно увела бы статус не туда.
var (
	// ErrHistoryForbidden — спрашивающий ни участник, ни друг участника (403 NOT_FRIENDS).
	ErrHistoryForbidden = errors.New("история видна своим и друзьям")
	// ErrHistoryMatchNotFound — матча нет (404 MATCH_NOT_FOUND).
	ErrHistoryMatchNotFound = errors.New("такого матча нет")
	// ErrHistoryMatchNotFinished — реплей просят у идущего матча (409 MATCH_NOT_FINISHED).
	ErrHistoryMatchNotFinished = errors.New("реплей доступен только после матча")
)

// HistoryFriendChecker — проверка дружбы, какой её видит история.
//
// ⭐ Взято интерфейсом, а не готовым сервисом друзей: истории незачем знать про заявки,
// согласия и приглашения за стол — ей нужен один ответ «свои ли они».
type HistoryFriendChecker interface {
	// IsFriend — состоят ли двое во взаимной (принятой) дружбе.
	IsFriend(ctx context.Context, userID, otherID string) (bool, error)
}

// historyStore — что истории нужно от базы.
//
// Интерфейс объявлен здесь, а не в репозитории: так сценарий проверяется без Postgres,
// и главное правило — «чужие карты не утекают» — покрыто тестом, который идёт мгновенно.
type historyStore interface {
	MatchesOf(ctx context.Context, userID, status string) ([]repository.HistoryMatch, error)
	FindMatch(ctx context.Context, matchID string) (repository.HistoryMatch, error)
	ParticipantsOf(ctx context.Context, matchID string) ([]repository.HistoryParticipant, error)
	PlayersOf(ctx context.Context, matchIDs []string) (map[string][]repository.HistoryPlayer, error)
	DealsOf(ctx context.Context, matchID string) ([]repository.HistoryDeal, error)
	DealSeatsOf(ctx context.Context, matchID string) (map[string][]repository.HistoryDealSeat, error)
	EventsOf(ctx context.Context, matchID string) ([]repository.HistoryEvent, error)
}

// MatchSummary — матч в списке: строка матча плюс то, что посчитано для смотрящего.
type MatchSummary struct {
	Match   repository.HistoryMatch
	Players []repository.HistoryPlayer
	// RatingCounted — ⭐ отменённый матч виден в истории, но рейтинга не касается:
	// без этого признака нулевая дельта выглядела бы как ничья.
	RatingCounted bool
	MyPlace       *int
	MyRatingDelta *string
}

// MatchDetails — матч с разбивкой по раздачам.
type MatchDetails struct {
	Match MatchSummary
	Deals []DealBreakdown
}

// DealBreakdown — раздача вместе с итогами мест.
type DealBreakdown struct {
	Deal  repository.HistoryDeal
	Seats []repository.HistoryDealSeat
}

// MatchReplay — лог матча глазами спрашивающего.
type MatchReplay struct {
	MatchID string
	Status  string
	// MySeat — место спрашивающего или -1, если он в матче не играл: тогда ему видно
	// только публичное.
	MySeat int
	Events []repository.HistoryEvent
}

// HistoryService — сценарии истории матчей.
type HistoryService struct {
	store   historyStore
	friends HistoryFriendChecker
}

// NewHistoryService собирает сценарии истории.
func NewHistoryService(store historyStore, friends HistoryFriendChecker) HistoryService {
	return HistoryService{store: store, friends: friends}
}

// Matches — матчи игрока whose, о которых спрашивает me; новые сверху.
//
// ⚠️ Итог в списке считается для ВЛАДЕЛЬЦА истории, а не для смотрящего: открыв историю
// друга, видишь его место в матче, а не своё. Так в Java (`summaryOf(match, whose)`),
// и это единственное осмысленное поведение: список-то его.
func (s HistoryService) Matches(ctx context.Context, me, whose, status string) ([]MatchSummary, error) {
	if err := s.requireVisible(ctx, me, whose); err != nil {
		return nil, err
	}

	matches, err := s.store.MatchesOf(ctx, whose, status)
	if err != nil {
		return nil, err
	}
	return s.summariesOf(ctx, matches, whose)
}

// Details — матч с разбивкой по раздачам.
func (s HistoryService) Details(ctx context.Context, me, matchID string) (MatchDetails, error) {
	if err := s.requireMatchVisible(ctx, matchID, me); err != nil {
		return MatchDetails{}, err
	}

	match, err := s.matchOrFail(ctx, matchID)
	if err != nil {
		return MatchDetails{}, err
	}
	summaries, err := s.summariesOf(ctx, []repository.HistoryMatch{match}, me)
	if err != nil {
		return MatchDetails{}, err
	}

	deals, err := s.store.DealsOf(ctx, matchID)
	if err != nil {
		return MatchDetails{}, err
	}
	seats, err := s.store.DealSeatsOf(ctx, matchID)
	if err != nil {
		return MatchDetails{}, err
	}

	breakdown := make([]DealBreakdown, 0, len(deals))
	for _, deal := range deals {
		breakdown = append(breakdown, DealBreakdown{
			Deal:  deal,
			Seats: orEmptyDealSeats(seats[deal.ID]),
		})
	}
	return MatchDetails{Match: summaries[0], Deals: breakdown}, nil
}

// Replay — лог матча, отфильтрованный по видимости.
//
// ⚠️ Только для законченных матчей: у идущего лог — это и есть текущая партия,
// и «посмотреть реплей» посреди неё означало бы читать её из другого окна.
func (s HistoryService) Replay(ctx context.Context, me, matchID string) (MatchReplay, error) {
	if err := s.requireMatchVisible(ctx, matchID, me); err != nil {
		return MatchReplay{}, err
	}

	match, err := s.matchOrFail(ctx, matchID)
	if err != nil {
		return MatchReplay{}, err
	}
	if match.Status == "IN_PROGRESS" || match.Status == "PAUSED" {
		return MatchReplay{}, ErrHistoryMatchNotFinished
	}

	participants, err := s.store.ParticipantsOf(ctx, matchID)
	if err != nil {
		return MatchReplay{}, err
	}
	seat := historySeatOf(participants, me)

	events, err := s.store.EventsOf(ctx, matchID)
	if err != nil {
		return MatchReplay{}, err
	}

	// ⭐ Здесь и происходит главное: не участник получает mySeat = -1 и видит только
	// публичные события. Утечка чужих карт задним числом — испорченная игра.
	visible := make([]repository.HistoryEvent, 0, len(events))
	for _, event := range events {
		if historyEventVisibleTo(event, seat) {
			visible = append(visible, event)
		}
	}
	return MatchReplay{MatchID: matchID, Status: match.Status, MySeat: seat, Events: visible}, nil
}

// requireVisible — чью историю можно смотреть: свою и друзей.
//
// ⭐ Рейтинг и статистика остаются открытыми — на них держится таблица лидеров, это
// агрегаты. История же — это «с кем и когда я играл», и посторонним её знать незачем.
func (s HistoryService) requireVisible(ctx context.Context, me, whose string) error {
	if me == whose {
		return nil
	}
	friend, err := s.friends.IsFriend(ctx, me, whose)
	if err != nil {
		return err
	}
	if !friend {
		return ErrHistoryForbidden
	}
	return nil
}

// requireMatchVisible — матч виден участнику и другу любого из участников.
//
// ⚠️ Проверка стоит ПЕРЕД чтением матча, как в Java: у несуществующего матча нет
// участников, и посторонний получает 403, а не 404. Это не оплошность, а полезное
// свойство — по разнице ответов не перебрать чужие идентификаторы матчей.
func (s HistoryService) requireMatchVisible(ctx context.Context, matchID, me string) error {
	participants, err := s.store.ParticipantsOf(ctx, matchID)
	if err != nil {
		return err
	}
	for _, participant := range participants {
		if participant.UserID == me {
			return nil
		}
	}
	for _, participant := range participants {
		friend, err := s.friends.IsFriend(ctx, me, participant.UserID)
		if err != nil {
			return err
		}
		if friend {
			return nil
		}
	}
	return ErrHistoryForbidden
}

func (s HistoryService) matchOrFail(ctx context.Context, matchID string) (repository.HistoryMatch, error) {
	match, err := s.store.FindMatch(ctx, matchID)
	if errors.Is(err, repository.ErrNotFound) {
		return repository.HistoryMatch{}, ErrHistoryMatchNotFound
	}
	return match, err
}

// summariesOf собирает список матчей с итогами игроков за один поход в базу.
func (s HistoryService) summariesOf(ctx context.Context, matches []repository.HistoryMatch,
	viewer string) ([]MatchSummary, error) {
	summaries := make([]MatchSummary, 0, len(matches))
	if len(matches) == 0 {
		return summaries, nil
	}

	ids := make([]string, 0, len(matches))
	for _, match := range matches {
		ids = append(ids, match.ID)
	}
	byMatch, err := s.store.PlayersOf(ctx, ids)
	if err != nil {
		return nil, err
	}

	for _, match := range matches {
		players := orEmptyPlayers(byMatch[match.ID])
		sortHistoryPlayersByPlace(players)

		summary := MatchSummary{
			Match:   match,
			Players: players,
			// Отменённый матч рейтинга не касается: считается только завершённый.
			RatingCounted: match.Status == "FINISHED",
		}
		for _, player := range players {
			if player.UserID == viewer {
				summary.MyPlace = player.Place
				summary.MyRatingDelta = player.RatingDelta
				break
			}
		}
		summaries = append(summaries, summary)
	}
	return summaries, nil
}

// sortHistoryPlayersByPlace — по месту, непроставленные в конец.
//
// ⚠️ Именно по месту, а не по номеру за столом, хотя из базы читается по seat_no:
// в списке итогов первым идёт победитель. Сортировка устойчивая — у отменённого матча
// места пусты у всех, и тогда порядок остаётся по местам за столом.
func sortHistoryPlayersByPlace(players []repository.HistoryPlayer) {
	sort.SliceStable(players, func(i, j int) bool {
		left, right := players[i].Place, players[j].Place
		if left == nil {
			return false
		}
		if right == nil {
			return true
		}
		return *left < *right
	})
}

// historySeatOf — место спрашивающего или -1, если он в матче не играл.
func historySeatOf(participants []repository.HistoryParticipant, userID string) int {
	for _, participant := range participants {
		if participant.UserID == userID {
			return participant.SeatNo
		}
	}
	return -1
}

// historyEventVisibleTo повторяет MatchEventRecord.isVisibleTo из Java: nil — публичное.
func historyEventVisibleTo(event repository.HistoryEvent, seatNo int) bool {
	return event.PrivateToSeat == nil || *event.PrivateToSeat == seatNo
}

// Пустой список обязан остаться списком: nil ушёл бы в ответ как null, а Java отдаёт [].
func orEmptyPlayers(players []repository.HistoryPlayer) []repository.HistoryPlayer {
	if players == nil {
		return []repository.HistoryPlayer{}
	}
	return players
}

func orEmptyDealSeats(seats []repository.HistoryDealSeat) []repository.HistoryDealSeat {
	if seats == nil {
		return []repository.HistoryDealSeat{}
	}
	return seats
}
