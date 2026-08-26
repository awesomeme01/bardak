// Package push — отправка уведомлений «твой ход» и «тебя ждут за столом».
//
// ⭐ Отправка идёт СВОЕЙ goroutine и никогда — на goroutine стола. Push-сервис браузера
// это чужой сервер в интернете: он может отвечать секунды, а стол на это время замер бы
// для всех, кто за ним сидит (ADR-007).
package push

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"

	"github.com/awesomeme01/bardak/back-go/internal/repository"
)

// Subscriptions — что нужно отправителю от базы.
type Subscriptions interface {
	FindByUserID(ctx context.Context, userID string) ([]repository.PushSubscription, error)
	DeleteByEndpoint(ctx context.Context, endpoint string) error
	MarkSent(ctx context.Context, id string, at time.Time) error
}

// ⭐ Проверка сборкой: репозиторий подписок подходит отправителю.
var _ Subscriptions = repository.PushSubscriptions{}

// queueSize — сколько уведомлений ждут отправки.
//
// ⚠️ Очередь ограничена: недоступный push-сервис не должен превращаться в растущую
// память. Уведомление — не часть партии, и потерять его дешевле, чем узел.
const queueSize = 256

// Sender — отправка уведомлений на подписки игрока.
type Sender struct {
	subscriptions Subscriptions
	options       Options
	now           func() time.Time
	log           *slog.Logger

	queue    chan delivery
	stopOnce sync.Once
	done     chan struct{}
}

// Options — ключи VAPID и контакт владельца сервиса.
type Options struct {
	PublicKey  string
	PrivateKey string
	// Subject — контакт владельца: `mailto:` или адрес сайта. Требование RFC 8292:
	// по нему push-сервис связывается, если отправка мешает.
	Subject string
	// TTL — сколько push-сервис хранит уведомление, если устройство офлайн.
	TTL time.Duration
}

type delivery struct {
	userID  string
	payload []byte
}

// NewSender собирает отправителя и поднимает его goroutine.
//
// ⭐ Отсутствие ключей — не поломка: локально играют с открытой вкладкой, и заводить
// VAPID ради этого незачем. Отправитель тогда просто выключен.
func NewSender(subscriptions Subscriptions, options Options, now func() time.Time,
	log *slog.Logger) *Sender {
	if now == nil {
		now = time.Now
	}
	if options.Subject == "" {
		options.Subject = "mailto:admin@bardak.local"
	}
	if options.TTL == 0 {
		options.TTL = 12 * time.Hour
	}

	sender := &Sender{
		subscriptions: subscriptions, options: options, now: now, log: log,
		queue: make(chan delivery, queueSize), done: make(chan struct{}),
	}
	if !sender.Enabled() {
		if log != nil {
			log.Info("push-уведомления выключены: ключи VAPID не заданы")
		}
		close(sender.done)
		return sender
	}
	go sender.loop()
	return sender
}

// Enabled — уведомления настроены.
func (s *Sender) Enabled() bool {
	return s != nil && s.options.PublicKey != "" && s.options.PrivateKey != ""
}

// Stop останавливает отправителя.
func (s *Sender) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		if s.Enabled() {
			close(s.queue)
			<-s.done
		}
	})
}

// NotifyTurn зовёт игрока к столу: его ход.
func (s *Sender) NotifyTurn(userID, tableName, tableID string) {
	s.send(userID, turnPayload(tableName, tableID))
}

// NotifyPaused зовёт игрока обратно: матч встал из-за него на паузу.
func (s *Sender) NotifyPaused(userID, tableName, tableID string, secondsLeft int64) {
	s.send(userID, pausedPayload(tableName, tableID, secondsLeft))
}

// NotifyInvite — друг зовёт за стол. Того, кто в сети, зовут сокетом; сюда доходят
// только ушедшие.
func (s *Sender) NotifyInvite(userID, fromName, tableName, tableID string) {
	s.send(userID, invitePayload(fromName, tableName, tableID))
}

// send кладёт уведомление в очередь и возвращает управление сразу.
func (s *Sender) send(userID string, payload map[string]string) {
	if !s.Enabled() {
		return
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return
	}

	select {
	case s.queue <- delivery{userID: userID, payload: encoded}:
	default:
		// ⚠️ Очередь переполнена — уведомление теряется молча для игры и громко в журнале.
		// Ждать здесь нельзя: зовущий — это goroutine стола.
		s.warn("очередь уведомлений переполнена, уведомление потеряно", "user", userID)
	}
}

func (s *Sender) loop() {
	defer close(s.done)
	for item := range s.queue {
		s.deliverAll(item)
	}
}

func (s *Sender) deliverAll(item delivery) {
	// Свой контекст: отправка живёт своей жизнью и не привязана ни к запросу, ни к столу.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	subscriptions, err := s.subscriptions.FindByUserID(ctx, item.userID)
	if err != nil {
		s.warn("не прочитал подписки игрока", "user", item.userID, "err", err)
		return
	}
	for _, subscription := range subscriptions {
		s.deliver(ctx, subscription, item.payload)
	}
}

func (s *Sender) deliver(ctx context.Context, subscription repository.PushSubscription,
	payload []byte) {
	response, err := webpush.SendNotificationWithContext(ctx, payload, &webpush.Subscription{
		Endpoint: subscription.Endpoint,
		Keys:     webpush.Keys{P256dh: subscription.P256dh, Auth: subscription.Auth},
	}, &webpush.Options{
		Subscriber:      s.options.Subject,
		VAPIDPublicKey:  s.options.PublicKey,
		VAPIDPrivateKey: s.options.PrivateKey,
		TTL:             int(s.options.TTL.Seconds()),
	})
	if err != nil {
		// Недоступный push-сервис не должен мешать игре: уведомление — не часть партии.
		s.warn("не удалось отправить уведомление", "endpoint", subscription.Endpoint, "err", err)
		return
	}
	defer response.Body.Close()

	// ⚠️ Просроченную подписку push-сервис отвергает кодами 404/410. Такую строку надо
	// удалять: устройство её больше не примет никогда, а копить мусор и стучаться в него
	// при каждом ходе — верный способ упереться в лимиты сервиса.
	if response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusGone {
		if err := s.subscriptions.DeleteByEndpoint(ctx, subscription.Endpoint); err != nil {
			s.warn("мёртвая подписка не удалилась", "endpoint", subscription.Endpoint, "err", err)
		}
		return
	}
	if response.StatusCode >= 300 {
		s.warn("push-сервис отказал", "status", response.StatusCode,
			"endpoint", subscription.Endpoint)
		return
	}

	if err := s.subscriptions.MarkSent(ctx, subscription.ID, s.now()); err != nil {
		s.warn("отметка об отправке не записалась", "id", subscription.ID, "err", err)
	}
}

func (s *Sender) warn(message string, args ...any) {
	if s.log != nil {
		s.log.Warn(message, args...)
	}
}

// ⭐ Тексты дословно как в Java: их показывает service worker игрока, и «улучшенная»
// формулировка была бы заметным изменением поведения, которого никто не просил.
//
// ⭐ Название стола в теле, а не только «твой ход»: у человека может идти несколько
// партий, и уведомление без стола заставляет открывать приложение, чтобы понять, где ждут.
func turnPayload(tableName, tableID string) map[string]string {
	payload := map[string]string{
		"type":  "YOUR_TURN",
		"title": "Твой ход",
		"body":  "За столом ждут тебя",
	}
	if name := strings.TrimSpace(tableName); name != "" {
		payload["body"] = fmt.Sprintf("Стол «%s» ждёт", name)
	}
	return withTable(payload, tableID)
}

// pausedPayload — главный повод для уведомления: матч стоит и ждёт пропавшего, а через
// отведённое время отменится совсем.
func pausedPayload(tableName, tableID string, secondsLeft int64) map[string]string {
	where := "Партия"
	if name := strings.TrimSpace(tableName); name != "" {
		where = fmt.Sprintf("Стол «%s»", name)
	}
	return withTable(map[string]string{
		"type":  "MATCH_PAUSED",
		"title": "Тебя ждут за столом",
		"body": fmt.Sprintf("%s на паузе: вернись за %d с, иначе матч отменят",
			where, secondsLeft),
	}, tableID)
}

func invitePayload(fromName, tableName, tableID string) map[string]string {
	payload := map[string]string{
		"type":  "TABLE_INVITE",
		"title": fmt.Sprintf("%s зовёт за стол", fromName),
		"body":  "Тебя ждут за столом",
	}
	if name := strings.TrimSpace(tableName); name != "" {
		payload["body"] = fmt.Sprintf("Стол «%s» собирается", name)
	}
	return withTable(payload, tableID)
}

// withTable добавляет стол, если он известен: по нему клик по уведомлению открывает
// нужную партию.
func withTable(payload map[string]string, tableID string) map[string]string {
	if tableID != "" {
		payload["tableId"] = tableID
	}
	return payload
}

// ErrKeysRejected — ключи VAPID не годятся. Узнать об этом надо на старте, а не при
// первом же ходе.
var ErrKeysRejected = errors.New("ключи VAPID не приняты")

// CheckKeys проверяет форму ключей, не отправляя ничего.
//
// ⭐ Кривые ключи — поломка, и узнать о ней надо ПРИ СТАРТЕ, а не при первом же ходе:
// иначе сервер поднимется здоровым, а уведомления окажутся мёртвыми ровно тогда, когда
// понадобятся. Так же ведёт себя Java: она собирает PushService на старте и падает,
// если ключи не приняты.
//
// Проверяется именно форма: закрытый ключ P-256 — 32 байта, открытый — 65 байт
// несжатой точки. Настоящую подпись без адреса push-сервиса не сделать.
func CheckKeys(options Options) error {
	if options.PublicKey == "" || options.PrivateKey == "" {
		return nil
	}

	private, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(options.PrivateKey, "="))
	if err != nil || len(private) != 32 {
		return fmt.Errorf("%w: закрытый ключ не 32 байта base64url", ErrKeysRejected)
	}
	public, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(options.PublicKey, "="))
	if err != nil || len(public) != 65 || public[0] != 4 {
		return fmt.Errorf("%w: открытый ключ не 65 байт несжатой точки P-256", ErrKeysRejected)
	}
	return nil
}
