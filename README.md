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

## Структура репозитория

```
bardak/
├── back-bardak/    # Spring Boot приложение (пока пусто, см. planning/08-roadmap.md → M1)
├── front-bardak/   # JS PWA (пока пусто)
└── planning/       # проектная документация — единственный источник контекста
```

## С чего начать чтение

1. [planning/00-overview.md](planning/00-overview.md) — что за проект и что входит в MVP
2. [planning/08-roadmap.md](planning/08-roadmap.md) — этапы и текущий
3. [planning/11-worklog.md](planning/11-worklog.md) — где остановились в прошлый раз
4. [planning/10-open-questions.md](planning/10-open-questions.md) — что не решено

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

**M0 — репозиторий и документация.** Кода ещё нет.
