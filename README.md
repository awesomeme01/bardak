# Bardak

Онлайн карточная игра «бардак» (аналог дурака) с авторизацией, столами до 5 игроков,
собственной системой рейтинга, историей партий и сменными наборами дизайна карт и стола.

## Стек

| Часть | Технологии |
|---|---|
| Бэкенд | Java 21, Spring Boot 3, Spring Security (JWT), Spring WebSocket, Spring Data JPA, PostgreSQL, Flyway, Gradle (Kotlin DSL) |
| Фронтенд | JS PWA (Vite), Service Worker, Web Push |
| Связь | REST — всё вне живой партии; WebSocket — партия и live-уведомления |
| Окружение | Docker Compose (Postgres, позже Redis) |

## Запуск

Нужны Docker и **Java 21** (на машине по умолчанию активна Java 8 — отсюда явный `JAVA_HOME`):

```bash
docker compose up -d                                          # Postgres
cd back-bardak
JAVA_HOME=$(/usr/libexec/java_home -v 21) ./gradlew bootRun
```

Открыть **http://localhost:8088/** — там видно ответ `/api/health` и живое WS-эхо.
Порт 8088, потому что 8080 занят Docker Desktop; меняется через `BARDAK_PORT`.

Фронт в dev-режиме отдаёт сам Spring из `front-bardak/` — сборка не нужна.

## Структура репозитория

```
bardak/
├── back-bardak/    # Spring Boot: /api/health, WS /ws, миграции, ассеты карт
├── front-bardak/   # JS PWA: пока страница-проба на ES-модулях, без сборщика
├── planning/       # проектная документация — единственный источник контекста
└── docker-compose.yml
```

## С чего начать чтение

1. [planning/00-overview.md](planning/00-overview.md) — что за проект и что входит в MVP
2. [planning/08-roadmap.md](planning/08-roadmap.md) — этапы и текущий
3. [planning/11-worklog.md](planning/11-worklog.md) — где остановились в прошлый раз
4. [planning/10-open-questions.md](planning/10-open-questions.md) — что не решено
5. **[planning/RULES-INPUT.md](planning/RULES-INPUT.md)** — форма для описания правил игры;
   пока не заполнена, а без неё не начать движок

Остальное — по мере надобности:
[01-knowledge-map](planning/01-knowledge-map.md) ·
[02-architecture](planning/02-architecture.md) ·
[03-domain-rules](planning/03-domain-rules.md) ·
[04-db-schema](planning/04-db-schema.md) ·
[05-api-contracts](planning/05-api-contracts.md) ·
[06-card-design-system](planning/06-card-design-system.md) ·
[07-rating-system](planning/07-rating-system.md) ·
[09-decisions](planning/09-decisions.md)

## Текущий статус

**M1 завершён** — каркас запускается: Spring Boot ходит в Postgres, отдаёт `/api/health`,
держит WebSocket с эхо; фронт всё это показывает.

Дальше:
- **M2** — авторизация (JWT, одноразовый ws-тикет). Правилами не блокируется.
- **M4** — движок. Ждёт заполнения [planning/RULES-INPUT.md](planning/RULES-INPUT.md):
  часть вопросов по навесам влияет на структуру состояния, а не только на правила.
