package protocol

import "github.com/awesomeme01/bardak/back-go/internal/domain/game"

// Codec — кодеки протокола одним значением.
//
// ⭐ Сценарии не импортируют транспорт: они объявляют у себя узкие интерфейсы («умеет
// закодировать карту», «умеет собрать снимок»), а сборка подставляет сюда этот тип.
// Так формат остаётся в одном месте, а правила остаются проверяемыми без него.
type Codec struct{}

// Encode — карта кодом протокола.
func (Codec) Encode(card game.Card) string { return EncodeCard(card) }

// SuitName — имя масти.
func (Codec) SuitName(suit game.Suit) string { return SuitName(suit) }

// EncodeMatchState — снимок состояния матча.
func (Codec) EncodeState(state game.MatchState) (string, error) { return EncodeMatchState(state) }

// DecodeState — состояние матча из снимка.
func (Codec) DecodeState(raw string) (game.MatchState, error) { return DecodeMatchState(raw) }
