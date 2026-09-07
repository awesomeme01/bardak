package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/awesomeme01/bardak/back-go/internal/repository"
)

// Запись партии, сыгранной за настоящим столом.
//
// ⭐ База здесь не нужна: проверяется, кого пускают в состав и как считаются места
// и рейтинг. Что это ложится в базу одной транзакцией — дело теста репозитория.

type fakeOfflineStore struct {
	seasonID string
	created  *repository.OfflineMatch
}

func (f *fakeOfflineStore) OpenSeasonID(context.Context) (string, bool, error) {
	if f.seasonID == "" {
		return "", false, nil
	}
	return f.seasonID, true, nil
}

func (f *fakeOfflineStore) CreateOffline(_ context.Context, match repository.OfflineMatch) error {
	f.created = &match
	return nil
}

// fakeFriends — все перечисленные пары считаются взаимными друзьями.
type fakeFriends map[string]bool

func (f fakeFriends) IsFriend(_ context.Context, userID, otherID string) (bool, error) {
	return f[userID+"|"+otherID], nil
}

func friendsOf(owner string, others ...string) fakeFriends {
	pairs := fakeFriends{}
	for _, other := range others {
		pairs[owner+"|"+other] = true
		pairs[other+"|"+owner] = true
	}
	return pairs
}

func offlineService(store *fakeOfflineStore, friends fakeFriends) OfflineMatchService {
	return NewOfflineMatchService(store, fakeRatings{}, friends,
		func() time.Time { return time.Unix(1_700_000_000, 0) })
}

func registerOrFail(t *testing.T, service OfflineMatchService,
	players ...OfflineOutcome) []RatingChange {
	t.Helper()
	_, changes, err := service.Register(context.Background(), players[0].UserID,
		OfflineMatchRequest{Players: players})
	if err != nil {
		t.Fatalf("партия не записалась: %v", err)
	}
	return changes
}

func TestOfflineMatchRanksPlayersByOutcome(t *testing.T) {
	store := &fakeOfflineStore{}
	service := offlineService(store, friendsOf("me", "b", "c"))

	changes := registerOrFail(t, service,
		OfflineOutcome{UserID: "me", Code: "K"},
		OfflineOutcome{UserID: "b", Code: OfflineOutcomeNone},
		OfflineOutcome{UserID: "c", Code: "ROYAL"})

	if changes[1].Place != 1 || changes[0].Place != 2 || changes[2].Place != 3 {
		t.Fatalf("места разошлись с исходами: %+v", changes)
	}
	// ⭐ Выигравший «на летит 6» против короля и королевского отсоса обязан вырасти,
	// а получивший королевский — упасть сильнее всех.
	if !(changes[1].Delta[0] != '-' && changes[2].Delta[0] == '-') {
		t.Fatalf("знаки дельт неправильные: %+v", changes)
	}
}

// ⭐ Ступень пишется в naves_level, степень — в loss_type, и только у проигравшего.
func TestOfflineMatchWritesLevelsAndDegrees(t *testing.T) {
	store := &fakeOfflineStore{}
	service := offlineService(store, friendsOf("me", "b"))

	registerOrFail(t, service,
		OfflineOutcome{UserID: "me", Code: "10"},
		OfflineOutcome{UserID: "b", Code: "SUPER_MEGA_FAIL"})

	seats := store.created.Outcomes
	if seats[0].NavesLevel == nil || *seats[0].NavesLevel != "10" || seats[0].LossType != nil {
		t.Fatalf("ступень записана неправильно: %+v", seats[0])
	}
	if seats[1].NavesLevel == nil || *seats[1].NavesLevel != JokerNavesLevel {
		t.Fatalf("проигравшему не записан джокер: %+v", seats[1])
	}
	if seats[1].LossType == nil || *seats[1].LossType != "SUPER_MEGA_FAIL" {
		t.Fatalf("степень записана неправильно: %+v", seats[1])
	}
	if store.created.LoserUserID == nil || *store.created.LoserUserID != "b" {
		t.Fatalf("главный проигравший определён неправильно: %+v", store.created.LoserUserID)
	}
}

// ⚠️ Вживую партия нередко кончается до джокера — тогда проигравшего нет вовсе,
// и подставлять сюда худшего значило бы записать проигравшим того, кто не проигрывал.
func TestOfflineMatchLeavesTheLoserEmptyWhenNobodyGotTheJoker(t *testing.T) {
	store := &fakeOfflineStore{}
	service := offlineService(store, friendsOf("me", "b"))

	registerOrFail(t, service,
		OfflineOutcome{UserID: "me", Code: "7"},
		OfflineOutcome{UserID: "b", Code: "A"})

	if store.created.LoserUserID != nil {
		t.Fatalf("проигравшего не было, а записан %v", *store.created.LoserUserID)
	}
}

// ⭐ Единственная защита от приписанного разгрома: в состав идут только друзья.
func TestOfflineMatchRefusesStrangers(t *testing.T) {
	store := &fakeOfflineStore{}
	service := offlineService(store, friendsOf("me", "b"))

	_, _, err := service.Register(context.Background(), "me", OfflineMatchRequest{
		Players: []OfflineOutcome{
			{UserID: "me", Code: "6"},
			{UserID: "stranger", Code: "ROYAL"},
		}})

	if !errors.Is(err, ErrOfflineNotFriends) {
		t.Fatalf("чужого пустили в состав: %v", err)
	}
	if store.created != nil {
		t.Fatalf("отказ обязан быть до записи, а партия записана: %+v", store.created)
	}
}

// Записать можно только ту партию, в которой играл сам.
func TestOfflineMatchRefusesAnOutsider(t *testing.T) {
	service := offlineService(&fakeOfflineStore{}, friendsOf("me", "b", "c"))

	_, _, err := service.Register(context.Background(), "me", OfflineMatchRequest{
		Players: []OfflineOutcome{{UserID: "b", Code: "6"}, {UserID: "c", Code: "ROYAL"}}})

	if !errors.Is(err, ErrOfflineNotAParticipant) {
		t.Fatalf("записали чужую партию: %v", err)
	}
}

func TestOfflineMatchChecksTheComposition(t *testing.T) {
	service := offlineService(&fakeOfflineStore{}, friendsOf("me", "b"))
	ctx := context.Background()

	cases := map[string]struct {
		players []OfflineOutcome
		want    error
	}{
		"один игрок": {
			players: []OfflineOutcome{{UserID: "me", Code: "6"}},
			want:    ErrOfflineTooFewPlayers,
		},
		"один дважды": {
			players: []OfflineOutcome{{UserID: "me", Code: "6"}, {UserID: "me", Code: "7"}},
			want:    ErrOfflineDuplicatePlayer,
		},
		"исход не со шкалы": {
			players: []OfflineOutcome{{UserID: "me", Code: "6"}, {UserID: "b", Code: "ПРОИГРАЛ"}},
			want:    ErrOfflineUnknownOutcome,
		},
	}

	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			_, _, err := service.Register(ctx, "me", OfflineMatchRequest{Players: test.players})
			if !errors.Is(err, test.want) {
				t.Fatalf("ждали %v, получили %v", test.want, err)
			}
		})
	}
}

// ⚠️ Партию из будущего записать нельзя: иначе рейтинг можно было бы «занять вперёд».
func TestOfflineMatchRefusesTheFuture(t *testing.T) {
	service := offlineService(&fakeOfflineStore{}, friendsOf("me", "b"))
	tomorrow := time.Unix(1_700_000_000, 0).Add(24 * time.Hour)

	_, _, err := service.Register(context.Background(), "me", OfflineMatchRequest{
		PlayedAt: &tomorrow,
		Players:  []OfflineOutcome{{UserID: "me", Code: "6"}, {UserID: "b", Code: "ROYAL"}}})

	if !errors.Is(err, ErrOfflineFuture) {
		t.Fatalf("записали партию из будущего: %v", err)
	}
}

// ⭐ Шкала собирается из правил игры, а не выписывается руками: порядок — от лучшего
// исхода к худшему, и он же порядок мест.
func TestOfflineOutcomeCodesRunFromBestToWorst(t *testing.T) {
	codes := OfflineOutcomeCodes()

	want := []string{OfflineOutcomeNone, "6", "7", "8", "9", "10", "J", "Q", "K", "A",
		JokerNavesLevel, "FAIL", "SUPER_FAIL", "SUPER_MEGA_FAIL", "SUPER_MEGA_SUCK", "ROYAL"}
	if len(codes) != len(want) {
		t.Fatalf("на шкале %d исходов, ждали %d: %v", len(codes), len(want), codes)
	}
	for index, code := range want {
		if codes[index] != code {
			t.Fatalf("на месте %d стоит %q, ждали %q (вся шкала: %v)", index, codes[index], code, codes)
		}
	}
}
