package protocol

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"
)

// Имена полей живого снимка.
//
// ⚠️ Этот тест появился после того, как настоящий фронт УПАЛ на первом же матче против Go:
// `TypeError: Cannot read properties of undefined (reading 'length')` в Seat.svelte.
// Причина — два поля, названные на проводе внутренними именами: `hungCards` вместо `hung`
// и `roundStarterSeat` вместо `attackerSeat`.
//
// ⭐ Ни один из шестидесяти модульных тестов этого не ловил, и differential тоже: он
// гоняется по REST, а STATE_SYNC живёт в сокете, где сравнения с Java не было ни разу.
// Поэтому проверяется не «поле есть», а ПОЛНЫЙ СПИСОК имён — новое поле и переименованное
// одинаково обязаны попасть сюда и в аудит (java-reference/websocket-contract.md §6.3).

func keysOf(t *testing.T, value any) []string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func TestStateSyncKeepsTheFieldNamesJavaSends(t *testing.T) {
	// Все поля заполнены, чтобы ни одно не выпало по omitempty.
	trump, protected, card := "SPADES", "CLUBS", "A-spades"
	victim, seconds := 1, 27
	sync := StateSync{
		TableID: "t", DealNo: 1, Phase: "ATTACK",
		TrumpSuit: &trump, TrumpCard: &card, ProtectedSuit: &protected,
		DeckLeft: 24, DiscardCount: 0, MyHand: []string{"6-spades"},
		IHaveHiddenCard: true, MySeat: 0, Table: []SlotView{}, Players: []SeatState{},
		RoundStarterSeat: 0, DefenderSeat: 1, CanAttackSeat: 0,
		HangingVictimSeat: &victim, TurnSecondsLeft: &seconds,
		AvailableActions: []ActionView{},
	}

	expected := []string{
		"availableActions", "attackerSeat", "canAttackSeat", "dealNo", "deckLeft",
		"defenderSeat", "discardCount", "hangingVictimSeat", "iHaveHiddenCard", "myHand",
		"mySeat", "phase", "players", "protectedSuit", "table", "tableId", "trumpCard",
		"trumpSuit", "turnSecondsLeft",
	}
	sort.Strings(expected)

	got := keysOf(t, sync)
	if strings.Join(got, ",") != strings.Join(expected, ",") {
		t.Fatalf("поля снимка разошлись с Java:\n получили: %s\n ждали:    %s",
			strings.Join(got, ","), strings.Join(expected, ","))
	}
}

func TestSeatStateKeepsTheFieldNamesJavaSends(t *testing.T) {
	rank := "7"
	place := 2
	seat := SeatState{
		SeatNo: 0, UserID: "u", DisplayName: "Игрок", CardsCount: 6,
		HasHiddenCard: true, HungCards: []string{"6-clubs"}, NavesLevel: 1,
		NextNavesRank: &rank, NextIsJoker: false, Passed: true, InDeal: true,
		ExitPlace: &place, StepsToJoker: 8,
	}

	expected := []string{
		"cardsCount", "displayName", "exitPlace", "hasHiddenCard", "hung", "inDeal",
		"navesLevel", "nextIsJoker", "nextNavesRank", "passed", "seatNo", "stepsToJoker",
		"userId",
	}
	sort.Strings(expected)

	got := keysOf(t, seat)
	if strings.Join(got, ",") != strings.Join(expected, ",") {
		t.Fatalf("поля места разошлись с Java:\n получили: %s\n ждали:    %s",
			strings.Join(got, ","), strings.Join(expected, ","))
	}
}

// ⚠️ Обратная сторона: в СНИМКЕ состояния (то, что лежит в базе) поле по-прежнему
// hungCards — так его пишет и читает Java. Спутать два формата — значит либо сломать
// живой экран, либо сделать нечитаемыми старые матчи.
func TestStoredSnapshotStillUsesHungCards(t *testing.T) {
	encoded, err := json.Marshal(snapshotResult{HungCards: []string{"6-clubs"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"hungCards"`) {
		t.Fatalf("снимок состояния перестал писать hungCards: %s", encoded)
	}
}
