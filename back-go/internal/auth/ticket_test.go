package auth

import (
	"sync"
	"testing"
	"time"
)

func TestTicketIsSingleUse(t *testing.T) {
	tickets := NewTickets(TicketTTL, nil)

	value, ttl, err := tickets.Issue("игрок-1")
	if err != nil {
		t.Fatal(err)
	}
	if ttl != TicketTTL {
		t.Errorf("срок %v, ждали %v", ttl, TicketTTL)
	}

	userID, ok := tickets.Redeem(value)
	if !ok || userID != "игрок-1" {
		t.Fatalf("первое предъявление не прошло: %q, %v", userID, ok)
	}
	// ⭐ Второй раз тот же тикет не работает — в этом весь смысл одноразовости.
	if _, ok := tickets.Redeem(value); ok {
		t.Error("тикет сработал дважды")
	}
}

func TestExpiredTicketIsRefused(t *testing.T) {
	now := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	current := now
	tickets := NewTickets(30*time.Second, func() time.Time { return current })

	value, _, _ := tickets.Issue("игрок-2")
	current = now.Add(31 * time.Second)

	if _, ok := tickets.Redeem(value); ok {
		t.Error("протухший тикет принят")
	}
}

func TestUnknownTicketIsRefused(t *testing.T) {
	tickets := NewTickets(TicketTTL, nil)
	if _, ok := tickets.Redeem("выдуманный"); ok {
		t.Error("выдуманный тикет принят")
	}
}

// ⚠️ Гашение атомарно: двое, предъявившие один тикет одновременно, не должны пройти оба.
func TestConcurrentRedeemLetsExactlyOneThrough(t *testing.T) {
	tickets := NewTickets(TicketTTL, nil)
	value, _, _ := tickets.Issue("игрок-3")

	const racers = 20
	var wg sync.WaitGroup
	var mu sync.Mutex
	winners := 0

	wg.Add(racers)
	for i := 0; i < racers; i++ {
		go func() {
			defer wg.Done()
			if _, ok := tickets.Redeem(value); ok {
				mu.Lock()
				winners++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if winners != 1 {
		t.Errorf("рукопожатие прошли %d из %d, а должен был ровно один", winners, racers)
	}
}
