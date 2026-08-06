# back-bardak

Бэкенд игры. Этап **M1**: приложение поднимается, ходит в Postgres, отдаёт `/api/health`
и держит WebSocket-соединение с эхо-ответом.

## Стек

- Java 21, Spring Boot 3.5
- Web, WebSocket, Data JPA, Validation, Actuator
- PostgreSQL 16 + Flyway
- Gradle, Kotlin DSL

## Запуск

Требуется **Java 21**. На машине по умолчанию активна Java 8, поэтому JDK передаётся явно:

```bash
# из корня репозитория
docker compose up -d                 # Postgres на 5432

cd back-bardak
JAVA_HOME=$(/usr/libexec/java_home -v 21) ./gradlew bootRun
```

Приложение поднимается на **http://localhost:8088** — порт 8080 на машине разработчика занят
Docker Desktop. Переопределяется переменной `BARDAK_PORT`.

| Что | Где |
|---|---|
| Фронт (dev) | http://localhost:8088/ |
| Health | http://localhost:8088/api/health |
| WebSocket | ws://localhost:8088/ws |
| Actuator | http://localhost:8088/actuator/health |

В dev-профиле Spring отдаёт фронт прямо из `../front-bardak/` — сборка не нужна, один порт,
никакого CORS. Когда обновится Node и появится Vite, фронт переедет на свой dev-сервер.

Полезное:

```bash
./gradlew build            # сборка
./gradlew test             # тесты
docker compose down        # остановить Postgres
docker compose down -v     # ... и стереть данные
```

## Структура

```
kz.bardak
├── common.web     # HealthController; сюда же общий обработчик ошибок
└── game.ws        # WebSocketConfig, EchoWebSocketHandler, Envelope
```

Целевая структура пакетов (появляется по мере этапов):

```
kz.bardak
├── auth          # регистрация, логин, JWT, ws-тикеты                    M2
├── user          # профиль                                              M2
├── lobby         # столы: создание, список, join/leave/ready             M3
├── game
│   ├── engine    # чистая игровая логика: состояние, команды, FSM        M4
│   ├── runtime   # реестр столов, очередь команд на стол, таймеры        M4
│   └── ws        # WebSocket, конверты, персональные проекции            M1 ✅
├── history       # лог событий матча, реплей                             M4
├── rating        # расчёт рейтинга по итогам матча                       M6
├── assets        # наборы карт, темы стола                               M3
└── common        # ошибки, конфиг, утилиты                               M1 ✅
```

Ключевое архитектурное правило: `game.engine` не знает ни про Spring, ни про БД, ни про
WebSocket — это чистые функции над состоянием, чтобы правила тестировались без поднятия
приложения. Подробнее — `../planning/02-architecture.md`.

## Ассеты

`assets/card-sets/<набор>/` — картинки карт, имена файлов заданы жёстко
(`6-diamonds.png`, `Joker.png`, `back.png`). См. `assets/card-sets/README.md`.

## Миграции

`src/main/resources/db/migration`. Схемой управляет только Flyway; `ddl-auto: validate`.
Порядок миграций по этапам — `../planning/04-db-schema.md`.
