package ws

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
)

// Размеры очередей.
//
// ⚠️ Обе ОГРАНИЧЕНЫ намеренно. Неограниченная очередь не исчезает под нагрузкой,
// а превращается в память, которая кончится позже и не там, где виновник.
const (
	// commandQueueSize — сколько команд стола ждут обработки.
	commandQueueSize = 64
	// outboundQueueSize — сколько сообщений ждёт отправки ОДНОМУ клиенту.
	outboundQueueSize = 256
)

// TableRuntime — владелец состояния одного стола.
//
// ⭐ Состоянием владеет ОДНА goroutine, и команды приходят к ней через канал. Так
// сериализация ходов достаётся даром: параллельных изменений просто не существует,
// и не нужно ни одного мьютекса вокруг игровых правил.
//
// В Java ту же роль играл однопоточный исполнитель на стол — и упирался в платформенные
// потоки: на 160 столах их набиралось 380 при пуле Tomcat, стоящем на своём потолке
// в 200 (ADR-062). Goroutine стоит килобайты, а не мегабайт.
type TableRuntime struct {
	TableID string

	commands chan tableCommand
	// stop закрывается при остановке; сам канал команд НЕ закрывается никогда.
	//
	// ⚠️ Закрывать канал, в который пишут другие goroutine, нельзя: отправка в закрытый
	// канал паникует, а `select` между «готов stop» и «готова отправка» выбирает случайно.
	// Получается падение раз через раз — то самое, что выглядит как флак, а на деле
	// гонка в остановке.
	stop     chan struct{}
	done     chan struct{}
	closeOne sync.Once

	mu        sync.RWMutex
	listeners map[string]*listener

	log *slog.Logger
}

// tableCommand — работа, выполняемая в goroutine стола.
type tableCommand struct {
	run func()
}

// listener — подписка одного игрока с СОБСТВЕННОЙ ограниченной очередью.
//
// ⚠️ Очередь на клиента, а не общая: без неё один медленный клиент задерживал бы
// рассылку всем остальным, то есть его плохая связь становилась бы общей проблемой.
type listener struct {
	userID  string
	out     chan []byte
	closed  chan struct{}
	oneShot sync.Once
}

// NewTableRuntime поднимает стол и его goroutine.
func NewTableRuntime(ctx context.Context, tableID string, log *slog.Logger) *TableRuntime {
	runtime := &TableRuntime{
		TableID:   tableID,
		commands:  make(chan tableCommand, commandQueueSize),
		stop:      make(chan struct{}),
		done:      make(chan struct{}),
		listeners: make(map[string]*listener),
		log:       log,
	}
	go runtime.loop(ctx)
	return runtime
}

// loop — единственное место, где меняется состояние стола.
func (r *TableRuntime) loop(ctx context.Context) {
	defer close(r.done)
	for {
		select {
		case <-ctx.Done():
			// ⭐ Контекст рвётся при остановке сервера: goroutine обязана завершиться,
			// иначе graceful shutdown ждал бы её вечно.
			return
		case <-r.stop:
			return
		case command := <-r.commands:
			command.run()
		}
	}
}

// Submit ставит работу в очередь стола.
//
// ⚠️ Очередь переполнена — команда ОТКЛОНЯЕТСЯ, а не ждёт. Ожидание здесь означало бы,
// что сокет-goroutine залипла на столе, и один зависший стол утянул бы за собой
// соединения, которые к нему не относятся.
func (r *TableRuntime) Submit(work func()) error {
	// Сначала проверяем остановку отдельно: смешивать её с отправкой в одном select
	// нельзя — выбор между готовыми случаями случаен, и закрытый стол принимал бы
	// команды через раз.
	select {
	case <-r.stop:
		return fmt.Errorf("стол %s уже закрыт", r.TableID)
	default:
	}

	select {
	case r.commands <- tableCommand{run: work}:
		return nil
	case <-r.stop:
		return fmt.Errorf("стол %s уже закрыт", r.TableID)
	default:
		return fmt.Errorf("очередь стола %s переполнена", r.TableID)
	}
}

// Subscribe подписывает игрока.
//
// ⭐ Один игрок — одна подписка: при повторном подключении старая закрывается. Иначе
// вторая вкладка получала бы события дважды, а первая висела бы мёртвой.
func (r *TableRuntime) Subscribe(userID string) *listener {
	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.listeners[userID]; ok {
		existing.close()
	}
	sub := &listener{
		userID: userID,
		out:    make(chan []byte, outboundQueueSize),
		closed: make(chan struct{}),
	}
	r.listeners[userID] = sub
	return sub
}

// Unsubscribe убирает подписку, если она всё ещё та самая.
func (r *TableRuntime) Unsubscribe(userID string, sub *listener) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if current, ok := r.listeners[userID]; ok && current == sub {
		delete(r.listeners, userID)
	}
	sub.close()
}

// SendTo отправляет одному игроку.
//
// ⚠️ Медленный клиент НЕ блокирует игру: переполненная очередь означает, что он не
// успевает читать, и его соединение закрывается. Он переподключится и попросит RESYNC —
// это дешевле, чем остановить стол ради одного зависшего.
func (r *TableRuntime) SendTo(userID string, message []byte) {
	r.mu.RLock()
	sub, ok := r.listeners[userID]
	r.mu.RUnlock()
	if !ok {
		return
	}

	select {
	case sub.out <- message:
	default:
		if r.log != nil {
			r.log.Warn("клиент не успевает читать — закрываю соединение",
				"table", r.TableID, "user", userID)
		}
		sub.close()
	}
}

// Broadcast отправляет всем подписчикам стола.
func (r *TableRuntime) Broadcast(message []byte) {
	r.mu.RLock()
	targets := make([]string, 0, len(r.listeners))
	for userID := range r.listeners {
		targets = append(targets, userID)
	}
	r.mu.RUnlock()

	for _, userID := range targets {
		r.SendTo(userID, message)
	}
}

// Listeners — сколько живых подписок. По нему стол выгружается, когда за ним никого.
func (r *TableRuntime) Listeners() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.listeners)
}

// Close останавливает стол и закрывает все подписки.
func (r *TableRuntime) Close() {
	r.closeOne.Do(func() {
		close(r.stop)
		r.mu.Lock()
		for _, sub := range r.listeners {
			sub.close()
		}
		r.listeners = map[string]*listener{}
		r.mu.Unlock()
	})
	<-r.done
}

// Out — канал исходящих сообщений подписчика.
func (l *listener) Out() <-chan []byte { return l.out }

// Closed — закрыт ли подписчик.
func (l *listener) Closed() <-chan struct{} { return l.closed }

func (l *listener) close() {
	l.oneShot.Do(func() { close(l.closed) })
}
