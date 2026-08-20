package ws

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"
)

// ⭐ Команды одного стола выполняются ПО ОЧЕРЕДИ, без мьютексов вокруг правил.
// Это и есть смысл «одна goroutine владеет партией».
func TestCommandsRunSequentially(t *testing.T) {
	table := NewTableRuntime(context.Background(), "t-1", nil)
	defer table.Close()

	const commands = 200
	counter := 0 // намеренно БЕЗ защиты: если очередь не последовательна, тест поймает гонку
	var wg sync.WaitGroup
	wg.Add(commands)

	for i := 0; i < commands; i++ {
		if err := table.Submit(func() { counter++; wg.Done() }); err != nil {
			// Очередь могла переполниться — тогда счётчик просто меньше.
			wg.Done()
		}
	}
	wg.Wait()

	if counter == 0 {
		t.Fatal("ни одна команда не выполнилась")
	}
}

// ⚠️ Медленный клиент не должен останавливать игру: переполненная очередь закрывает
// ЕГО соединение, а стол продолжает работать.
func TestSlowClientIsDisconnectedNotBlocking(t *testing.T) {
	table := NewTableRuntime(context.Background(), "t-2", nil)
	defer table.Close()

	slow := table.Subscribe("медленный")
	fast := table.Subscribe("быстрый")

	// Никто не читает у медленного — забиваем его очередь с запасом.
	for i := 0; i < outboundQueueSize+10; i++ {
		table.SendTo("медленный", []byte("сообщение"))
	}

	select {
	case <-slow.Closed():
	case <-time.After(2 * time.Second):
		t.Fatal("медленный клиент не отключён — он блокирует рассылку")
	}

	// ⭐ Главное: быстрый по-прежнему получает сообщения.
	table.SendTo("быстрый", []byte("привет"))
	select {
	case msg := <-fast.Out():
		if string(msg) != "привет" {
			t.Errorf("быстрому пришло %q", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("быстрый клиент перестал получать из-за медленного")
	}
}

// ⭐ Один игрок — одна подписка: повторное подключение закрывает старую.
// Иначе вторая вкладка получала бы события дважды, а первая висела бы мёртвой.
func TestSecondSubscriptionClosesTheFirst(t *testing.T) {
	table := NewTableRuntime(context.Background(), "t-3", nil)
	defer table.Close()

	first := table.Subscribe("игрок")
	second := table.Subscribe("игрок")

	select {
	case <-first.Closed():
	case <-time.After(time.Second):
		t.Fatal("первая подписка не закрыта при повторном подключении")
	}
	if table.Listeners() != 1 {
		t.Errorf("подписок %d, ждали одну", table.Listeners())
	}

	table.SendTo("игрок", []byte("ход"))
	select {
	case <-second.Out():
	case <-time.After(time.Second):
		t.Fatal("новая подписка не получает сообщений")
	}
}

// ⚠️ Отмена контекста обязана завершать goroutine стола: иначе graceful shutdown
// ждал бы её вечно, а сервер не остановился бы по SIGTERM.
func TestRuntimeStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	NewTableRuntime(ctx, "t-4", nil)

	before := runtime.NumGoroutine() - 1 // сама goroutine стола ещё жива
	cancel()

	deadline := time.After(3 * time.Second)
	for {
		if runtime.NumGoroutine() <= before {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("goroutine стола не завершилась: было %d, стало %d",
				before, runtime.NumGoroutine())
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// Утечки goroutine: поднимаем и закрываем много столов, число goroutine возвращается.
func TestNoGoroutineLeakAcrossTables(t *testing.T) {
	// Даём улечься тому, что осталось от предыдущих тестов.
	time.Sleep(100 * time.Millisecond)
	before := runtime.NumGoroutine()

	for i := 0; i < 50; i++ {
		table := NewTableRuntime(context.Background(), "leak", nil)
		table.Subscribe("игрок")
		table.Close()
	}

	deadline := time.After(5 * time.Second)
	for {
		current := runtime.NumGoroutine()
		if current <= before+2 { // небольшой запас на служебные
			return
		}
		select {
		case <-deadline:
			t.Fatalf("утекли goroutine: было %d, стало %d", before, current)
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// Закрытый стол не принимает команды — вместо тихой потери возвращается ошибка.
func TestClosedTableRefusesCommands(t *testing.T) {
	table := NewTableRuntime(context.Background(), "t-5", nil)
	table.Close()

	if err := table.Submit(func() {}); err == nil {
		t.Error("закрытый стол принял команду — она была бы потеряна молча")
	}
}
