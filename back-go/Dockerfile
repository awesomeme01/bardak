# Сборка бэкенда и фронта в один образ.
#
# ⭐ Фронт кладётся ВНУТРЬ образа приложения, а не раздаётся отдельным контейнером: Go
# и так умеет его отдавать (BARDAK_FRONTEND_PATH), а лишний слой в схеме — это ещё одно
# место, где версии фронта и бэкенда могут разъехаться. Здесь они собираются вместе
# и уезжают вместе.

# ── фронт ───────────────────────────────────────────────────────────────────
FROM node:20-alpine AS frontend
WORKDIR /src
# Сначала манифесты: слой с зависимостями переживает правки кода.
COPY front-bardak/package.json front-bardak/package-lock.json* ./
RUN npm ci --no-audit --no-fund
COPY front-bardak/ ./
RUN npm run build

# ── бэкенд ──────────────────────────────────────────────────────────────────
FROM golang:1.26-alpine AS backend
WORKDIR /src
COPY back-go/go.mod back-go/go.sum ./
RUN go mod download
COPY back-go/ ./
# ⚠️ CGO выключен намеренно: бинарник должен запускаться в образе без libc.
# Версия подставляется линковщиком — её отдаёт /api/health, и «dev» в проде
# означал бы, что никто не знает, что именно там крутится.
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
        -ldflags "-s -w -X github.com/awesomeme01/bardak/back-go/internal/server.buildVersion=${VERSION}" \
        -o /out/bardak ./cmd/server

# ── образ ───────────────────────────────────────────────────────────────────
FROM alpine:3.20
# Корневые сертификаты нужны push-уведомлениям: они ходят на чужой сервер по HTTPS.
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 bardak
WORKDIR /app

COPY --from=backend /out/bardak /app/bardak
COPY --from=frontend /src/dist /app/frontend
COPY assets /app/assets

ENV BARDAK_FRONTEND_PATH=/app/frontend \
    BARDAK_ASSETS_PATH=/app/assets \
    BARDAK_PORT=8088 \
    BARDAK_ENV=prod

USER bardak
EXPOSE 8088

# ⚠️ Проверка живости бьёт в ту же ручку, что и люди: сервер, «здоровый» по своей особой
# метрике и мёртвый по /api/health, — это худший из возможных вариантов.
HEALTHCHECK --interval=15s --timeout=5s --start-period=20s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8088/api/health >/dev/null || exit 1

ENTRYPOINT ["/app/bardak"]
