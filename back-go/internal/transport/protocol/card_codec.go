// Package protocol — проводные представления: коды карт, снимок состояния, DTO событий.
//
// ⭐ Кодирование живёт ЗДЕСЬ, а не в правилах игры. Домен не знает ни про JSON, ни про
// базу, и размечать его аннотациями значит впустить инфраструктуру в единственное место,
// которое от неё намеренно свободно. Цена — этот пакет; она оплачивается тестом
// на круговой прогон.
package protocol

import (
	"fmt"
	"strings"

	"github.com/awesomeme01/bardak/back-go/internal/domain/game"
)

// jokerPrefix — как в Java.
const jokerPrefix = "Joker-"

// suitNames — имена мастей на проводе. ⚠️ Ровно как `Suit.name()` в Java: снимки,
// записанные ею, читаются этими же строками.
var suitNames = map[game.Suit]string{
	game.Diamonds: "DIAMONDS",
	game.Hearts:   "HEARTS",
	game.Spades:   "SPADES",
	game.Clubs:    "CLUBS",
}

var suitByName = map[string]game.Suit{
	"DIAMONDS": game.Diamonds,
	"HEARTS":   game.Hearts,
	"SPADES":   game.Spades,
	"CLUBS":    game.Clubs,
}

// SuitName — имя масти для снимка и протокола.
func SuitName(suit game.Suit) string {
	if name, ok := suitNames[suit]; ok {
		return name
	}
	return "UNKNOWN"
}

// ParseSuit разбирает имя масти.
func ParseSuit(name string) (game.Suit, error) {
	if suit, ok := suitByName[strings.ToUpper(strings.TrimSpace(name))]; ok {
		return suit, nil
	}
	return 0, fmt.Errorf("неизвестная масть: %q", name)
}

// EncodeCard — код карты на проводе: «A-spades», «10-hearts», «Joker-1».
//
// ⚠️ Это НЕ то же, что человекочитаемый код карты («A♠»): тот идёт в логи, а этот —
// в протокол и в снимок. Перепутать — значит записать снимок, который Java не прочтёт.
func EncodeCard(card game.Card) string {
	switch actual := card.(type) {
	case game.Joker:
		return fmt.Sprintf("%s%d", jokerPrefix, actual.Number)
	case game.Pip:
		return actual.Rank.Code() + "-" + strings.ToLower(SuitName(actual.Suit))
	default:
		return ""
	}
}

// DecodeCard разбирает код карты.
func DecodeCard(code string) (game.Card, error) {
	trimmed := strings.TrimSpace(code)
	if trimmed == "" {
		return nil, fmt.Errorf("пустой код карты")
	}
	if strings.HasPrefix(trimmed, jokerPrefix) {
		var number int
		if _, err := fmt.Sscanf(trimmed[len(jokerPrefix):], "%d", &number); err != nil {
			return nil, fmt.Errorf("номер джокера не разобран: %q", code)
		}
		return game.NewJoker(number)
	}

	// ⚠️ Разделитель ищется С КОНЦА: ранг «10» сам содержит цифры, но не дефис,
	// а вот масть — одно слово. Поиск с начала сломался бы на первом же составном коде.
	separator := strings.LastIndex(trimmed, "-")
	if separator <= 0 {
		return nil, fmt.Errorf("код карты без разделителя: %q", code)
	}
	rank, err := parseRank(trimmed[:separator])
	if err != nil {
		return nil, err
	}
	suit, err := ParseSuit(trimmed[separator+1:])
	if err != nil {
		return nil, err
	}
	return game.NewPip(rank, suit), nil
}

func parseRank(code string) (game.Rank, error) {
	for _, rank := range game.Ranks() {
		if rank.Code() == code {
			return rank, nil
		}
	}
	return 0, fmt.Errorf("неизвестный ранг: %q", code)
}

// EncodeCards — список кодов.
func EncodeCards(cards []game.Card) []string {
	out := make([]string, 0, len(cards))
	for _, card := range cards {
		out = append(out, EncodeCard(card))
	}
	return out
}

// DecodeCards разбирает список кодов.
func DecodeCards(codes []string) ([]game.Card, error) {
	out := make([]game.Card, 0, len(codes))
	for _, code := range codes {
		card, err := DecodeCard(code)
		if err != nil {
			return nil, err
		}
		out = append(out, card)
	}
	return out, nil
}
