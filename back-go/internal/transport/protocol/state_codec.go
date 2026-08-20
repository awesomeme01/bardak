package protocol

import (
	"encoding/json"
	"fmt"

	"github.com/awesomeme01/bardak/back-go/internal/domain/game"
)

// Снимок состояния матча в JSON и обратно.
//
// ⚠️ Формат ОБЯЗАН совпадать с Java до последнего поля: в окне отката матч, начатый
// на одном бэкенде, поднимается на другом ровно из этого снимка. Разъехавшееся поле
// означает не ошибку разбора, а потерянную партию.
//
// Кодек написан руками, а не тегами на типах домена: домен не знает про JSON, и
// размечать его — впустить инфраструктуру туда, где её намеренно нет.

type snapshotRoot struct {
	Phase       string          `json:"phase"`
	DealNo      int             `json:"dealNo"`
	MatchSeed   int64           `json:"matchSeed"`
	NavesLevels []int           `json:"navesLevels"`
	Deal        snapshotDeal    `json:"deal"`
	Results     []snapshotDeal2 `json:"results"`
}

type snapshotDeal struct {
	Phase                  string           `json:"phase"`
	Trump                  *string          `json:"trump,omitempty"`
	Deck                   []string         `json:"deck"`
	Players                []snapshotPlayer `json:"players"`
	Table                  []snapshotSlot   `json:"table"`
	RoundStarterSeat       int              `json:"roundStarterSeat"`
	AttackRightSeat        int              `json:"attackRightSeat"`
	DefenderSeat           int              `json:"defenderSeat"`
	PassedSeats            []int            `json:"passedSeats"`
	ExitOrder              []int            `json:"exitOrder"`
	AnyCardBeatenThisRound bool             `json:"anyCardBeatenThisRound"`
	AnyPileDiscarded       bool             `json:"anyPileDiscarded"`
	LastAttackCards        []string         `json:"lastAttackCards"`
	RngSeed                int64            `json:"rngSeed"`
	DiceRolls              int              `json:"diceRolls"`
	HangingWindow          *snapshotWindow  `json:"hangingWindow,omitempty"`
	PendingHiddenTrump     *snapshotPending `json:"pendingHiddenTrump,omitempty"`
}

type snapshotPlayer struct {
	SeatNo          int      `json:"seatNo"`
	Hand            []string `json:"hand"`
	FaceDownCard    *string  `json:"faceDownCard,omitempty"`
	InDeal          bool     `json:"inDeal"`
	NavesLevel      int      `json:"navesLevel"`
	HungCards       []string `json:"hungCards"`
	JokerHangerSeat int      `json:"jokerHangerSeat"`
}

type snapshotSlot struct {
	Attack  string  `json:"attack"`
	Defence *string `json:"defence,omitempty"`
}

type snapshotWindow struct {
	VictimSeat         int            `json:"victimSeat"`
	Steps              [][]int        `json:"steps"`
	StepIndex          int            `json:"stepIndex"`
	Claims             []snapshotClam `json:"claims"`
	Decided            []int          `json:"decided"`
	EveryClaimantHangs bool           `json:"everyClaimantHangs"`
}

type snapshotClam struct {
	SeatNo int    `json:"seatNo"`
	Card   string `json:"card"`
}

type snapshotPending struct {
	Card          string `json:"card"`
	RecipientSeat int    `json:"recipientSeat"`
	ChooserSeat   int    `json:"chooserSeat"`
}

// snapshotDeal2 — итог сыгранной раздачи.
type snapshotDeal2 struct {
	DealLoserSeat   int              `json:"dealLoserSeat"`
	TrumpSuit       *string          `json:"trumpSuit,omitempty"`
	LastAttackCards []string         `json:"lastAttackCards"`
	Players         []snapshotResult `json:"players"`
}

type snapshotResult struct {
	SeatNo      int              `json:"seatNo"`
	LevelBefore int              `json:"levelBefore"`
	LevelAfter  int              `json:"levelAfter"`
	Place       int              `json:"place"`
	LossDegree  *string          `json:"lossDegree,omitempty"`
	HungCards   []string         `json:"hungCards"`
	Changes     []snapshotChange `json:"changes"`
}

type snapshotChange struct {
	Reason string `json:"reason"`
	Amount int    `json:"amount"`
}

// EncodeMatchState записывает снимок.
func EncodeMatchState(state game.MatchState) (string, error) {
	root := snapshotRoot{
		Phase:       state.Phase.String(),
		DealNo:      state.DealNo,
		MatchSeed:   state.MatchSeed,
		NavesLevels: intsOrEmpty(state.NavesLevels),
		Deal:        encodeDeal(state.Deal),
		Results:     encodeResults(state.Results),
	}
	raw, err := json.Marshal(root)
	if err != nil {
		return "", fmt.Errorf("снимок матча не записан: %w", err)
	}
	return string(raw), nil
}

// DecodeMatchState читает снимок — в том числе записанный Java.
func DecodeMatchState(raw string) (game.MatchState, error) {
	var root snapshotRoot
	if err := json.Unmarshal([]byte(raw), &root); err != nil {
		return game.MatchState{}, fmt.Errorf("снимок матча не разбирается: %w", err)
	}

	phase := game.MatchInDeal
	if root.Phase == game.MatchOver.String() {
		phase = game.MatchOver
	}
	deal, err := decodeDeal(root.Deal)
	if err != nil {
		return game.MatchState{}, err
	}
	results, err := decodeResults(root.Results)
	if err != nil {
		return game.MatchState{}, err
	}

	return game.MatchState{
		Phase:       phase,
		NavesLevels: intsOrEmpty(root.NavesLevels),
		DealNo:      root.DealNo,
		MatchSeed:   root.MatchSeed,
		Deal:        deal,
		Results:     results,
	}, nil
}

func encodeDeal(deal game.DealState) snapshotDeal {
	node := snapshotDeal{
		Phase:                  deal.Phase.String(),
		Deck:                   EncodeCards(deal.Deck),
		Table:                  make([]snapshotSlot, 0, len(deal.Table)),
		RoundStarterSeat:       deal.RoundStarterSeat,
		AttackRightSeat:        deal.AttackRightSeat,
		DefenderSeat:           deal.DefenderSeat,
		PassedSeats:            intsOrEmpty(deal.PassedSeats),
		ExitOrder:              intsOrEmpty(deal.ExitOrder),
		AnyCardBeatenThisRound: deal.AnyCardBeatenThisRound,
		AnyPileDiscarded:       deal.AnyPileDiscarded,
		LastAttackCards:        EncodeCards(deal.LastAttackCards),
		RngSeed:                deal.RngSeed,
		DiceRolls:              deal.DiceRolls,
	}
	if deal.Trump != nil {
		name := SuitName(deal.Trump.Suit)
		node.Trump = &name
	}
	for _, player := range deal.Players {
		node.Players = append(node.Players, encodePlayer(player))
	}
	if node.Players == nil {
		node.Players = []snapshotPlayer{}
	}
	for _, slot := range deal.Table {
		entry := snapshotSlot{Attack: EncodeCard(slot.Attack)}
		if slot.Defence != nil {
			defence := EncodeCard(slot.Defence)
			entry.Defence = &defence
		}
		node.Table = append(node.Table, entry)
	}
	if deal.HangingWindow != nil {
		node.HangingWindow = encodeWindow(*deal.HangingWindow)
	}
	if deal.PendingHiddenTrump != nil {
		node.PendingHiddenTrump = &snapshotPending{
			Card:          EncodeCard(deal.PendingHiddenTrump.Card),
			RecipientSeat: deal.PendingHiddenTrump.RecipientSeat,
			ChooserSeat:   deal.PendingHiddenTrump.ChooserSeat,
		}
	}
	return node
}

func decodeDeal(node snapshotDeal) (game.DealState, error) {
	phase, err := game.ParseDealPhase(node.Phase)
	if err != nil {
		return game.DealState{}, err
	}

	deal := game.DealState{
		Phase:                  phase,
		RoundStarterSeat:       node.RoundStarterSeat,
		AttackRightSeat:        node.AttackRightSeat,
		DefenderSeat:           node.DefenderSeat,
		PassedSeats:            intsOrEmpty(node.PassedSeats),
		ExitOrder:              intsOrEmpty(node.ExitOrder),
		AnyCardBeatenThisRound: node.AnyCardBeatenThisRound,
		AnyPileDiscarded:       node.AnyPileDiscarded,
		RngSeed:                node.RngSeed,
		DiceRolls:              node.DiceRolls,
	}

	if node.Trump != nil {
		suit, err := ParseSuit(*node.Trump)
		if err != nil {
			return game.DealState{}, err
		}
		trump := game.NewTrump(suit)
		deal.Trump = &trump
	}
	if deal.Deck, err = DecodeCards(node.Deck); err != nil {
		return game.DealState{}, err
	}
	if deal.LastAttackCards, err = DecodeCards(node.LastAttackCards); err != nil {
		return game.DealState{}, err
	}

	deal.Players = make([]game.PlayerState, 0, len(node.Players))
	for _, player := range node.Players {
		decoded, err := decodePlayer(player)
		if err != nil {
			return game.DealState{}, err
		}
		deal.Players = append(deal.Players, decoded)
	}

	deal.Table = make([]game.TableSlot, 0, len(node.Table))
	for _, slot := range node.Table {
		attack, err := DecodeCard(slot.Attack)
		if err != nil {
			return game.DealState{}, err
		}
		entry := game.NewSlot(attack)
		if slot.Defence != nil {
			defence, err := DecodeCard(*slot.Defence)
			if err != nil {
				return game.DealState{}, err
			}
			if entry, err = entry.BeatenWith(defence); err != nil {
				return game.DealState{}, err
			}
		}
		deal.Table = append(deal.Table, entry)
	}

	if node.HangingWindow != nil {
		window, err := decodeWindow(*node.HangingWindow)
		if err != nil {
			return game.DealState{}, err
		}
		deal.HangingWindow = &window
	}
	if node.PendingHiddenTrump != nil {
		card, err := DecodeCard(node.PendingHiddenTrump.Card)
		if err != nil {
			return game.DealState{}, err
		}
		deal.PendingHiddenTrump = &game.PendingHiddenTrump{
			Card:          card,
			RecipientSeat: node.PendingHiddenTrump.RecipientSeat,
			ChooserSeat:   node.PendingHiddenTrump.ChooserSeat,
		}
	}
	return deal, nil
}

func encodePlayer(player game.PlayerState) snapshotPlayer {
	node := snapshotPlayer{
		SeatNo:          player.SeatNo,
		Hand:            EncodeCards(player.Hand),
		InDeal:          player.InDeal,
		NavesLevel:      player.NavesLevel,
		HungCards:       EncodeCards(player.HungCards),
		JokerHangerSeat: player.JokerHangerSeat,
	}
	if player.FaceDownCard != nil {
		hidden := EncodeCard(player.FaceDownCard)
		node.FaceDownCard = &hidden
	}
	return node
}

func decodePlayer(node snapshotPlayer) (game.PlayerState, error) {
	hand, err := DecodeCards(node.Hand)
	if err != nil {
		return game.PlayerState{}, err
	}
	hung, err := DecodeCards(node.HungCards)
	if err != nil {
		return game.PlayerState{}, err
	}
	player := game.PlayerState{
		SeatNo:          node.SeatNo,
		Hand:            hand,
		InDeal:          node.InDeal,
		NavesLevel:      node.NavesLevel,
		HungCards:       hung,
		JokerHangerSeat: node.JokerHangerSeat,
	}
	if node.FaceDownCard != nil {
		card, err := DecodeCard(*node.FaceDownCard)
		if err != nil {
			return game.PlayerState{}, err
		}
		player.FaceDownCard = card
	}
	return player, nil
}

func encodeWindow(window game.HangingWindow) *snapshotWindow {
	node := &snapshotWindow{
		VictimSeat:         window.VictimSeat,
		Steps:              window.Steps,
		StepIndex:          window.StepIndex,
		Claims:             make([]snapshotClam, 0, len(window.Claims)),
		Decided:            intsOrEmpty(window.Decided),
		EveryClaimantHangs: window.EveryClaimantHangs,
	}
	if node.Steps == nil {
		node.Steps = [][]int{}
	}
	for _, claim := range window.Claims {
		node.Claims = append(node.Claims, snapshotClam{
			SeatNo: claim.SeatNo, Card: EncodeCard(claim.Card),
		})
	}
	return node
}

func decodeWindow(node snapshotWindow) (game.HangingWindow, error) {
	window := game.HangingWindow{
		VictimSeat:         node.VictimSeat,
		Steps:              node.Steps,
		StepIndex:          node.StepIndex,
		Claims:             make([]game.HangClaim, 0, len(node.Claims)),
		Decided:            intsOrEmpty(node.Decided),
		EveryClaimantHangs: node.EveryClaimantHangs,
	}
	if window.Steps == nil {
		window.Steps = [][]int{}
	}
	for _, claim := range node.Claims {
		card, err := DecodeCard(claim.Card)
		if err != nil {
			return game.HangingWindow{}, err
		}
		window.Claims = append(window.Claims, game.HangClaim{SeatNo: claim.SeatNo, Card: card})
	}
	return window, nil
}

func encodeResults(results []game.DealOutcome) []snapshotDeal2 {
	out := make([]snapshotDeal2, 0, len(results))
	for _, outcome := range results {
		node := snapshotDeal2{
			DealLoserSeat:   outcome.DealLoserSeat,
			LastAttackCards: EncodeCards(outcome.LastAttackCards),
			Players:         make([]snapshotResult, 0, len(outcome.Players)),
		}
		if outcome.TrumpSuit != nil {
			name := SuitName(*outcome.TrumpSuit)
			node.TrumpSuit = &name
		}
		for _, player := range outcome.Players {
			entry := snapshotResult{
				SeatNo:      player.SeatNo,
				LevelBefore: player.LevelBefore,
				LevelAfter:  player.LevelAfter,
				Place:       player.Place,
				HungCards:   EncodeCards(player.HungCards),
				Changes:     make([]snapshotChange, 0, len(player.Changes)),
			}
			// ⚠️ «Степени нет» в Java было null и поле выпадало. В Go нулевое значение
			// NoLossDegree, и его надо не записать, а не записать как «NONE».
			if player.LossDegree != game.NoLossDegree {
				degree := player.LossDegree.String()
				entry.LossDegree = &degree
			}
			for _, change := range player.Changes {
				entry.Changes = append(entry.Changes, snapshotChange{
					Reason: string(change.Reason), Amount: change.Amount,
				})
			}
			node.Players = append(node.Players, entry)
		}
		out = append(out, node)
	}
	return out
}

func decodeResults(nodes []snapshotDeal2) ([]game.DealOutcome, error) {
	out := make([]game.DealOutcome, 0, len(nodes))
	for _, node := range nodes {
		outcome := game.DealOutcome{
			DealLoserSeat: node.DealLoserSeat,
			Players:       make([]game.PlayerOutcome, 0, len(node.Players)),
		}
		var err error
		if outcome.LastAttackCards, err = DecodeCards(node.LastAttackCards); err != nil {
			return nil, err
		}
		if node.TrumpSuit != nil {
			suit, err := ParseSuit(*node.TrumpSuit)
			if err != nil {
				return nil, err
			}
			outcome.TrumpSuit = &suit
		}
		for _, player := range node.Players {
			hung, err := DecodeCards(player.HungCards)
			if err != nil {
				return nil, err
			}
			entry := game.PlayerOutcome{
				SeatNo:      player.SeatNo,
				LevelBefore: player.LevelBefore,
				LevelAfter:  player.LevelAfter,
				Place:       player.Place,
				HungCards:   hung,
				Changes:     make([]game.LevelChange, 0, len(player.Changes)),
			}
			if player.LossDegree != nil {
				entry.LossDegree = parseLossDegree(*player.LossDegree)
			}
			for _, change := range player.Changes {
				entry.Changes = append(entry.Changes, game.LevelChange{
					Reason: game.LevelChangeReason(change.Reason), Amount: change.Amount,
				})
			}
			outcome.Players = append(outcome.Players, entry)
		}
		out = append(out, outcome)
	}
	return out, nil
}

func parseLossDegree(name string) game.LossDegree {
	for degree := game.NoLossDegree; ; degree++ {
		text := degree.String()
		if text == "?" {
			return game.NoLossDegree
		}
		if text == name {
			return degree
		}
	}
}

// intsOrEmpty — пустой список остаётся списком, а не превращается в null.
//
// ⚠️ Java пишет `[]`, и снимок с `null` вместо массива она бы не разобрала.
func intsOrEmpty(values []int) []int {
	if values == nil {
		return []int{}
	}
	return values
}
