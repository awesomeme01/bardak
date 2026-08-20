package ws

import (
	"sync"
	"testing"
)

// ⭐ Клиент переотправляет команду после обрыва — он не знает, дошла ли она.
func TestRepeatedCommandIsRecognised(t *testing.T) {
	memory := NewCommandMemory()

	if memory.AlreadyApplied("c-1") {
		t.Error("неизвестная команда не может быть применённой")
	}
	memory.Remember("c-1")
	if !memory.AlreadyApplied("c-1") {
		t.Error("повтор не распознан — ход применился бы дважды")
	}
	if memory.AlreadyApplied("c-2") {
		t.Error("чужой идентификатор принят за свой")
	}
}

// ⚠️ Пустой идентификатор не запоминается: иначе все команды без id слились бы в одну.
func TestEmptyIDIsNotRemembered(t *testing.T) {
	memory := NewCommandMemory()
	memory.Remember("")
	if memory.AlreadyApplied("") {
		t.Error("пустой идентификатор не должен считаться применённой командой")
	}
}

// Окно конечное: помнить всё за матч — это память, растущая вместе с партией.
func TestWindowSlides(t *testing.T) {
	memory := NewCommandMemory()

	for i := 0; i < rememberedCommands+50; i++ {
		memory.Remember(string(rune('a'+i%26)) + string(rune('0'+i/26)))
	}
	if len(memory.seen) > rememberedCommands {
		t.Errorf("окно разрослось до %d, а потолок %d", len(memory.seen), rememberedCommands)
	}
	// Последняя команда обязана помниться: именно её клиент и переотправляет.
	last := string(rune('a'+(rememberedCommands+49)%26)) + string(rune('0'+(rememberedCommands+49)/26))
	if !memory.AlreadyApplied(last) {
		t.Error("последняя команда забыта — а её-то и переотправляют")
	}
}

func TestMemoryIsSafeUnderConcurrency(t *testing.T) {
	memory := NewCommandMemory()
	var wg sync.WaitGroup
	wg.Add(50)
	for i := 0; i < 50; i++ {
		go func(n int) {
			defer wg.Done()
			id := string(rune('a' + n%26))
			memory.Remember(id)
			memory.AlreadyApplied(id)
		}(i)
	}
	wg.Wait()
}
