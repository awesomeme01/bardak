package protocol

import (
	"encoding/json"
	"fmt"

	"github.com/awesomeme01/bardak/back-go/internal/domain/game"
)

// commandPayload — поля, которые может нести игровая команда.
//
// ⚠️ Разбирается мягко: лишние поля игнорируются, отсутствующие остаются пустыми.
// Строгий разбор рвал бы игру на клиенте, который прислал на одно поле больше.
type commandPayload struct {
	CardCode       string `json:"cardCode"`
	TargetCardCode string `json:"targetCardCode"`
	// MoreCards — дополнительные копии того же ранга при навесе отстающему (§2.3).
	MoreCards []string `json:"moreCards"`
	Suit      string   `json:"suit"`
}

// ToCommand переводит сообщение протокола в команду движка.
//
// ⭐ Клиент присылает НАМЕРЕНИЕ, а роль и фазу берёт сервер из состояния. Поэтому здесь
// нет ни выбора «атака или защита» по флагу клиента, ни доверия к месту: место приходит
// от того, кто уже прошёл рукопожатие.
//
// ⚠️ PLAY_CARD с указанной целью — это защита, без цели — атака. Смысл задаёт роль,
// и различать их отдельными типами команд протокола не нужно.
func ToCommand(commandType string, seatNo int, raw json.RawMessage) (game.DealCommand, error) {
	var payload commandPayload
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &payload); err != nil {
			return nil, fmt.Errorf("полезная нагрузка команды не разобрана: %w", err)
		}
	}

	switch commandType {
	case "PLAY_CARD":
		card, err := DecodeCard(payload.CardCode)
		if err != nil {
			return nil, err
		}
		if payload.TargetCardCode == "" {
			return game.AttackCommand{Seat: seatNo, Card: card}, nil
		}
		target, err := DecodeCard(payload.TargetCardCode)
		if err != nil {
			return nil, err
		}
		return game.DefendCommand{Seat: seatNo, Card: card, Target: target}, nil

	case "TRANSFER":
		card, err := DecodeCard(payload.CardCode)
		if err != nil {
			return nil, err
		}
		return game.TransferCommand{Seat: seatNo, Card: card}, nil

	case "PASS":
		return game.PassCommand{Seat: seatNo}, nil

	case "TAKE":
		return game.TakeCommand{Seat: seatNo}, nil

	case "HANG_CARD":
		card, err := DecodeCard(payload.CardCode)
		if err != nil {
			return nil, err
		}
		// ⚠️ Именно nil, а не пустой слайс: «дополнительных карт нет» и «их ноль штук» —
		// одно и то же, а круговой прогон команды эти два состояния различает.
		var also []game.Card
		for _, code := range payload.MoreCards {
			extra, err := DecodeCard(code)
			if err != nil {
				return nil, err
			}
			also = append(also, extra)
		}
		return game.HangCardCommand{Seat: seatNo, Card: card, Also: also}, nil

	case "RECALL_CARD":
		card, err := DecodeCard(payload.CardCode)
		if err != nil {
			return nil, err
		}
		return game.RecallCardCommand{Seat: seatNo, Card: card}, nil

	case "PIN_CARD":
		card, err := DecodeCard(payload.CardCode)
		if err != nil {
			return nil, err
		}
		return game.PinCardCommand{Seat: seatNo, Card: card}, nil

	case "HANG_SKIP":
		return game.HangSkipCommand{Seat: seatNo}, nil

	case "CHOOSE_TRUMP":
		suit, err := ParseSuit(payload.Suit)
		if err != nil {
			return nil, err
		}
		return game.ChooseTrumpCommand{Seat: seatNo, Suit: suit}, nil

	case "REVEAL_FACE_DOWN":
		// ⭐ Карта НЕ называется: игрок сам её не видит. С целью — это вскрытие ради
		// защиты, без цели — ради атаки.
		if payload.TargetCardCode == "" {
			return game.RevealFaceDownCommand{Seat: seatNo}, nil
		}
		target, err := DecodeCard(payload.TargetCardCode)
		if err != nil {
			return nil, err
		}
		return game.RevealFaceDownToDefendCommand{Seat: seatNo, Target: target}, nil

	default:
		return nil, fmt.Errorf("неизвестная команда: %q", commandType)
	}
}
