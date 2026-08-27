package http

import (
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
)

// ⚠️ Системная таблица MIME отдаёт .m4a как audio/mp4a-latm — Chrome и Safari такое
// проигрывают, а Firefox вправе отказаться, и звук стола там молча исчез бы.
// audio/mp4 — тип, который понимают все.
func init() {
	_ = mime.AddExtensionType(".m4a", "audio/mp4")
}

// StaticHandlers — собранный фронт и ассеты наборов карт.
//
// ⭐ Раздаёт их САМ бэкенд, как и Java: фронт и API живут на одном адресе, поэтому
// у сокета правильный Origin, у cookie правильный домен, а ссылка на игру одна.
// Отдельный статический сервер потребовал бы CORS и второй адрес в каждой инструкции.
type StaticHandlers struct {
	// FrontendPath — каталог собранного фронта (front-bardak/dist).
	FrontendPath string
	// AssetsPath — каталог наборов карт и тем стола.
	AssetsPath string
	Log        *slog.Logger
}

// Routes вешает раздачу.
//
// ⚠️ Порядок важен: `/assets/**` объявляется отдельно и ДО общего перехвата, иначе
// картинка карты уехала бы в SPA-заглушку и приходила бы как index.html с кодом 200 —
// а на экране была бы битая рубашка вместо шестёрки бубён.
func (h StaticHandlers) Routes(router chi.Router) {
	if h.AssetsPath != "" {
		router.Handle("/assets/*", http.StripPrefix("/assets/",
			http.FileServer(http.Dir(h.AssetsPath))))
	}
	if h.FrontendPath == "" {
		return
	}

	files := http.FileServer(http.Dir(h.FrontendPath))
	spa := func(w http.ResponseWriter, r *http.Request) {
		// ⭐ Есть файл — отдаём файл; нет — отдаём index.html. Так работает любой SPA:
		// адрес /table/КОД существует только в браузере, на диске его нет, и 404 на нём
		// означал бы, что игру нельзя открыть по ссылке.
		clean := filepath.Clean(strings.TrimPrefix(r.URL.Path, "/"))
		if clean != "." && !strings.HasPrefix(clean, "..") {
			if info, err := os.Stat(filepath.Join(h.FrontendPath, clean)); err == nil && !info.IsDir() {
				files.ServeHTTP(w, r)
				return
			}
		}
		http.ServeFile(w, r, filepath.Join(h.FrontendPath, "index.html"))
	}

	router.Get("/", spa)
	router.NotFound(func(w http.ResponseWriter, r *http.Request) {
		// ⚠️ Заглушка только для GET и только вне API: на неизвестную ручку API обязан
		// приходить честный JSON-отказ, иначе клиент разбирал бы HTML как ответ сервера.
		if r.Method != http.MethodGet || strings.HasPrefix(r.URL.Path, "/api/") ||
			r.URL.Path == "/ws" {
			WriteError(w, r, h.Log, ErrNotFound)
			return
		}
		spa(w, r)
	})
}
