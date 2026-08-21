package ws

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/awesomeme01/bardak/back-go/internal/application"
	"github.com/awesomeme01/bardak/back-go/internal/domain/game"
)

// Порядок рассылки — часть контракта, и все четыре правила ломаются молча.

func sessionForBroadcast(t *testing.T) *application.MatchSession {
	t.Helper()
	config := game.DefaultRulesConfig()

	deal := game.DealState{
		Phase: game.PhaseAttack,
		Players: []game.PlayerState{
			game.NewPlayerState(0, []game.Card{game.NewPip(game.Six, game.Clubs)}, nil),
			game.NewPlayerState(1, []game.Card{game.NewPip(game.Ace, game.Spades)}, nil),
		},
		Table:           []game.TableSlot{},
		PassedSeats:     []int{},
		ExitOrder:       []int{},
		LastAttackCards: []game.Card{},
		DefenderSeat:    1,
	}
	trump := game.NewTrump(game.Hearts)
	deal.Trump = &trump

	return application.NewMatchSession("t-1", "m-1", []application.SeatOwner{
		{SeatNo: 0, UserID: "игрок-0", DisplayName: "Первый"},
		{SeatNo: 1, UserID: "игрок-1", DisplayName: "Второй"},
	}, config, game.MatchState{
		Phase: game.MatchInDeal, NavesLevels: []int{-1, -1}, DealNo: 1, Deal: deal,
	})
}

func drain(t *testing.T, out <-chan []byte) []Envelope {
	t.Helper()
	var got []Envelope
	for {
		select {
		case raw := <-out:
			var envelope Envelope
			if err := json.Unmarshal(raw, &envelope); err != nil {
				t.Fatalf("сообщение не разбирается: %v", err)
			}
			got = append(got, envelope)
		case <-time.After(300 * time.Millisecond):
			return got
		}
	}
}

// ⭐ События уходят ПЕРЕД снимком: клиент анимирует по событию, а состояние берёт
// из снимка. Приди снимок раньше — игрок увидел бы следствие раньше причины.
func TestEventsGoBeforeStateSync(t *testing.T) {
	runtime := NewTableRuntime(context.Background(), "t-1", nil)
	defer runtime.Close()
	session := sessionForBroadcast(t)

	first := runtime.Subscribe("игрок-0")
	runtime.Subscribe("игрок-1")

	Broadcast(runtime, session, 1, []game.DealEvent{
		game.NewCardAttacked(0, game.NewPip(game.Six, game.Clubs)),
		game.NewPassed(0),
	}, nil)

	got := drain(t, first.Out())
	if len(got) != 3 {
		t.Fatalf("получено %d сообщений, ждали два события и снимок", len(got))
	}
	if got[0].Type != "CARD_ATTACKED" || got[1].Type != "PASSED" {
		t.Errorf("порядок событий нарушен: %s, %s", got[0].Type, got[1].Type)
	}
	if got[2].Type != "STATE_SYNC" {
		t.Errorf("последним обязан идти снимок, пришло %s", got[2].Type)
	}
}

// ⚠️ Номер растёт независимо от видимости: игрок, которому событие не видно, получает
// ДЫРУ в нумерации. Нумеровать только отправленное — значит разойтись с журналом матча.
func TestSeqSkipsInvisibleEvents(t *testing.T) {
	runtime := NewTableRuntime(context.Background(), "t-1", nil)
	defer runtime.Close()
	session := sessionForBroadcast(t)

	watcher := runtime.Subscribe("игрок-0")
	runtime.Subscribe("игрок-1")

	Broadcast(runtime, session, 10, []game.DealEvent{
		game.NewCardAttacked(0, game.NewPip(game.Six, game.Clubs)),
		// Приватное событие чужого места — этому игроку невидимо.
		game.FaceDownRevealed{Seat: 1, Card: game.NewPip(game.King, game.Spades)},
		game.NewPassed(0),
	}, nil)

	got := drain(t, watcher.Out())
	var seqs []int
	for _, envelope := range got {
		if envelope.Seq != nil {
			seqs = append(seqs, *envelope.Seq)
		}
	}
	if len(seqs) != 2 {
		t.Fatalf("видимых событий %d, ждали два", len(seqs))
	}
	if seqs[0] != 10 || seqs[1] != 12 {
		t.Errorf("номера %v — ждали 10 и 12: пропущенный номер 11 принадлежит скрытому событию", seqs)
	}
}

// ⭐ Приватное событие видит ТОЛЬКО его владелец: вскрытая карта уходит в его руку,
// а чужую руку не видит никто.
func TestPrivateEventReachesOnlyItsOwner(t *testing.T) {
	runtime := NewTableRuntime(context.Background(), "t-1", nil)
	defer runtime.Close()
	session := sessionForBroadcast(t)

	stranger := runtime.Subscribe("игрок-0")
	owner := runtime.Subscribe("игрок-1")

	hidden := game.NewPip(game.King, game.Spades)
	Broadcast(runtime, session, 1, []game.DealEvent{
		game.FaceDownRevealed{Seat: 1, Card: hidden},
	}, nil)

	for _, envelope := range drain(t, stranger.Out()) {
		if envelope.Type == "FACE_DOWN_REVEALED" {
			t.Fatal("чужая скрытая карта ушла постороннему — это испорченная игра")
		}
	}

	sawIt := false
	for _, envelope := range drain(t, owner.Out()) {
		if envelope.Type == "FACE_DOWN_REVEALED" {
			sawIt = true
			var payload map[string]any
			_ = json.Unmarshal(envelope.Payload, &payload)
			if payload["cardCode"] != "K-spades" {
				t.Errorf("владельцу пришла не та карта: %v", payload["cardCode"])
			}
		}
	}
	if !sawIt {
		t.Error("владелец не увидел собственную вскрытую карту")
	}
}

// ⚠️ Снимок персональный: в чужой руке видно только количество карт.
func TestStateSyncHidesOtherHands(t *testing.T) {
	runtime := NewTableRuntime(context.Background(), "t-1", nil)
	defer runtime.Close()
	session := sessionForBroadcast(t)

	first := runtime.Subscribe("игрок-0")
	runtime.Subscribe("игрок-1")

	Broadcast(runtime, session, 1, nil, nil)

	for _, envelope := range drain(t, first.Out()) {
		if envelope.Type != "STATE_SYNC" {
			continue
		}
		body := string(envelope.Payload)
		// У места 1 на руках туз пик — посторонний его видеть не должен.
		if contains(body, "A-spades") {
			t.Fatalf("чужая карта попала в снимок: %s", body)
		}
		if !contains(body, "6-clubs") {
			t.Error("своя карта пропала из снимка")
		}
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
