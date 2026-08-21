package protocol

import (
	"encoding/json"
	"testing"

	"github.com/awesomeme01/bardak/back-go/internal/domain/game"
)

// ⚠️ PLAY_CARD с целью — защита, без цели — атака. Смысл задаёт роль, и перепутать
// их значит превратить отбой в подкидывание.
func TestPlayCardSplitsByTarget(t *testing.T) {
	attack, err := ToCommand("PLAY_CARD", 1, json.RawMessage(`{"cardCode":"A-spades"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := attack.(game.AttackCommand); !ok {
		t.Errorf("без цели это атака, получили %T", attack)
	}

	defence, err := ToCommand("PLAY_CARD", 1,
		json.RawMessage(`{"cardCode":"A-spades","targetCardCode":"6-clubs"}`))
	if err != nil {
		t.Fatal(err)
	}
	command, ok := defence.(game.DefendCommand)
	if !ok {
		t.Fatalf("с целью это защита, получили %T", defence)
	}
	if command.Target != game.NewPip(game.Six, game.Clubs) {
		t.Error("цель разобрана неверно — отбито было бы не то")
	}
}

// ⭐ Место берётся от того, кто прошёл рукопожатие, а не из тела команды.
func TestSeatComesFromCallerNotPayload(t *testing.T) {
	command, err := ToCommand("PASS", 3, json.RawMessage(`{"seatNo":0}`))
	if err != nil {
		t.Fatal(err)
	}
	if command.SeatNo() != 3 {
		t.Errorf("место %d — клиент не должен назначать себе место", command.SeatNo())
	}
}

// Вскрытие скрытой карты не называет её: игрок сам её не видит.
func TestRevealSplitsByTarget(t *testing.T) {
	forAttack, err := ToCommand("REVEAL_FACE_DOWN", 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := forAttack.(game.RevealFaceDownCommand); !ok {
		t.Errorf("без цели это вскрытие ради атаки, получили %T", forAttack)
	}

	forDefence, err := ToCommand("REVEAL_FACE_DOWN", 0,
		json.RawMessage(`{"targetCardCode":"7-hearts"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := forDefence.(game.RevealFaceDownToDefendCommand); !ok {
		t.Errorf("с целью это вскрытие ради защиты, получили %T", forDefence)
	}
}

// ⚠️ Лишние поля игнорируются: строгий разбор рвал бы игру у клиента, который
// прислал на одно поле больше.
func TestUnknownPayloadFieldsAreIgnored(t *testing.T) {
	command, err := ToCommand("TAKE", 2, json.RawMessage(`{"будущее":"поле","x":1}`))
	if err != nil {
		t.Fatalf("лишние поля не должны ломать разбор: %v", err)
	}
	if _, ok := command.(game.TakeCommand); !ok {
		t.Errorf("получили %T", command)
	}
}

func TestBrokenCardCodeIsRejected(t *testing.T) {
	if _, err := ToCommand("PLAY_CARD", 0, json.RawMessage(`{"cardCode":"мусор"}`)); err == nil {
		t.Error("негодный код карты должен отвергаться, а не превращаться в случайную карту")
	}
	if _, err := ToCommand("НЕТ_ТАКОЙ", 0, nil); err == nil {
		t.Error("неизвестная команда должна отвергаться")
	}
}

func TestChooseTrumpParsesSuit(t *testing.T) {
	command, err := ToCommand("CHOOSE_TRUMP", 1, json.RawMessage(`{"suit":"HEARTS"}`))
	if err != nil {
		t.Fatal(err)
	}
	choose, ok := command.(game.ChooseTrumpCommand)
	if !ok || choose.Suit != game.Hearts {
		t.Error("масть козыря разобрана неверно")
	}
}

// Круговой прогон: действие из проекции обязано разбираться обратно в ту же команду.
// Иначе клиент нажимает кнопку, которую сервер сам же и предложил, а она не проходит.
func TestActionsRoundTripBackToCommands(t *testing.T) {
	commands := []game.DealCommand{
		game.AttackCommand{Seat: 1, Card: game.NewPip(game.Ace, game.Spades)},
		game.DefendCommand{Seat: 1, Card: game.NewPip(game.Ace, game.Spades),
			Target: game.NewPip(game.Six, game.Clubs)},
		game.TransferCommand{Seat: 1, Card: game.NewPip(game.Seven, game.Hearts)},
		game.PassCommand{Seat: 1},
		game.TakeCommand{Seat: 1},
		game.HangCardCommand{Seat: 1, Card: game.MustJoker(2)},
		game.HangSkipCommand{Seat: 1},
		game.ChooseTrumpCommand{Seat: 1, Suit: game.Diamonds},
		game.RevealFaceDownCommand{Seat: 1},
		game.RevealFaceDownToDefendCommand{Seat: 1, Target: game.NewPip(game.Ten, game.Hearts)},
	}

	for _, original := range commands {
		action := ToActionView(original)
		raw, err := json.Marshal(action.Payload)
		if err != nil {
			t.Fatal(err)
		}
		back, err := ToCommand(action.Type, 1, raw)
		if err != nil {
			t.Errorf("%T: действие %q не разбирается обратно: %v", original, action.Type, err)
			continue
		}
		if back != original {
			t.Errorf("%T разошлось после кругового прогона: %#v", original, back)
		}
	}
}
