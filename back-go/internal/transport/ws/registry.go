package ws

import (
	"context"
	"log/slog"
	"sync"
)

// TableRegistry — реестр живых столов.
//
// ⭐ Стол поднимается по первому подключению и выгружается, когда за ним никого
// не осталось: держать goroutine и очередь на пустой стол незачем.
type TableRegistry struct {
	ctx context.Context
	log *slog.Logger

	mu     sync.Mutex
	tables map[string]*TableRuntime
}

// NewTableRegistry собирает реестр. Контекст — жизнь сервера: по его отмене goroutine
// всех столов завершаются, иначе остановка ждала бы их вечно.
func NewTableRegistry(ctx context.Context, log *slog.Logger) *TableRegistry {
	return &TableRegistry{ctx: ctx, log: log, tables: map[string]*TableRuntime{}}
}

// RuntimeFor — стол, поднимая его при необходимости.
func (r *TableRegistry) RuntimeFor(tableID string) *TableRuntime {
	r.mu.Lock()
	defer r.mu.Unlock()

	if runtime, ok := r.tables[tableID]; ok {
		return runtime
	}
	runtime := NewTableRuntime(r.ctx, tableID, r.log)
	r.tables[tableID] = runtime
	return runtime
}

// Find — уже поднятый стол. Второе значение false — стол спит, и будить его незачем:
// по этому пути ходят обрывы связи, а не команды.
func (r *TableRegistry) Find(tableID string) (*TableRuntime, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	runtime, ok := r.tables[tableID]
	return runtime, ok
}

// Subscribe подписывает игрока на события стола и качает их в его сокет.
//
// ⭐ Подписка и качалка заводятся ВМЕСТЕ: подписка без читателя молча наполняет очередь,
// пока та не переполнится, и игрок теряет соединение из-за того, что его никто не слушал.
func (r *TableRegistry) Subscribe(runtime *TableRuntime, client Client) {
	sub := runtime.Subscribe(client.UserID)
	go func() {
		for {
			select {
			case <-sub.Closed():
				return
			case <-r.ctx.Done():
				return
			case message := <-sub.Out():
				client.SendRaw(message)
			}
		}
	}()
}

// Unsubscribe отписывает игрока и выгружает стол, если он опустел.
func (r *TableRegistry) Unsubscribe(tableID, userID string) {
	r.mu.Lock()
	runtime, ok := r.tables[tableID]
	r.mu.Unlock()
	if !ok {
		return
	}

	runtime.Drop(userID)
	if runtime.Listeners() > 0 {
		return
	}

	// ⚠️ Проверка повторяется под замком: пока стол пустел, к нему мог подключиться
	// новый игрок. Выгрузить стол под ним значило бы оборвать матч на ровном месте.
	r.mu.Lock()
	current, still := r.tables[tableID]
	if still && current == runtime && runtime.Listeners() == 0 {
		delete(r.tables, tableID)
	} else {
		runtime = nil
	}
	r.mu.Unlock()

	if runtime != nil {
		runtime.Close()
		if r.log != nil {
			r.log.Debug("стол опустел и выгружен", "table", tableID)
		}
	}
}

// Size — сколько столов держит узел. Нужен наблюдаемости и тестам.
func (r *TableRegistry) Size() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.tables)
}

// CloseAll гасит все столы: сервер завершается.
func (r *TableRegistry) CloseAll() {
	r.mu.Lock()
	tables := make([]*TableRuntime, 0, len(r.tables))
	for _, runtime := range r.tables {
		tables = append(tables, runtime)
	}
	r.tables = map[string]*TableRuntime{}
	r.mu.Unlock()

	for _, runtime := range tables {
		runtime.Close()
	}
}
