# Bardak

Онлайн карточная игра «бардак» (аналог дурака) с авторизацией, столами до 5 игроков,
собственной системой рейтинга, историей партий и сменными наборами дизайна карт и стола.

## Стек

| Часть | Технологии |
|---|---|
| Бэкенд | Java 21, Spring Boot 3, Spring Security (JWT), Spring WebSocket, Spring Data JPA, PostgreSQL, Flyway, Gradle (Kotlin DSL) |
| Фронтенд | Svelte 5 (Vite), PWA, Service Worker, Web Push |
| Связь | REST — всё вне живой партии; WebSocket — партия и live-уведомления |
| Окружение | Docker Compose (Postgres, позже Redis) |

## Запуск

Нужны Docker и **Java 21** (на машине по умолчанию активна Java 8 — отсюда явный `JAVA_HOME`):

```bash
docker compose up -d                                          # Postgres
cd back-bardak
JAVA_HOME=$(/usr/libexec/java_home -v 21) ./gradlew bootRun
```

Открыть **http://localhost:8088/**. Порт 8088, потому что 8080 занят Docker Desktop;
меняется через `BARDAK_PORT`. Код приглашения для регистрации — `bardak-2026`
(`BARDAK_INVITE_CODES`).

Spring отдаёт **собранный** фронт из `front-bardak/dist/`, поэтому после правок фронта:

```bash
cd front-bardak && npm run build      # нужен Node 20+, на машине по умолчанию Node 10
```

### Игра по локальной сети

Адрес машины подставляется сам: разрешены частные диапазоны (`192.168.*`, `10.*`, `172.16.*`),
и `Origin` для WebSocket на них проходит. Достаточно раздать ссылку:

```bash
echo "http://$(ipconfig getifaddr en0):8088/"
```

⭐ **Звать за стол лучше ссылкой, а не кодом** (ADR-063). В комнате ожидания есть готовая
`http://<хост>:8088/?t=КОД` — она доводит до места сама: вошедшего сажает за стол сразу,
у кого учётки нет — показывает, куда зовут, и сажает после регистрации. Код рядом остаётся
для тех, кому проще продиктовать шесть символов голосом.

### Проверки

```bash
cd back-bardak && ./gradlew check     # 279 юнит + 93 интеграционных (Testcontainers)
cd front-bardak && npm run check      # svelte-check: имена и типы в разметке
tools/smoke/run.sh                    # боты играют составы 2–5 против живого сервера
tools/smoke/loadtest.mjs ramp         # сколько столов держит один узел (M9)
```

⭐ Дымовые проверки — не дублирование `check`: они ловят то, что живёт на стыке слоёв.
Подробности и разбор кодов отказов — в [tools/smoke/README.md](tools/smoke/README.md).

⚠️ `npm run check` — не украшение. **`vite build` собирает разметку с несуществующим
именем без единого слова**, и падает уже экран у игрока: так `compact` пережил
переименование пропса. Проверено обратно — вносим `{compact}` в шаблон, сборка зелёная,
`check` красный.

## Структура репозитория

```
bardak/
├── back-bardak/    # Spring Boot: движок, WS-протокол, миграции, ассеты карт
├── front-bardak/   # Svelte-PWA: лобби, стол, история, реплей, друзья
├── planning/       # проектная документация — единственный источник контекста
├── tools/smoke/    # боты играют настоящие матчи против живого сервера
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

**M0–M8 закрыты.** Игра играется: столы на 2–5 человек, полные правила бардака с навесами,
переводами и джокерами, рейтинг с историей и реплеем, PWA с уведомлениями, друзья
с онлайн-статусом и приглашением за стол. Работает по локальной сети.

Дальше — **M9, измерить и сократить** (ADR-061): бэкенд не расширяем, а упрощаем.
Postgres плюс Java-сервис — это уже много для игры на вечер вдвоём-втроём.
- нагрузочная проверка: сколько столов держит один узел;
- что из бэкенда можно убрать, не потеряв игру;
- Redis, второй инстанс и sticky routing **отменены**: присутствие в памяти узла —
  окончательное решение для текущего размера задачи.

Долгов по этому списку не осталось: `svelte-check` стоит (`npm run check` во `front-bardak`),
реплей переехал на отдельный экран.
