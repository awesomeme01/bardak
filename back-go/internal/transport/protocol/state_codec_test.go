package protocol

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/awesomeme01/bardak/back-go/internal/domain/game"
)

// ⭐ Круговой прогон: снимок, разобранный обратно, обязан совпасть с исходным состоянием
// до последнего поля. Это цена того, что кодек написан руками, и она оплачивается здесь.
func TestSnapshotRoundTrip(t *testing.T) {
	trump := game.NewTrump(game.Hearts)
	hidden := game.NewPip(game.King, game.Spades)
	degree := game.LossDegree(1) // ROYAL
	suit := game.Clubs

	original := game.MatchState{
		Phase:       game.MatchInDeal,
		NavesLevels: []int{0, 3, -1},
		DealNo:      7,
		MatchSeed:   1234567,
		Deal: game.DealState{
			Phase: game.PhaseDefend,
			Trump: &trump,
			Deck:  []game.Card{game.NewPip(game.Six, game.Clubs), game.MustJoker(2)},
			Players: []game.PlayerState{
				{SeatNo: 0, Hand: []game.Card{game.NewPip(game.Ace, game.Spades)},
					InDeal: true, NavesLevel: 0, HungCards: []game.Card{}, JokerHangerSeat: -1},
				{SeatNo: 1, Hand: []game.Card{}, FaceDownCard: hidden, InDeal: true,
					NavesLevel: 3, HungCards: []game.Card{game.MustJoker(1)}, JokerHangerSeat: 0},
			},
			Table: []game.TableSlot{
				{Attack: game.NewPip(game.Seven, game.Diamonds), Defence: game.NewPip(game.Nine, game.Diamonds)},
				{Attack: game.NewPip(game.Ten, game.Hearts)},
			},
			RoundStarterSeat:       1,
			AttackRightSeat:        1,
			DefenderSeat:           0,
			PassedSeats:            []int{1},
			ExitOrder:              []int{2},
			AnyCardBeatenThisRound: true,
			AnyPileDiscarded:       true,
			LastAttackCards:        []game.Card{game.NewPip(game.Eight, game.Clubs)},
			RngSeed:                999,
			DiceRolls:              2,
			HangingWindow: &game.HangingWindow{
				VictimSeat: 0, Steps: [][]int{{1}, {2}}, StepIndex: 1,
				Claims:  []game.HangClaim{{SeatNo: 2, Card: game.NewPip(game.Six, game.Hearts)}},
				Decided: []int{2}, EveryClaimantHangs: true,
			},
			PendingHiddenTrump: &game.PendingHiddenTrump{
				Card: game.MustJoker(3), RecipientSeat: 1, ChooserSeat: 0,
			},
		},
		Results: []game.DealOutcome{{
			DealLoserSeat:   1,
			TrumpSuit:       &suit,
			LastAttackCards: []game.Card{game.NewPip(game.Eight, game.Spades)},
			Players: []game.PlayerOutcome{{
				SeatNo: 1, LevelBefore: 2, LevelAfter: 3, Place: 2, LossDegree: degree,
				HungCards: []game.Card{game.NewPip(game.Six, game.Clubs)},
				Changes:   []game.LevelChange{{Reason: game.LostDeal, Amount: 1}},
			}},
		}},
	}

	raw, err := EncodeMatchState(original)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := DecodeMatchState(raw)
	if err != nil {
		t.Fatalf("свой же снимок не разобран: %v", err)
	}

	if !reflect.DeepEqual(original, restored) {
		t.Errorf("состояние не совпало после кругового прогона\nбыло:  %+v\nстало: %+v",
			original.Deal.Phase, restored.Deal.Phase)
		if !reflect.DeepEqual(original.Deal.Players, restored.Deal.Players) {
			t.Error("разошлись игроки")
		}
		if !reflect.DeepEqual(original.Results, restored.Results) {
			t.Error("разошлись итоги раздач")
		}
	}
}

// ⚠️ «Степени нет» в Java было null и поле выпадало из JSON. В Go нулевое значение,
// и его надо НЕ ЗАПИСАТЬ, а не записать как «NONE» — иначе Java прочтёт мусор.
func TestNoLossDegreeIsOmitted(t *testing.T) {
	state := game.MatchState{
		Phase:       game.MatchInDeal,
		NavesLevels: []int{0},
		Deal:        game.DealState{Phase: game.PhaseAttack, Players: []game.PlayerState{}},
		Results: []game.DealOutcome{{
			Players: []game.PlayerOutcome{{SeatNo: 0, LossDegree: game.NoLossDegree}},
		}},
	}

	raw, err := EncodeMatchState(state)
	if err != nil {
		t.Fatal(err)
	}
	var probe map[string]any
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		t.Fatal(err)
	}
	results := probe["results"].([]any)
	player := results[0].(map[string]any)["players"].([]any)[0].(map[string]any)
	if _, exists := player["lossDegree"]; exists {
		t.Error("поле lossDegree должно ОТСУТСТВОВАТЬ, когда степени нет")
	}
}

// ⚠️ Пустой список остаётся списком, а не превращается в null: Java пишет [],
// и снимок с null вместо массива она бы не разобрала.
func TestEmptyListsStayLists(t *testing.T) {
	state := game.MatchState{
		Phase: game.MatchInDeal,
		Deal:  game.DealState{Phase: game.PhaseAttack},
	}
	raw, err := EncodeMatchState(state)
	if err != nil {
		t.Fatal(err)
	}
	for _, mustBeList := range []string{`"navesLevels":[]`, `"deck":[]`, `"players":[]`,
		`"table":[]`, `"passedSeats":[]`, `"exitOrder":[]`, `"lastAttackCards":[]`} {
		if !contains(raw, mustBeList) {
			t.Errorf("ожидал %s в снимке, а его нет: %s", mustBeList, raw)
		}
	}
}

// ⭐ ГЛАВНАЯ проверка совместимости: снимок, записанный ЖИВОЙ Java, обязан читаться Go.
// Без этого матч не передать между бэкендами, а это и есть требование окна отката.
func TestJavaSnapshotIsReadable(t *testing.T) {
	path := os.Getenv("BARDAK_JAVA_SNAPSHOT")
	if path == "" {
		t.Skip("BARDAK_JAVA_SNAPSHOT не задан — проверка совместимости пропущена")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	state, err := DecodeMatchState(string(raw))
	if err != nil {
		t.Fatalf("снимок Java не читается Go-версией: %v", err)
	}
	if len(state.Deal.Players) == 0 {
		t.Fatal("в разобранном снимке нет игроков — разошёлся формат")
	}
	t.Logf("снимок Java прочитан: раздача %d, фаза %s, игроков %d, итогов %d",
		state.DealNo, state.Deal.Phase, len(state.Deal.Players), len(state.Results))

	// ⚠️ И обратно: записанное Go должно читаться Go же без потерь.
	again, err := EncodeMatchState(state)
	if err != nil {
		t.Fatal(err)
	}
	back, err := DecodeMatchState(again)
	if err != nil {
		t.Fatalf("свой снимок после чтения Java не разбирается: %v", err)
	}
	if len(back.Deal.Players) != len(state.Deal.Players) {
		t.Error("после кругового прогона потерялись игроки")
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

// ⭐ Пишет снимок кодеком Go в файл, чтобы его прочитала Java. Обратное направление
// совместимости: требование прямо просит, чтобы Java читала данные, записанные Go.
//
// Запускается только с BARDAK_GO_SNAPSHOT_OUT — обычному прогону файлы не нужны.
func TestWriteSnapshotForJava(t *testing.T) {
	source := os.Getenv("BARDAK_JAVA_SNAPSHOT")
	target := os.Getenv("BARDAK_GO_SNAPSHOT_OUT")
	if source == "" || target == "" {
		t.Skip("BARDAK_JAVA_SNAPSHOT/BARDAK_GO_SNAPSHOT_OUT не заданы")
	}

	raw, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	state, err := DecodeMatchState(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	written, err := EncodeMatchState(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(written), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("снимок записан кодеком Go: %d байт", len(written))
}
