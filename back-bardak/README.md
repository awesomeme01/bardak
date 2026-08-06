# back-bardak

Бэкенд игры. **Пока пусто** — код появится на этапе M1 (см. `../planning/08-roadmap.md`).

## Планируемый стек

- Java 21
- Spring Boot 3 (Web, WebSocket, Security, Data JPA, Validation, Actuator)
- PostgreSQL + Flyway
- Gradle, Kotlin DSL (`build.gradle.kts`)
- Тесты: JUnit 5, Testcontainers

## Планируемая структура пакетов

```
kz.bardak
├── auth          # регистрация, логин, JWT, refresh-токены
├── user          # профиль, аватар
├── lobby         # столы: создание, список, join/leave/ready
├── game
│   ├── engine    # чистая игровая логика: состояние, команды, FSM. Без Spring
│   ├── runtime   # реестр столов, очередь команд на стол, таймеры хода
│   └── ws        # WebSocket-хендлеры, конверты, персональные проекции состояния
├── history       # лог событий партии, реплей
├── rating        # расчёт рейтинга по итогам партии
├── assets        # наборы карт, темы стола, манифесты
└── common        # ошибки, конфиг, утилиты
```

Ключевое архитектурное правило: `game.engine` не знает ни про Spring, ни про БД, ни про WebSocket —
это чистые функции над состоянием, чтобы правила тестировались без поднятия приложения.
Подробнее — `../planning/02-architecture.md`.

## Как будет запускаться (после M1)

```bash
docker compose up -d          # postgres
./gradlew bootRun
```
