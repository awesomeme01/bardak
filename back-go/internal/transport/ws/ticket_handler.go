package ws

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	apihttp "github.com/awesomeme01/bardak/back-go/internal/transport/http"
)

// TicketIssuer — выдача одноразового тикета.
type TicketIssuer interface {
	Issue(userID string) (string, time.Duration, error)
}

// TicketHandler — POST /api/auth/ws-ticket.
//
// ⚠️ Ручка лежит в пакете сокета, а не рядом с остальным входом: тикет существует
// только ради рукопожатия WebSocket, и держать его отдельно от сокета — значит
// однажды поменять одно и забыть другое.
type TicketHandler struct {
	Tickets TicketIssuer
	Log     *slog.Logger
}

// Routes вешает путь.
func (h TicketHandler) Routes(router chi.Router) {
	router.Post("/api/auth/ws-ticket", h.issue)
}

func (h TicketHandler) issue(w http.ResponseWriter, r *http.Request) {
	principal, ok := apihttp.PrincipalFrom(r.Context())
	if !ok {
		// Сюда не попасть: путь закрыт слоем авторизации. Но молча отдавать тикет
		// без владельца нельзя — это был бы пропуск в чужую игру.
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	value, ttl, err := h.Tickets.Issue(principal.UserID)
	if err != nil {
		apihttp.WriteError(w, r, h.Log, apihttp.ErrInternal)
		return
	}
	apihttp.WriteJSON(w, http.StatusOK, map[string]any{
		"ticket":    value,
		"expiresIn": int64(ttl.Seconds()),
	})
}
