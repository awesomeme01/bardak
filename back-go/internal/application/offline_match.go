package application

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/awesomeme01/bardak/back-go/internal/domain/game"
	"github.com/awesomeme01/bardak/back-go/internal/repository"
)

// Регистрация партии, сыгранной за настоящим столом.
//
// ⭐ Компания играет вживую годами, и весь этот опыт в рейтинге не участвовал вовсе.
// Оффлайн-партия закрывает разрыв: один из игроков записывает состав и то, кто чем
// закончил, — и матч встаёт в общую историю наравне с онлайновым.
//
// ⚠️ Что записать НЕЛЬЗЯ — счёт навесов: кто кому что навесил, за реальным столом никто
// не протоколирует. Поэтому оффлайн-партия даёт места, ступени и рейтинг, но статистики
// навесов у неё нет и быть не может; онлайн-матчи её ведут сами.

// Ошибки регистрации оффлайн-партии.
var (
	// ErrOfflineTooFewPlayers — партия начинается с двоих.
	ErrOfflineTooFewPlayers = errors.New("в партии должно быть хотя бы двое")
	// ErrOfflineTooManyPlayers — за столом не бывает столько человек.
	ErrOfflineTooManyPlayers = errors.New("за столом не бывает больше восьми")
	// ErrOfflineDuplicatePlayer — один игрок дважды за одним столом.
	ErrOfflineDuplicatePlayer = errors.New("игрок указан дважды")
	// ErrOfflineNotAParticipant — записать можно только ту партию, в которой играл сам.
	ErrOfflineNotAParticipant = errors.New("записать можно только свою партию")
	// ErrOfflineNotFriends — в состав можно ставить себя и своих друзей.
	ErrOfflineNotFriends = errors.New("в состав идут только друзья")
	// ErrOfflineUnknownOutcome — такого исхода на шкале нет.
	ErrOfflineUnknownOutcome = errors.New("неизвестный исход")
	// ErrOfflineFuture — партия из будущего.
	ErrOfflineFuture = errors.New("партия не может быть сыграна в будущем")
)

// maxOfflineSeats — вживую садятся и вшестером, и всемером; онлайн-стол ограничен пятью,
// но оффлайн-партию это ограничение не касается — она уже сыграна.
const maxOfflineSeats = 8

// OfflineOutcome — что выставили одному игроку.
type OfflineOutcome struct {
	UserID string
	// Code — исход по шкале: «NONE», ступень («6»…«A»), «Jk» или степень проигрыша.
	Code string
}

// OfflineMatchRequest — записываемая партия.
type OfflineMatchRequest struct {
	// PlayedAt — когда играли; пусто — сейчас.
	PlayedAt *time.Time
	Players  []OfflineOutcome
}

// outcomeOnScale — исход в том виде, в каком он ложится в базу и в рейтинг.
type outcomeOnScale struct {
	// Level — ступень навесов для колонки naves_level; пусто — «летит 6».
	Level *string
	// Degree — степень проигрыша; пусто у не проигравшего.
	Degree *string
	Damage float64
}

// OfflineOutcomeCode — «летит 6»: навесов не было вовсе.
const OfflineOutcomeNone = "NONE"

// offlineScale — шкала исходов, разрешённых в оффлайн-партии.
//
// ⚠️ Собирается ИЗ ПРАВИЛ ИГРЫ, а не выписывается руками: ступени берутся из полной шкалы
// навесов, степени — из перечисления степеней, а ущерб считает та же DamageOf, что и
// онлайн-матч. Выписанная копия разошлась бы с рейтингом на первой же правке шкалы.
var offlineScale = buildOfflineScale()

func buildOfflineScale() map[string]outcomeOnScale {
	scale := game.FullNavesScale()
	table := map[string]outcomeOnScale{
		OfflineOutcomeNone: {Damage: DamageOf(scale, game.NoNaves, game.NoLossDegree)},
	}

	for level, rank := range scale.Ranks {
		code := rank.Code()
		table[code] = outcomeOnScale{
			Level:  stringPtr(code),
			Damage: DamageOf(scale, level, game.NoLossDegree),
		}
	}

	joker := scale.JokerLevel()
	table[JokerNavesLevel] = outcomeOnScale{
		Level:  stringPtr(JokerNavesLevel),
		Damage: DamageOf(scale, joker, game.NoLossDegree),
	}
	for _, degree := range []game.LossDegree{game.LossFail, game.LossSuperFail,
		game.LossSuperMegaFail, game.LossSuperMegaSuck, game.LossRoyal} {
		name := degree.String()
		table[name] = outcomeOnScale{
			Level:  stringPtr(JokerNavesLevel),
			Degree: stringPtr(name),
			Damage: DamageOf(scale, joker, degree),
		}
	}
	return table
}

func stringPtr(value string) *string { return &value }

// OfflineOutcomeCodes — исходы шкалы от лучшего к худшему, для формы записи.
//
// ⭐ Порядок несёт смысл: экран рисует его сверху вниз, и он же — порядок мест.
func OfflineOutcomeCodes() []string {
	codes := make([]string, 0, len(offlineScale))
	for code := range offlineScale {
		codes = append(codes, code)
	}
	sort.Slice(codes, func(i, j int) bool {
		return offlineScale[codes[i]].Damage < offlineScale[codes[j]].Damage
	})
	return codes
}

// OfflineMatchStore — что записи оффлайн-партии нужно от базы.
type OfflineMatchStore interface {
	OpenSeasonID(ctx context.Context) (string, bool, error)
	CreateOffline(ctx context.Context, match repository.OfflineMatch) error
}

// ⭐ Проверка сборкой: репозиторий подходит сценарию.
var _ OfflineMatchStore = repository.MatchResults{}

// OfflineMatchService — запись партии, сыгранной вживую.
type OfflineMatchService struct {
	matches OfflineMatchStore
	ratings RatingReader
	friends HistoryFriendChecker
	now     func() time.Time
	newID   func() string
}

// NewOfflineMatchService собирает сценарий.
func NewOfflineMatchService(matches OfflineMatchStore, ratings RatingReader,
	friends HistoryFriendChecker, now func() time.Time) OfflineMatchService {
	if now == nil {
		now = time.Now
	}
	return OfflineMatchService{matches: matches, ratings: ratings, friends: friends,
		now: now, newID: uuid.NewString}
}

// Register записывает партию и возвращает изменения рейтинга её участников.
func (s OfflineMatchService) Register(ctx context.Context, creatorID string,
	request OfflineMatchRequest) (string, []RatingChange, error) {
	outcomes, err := s.validate(ctx, creatorID, request)
	if err != nil {
		return "", nil, err
	}

	playedAt := s.now()
	if request.PlayedAt != nil {
		playedAt = *request.PlayedAt
	}

	participants := make([]EloParticipant, 0, len(request.Players))
	before := make([]string, 0, len(request.Players))
	for index, player := range request.Players {
		rating, err := s.ratingOf(ctx, player.UserID)
		if err != nil {
			return "", nil, err
		}
		value, err := strconv.ParseFloat(rating.Rating, 64)
		if err != nil {
			return "", nil, fmt.Errorf("рейтинг игрока %s не разобран: %w", player.UserID, err)
		}
		before = append(before, rating.Rating)
		participants = append(participants, EloParticipant{
			Rating: value, MatchesPlayed: rating.MatchesPlayed, Damage: outcomes[index].Damage,
		})
	}

	updated, err := Recalculate(participants)
	if err != nil {
		return "", nil, err
	}

	seasonID, hasSeason, err := s.matches.OpenSeasonID(ctx)
	if err != nil {
		return "", nil, err
	}

	places := placesByDamage(outcomes)
	matchID := s.newID()
	seats := make([]repository.SeatOutcome, 0, len(request.Players))
	changes := make([]RatingChange, 0, len(request.Players))
	for index, player := range request.Players {
		after := formatRating(updated[index])
		seats = append(seats, repository.SeatOutcome{
			UserID: player.UserID, SeatNo: index, Place: places[index],
			NavesLevel: outcomes[index].Level, LossType: outcomes[index].Degree,
			RatingBefore: before[index], RatingAfter: after,
		})
		changes = append(changes, RatingChange{
			UserID: player.UserID, SeatNo: index, Place: places[index],
			NavesLevel: outcomes[index].Level, LossDegree: outcomes[index].Degree,
			Before: before[index], After: after,
			Delta: formatRating(updated[index] - participants[index].Rating),
		})
	}

	match := repository.OfflineMatch{
		MatchID: matchID, CreatedBy: creatorID, PlayedAt: playedAt, Outcomes: seats,
	}
	if hasSeason {
		match.SeasonID = &seasonID
	}
	// ⚠️ Главного проигравшего может не быть вовсе: вживую партия нередко кончается
	// до того, как кому-то навесили джокер. Онлайн так не бывает, и подставлять сюда
	// «худшего» значило бы записать проигравшим того, кто не проигрывал.
	if loser, found := heaviestLoser(request.Players, outcomes); found {
		match.LoserUserID = &loser
	}

	if err := s.matches.CreateOffline(ctx, match); err != nil {
		return "", nil, err
	}
	return matchID, changes, nil
}

// validate проверяет состав и исходы, возвращая исходы на шкале в том же порядке.
func (s OfflineMatchService) validate(ctx context.Context, creatorID string,
	request OfflineMatchRequest) ([]outcomeOnScale, error) {
	switch {
	case len(request.Players) < 2:
		return nil, ErrOfflineTooFewPlayers
	case len(request.Players) > maxOfflineSeats:
		return nil, ErrOfflineTooManyPlayers
	}
	if request.PlayedAt != nil && request.PlayedAt.After(s.now()) {
		return nil, ErrOfflineFuture
	}

	seen := make(map[string]bool, len(request.Players))
	outcomes := make([]outcomeOnScale, 0, len(request.Players))
	creatorPlays := false
	for _, player := range request.Players {
		if seen[player.UserID] {
			return nil, ErrOfflineDuplicatePlayer
		}
		seen[player.UserID] = true

		outcome, ok := offlineScale[player.Code]
		if !ok {
			return nil, ErrOfflineUnknownOutcome
		}
		outcomes = append(outcomes, outcome)

		if player.UserID == creatorID {
			creatorPlays = true
			continue
		}
		// ⭐ Состав ограничен друзьями, и это единственная защита от того, чтобы
		// приписать чужому человеку разгромный проигрыш: партию никто не подтверждает,
		// поэтому записать её можно только про тех, кто уже согласился быть другом.
		friend, err := s.friends.IsFriend(ctx, creatorID, player.UserID)
		if err != nil {
			return nil, err
		}
		if !friend {
			return nil, ErrOfflineNotFriends
		}
	}
	if !creatorPlays {
		return nil, ErrOfflineNotAParticipant
	}
	return outcomes, nil
}

// ratingOf — текущий рейтинг; у не игравшего его ещё нет, и это не ошибка.
func (s OfflineMatchService) ratingOf(ctx context.Context,
	userID string) (repository.UserRating, error) {
	rating, err := s.ratings.FindRating(ctx, userID)
	if errors.Is(err, repository.ErrNotFound) {
		return repository.UserRating{UserID: userID, Rating: "1000.00", MatchesPlayed: 0}, nil
	}
	if err != nil {
		return repository.UserRating{}, err
	}
	return rating, nil
}

// placesByDamage — места по шкале ущерба; равный ущерб делит соседние места по порядку
// в составе, как и у онлайн-матча.
func placesByDamage(outcomes []outcomeOnScale) []int {
	order := make([]int, len(outcomes))
	for index := range order {
		order[index] = index
	}
	sort.SliceStable(order, func(left, right int) bool {
		return outcomes[order[left]].Damage < outcomes[order[right]].Damage
	})

	places := make([]int, len(outcomes))
	for position, index := range order {
		places[index] = position + 1
	}
	return places
}

// heaviestLoser — кому досталось тяжелее всех, если до джокера кто-то дошёл.
func heaviestLoser(players []OfflineOutcome, outcomes []outcomeOnScale) (string, bool) {
	worst, found := 0, false
	for index := range players {
		if outcomes[index].Degree == nil {
			continue
		}
		if !found || outcomes[index].Damage > outcomes[worst].Damage {
			worst, found = index, true
		}
	}
	if !found {
		return "", false
	}
	return players[worst].UserID, true
}
