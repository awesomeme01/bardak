package application

import (
	"context"
	"encoding/json"
	"sync"
)

// Presence — кто сейчас в сети и куда доставлять оклики.
//
// ⭐ Присутствие считается по ЖИВЫМ СОКЕТАМ, а не по «последней активности». Игра идёт
// через WebSocket, и открытое соединение — это и есть присутствие. Отметка времени врала
// бы в обе стороны: закрывший вкладку числился бы онлайн ещё минуту, а задумавшийся над
// ходом успел бы «уйти».
//
// ⚠️ Хранилище в памяти узла. Со вторым узлом друзья перестали бы видеть друг друга,
// но второй узел отменён решением (ADR-061), и это осознанная плата, а не недосмотр.
type Presence struct {
	mu       sync.RWMutex
	channels map[string]map[string]func([]byte)
}

// NewPresence заводит реестр присутствия.
func NewPresence() *Presence {
	return &Presence{channels: map[string]map[string]func([]byte){}}
}

// Attach регистрирует канал доставки игрока и возвращает функцию отключения.
//
// ⭐ Каналов у игрока может быть несколько — две вкладки это норма. Онлайн он, пока жив
// хотя бы один; оклик уходит во все, потому что неизвестно, на какую вкладку он смотрит.
func (p *Presence) Attach(userID, channelID string, send func([]byte)) func() {
	p.mu.Lock()
	if p.channels[userID] == nil {
		p.channels[userID] = map[string]func([]byte){}
	}
	p.channels[userID][channelID] = send
	p.mu.Unlock()

	return func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		if channels, ok := p.channels[userID]; ok {
			delete(channels, channelID)
			if len(channels) == 0 {
				delete(p.channels, userID)
			}
		}
	}
}

// IsOnline — есть ли у игрока хоть одно живое соединение.
func (p *Presence) IsOnline(userID string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.channels[userID]) > 0
}

// SendTableInvite доставляет оклик за стол.
//
// Возвращает, дошло ли ПРЯМО СЕЙЧАС: приглашение нигде не хранится, и экран обязан честно
// показать «позвал» либо «его нет в сети — отправил уведомление». Обещать доставку,
// которой не было, хуже, чем признать её отсутствие.
func (p *Presence) SendTableInvite(_ context.Context, friendID, fromName string,
	table InviteTable) bool {
	message, err := json.Marshal(map[string]any{
		"v":    1,
		"type": "TABLE_INVITE",
		"payload": map[string]string{
			"fromName":  fromName,
			"tableId":   table.ID,
			"tableName": table.Name,
			"tableCode": table.Code,
		},
	})
	if err != nil {
		return false
	}

	p.mu.RLock()
	targets := make([]func([]byte), 0, len(p.channels[friendID]))
	for _, send := range p.channels[friendID] {
		targets = append(targets, send)
	}
	p.mu.RUnlock()

	for _, send := range targets {
		send(message)
	}
	return len(targets) > 0
}
