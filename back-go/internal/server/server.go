// Package server собирает бэкенд целиком: репозитории, сценарии, ручки и сокет.
//
// ⭐ Сборка вынесена из main намеренно: ровно её поднимает сквозной тест. Собери он
// свою копию — проверял бы не то, что работает в бою, и первое же расхождение в проводке
// нашлось бы уже на живых игроках.
package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/awesomeme01/bardak/back-go/internal/application"
	"github.com/awesomeme01/bardak/back-go/internal/auth"
	"github.com/awesomeme01/bardak/back-go/internal/config"
	"github.com/awesomeme01/bardak/back-go/internal/observability"
	"github.com/awesomeme01/bardak/back-go/internal/repository"
	apihttp "github.com/awesomeme01/bardak/back-go/internal/transport/http"
	"github.com/awesomeme01/bardak/back-go/internal/transport/protocol"
	"github.com/awesomeme01/bardak/back-go/internal/transport/ws"
)

// Build собирает обработчик и возвращает завершение: столы и таймеры надо гасить,
// иначе остановка сервера ждала бы их goroutine вечно.
//
// Контекст — жизнь сервера: по его отмене goroutine столов завершаются.
func Build(ctx context.Context, cfg config.Config, pool *pgxpool.Pool,
	log *slog.Logger) (http.Handler, func()) {
	router := chi.NewRouter()
	router.Use(middleware.RealIP)
	router.Use(traceMiddleware)
	router.Use(middleware.Recoverer)
	// ⚠️ Авторизация ДО маршрутизации, как в Java: неизвестный путь без токена отвечает
	// 401, а не 404 — сервер не сообщает, существует ли адрес, тому, кто не представился.
	router.Use(apihttp.Authenticate(auth.NewTokenService(cfg.JWTSecret, 15*time.Minute, time.Now)))

	router.Method(http.MethodGet, "/api/health", observability.Health{
		DB:      poolAdapter{pool},
		Version: Version(),
		Log:     log,
	})

	// ── Репозитории ─────────────────────────────────────────────────────────
	users := repository.NewUsers(pool)
	refreshTokens := repository.NewRefreshTokens(pool)
	cardSets := repository.NewCardSets(pool)
	themes := repository.NewTableThemes(pool)
	tables := repository.NewTables(pool)
	ratings := repository.NewRatings(pool)
	history := repository.NewMatchHistory(pool)
	matchLog := repository.NewMatchLog(pool)
	matchPlayers := repository.NewMatchPlayers(pool)
	matchResults := repository.NewMatchResults(pool)
	dealHistory := repository.NewDealHistory(pool)
	friendships := repository.NewFriendships(pool)
	pushes := repository.NewPushSubscriptions(pool)

	// ── Сценарии ────────────────────────────────────────────────────────────
	tokens := auth.NewTokenService(cfg.JWTSecret, 15*time.Minute, time.Now)
	tickets := auth.NewTickets(auth.TicketTTL, time.Now)

	authService := application.NewAuthService(users, refreshTokens, tokens,
		cfg.IsInviteCodeValid, 15*time.Minute, 30*24*time.Hour, time.Now, log)
	profileService := application.NewProfileService(users)
	lobbyService := application.NewLobbyService(tables, time.Now, log)
	ratingService := application.NewRatingService(ratings, users, cfg.IsSeasonAdmin, time.Now)
	statsService := application.NewStatsService(ratings)
	// ⭐ Присутствие и доставка приглашений живут в памяти узла: со вторым узлом это
	// сломалось бы, но второй узел отменён решением (ADR-061), и это осознанная плата.
	presence := application.NewPresence()
	friendService := application.NewFriendService(friendships, users, presence,
		presence, application.TableInviteLookup{Tables: tables}, time.Now)
	historyService := application.NewHistoryService(history, friendService)
	pushService := application.NewPushSubscriptionService(pushes, cfg.VAPIDPublic, cfg.VAPIDPrivate)

	// ── Матч ────────────────────────────────────────────────────────────────
	codec := protocol.Codec{}
	matchService := application.NewMatchService(lobbyService, tables, matchLog, matchPlayers,
		history, codec, nil, nil, log)
	resultService := application.NewMatchResultService(matchResults, ratings, time.Now)
	dealRecorder := application.NewDealRecorder(dealHistory, codec, nil, time.Now)
	turnClock := application.NewTurnClock()

	// ⭐ Реестр столов живёт с контекстом сервера: по SIGTERM goroutine всех столов
	// завершаются, иначе остановка ждала бы их вечно.
	registry := ws.NewTableRegistry(ctx, log)

	// ── Ручки ───────────────────────────────────────────────────────────────
	apihttp.AuthHandlers{Auth: authService, Log: log}.Routes(router)
	apihttp.ProfileHandlers{Profile: profileService, Auth: authService, Log: log}.Routes(router)
	apihttp.CatalogHandlers{CardSets: cardSets, Themes: themes, Log: log}.Routes(router)
	apihttp.TableHandlers{Lobby: lobbyService, Log: log}.Routes(router)
	apihttp.RatingHandlers{Rating: ratingService, Stats: statsService, Log: log}.Routes(router)
	apihttp.HistoryHandlers{History: historyService, Log: log}.Routes(router)
	apihttp.SocialHandlers{Friends: friendService, Push: pushService, Log: log}.Routes(router)
	ws.TicketHandler{Tickets: tickets, Log: log}.Routes(router)

	// ⚠️ Порядок маршрутизаторов важен: команда достаётся ПЕРВОМУ, кто её берёт.
	// Игровые и лоббийные типы не пересекаются, но проверять это должен порядок,
	// а не совпадение двух списков.
	router.Method(http.MethodGet, "/ws", ws.Handler{
		Tickets: tickets,
		Base:    ctx,
		Routers: []ws.CommandRouter{
			ws.GameRouter{
				Matches: matchService, Results: resultService, Deals: dealRecorder,
				Log: matchLog, Lobby: lobbyService, State: codec,
				Registry: registry, Clock: turnClock,
				AutoMove: cfg.AutoMove, TurnTimeout: cfg.TurnTimeout,
				DisconnectGrace: cfg.DisconnectGrace, Logger: log,
			},
			ws.LobbyRouter{Lobby: lobbyService, Names: tables, Registry: registry, Log: log},
		},
		Origins: append(append([]string{}, cfg.WSOrigins...), cfg.WSOriginPatterns...),
		Log:     log,
	})

	// ⚠️ Статика объявляется ПОСЛЕ ручек API: она забирает себе всё неизвестное, включая
	// заглушку 404, и объявленная раньше перехватила бы часть API на себя.
	//
	// Неизвестный путь API — 404 в общем формате, а не 500 и не index.html.
	apihttp.StaticHandlers{
		FrontendPath: cfg.FrontendPath,
		AssetsPath:   cfg.AssetsPath,
		Log:          log,
	}.Routes(router)
	router.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		apihttp.WriteError(w, r, log, apihttp.NewFault(http.StatusMethodNotAllowed,
			"METHOD_NOT_ALLOWED", "Метод не поддержан"))
	})

	return router, func() {
		turnClock.StopAll()
		registry.CloseAll()
	}
}

// traceMiddleware вешает на запрос опознавательный номер и кладёт его в контекст.
//
// ⚠️ Формат — 8 шестнадцатеричных символов, как у Java (`UUID.randomUUID().toString()
// .substring(0, 8)`). Номер уходит клиенту в поле traceId ответа об ошибке, попадает
// в скриншоты жалоб и в тесты; чужой формат сделал бы старые и новые логи несравнимыми.
func traceMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(observability.WithTrace(r.Context(), newTraceID())))
	})
}

func newTraceID() string {
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// Случайность кончиться не может, но если бы могла — лучше пустой номер,
		// чем упавший запрос: traceId нужен для разбора, а не для работы.
		return ""
	}
	return hex.EncodeToString(buf[:])
}

// Version — версия сборки; подставляется линковщиком, иначе «dev», как в Java.
var buildVersion string

// Version — что отдаёт healthcheck.
func Version() string {
	if buildVersion == "" {
		return "dev"
	}
	return buildVersion
}

// poolAdapter сводит pgxpool к узкому интерфейсу health-проверки: она не должна знать
// про пул больше, чем «дай одну строку».
type poolAdapter struct{ pool *pgxpool.Pool }

func (a poolAdapter) QueryRow(ctx context.Context, sql string, args ...any) observability.Row {
	return a.pool.QueryRow(ctx, sql, args...)
}
