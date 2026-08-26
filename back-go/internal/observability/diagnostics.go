package observability

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"runtime"
	"time"
)

// Diagnostics — служебный сервер: pprof и счётчики рантайма.
//
// ⭐ ОТДЕЛЬНЫЙ порт, а не путь в основном сервере, и по умолчанию выключен. Профили
// показывают внутренности процесса целиком; отдавать их в интернет нельзя, а гадать
// «закрыт ли этот путь авторизацией» — плохой способ это решать. Слушает localhost,
// то есть добраться можно только с самой машины (или через ssh -L).
//
// ⚠️ Счётчики НЕ добавлены в /api/health намеренно: её форма дословно повторяет Java,
// по ней смотрят и люди, и tools/smoke/run.sh, и пока эталон жив — менять её нельзя
// даже к лучшему.
type Diagnostics struct {
	// Pool — статистика пула соединений; nil, если базы нет.
	Pool func() PoolStats
	Log  *slog.Logger
}

// PoolStats — то немногое, что диагностике нужно знать о пуле.
type PoolStats struct {
	Total        int32 `json:"total"`
	Idle         int32 `json:"idle"`
	Acquired     int32 `json:"acquired"`
	MaxConns     int32 `json:"maxConns"`
	AcquireCount int64 `json:"acquireCount"`
	// EmptyAcquireCount — сколько раз за соединением пришлось ЖДАТЬ. Растущее число
	// здесь — первый признак, что пул мал, задолго до того, как это увидит игрок.
	EmptyAcquireCount int64 `json:"emptyAcquireCount"`
}

// Serve поднимает служебный сервер и возвращает его остановку.
//
// Пустой адрес — диагностики нет вовсе.
func (d Diagnostics) Serve(ctx context.Context, addr string) func() {
	if addr == "" {
		return func() {}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	mux.Handle("/debug/runtime", d.runtimeHandler())

	server := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed && d.Log != nil {
			d.Log.Warn("служебный сервер не поднялся", "addr", addr, "err", err)
		}
	}()
	if d.Log != nil {
		d.Log.Info("служебный сервер поднят", "addr", addr)
	}

	return func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = server.Shutdown(stopCtx)
	}
}

// runtimeHandler отдаёт то, по чему видно состояние узла: goroutine и пул.
//
// ⭐ Goroutine здесь — главное число. Стол живёт своей goroutine, и их количество
// показывает, сколько столов на самом деле не закрылось. В Java на этом же месте
// считались потоки, и именно они оказались настоящим пределом узла (ADR-062).
func (d Diagnostics) runtimeHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var memory runtime.MemStats
		runtime.ReadMemStats(&memory)

		body := map[string]any{
			"goroutines": runtime.NumGoroutine(),
			"heapMB":     memory.HeapAlloc / 1024 / 1024,
			"gc":         memory.NumGC,
			"cpus":       runtime.NumCPU(),
			"ts":         time.Now().UnixMilli(),
		}
		if d.Pool != nil {
			body["pool"] = d.Pool()
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(body); err != nil && d.Log != nil {
			d.Log.Warn("не отдал счётчики рантайма", "err", err)
		}
	})
}
