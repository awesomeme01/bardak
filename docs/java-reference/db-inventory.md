# Инвентарь PostgreSQL и семантика доступа к данным

Снято с кода: `back-bardak/src/main/resources/db/migration/V1..V11`, все 18 `@Entity` в
`back-bardak/src/main/java/kz/bardak`, сервисы с `@Transactional`, `application.yml`.

Источник истины — миграции и Java-код. Где `planning/04-db-schema.md` расходится с ними,
написано как в коде, а расхождение вынесено в раздел «Расхождения с planning/04-db-schema.md».

## 0. Общая рамка

| Что | Значение | Откуда |
|---|---|---|
| СУБД | PostgreSQL, `jdbc:postgresql://localhost:5432/bardak` | `application.yml` |
| Миграции | Flyway, `classpath:db/migration`, `baseline-on-migrate: false` | `application.yml` |
| `ddl-auto` | `validate` — Hibernate схему не меняет никогда | `application.yml` |
| `open-in-view` | `false` — ленивая загрузка вне транзакции недоступна | `application.yml` |
| Часовой пояс JDBC | `hibernate.jdbc.time_zone: UTC` | `application.yml` |
| Пул | Hikari, `maximum-pool-size: 10` | `application.yml` |

Все временные метки — `timestamptz`. Идентификаторы сущностей — `uuid`, генерируются
в Java (`UUID.randomUUID()`), а не базой; единственные последовательности —
`match_events.id` и `rating_history.id` (`bigserial`).

⭐ Между сущностями **нет ни одной JPA-ассоциации**: ни `@ManyToOne`, ни `@OneToMany`.
Связи выражены голыми `UUID`-колонками, а внешние ключи живут только в базе. Для порта
на Go это удобно: никаких каскадов на уровне ORM, никакого lazy-loading — каждый
репозиторий работает со своей таблицей.

Триггеров, хранимых процедур и представлений в миграциях **нет** (проверено поиском
по `trigger`/`function` — ни одного вхождения).

---

## 1. Таблицы

### 1.1 `users` (V1, `+ avatar` в V8) — сущность `kz.bardak.auth.domain.User`

| Колонка | Тип | Null | Default | Заметка |
|---|---|---|---|---|
| `id` | `uuid` | not null | — | PK, задаётся кодом |
| `username` | `varchar(32)` | not null | — | логин; **UNIQUE снят в V10**, см. ниже |
| `display_name` | `varchar(64)` | not null | — | имя за столом |
| `email` | `varchar(255)` | null | — | `unique` (`users_email_key`) |
| `password_hash` | `varchar(255)` | not null | — | BCrypt |
| `avatar_url` | `varchar(512)` | null | — | **код его никогда не пишет**, только читает |
| `avatar` | `varchar(8)` | null | — | V8: эмодзи-мордочка; `null` — выводится из id |
| `status` | `varchar(16)` | not null | `'ACTIVE'` | check `users_status_check in ('ACTIVE','BLOCKED')` |
| `created_at` | `timestamptz` | not null | `now()` | `insertable=false` — ставит база |
| `updated_at` | `timestamptz` | not null | `now()` | `insertable=false`, **не обновляется**, см. ловушку |

PK: `users_pkey (id)`.

Индексы:
- `users_email_key` — UNIQUE `(email)` (из `unique` в DDL; `NULL` не конфликтуют, поэтому
  сколько угодно пользователей без почты);
- `ux_users_username_lower` — **UNIQUE `(lower(username))`**, V10;
- `users_username_key` — UNIQUE `(username)`, создан в V1 и **удалён в V10**
  (`alter table users drop constraint if exists users_username_key`).

Мэппинг: `User.username` размечен `@Column(nullable = false, unique = true)` — при
`ddl-auto: validate` это ни на что не влияет (Hibernate уникальность не проверяет), но
читать аннотацию как «в базе есть простой UNIQUE по username» **нельзя**: его нет с V10.
Уникальность держит только функциональный индекс по `lower(username)`.

`status` — `@Enumerated(EnumType.STRING)`, значения `ACTIVE`, `BLOCKED`. Удаления
пользователей нет: на них ссылается история матчей, «удалённый» — это `BLOCKED`.
Кода, который выставляет `BLOCKED`, в проекте **не найдено** — только чтение через
`User.isActive()` в `AuthService.login`/`refresh`.

### 1.2 `refresh_tokens` (V1) — `kz.bardak.auth.domain.RefreshToken`

| Колонка | Тип | Null | Default |
|---|---|---|---|
| `id` | `uuid` | not null | — (PK) |
| `user_id` | `uuid` | not null | — |
| `token_hash` | `varchar(255)` | not null | — |
| `expires_at` | `timestamptz` | not null | — |
| `revoked_at` | `timestamptz` | null | — |
| `user_agent` | `varchar(255)` | null | — |
| `created_at` | `timestamptz` | not null | `now()` (`insertable=false`) |

FK: `user_id → users(id) ON DELETE CASCADE`.

Индексы:
- `refresh_tokens_token_hash_key` — UNIQUE `(token_hash)`;
- `idx_refresh_tokens_user` — **частичный** `(user_id) where revoked_at is null`:
  в индексе только живые токены, отозванные его не раздувают.

⭐ Хранится SHA-256-хеш токена в Base64 (`RefreshTokenService.hash`), а не сам токен.
Сам токен — 32 случайных байта, Base64-URL без паддинга, отдаётся клиенту один раз.

### 1.3 `card_sets` (V2) — `kz.bardak.lobby.domain.CardSet`

| Колонка | Тип | Null | Default | Мэппится в JPA |
|---|---|---|---|---|
| `id` | `uuid` | not null | — (PK) | да |
| `code` | `varchar(64)` | not null | — | да, UNIQUE |
| `name` | `varchar(128)` | not null | — | да |
| `description` | `text` | null | — | да |
| `version` | `varchar(16)` | not null | — | да (semver манифеста, **не** оптимистичная блокировка) |
| `preview_url` | `varchar(512)` | null | — | да |
| `is_default` | `boolean` | not null | `false` | да |
| `is_public` | `boolean` | not null | `true` | **нет** |
| `owner_user_id` | `uuid` | null | — | **нет** (`null` — системный набор) |
| `created_at` | `timestamptz` | not null | `now()` | **нет** |

FK: `owner_user_id → users(id)` без `ON DELETE` (то есть `NO ACTION`).

Индексы:
- `card_sets_code_key` — UNIQUE `(code)`;
- `idx_card_sets_single_default` — **частичный UNIQUE** `(is_default) where is_default`:
  набор по умолчанию ровно один. Строк с `is_default = false` может быть сколько угодно —
  они в индекс не попадают.

Сид V2: один набор `classic` (`11111111-…-1111`), `is_default = true`.

### 1.4 `card_assets` (V2) — `kz.bardak.lobby.domain.CardAsset`

| Колонка | Тип | Null | Default |
|---|---|---|---|
| `id` | `uuid` | not null | — (PK) |
| `card_set_id` | `uuid` | not null | — |
| `card_code` | `varchar(24)` | not null | — |
| `asset_url` | `varchar(512)` | not null | — |
| `mime` | `varchar(64)` | not null | — |
| `ordinal` | `smallint` | not null | `0` |

FK: `card_set_id → card_sets(id) ON DELETE CASCADE`.
Индекс: `card_assets_card_set_id_card_code_key` — UNIQUE `(card_set_id, card_code)`.

Сид: 52 карты генерируются `cross join` рангов и мастей (`ordinal = rank*10 + suit`),
плюс `Joker` (`ordinal 200`) и `back` (`ordinal 201`).

⚠️ **Алфавит `card_code` не совпадает с кодами карт движка.** Движок кодирует джокеров
номером — `Joker-1`, `Joker-2` (`CardCodec`), и именно такие коды лежат в JSON-полях
(`match_events.payload`, `deals.last_attack_cards`, `deal_results.hung_cards`,
`match_snapshots.state`). В `card_assets` джокер один — `Joker`, а `back` вообще не код
движка, а рубашка. Схлопывание `Joker-N → Joker` делает клиент (`front-bardak/src/lib/Card.svelte`).
Комментарий в V2 («Код карты движка: 6-diamonds, 10-hearts, Joker, back») в этой части
вводит в заблуждение.

### 1.5 `table_themes` (V2) — `kz.bardak.lobby.domain.TableTheme`

| Колонка | Тип | Null | Default | Мэппится |
|---|---|---|---|---|
| `id` | `uuid` | not null | — (PK) | да |
| `code` | `varchar(64)` | not null | — | да, UNIQUE |
| `name` | `varchar(128)` | not null | — | да |
| `background_url` | `varchar(512)` | null | — | поле есть, геттера нет |
| `felt_color` | `varchar(16)` | null | — | да |
| `default_back_code` | `varchar(16)` | null | — | да |
| `preview_url` | `varchar(512)` | null | — | **нет** |
| `is_default` | `boolean` | not null | `false` | да |
| `created_at` | `timestamptz` | not null | `now()` | **нет** |

Индексы: `table_themes_code_key` UNIQUE `(code)`;
`idx_table_themes_single_default` — частичный UNIQUE `(is_default) where is_default`.

Сид: `green-felt` (`22222222-…-2222`), `felt_color = '#1f6f43'`, `is_default = true`.

### 1.6 `game_tables` (V2) — `kz.bardak.lobby.domain.GameTable`

| Колонка | Тип | Null | Default | Заметка |
|---|---|---|---|---|
| `id` | `uuid` | not null | — | PK |
| `code` | `varchar(8)` | not null | — | UNIQUE; код приглашения, 6 символов |
| `name` | `varchar(64)` | not null | — | |
| `host_user_id` | `uuid` | not null | — | FK → `users(id)` |
| `max_players` | `smallint` | not null | — | check `between 2 and 5` |
| `status` | `varchar(16)` | not null | — | check `in ('WAITING','IN_MATCH','CLOSED')` |
| `card_set_id` | `uuid` | not null | — | FK → `card_sets(id)` |
| `theme_id` | `uuid` | not null | — | FK → `table_themes(id)` |
| `rules_config` | `jsonb` | not null | `'{}'::jsonb` | ⭐ см. §5 |
| `is_private` | `boolean` | not null | `false` | приватный не виден в лобби, только по коду |
| `version` | `integer` | not null | `0` | ⭐ `@Version`, см. §4 |
| `created_at` | `timestamptz` | not null | `now()` | `insertable=false` |
| `closed_at` | `timestamptz` | null | — | |

Ограничения: `game_tables_status_check`, `game_tables_max_players_check`.
Индексы: `game_tables_code_key` UNIQUE `(code)`;
`idx_game_tables_open` — **частичный** `(status) where status = 'WAITING'` (список лобби).

Код приглашения генерируется из алфавита `ABCDEFGHJKLMNPQRSTUVWXYZ23456789` (без похожих
символов — код диктуют голосом), 6 знаков, до 5 попыток с проверкой `existsByCode`;
поиск по коду делает `toUpperCase()` перед запросом.

Строки `game_tables` **никогда не удаляются** — только `status = CLOSED` + `closed_at`.

### 1.7 `table_players` (V2, `+ ux_table_players_user` в V9) — `kz.bardak.lobby.domain.TablePlayer`

| Колонка | Тип | Null | Default |
|---|---|---|---|
| `table_id` | `uuid` | not null | — |
| `user_id` | `uuid` | not null | — |
| `seat_no` | `smallint` | not null | — |
| `state` | `varchar(16)` | not null | — |
| `joined_at` | `timestamptz` | not null | `now()` (`insertable=false`) |

PK: `(table_id, user_id)` — в JPA `@IdClass(TablePlayer.Key.class)`, `Key` — `record`.
FK: `table_id → game_tables(id) ON DELETE CASCADE`; `user_id → users(id)` (`NO ACTION`).
Check: `table_players_state_check in ('JOINED','READY','LEFT')`,
`table_players_seat_check seat_no between 0 and 4`.

Индексы:
- `table_players_table_id_seat_no_key` — **UNIQUE `(table_id, seat_no)`** (см. §3.1);
- `ux_table_players_user` — **UNIQUE `(user_id)`** (V9, см. §3.2).

⚠️ `state = 'LEFT'` в базу **никогда не записывается**: `LobbyService.leave()` строку
**удаляет**, а не помечает. Методы `TablePlayer.leave()` и `TablePlayer.rejoin()` в
продовом коде не вызываются нигде (проверено поиском) — это мёртвый код, оставшийся от
раннего замысла. Фактические значения в колонке — только `JOINED` и `READY`.
Строку удаляют, потому что занятость места определяет уникальный индекс
`(table_id, seat_no)`, а пометка в строке место не отпускает.

### 1.8 `matches` (V3) — `kz.bardak.history.domain.MatchRecord`

| Колонка | Тип | Null | Default | Заметка |
|---|---|---|---|---|
| `id` | `uuid` | not null | — | PK |
| `table_id` | `uuid` | not null | — | FK → `game_tables(id)`, без каскада |
| `status` | `varchar(16)` | not null | — | check `in ('IN_PROGRESS','PAUSED','FINISHED','ABORTED')` |
| `players_count` | `smallint` | not null | — | |
| `deals_played` | `smallint` | not null | `0` | |
| `rng_seed` | `bigint` | not null | — | ⭐ клиенту не отдаётся до конца матча |
| `rules_snapshot` | `jsonb` | not null | — | копия `rules_config` на момент старта |
| `started_at` | `timestamptz` | not null | `now()` (`insertable=false`) | |
| `finished_at` | `timestamptz` | null | — | |
| `loser_user_id` | `uuid` | null | — | FK → `users(id)`; главный проигравший |
| `abort_reason` | `varchar(255)` | null | — | |

Индексы:
- `idx_matches_table_time` — `(table_id, started_at desc)`;
- `idx_matches_status` — **частичный** `(status) where status in ('IN_PROGRESS','PAUSED')`.

⚠️ **`status = 'PAUSED'` в базу не записывается никогда.** У `MatchRecord` есть только
`finish()` и `abort()`; метода, ставящего `PAUSED`, нет. Пауза при отвале игрока живёт
исключительно в памяти и в WS-событии `MATCH_PAUSED` (`GameCommandHandler`, `PushSender`).
Из-за этого частичный индекс `idx_matches_status` фактически покрывает только
`IN_PROGRESS`, а `MatchHistoryService.replay` зря проверяет `PAUSED`.

`abort_reason` пишется двумя строками из `GameCommandHandler`:
`"Игрок вышел из матча"` и `"Игрок не вернулся за отведённое время"`.

### 1.9 `match_players` (V3) — `kz.bardak.history.domain.MatchPlayerRecord`

| Колонка | Тип | Null | Default |
|---|---|---|---|
| `match_id` | `uuid` | not null | — |
| `user_id` | `uuid` | not null | — |
| `seat_no` | `smallint` | not null | — |
| `naves_level` | `varchar(2)` | null | — |
| `loss_type` | `varchar(24)` | null | — |
| `place` | `smallint` | null | — |
| `rating_before` | `numeric(8,2)` | null | — |
| `rating_after` | `numeric(8,2)` | null | — |
| `rating_delta` | `numeric(8,2)` | null | — |

PK `(match_id, user_id)` через `@IdClass`. FK: `match_id → matches(id) ON DELETE CASCADE`;
`user_id → users(id)` (`NO ACTION`). Check `match_players_loss_type_check`:
`loss_type is null or in ('ROYAL','SUPER_MEGA_SUCK','SUPER_MEGA_FAIL','SUPER_FAIL','FAIL')`.
Индекс `idx_match_players_user (user_id)`.

Строка создаётся пустой при старте матча (`MatchResultService.startMatch`), итог
дописывается в `finish()`. У отменённого матча итог остаётся `null` — по этому признаку
`StatsService` отфильтровывает отменённые (`row.place() != null`).

`naves_level` — коды `NavesLevelCodec`: ранг из шкалы (`"6"`…`"A"`, `"10"`) либо
**`"Jk"`** для джокера; `null` — навесов не было («летит 6»).

### 1.10 `deals` (V3, `trump_suit` расширен в V6) — `kz.bardak.history.domain.DealRecord`

| Колонка | Тип | Null | Default |
|---|---|---|---|
| `id` | `uuid` | not null | — (PK) |
| `match_id` | `uuid` | not null | — |
| `deal_no` | `smallint` | not null | — |
| `trump_suit` | `varchar(16)` | null | — (V3 был `varchar(2)`, V6 → `varchar(16)`) |
| `started_at` | `timestamptz` | not null | `now()` (`insertable=false`) |
| `finished_at` | `timestamptz` | null | — |
| `loser_seat` | `smallint` | null | — |
| `last_attack_cards` | `jsonb` | not null | `'[]'::jsonb` |

FK: `match_id → matches(id) ON DELETE CASCADE`.
Индекс: `deals_match_id_deal_no_key` — UNIQUE `(match_id, deal_no)`.

`trump_suit` хранит **имя масти движка**: `DIAMONDS`, `HEARTS`, `SPADES`, `CLUBS`
(`Suit.name()`), а не значок. Ради этого V6 и расширил колонку — «двух символов под имя
не хватает».

Хотя `loser_seat` и `finished_at` в схеме nullable, единственный конструктор
`DealRecord` требует оба, так что через код `null` там не появляется.

### 1.11 `deal_results` (V3) — `kz.bardak.history.domain.DealResultRecord`

| Колонка | Тип | Null | Default |
|---|---|---|---|
| `deal_id` | `uuid` | not null | — |
| `seat_no` | `smallint` | not null | — |
| `place` | `smallint` | null | — |
| `hung_cards` | `jsonb` | not null | `'[]'::jsonb` |
| `naves_level_before` | `varchar(2)` | null | — |
| `naves_level_after` | `varchar(2)` | null | — |
| `level_changes` | `jsonb` | not null | `'[]'::jsonb` |

PK `(deal_id, seat_no)` через `@IdClass`. `Key` здесь — обычный класс с `equals`/`hashCode`
(в отличие от `TablePlayer.Key` и `MatchSnapshotRecord.Key`, которые `record`); единого
стиля в проекте нет, на поведение это не влияет.
FK: `deal_id → deals(id) ON DELETE CASCADE`. Индексов, кроме PK, нет.

### 1.12 `match_events` (V3, `+ private_to_seat` в V4) — `kz.bardak.history.domain.MatchEventRecord`

| Колонка | Тип | Null | Default |
|---|---|---|---|
| `id` | `bigserial` | not null | из последовательности (PK, `@GeneratedValue(IDENTITY)`) |
| `match_id` | `uuid` | not null | — |
| `seq` | `integer` | not null | — |
| `deal_no` | `smallint` | null | — |
| `type` | `varchar(32)` | not null | — |
| `actor_seat` | `smallint` | null | — |
| `payload` | `jsonb` | not null | — |
| `created_at` | `timestamptz` | not null | `now()` |
| `private_to_seat` | `smallint` | null | — (V4) |

FK: `match_id → matches(id) ON DELETE CASCADE`.
Индексы:
- `match_events_match_id_seq_key` — **UNIQUE `(match_id, seq)`**: гарантия отсутствия дыр
  и дублей при гонках;
- `idx_match_events_match` — `(match_id, seq)` (дублирует уникальный по составу; на порт
  влияет только тем, что в схеме два индекса, а не один).

⭐ Таблица **append-only**: в коде есть только `events.save(new MatchEventRecord(...))`,
`UPDATE`/`DELETE` по ней нет нигде. `created_at` в сущности вообще не мэппится.

⭐ `private_to_seat` — записанная вместе с событием видимость: `null` — публичное,
иначе номер места, которому событие видно. Фильтрация при догоне (`RESYNC`) и в реплее
идёт **по этой колонке**, а не пересчётом правил (`MatchLog.since` →
`MatchEventRecord.isVisibleTo`). Смысл: правило видимости не должно жить в двух местах.
Единственный приватный случай в проде — `MOVE_REJECTED` (`MatchLog.appendRejected` кладёт
`privateToSeat = actorSeat`) плюс события с `DealEvent.privateToSeat()` от движка.

`seq` — сквозной по матчу, с 1, выдаётся из памяти (`MatchSession.lastSeq`), а не базой.
Это значит: при двух живых процессах на одном матче уникальный индекс `(match_id, seq)`
поймает коллизию, но восстановиться из неё код не умеет — единственность процесса
обеспечивается тем, что сессия матча живёт в одном узле (`TableRegistry`).

### 1.13 `match_snapshots` (V3) — `kz.bardak.history.domain.MatchSnapshotRecord`

| Колонка | Тип | Null | Default |
|---|---|---|---|
| `match_id` | `uuid` | not null | — |
| `seq` | `integer` | not null | — |
| `state` | `jsonb` | not null | — |
| `created_at` | `timestamptz` | not null | `now()` (`insertable=false`) |

PK `(match_id, seq)` через `@IdClass` (`Key` — `record`).
FK: `match_id → matches(id) ON DELETE CASCADE`. Больше индексов нет; чтение —
`findFirstByMatchIdOrderBySeqDesc` (последний снимок), то есть скан по PK в обратном порядке.

⚠️ Комментарий миграции («Пишется на границе раздач — там оно компактно») **устарел**:
`GameCommandHandler.saveSnapshot` вызывается **после каждого хода**. Причина названа в
javadoc `MatchLog.snapshot`: движок применяет команды, а не события, и проиграть лог
поверх старого снимка нечем — всё после снимка было бы потеряно.

### 1.14 `user_rating` (V5) — `kz.bardak.rating.domain.UserRating`

| Колонка | Тип | Null | Default |
|---|---|---|---|
| `user_id` | `uuid` | not null | — (PK) |
| `rating` | `numeric(8,2)` | not null | `1000` |
| `deviation` | `numeric(8,2)` | not null | `350` |
| `volatility` | `numeric(8,5)` | not null | `0.06` |
| `matches_played` | `integer` | not null | `0` |
| `updated_at` | `timestamptz` | not null | `now()` |

FK: `user_id → users(id) ON DELETE CASCADE`. Индексов, кроме PK, нет — лидерборд
(`findAllByOrderByRatingDesc`) читает таблицу целиком и сортирует.

`deviation` и `volatility` MVP не считает: они заводятся под будущий Glicko-2/OpenSkill.
Значения по умолчанию дублируются в конструкторе `UserRating` (350 и 0.06), то есть база
их фактически никогда не подставляет.

⚠️ `updated_at` в сущности **не** помечен `insertable=false` — Java всегда пишет своё
значение из `Clock`, дефолт `now()` не срабатывает.

### 1.15 `rating_history` (V5) — `kz.bardak.rating.domain.RatingHistoryEntry`

| Колонка | Тип | Null | Default |
|---|---|---|---|
| `id` | `bigserial` | not null | PK, `@GeneratedValue(IDENTITY)` |
| `user_id` | `uuid` | not null | — |
| `match_id` | `uuid` | not null | — |
| `rating_before` | `numeric(8,2)` | not null | — |
| `rating_after` | `numeric(8,2)` | not null | — |
| `deviation_after` | `numeric(8,2)` | not null | — |
| `place` | `smallint` | not null | — |
| `players_count` | `smallint` | not null | — |
| `season_id` | `uuid` | null | — (добавлена `alter table` в той же V5) |
| `created_at` | `timestamptz` | not null | `now()` (`insertable=false`) |

FK: `user_id → users(id) ON DELETE CASCADE`; `match_id → matches(id) ON DELETE CASCADE`;
`season_id → seasons(id)` (`NO ACTION`).

Индексы:
- `rating_history_user_id_match_id_key` — **UNIQUE `(user_id, match_id)`**: матч
  закрывается ровно один раз, защита от двойного начисления (см. §3.3);
- `idx_rating_history_user_time` — `(user_id, created_at desc)`.

### 1.16 `seasons` (V5) — `kz.bardak.rating.domain.Season`

| Колонка | Тип | Null | Default | Мэппится |
|---|---|---|---|---|
| `id` | `uuid` | not null | — (PK) | да |
| `name` | `varchar(64)` | not null | — | да |
| `started_at` | `timestamptz` | not null | `now()` | да, `updatable=false`, но **insertable** — пишет код |
| `closed_at` | `timestamptz` | null | — | да |
| `created_at` | `timestamptz` | not null | `now()` | **нет** |

Индекс: `idx_seasons_single_open` — **частичный UNIQUE `(closed_at) where closed_at is null`**:
открытый сезон ровно один (см. §3.4). Сид V5: «Первый сезон» (`33333333-…-3333`).

### 1.17 `push_subscriptions` (V7) — `kz.bardak.push.domain.PushSubscription`

| Колонка | Тип | Null | Default |
|---|---|---|---|
| `id` | `uuid` | not null | — (PK) |
| `user_id` | `uuid` | not null | — |
| `endpoint` | `varchar(512)` | not null | — |
| `p256dh` | `varchar(128)` | not null | — |
| `auth` | `varchar(64)` | not null | — |
| `user_agent` | `varchar(256)` | null | — |
| `created_at` | `timestamptz` | not null | `now()` (`insertable=false`) |
| `last_sent_at` | `timestamptz` | null | — |

FK: `user_id → users(id) ON DELETE CASCADE`.
Индексы: `push_subscriptions_endpoint_key` — **UNIQUE `(endpoint)`**;
`idx_push_subscriptions_user (user_id)`.

⭐ Подписка принадлежит **устройству**, а не человеку: телефон и ноутбук — две строки.
Ключ — `endpoint`, его выдаёт push-сервис браузера.

⚠️ Повторная подписка на тот же `endpoint` **обновляет строку** (`PushSubscription.reassign`),
а не пересоздаёт. Причина в javadoc: удалить и вставить в одной транзакции нельзя —
Hibernate откладывает `DELETE` до сброса, `INSERT` успевает первым и падает на UNIQUE.
В Go это ограничение снимается обычным `INSERT … ON CONFLICT (endpoint) DO UPDATE`,
но семантику «владелец и ключи меняются, `id` и `created_at` сохраняются» надо повторить.

Строки удаляются в двух местах: `unsubscribe(endpoint)` и `PushSender.deliver` при ответе
push-сервиса **404 или 410** (`PushSender.GONE = List.of(404, 410)`) — подписка мертва.

### 1.18 `friendships` (V11) — `kz.bardak.social.domain.Friendship`

| Колонка | Тип | Null | Default |
|---|---|---|---|
| `low_user_id` | `uuid` | not null | — |
| `high_user_id` | `uuid` | not null | — |
| `requested_by` | `uuid` | not null | — |
| `status` | `varchar(16)` | not null | — |
| `created_at` | `timestamptz` | not null | `now()` (`insertable=false`) |
| `decided_at` | `timestamptz` | null | — |

PK `(low_user_id, high_user_id)` через `@IdClass`.
FK: все три `uuid` → `users(id)` без `ON DELETE`.
Check: `status in ('PENDING','ACCEPTED')`, `friendships_not_self (low <> high)`,
`friendships_ordered (low_user_id < high_user_id)`.
Индекс: `idx_friendships_high (high_user_id)` — вторая половина пары; поиск «все пары
игрока» идёт `where low = :id or high = :id`, и без этого индекса половина запроса шла бы
сканом.

⭐ **Одна строка на пару, а не две зеркальные.** Заявка и дружба — одна и та же строка в
разных `status`; отказ и разрыв — `DELETE`, отказы не хранятся.

⚠️ Порядок пары считается **по канонической строковой записи UUID**
(`Friendship.comparePairOrder` → `one.toString().compareTo(two.toString())`), а **не**
`UUID.compareTo`. `UUID.compareTo` сравнивает два **знаковых** `long`, Postgres — побайтово;
на идентификаторах со старшим единичным битом порядки противоположны, и строка падала на
`friendships_ordered`. Для Go: сравнивать нужно тем же способом, что и Postgres —
побайтово (`bytes.Compare` по 16 байтам) или по hex-строке, они совпадают.

---

## 2. Внешние ключи и `ON DELETE` — сводка

| Таблица.колонка | → | `ON DELETE` |
|---|---|---|
| `refresh_tokens.user_id` | `users.id` | **CASCADE** |
| `card_sets.owner_user_id` | `users.id` | NO ACTION |
| `card_assets.card_set_id` | `card_sets.id` | **CASCADE** |
| `game_tables.host_user_id` | `users.id` | NO ACTION |
| `game_tables.card_set_id` | `card_sets.id` | NO ACTION |
| `game_tables.theme_id` | `table_themes.id` | NO ACTION |
| `table_players.table_id` | `game_tables.id` | **CASCADE** |
| `table_players.user_id` | `users.id` | NO ACTION |
| `matches.table_id` | `game_tables.id` | NO ACTION |
| `matches.loser_user_id` | `users.id` | NO ACTION |
| `match_players.match_id` | `matches.id` | **CASCADE** |
| `match_players.user_id` | `users.id` | NO ACTION |
| `deals.match_id` | `matches.id` | **CASCADE** |
| `deal_results.deal_id` | `deals.id` | **CASCADE** |
| `match_events.match_id` | `matches.id` | **CASCADE** |
| `match_snapshots.match_id` | `matches.id` | **CASCADE** |
| `user_rating.user_id` | `users.id` | **CASCADE** |
| `rating_history.user_id` | `users.id` | **CASCADE** |
| `rating_history.match_id` | `matches.id` | **CASCADE** |
| `rating_history.season_id` | `seasons.id` | NO ACTION |
| `push_subscriptions.user_id` | `users.id` | **CASCADE** |
| `friendships.low_user_id` / `.high_user_id` / `.requested_by` | `users.id` | NO ACTION |

Практические следствия:
- Удалить `users` физически **невозможно**, пока есть `game_tables.host_user_id`,
  `table_players.user_id`, `match_players.user_id`, `matches.loser_user_id` или
  `friendships.*` — они `NO ACTION`. Поэтому «удаление» пользователя задумано как
  `status = BLOCKED` (кода, который это делает, в проекте нет).
- Удалить `game_tables` нельзя, пока по столу есть `matches` (`NO ACTION`). Столы и не
  удаляются — только закрываются.
- Удаление `matches` вычищает всю историю матча: игроков, раздачи с результатами, лог,
  снимки и строки рейтинговой истории. Кода, который удаляет матчи, тоже нет.
- Каскад `table_players.table_id` работает вхолостую по той же причине.

---

## 3. Инварианты, которые держит база вместо кода ⭐

### 3.1 `unique (table_id, seat_no)` — место не достаётся двоим

Гонка: двое одновременно нажимают «сесть», оба читают список занятых мест, оба выбирают
одно и то же минимальное свободное. Проверка в коде проходит у обоих.

Закрывает индекс: второй `INSERT` нарушает уникальность.

Обработка (`LobbyService.join` + `SeatAllocator.allocate`):
- каждая попытка занять место идёт в **своей** транзакции (`REQUIRES_NEW`), потому что
  после нарушения ограничения продолжать в той же транзакции нельзя — Postgres отвечает
  «current transaction is aborted» на любую следующую команду;
- `SeatAllocator` использует `saveAndFlush`, чтобы нарушение всплыло **внутри** вложенной
  транзакции, а не при коммите внешней;
- цикл — до 5 попыток (`SEAT_ATTEMPTS`); исчерпав их, отвечает `409 TABLE_FULL`.

Для Go: цикл `SELECT свободные → INSERT`, ловим `23505` на
`table_players_table_id_seat_no_key`, повторяем; отдельная транзакция на попытку.

### 3.2 `ux_table_players_user` (V9) — один игрок, один стол

⚠️ Индекс `(table_id, seat_no)` защищал **место** от двух игроков, но не **игрока** от двух
мест. Пять одновременных «Создать стол» заводили пять столов и сажали за них одного
человека: каждый запрос успевал проверить «сижу ли я где-то» раньше, чем сосед вставлял
строку. Между чтением и вставкой всегда есть щель — проверкой в коде это не закрыть.

V9 сначала **чистит существующие дубли** (оставляет самое раннее по
`(joined_at, table_id)`), потом ставит `create unique index ux_table_players_user on
table_players (user_id)`.

Как это видно в коде:
- `LobbyService.join` проверяет `currentTableOf(userId)` **до** попытки — не вместо
  индекса, а ради внятного ответа `409 ALREADY_AT_TABLE`. Без проверки вставка падала бы
  на уникальности, цикл исчерпывал попытки и врал «нет свободных мест» про пустой стол;
- та же проверка повторяется **внутри** `catch`: если увели не место, а самого игрока за
  другой стол, повторять бессмысленно;
- `LobbyService.create` сначала зовёт `releaseSeatBeforeNewTable` — встать из-за прошлого
  стола (и закрыть его, если он опустел). Посреди матча новый стол не создаётся вовсе
  (`409 MATCH_IN_PROGRESS`).

Для Go: `ux_table_players_user` — глобальный уникальный индекс по `user_id`, то есть
строка в `table_players` существует ровно одна на игрока во всей базе.

### 3.3 `unique (user_id, match_id)` в `rating_history` — матч закрывается один раз

Повторный вызов `MatchResultService.finishMatch` раздул бы рейтинг вдвое. Код проверяет
`histories.existsByMatchId(matchId)` и просто пишет `WARN`, если матч уже посчитан, —
но настоящая гарантия у индекса. Аналогично `DealHistory.record` проверяет
`existsByMatchIdAndDealNo` перед вставкой, опираясь на `unique (match_id, deal_no)`.

### 3.4 Частичные UNIQUE — «ровно один такой»

| Индекс | Инвариант |
|---|---|
| `idx_card_sets_single_default (is_default) where is_default` | один набор карт по умолчанию |
| `idx_table_themes_single_default (is_default) where is_default` | одна тема по умолчанию |
| `idx_seasons_single_open (closed_at) where closed_at is null` | один открытый сезон |

⚠️ Ловушка порядка операций: `RatingService.closeAndOpen` в одной транзакции сначала
**закрывает** текущий сезон, потом открывает новый. Индекс не `DEFERRABLE`, проверка идёт
на каждом операторе — обратный порядок упал бы на уникальности. В Go порядок обязателен
такой же.

### 3.5 `ux_users_username_lower` (V10) — логин без учёта регистра

⚠️ Уникальность стояла на самой строке, а поиск шёл точным совпадением. «Shabdan» и
«shabdan» были разными людьми, а телефонная клавиатура, которая сама ставит заглавную
первую букву, превращала вход с верным паролем в «неверный логин или пароль».

V10 делает две вещи и **именно в этом порядке**:
1. `alter table users drop constraint if exists users_username_key` — сначала снимается
   ограничение. Обратный порядок не работает: за `users_username_key` стоит constraint, и
   `drop index` на него ругается — индекс нельзя убрать, пока ограничение на него опирается;
2. `create unique index ux_users_username_lower on users (lower(username))`.

Регистр **сохраняется как набрали**: индекс по `lower()`, само поле не трогается.
Поиск в коде — `findByUsernameIgnoreCase` / `existsByUsernameIgnoreCase`. Стандартное
поведение `IgnoreCase` в Spring Data — `where upper(u.username) = upper(?)`, то есть
запрос **не попадает** в индекс по `lower()`. На объёмах этого проекта это неважно, но
при переносе на Go стоит писать `where lower(username) = lower($1)` — тогда индекс
используется.

Гонка, которую закрывает индекс: две одновременные регистрации `shabdan` и `Shabdan` —
обе проходят `existsByUsernameIgnoreCase`, вторая падает на уникальности.
⚠️ В `AuthService.register` этот случай **не перехвачен**: нарушение уникальности
всплывает как `DataIntegrityViolationException` и попадает в
`ApiExceptionHandler.handleUnexpected` → `500 INTERNAL_ERROR`, а не `409 USERNAME_TAKEN`.

### 3.6 `unique (match_id, seq)` в `match_events`

Гарантия отсутствия дыр и дублей в логе при гонках. Таблица append-only, `seq` сквозной
по матчу с 1.

---

## 4. Оптимистичная блокировка

`@Version` во всём проекте **ровно один** — `GameTable.version` (`integer not null default 0`).
Ни одна другая сущность версионирования не имеет.

Что происходит при конфликте: Hibernate делает
`UPDATE game_tables SET … , version = version + 1 WHERE id = ? AND version = ?`;
0 обновлённых строк → `ObjectOptimisticLockingFailureException`.

Где ловится:
- `LobbyService.releaseSeatBeforeNewTable` — **глотается**: «место уже освободили — так и
  требовалось». Ловится вместе с `EmptyResultDataAccessException`;
- `LobbyService.join` — в цикле попыток, вместе с `DataIntegrityViolationException`:
  попытка повторяется.

Где **не** ловится: `setReady`, `startMatch`, `finishMatch`, `close`, а также любые
операции над столом из `MatchService`. Там исключение уходит в
`ApiExceptionHandler.handleUnexpected` → `500 INTERNAL_ERROR` с `traceId`.
⚠️ Специального обработчика для `ObjectOptimisticLockingFailureException` и
`DataIntegrityViolationException` в `ApiExceptionHandler` нет.

Пессимистичных блокировок (`@Lock`, `LockModeType`, `SELECT … FOR UPDATE`) в коде
**не найдено** ни одной.

⚠️ Деталь мэппинга: `version` объявлен примитивом `int`. Из-за этого Spring Data не может
использовать версию для определения «новая ли сущность» и падает обратно на «id == null»;
`id` всегда задан в конструкторе, значит `save()` для `GameTable` (и для всех сущностей с
присвоенным `@Id`, включая все `@IdClass`-таблицы) выполняет `merge`, то есть
**SELECT перед записью**. Практическое следствие для Go: там, где Java-код зовёт
`repository.save(new Entity(...))`, физически идёт `SELECT … ; INSERT|UPDATE`, а не чистый
`INSERT`. Для `MatchSnapshotRecord` это означает, что повторный снимок с тем же
`(match_id, seq)` **перезапишет строку**, а не упадёт на PK.

---

## 5. JSON-поля

Все JSON-колонки — `jsonb`, в Java хранятся как **`String`** с
`@JdbcTypeCode(SqlTypes.JSON)`; сериализация ручная через Jackson, никаких
`@Type`-конвертеров в объекты.

| Колонка | Форма | Кто пишет |
|---|---|---|
| `game_tables.rules_config` | объект; `default '{}'` | `TableController.asJson(Map)` — **произвольный** `Map` из запроса клиента, сериализуется как есть, без валидации схемы |
| `matches.rules_snapshot` | тот же объект | `MatchService.start` кладёт **байт-в-байт** строку `table.rulesConfig()` |
| `deals.last_attack_cards` | массив строк-кодов карт; `default '[]'` | `DealHistory.cardsJson` |
| `deal_results.hung_cards` | массив строк-кодов карт; `default '[]'` | `DealHistory.cardsJson` |
| `deal_results.level_changes` | массив объектов `{"reason": …, "amount": int}`; `default '[]'` | `DealHistory.changesJson` |
| `match_events.payload` | объект, форма зависит от типа события | `GameProtocol.toEventPayload` / `MatchLog.appendRejected` |
| `match_snapshots.state` | объект полного состояния матча | `MatchStateCodec.encode` |

Ни по одному JSON-полю **не ищут и не джойнят** — GIN-индексов на них нет, и в коде нет
ни одного запроса по содержимому JSON.

### 5.1 `rules_config` / `rules_snapshot`

Читается `RulesConfigCodec.parse` — и он берёт только известные ключи, а всё остальное
игнорирует; чего нет — берётся из `RulesConfig.defaults()`. Фактически читаемые ключи:

```
dealSize, maxAttackFirstRound, maxAttackPerRound,
transfersEnabled, jokersEnabled,
naves.enabled, naves.scale (массив кодов рангов)
```

⚠️ Ключи `turnTimeoutSeconds`, `disconnectGraceSeconds`, `attackOrder`,
`showRejectedAttempts`, `naves.finalCard`, перечисленные в `planning/04-db-schema.md`,
кодеком **не читаются** — таймауты берутся из `bardak.game.*` в `application.yml`.
При ошибке разбора (`RuntimeException`/`JsonProcessingException`) пишется `WARN` и стол
играет по умолчаниям — упасть на кривом JSON стол не может.

### 5.2 `match_events.payload`

Всегда есть `seatNo`. Дальше по типу события (`GameProtocol.toEventPayload`):

| Тип | Ключи |
|---|---|
| `CARD_ATTACKED`, `FACE_DOWN_REVEALED`, `HIDDEN_TRUMP_REVEALED` | `cardCode` |
| `CARD_DEFENDED` | `cardCode`, `targetCardCode` |
| `ATTACK_TRANSFERRED` | `cardCode`, `toSeatNo` |
| `TRUMP_CHANGED`, `TRUMP_CHOSEN` | `suit` (имя масти) |
| `CARD_HUNG` | `cardCode`, `victimSeat` |
| `NAVES_LEVEL_CHANGED` | `level` (индекс ступени, **не** код!) |
| `CARDS_TAKEN`, `ROUND_BEATEN`, `CARDS_DRAWN` | `count` |
| `DICE_ROLLED` | `participants` |
| `MOVE_REJECTED` | `command`, `reason` (без `seatNo`) |
| прочие (`PASSED`, `PLAYER_LEFT_DEAL`, …) | только `seatNo` |

Имя типа выводится из имени Java-класса события: `CardAttacked → CARD_ATTACKED`
(`GameProtocol.eventType`). Для Go это надо зафиксировать таблицей — иначе переименование
типа в Go молча сломает совместимость со старым логом.

⚠️ `payload` содержит **полную** информацию, включая скрытую (какая карта у кого). Наружу
сырой лог не отдаётся никогда — только через проекцию и фильтр `private_to_seat`.

### 5.3 `match_snapshots.state`

`MatchStateCodec` — ручной кодек (без Jackson-аннотаций на записях движка: `game.rules`
намеренно не знает ни про Spring, ни про JSON, ни про базу). Корень:

```json
{"phase": …, "dealNo": …, "matchSeed": …, "navesLevels": [...], "deal": {...}, "results": [...]}
```

`deal` содержит `phase`, необязательный `trump`, `deck`, `players`, `table`
(`{attack, defence?}`), `roundStarterSeat`, `attackRightSeat`, `defenderSeat`,
`passedSeats`, `exitOrder`, `anyCardBeatenThisRound`, `anyPileDiscarded`,
`lastAttackCards`, `rngSeed`, `diceRolls` и необязательные `hangingWindow`,
`pendingHiddenTrump`.

Инвариант, на который опирается порт: снимок, разобранный обратно, обязан совпасть с
исходным состоянием **до последнего поля** (в проекте это закрыто тестом на круговой прогон).

### 5.4 Формат кода карты

`CardCodec`: `<rank>-<suit в нижнем регистре>` (`6-diamonds`, `10-hearts`, `A-spades`)
и `Joker-<номер>` (`Joker-1`). Коды — неизменяемый контракт: на них завязан весь
исторический лог и манифесты наборов, менять их нельзя даже косметически.

---

## 6. Границы транзакций

Все аннотации — `org.springframework.transaction.annotation.Transactional`, распространение
по умолчанию `REQUIRED`, если не указано иное. Изоляция нигде не переопределена (значит —
`READ COMMITTED` Postgres). `timeout` нигде не задан. `rollbackFor` нигде не задан, то есть
откат идёт только на unchecked-исключениях (`ApiException` — unchecked, откатывает).

### 6.1 `auth`

| Метод | Аннотация | Что внутри |
|---|---|---|
| `AuthService.register` | `@Transactional` | проверка инвайта, `existsByUsernameIgnoreCase`, `users.save`, выдача пары токенов |
| `AuthService.login` | `@Transactional` | поиск по логину, BCrypt-сверка, выдача пары |
| `AuthService.refresh` | `@Transactional` | `refreshTokens.rotate` + чтение пользователя + выдача пары |
| `AuthService.logout` | `@Transactional` | `refreshTokens.revoke` |
| `AuthService.profile` | `@Transactional(readOnly = true)` | чтение `users` |
| `AuthService.updateProfile` | `@Transactional` | `User.rename` + `save` |
| `AuthService.changePassword` | `@Transactional` | сверка старого пароля, `save`, затем `revokeAllOf` |
| `RefreshTokenService.issue` | `@Transactional` | генерация токена, `save` хеша |
| `RefreshTokenService.rotate` | `@Transactional` | поиск по хешу, проверки, `revoke` + `save` |
| `RefreshTokenService.revoke` | `@Transactional` | пометить отозванным |
| `RefreshTokenService.revokeAllOf` | **без аннотации** | делегирует в `TokenSeriesRevoker` |
| `TokenSeriesRevoker.revokeAll` | `@Transactional(propagation = REQUIRES_NEW)` | `UPDATE … set revoked_at where user_id = ? and revoked_at is null` |

⭐ **Главная транзакционная тонкость проекта.** `TokenSeriesRevoker` вынесен в отдельный
бин именно ради `REQUIRES_NEW`: отзыв всей серии происходит при **краже** токена, а
`rotate` в этот момент **бросает исключение**. В общей транзакции отзыв откатился бы
вместе с ней — и вор остался бы внутри. Отдельная транзакция коммитится независимо,
после чего внешняя откатывается.

Для Go: «повторное предъявление уже отозванного refresh-токена → отозвать всю серию
пользователя, зафиксировать это, и только потом вернуть 401» — коммит отзыва обязан
пережить ошибку запроса.

`@Modifying(clearAutomatically = true, flushAutomatically = true)` на `revokeAllForUser`:
перед bulk-`UPDATE` контекст сбрасывается на диск, после — очищается, чтобы в кэше не
осталось устаревших `RefreshToken`.

### 6.2 `lobby`

| Метод | Аннотация | Что внутри |
|---|---|---|
| `openTables`, `byId`, `byCode`, `seats`, `currentTableOf`, `isReadyToStart` | `readOnly = true` | чтение |
| `create` | **без аннотации, намеренно** | `releaseSeatBeforeNewTable` → `tables.save` → `join` → при ошибке `closeIfDeserted` |
| `join` | **без аннотации, намеренно** | цикл до 5 попыток `SeatAllocator.allocate` |
| `SeatAllocator.allocate` | `REQUIRES_NEW` | `findByTableIdOrderBySeatNo` + `saveAndFlush(new TablePlayer)` |
| `leave` | `@Transactional` | проверка `IN_MATCH`, `players.delete(seat)` |
| `setReady` | `@Transactional` | смена `state`, `save` |
| `startMatch` | `@Transactional` | `status = IN_MATCH`, `save` (задевает `@Version`) |
| `finishMatch` | `@Transactional` | `status = WAITING` + сброс `READY` всем |
| `close` | `@Transactional` | проверка хоста и `IN_MATCH`, `status = CLOSED`, `closed_at` |

⚠️ **Самовызовы обходят прокси.** `create()` вызывает `this.releaseSeatBeforeNewTable()`
(а та — `this.leave()`) и `this.join()`, а `join()` вызывает `this.byId()`. Поскольку это
вызовы внутри одного бина, аннотации `@Transactional` на `leave()` и `byId()`
**не применяются**: каждая операция репозитория выполняется в собственной транзакции.
Автор об этом знает — в `create()` есть комментарий «вызов из этого же класса всё равно
прошёл бы мимо прокси». Через прокси идёт только `SeatAllocator.allocate` (другой бин),
и именно поэтому его `REQUIRES_NEW` работает.

Практическое следствие для Go: создание стола **не атомарно**. Есть окно, в котором стол
уже вставлен, а хозяин ещё не посажен; на ошибке посадки код вызывает `closeIfDeserted`,
то есть компенсирует вручную, а не откатом. Повторять надо именно эту семантику, если не
хочется расхождения в поведении при гонках.

### 6.3 `game` / `history`

| Метод | Аннотация | Что внутри |
|---|---|---|
| `MatchLog.startMatch` | `@Transactional` | вставка `matches` |
| `MatchLog.append` | `@Transactional` | цикл вставок `match_events`, возвращает последний `seq` |
| `MatchLog.appendRejected` | `@Transactional` | одна вставка `MOVE_REJECTED` с `private_to_seat = actorSeat` |
| `MatchLog.snapshot` | `@Transactional` | вставка/перезапись `match_snapshots` |
| `MatchLog.since` | `readOnly = true` | выборка + фильтр по `isVisibleTo(seat)` **в Java**, не в SQL |
| `MatchLog.latestSnapshot`, `activeMatchFor` | `readOnly = true` | чтение |
| `MatchLog.dealsPlayed`, `finish`, `abort` | `@Transactional` | чтение + мутация + `save` |
| `DealHistory.record` | `@Transactional` | защита `existsByMatchIdAndDealNo`, затем `deals.save` + N × `deal_results.save` |
| `MatchHistoryService.matchesOf`, `details`, `dealsOf`, `replay` | `readOnly = true` | чтение истории |

⚠️ `MatchLog.since` фильтрует видимость **после** выборки, в памяти. Запрос тянет из базы
и приватные чужие события; наружу они не уходят, но по сети из БД едут. Для Go это можно
(и стоит) сделать условием `and (private_to_seat is null or private_to_seat = $2)` — на
контракт это не влияет.

⚠️ Запись хода **не атомарна целиком**: `MatchLog.append` (события) и `MatchLog.snapshot`
(снимок) — **две разные транзакции**, вызываемые из `GameCommandHandler` по очереди.
Между ними возможен сбой: события записаны, снимок нет. Восстановление это переживает —
матч поднимется из предыдущего снимка, — но событий в логе окажется больше, чем «покрыто»
снимком. Порядок «сначала записать, потом разослать клиентам» соблюдается сознательно
(иначе после падения клиенты видели бы ход, которого в истории нет).

### 6.4 `rating`

| Метод | Аннотация | Что внутри |
|---|---|---|
| `MatchResultService.startMatch` | `@Transactional` | вставка пустых `match_players` по местам |
| `MatchResultService.finishMatch` | `@Transactional` | ⭐ **всё одной транзакцией** |
| `RatingService.of`, `leaderboard`, `seasons` | `readOnly = true` | чтение |
| `RatingService.closeAndOpen` | `@Transactional` | закрыть открытый сезон → открыть новый |
| `StatsService.of` | `readOnly = true` | считает статистику на лету по всем матчам игрока |

⭐ `finishMatch` — самая нагруженная транзакция проекта. Внутри неё:
`existsByMatchId` (защита от повторного начисления) → чтение открытого сезона →
чтение/создание `user_rating` для каждого игрока → пересчёт Elo →
`user_rating.save` + `rating_history.save` + `match_players.finish` для каждого →
`matchLog.finish` (та же транзакция, `REQUIRED`, `matches.status = FINISHED`).

Причина, названная в javadoc: «матч записался, а рейтинг нет» даёт неисправимое расхождение
между историей и текущим рейтингом — починить его потом можно только руками и на глаз.
Для Go это жёсткое требование: одна транзакция на весь итог матча.

Отменённый матч (`ABORTED`) сюда не приходит вообще — `rating_*` остаются `null`, строк в
`rating_history` не появляется.

### 6.5 `social` и `push`

| Метод | Аннотация | Заметка |
|---|---|---|
| `FriendService.request` | `@Transactional` | ⚠️ встречная заявка = согласие: если пара уже есть и её может принять текущий, вместо второй заявки идёт `accept` |
| `FriendService.accept` | `@Transactional` | идемпотентен: уже принятая пара возвращается как есть |
| `FriendService.remove` | `@Transactional` | `DELETE` — «отклонить» и «удалить из друзей» одно и то же |
| `FriendService.list`, `isFriend` | `readOnly = true` | `list` тянет всех «других» одним `findAllById` |
| `FriendService.invite` | `readOnly = true` | ⚠️ **внутри readOnly-транзакции идёт отправка по сокету** (`invites.send`) — сетевой вызов держит соединение из пула |
| `PushSubscriptionService.subscribe` | `@Transactional` | upsert по `endpoint` через `reassign` |
| `PushSubscriptionService.unsubscribe` | `@Transactional` | `deleteByEndpoint` |
| `PushSubscriptionService.of` | `readOnly = true` | |
| `PushSender.deliverAll` | **без транзакции, намеренно** | вызывается с собственного потока отправителя — `@Transactional` там всё равно не сработал бы (вызов мимо прокси). Каждое обращение к репозиторию — своя транзакция; общего инварианта между строками нет |

### 6.6 Сводка `REQUIRES_NEW`

Ровно два места, и оба существуют по одной причине — **пережить откат внешней транзакции**
или **пережить нарушение ограничения**:

1. `TokenSeriesRevoker.revokeAll` — отзыв серии должен закоммититься, хотя `rotate` падает;
2. `SeatAllocator.allocate` — после нарушения UNIQUE транзакция в Postgres непригодна,
   повторять попытку можно только в новой.

---

## 7. Расхождения с `planning/04-db-schema.md`

Документ в целом соответствует миграциям. Расхождения — ниже; везде верна **миграция**.

| # | Док | Код/миграция |
|---|---|---|
| 1 | `users.status varchar(16) not null` без дефолта, без check | В V1 есть `default 'ACTIVE'` и `constraint users_status_check` |
| 2 | В DDL-блоке `users` нет колонки `avatar` | V8 добавил `avatar varchar(8)`. Блок DDL в доке показывает состояние на V1 |
| 3 | `game_tables` — без check-ограничений | V2 добавляет `game_tables_status_check` и `game_tables_max_players_check (2..5)` |
| 4 | `table_players` — без check-ограничений | V2 добавляет `table_players_state_check` и `table_players_seat_check (0..4)` |
| 5 | `matches` — без check | V3 добавляет `matches_status_check` |
| 6 | «Снапшот пишется раз в N событий (например, каждые 50) и обязательно на границе раздач» | Снимок пишется **после каждого хода** (`GameCommandHandler.saveSnapshot`). Комментарий в самой миграции V3 («Пишется на границе раздач») тоже устарел |
| 7 | `naves_level -- '6'..'A', 'JK'` | Код пишет **`"Jk"`** (`NavesLevelCodec.JOKER`), с маленькой `k`. Расхождение в регистре — при переносе легко потерять |
| 8 | Таблицы `push_subscriptions` в разделах DDL **нет вовсе** (только строка в таблице миграций) | V7 её создаёт; см. §1.17 |
| 9 | `matches.status` включает `PAUSED` как рабочее состояние | В базу `PAUSED` **никогда не пишется**: пауза живёт в памяти и в WS-событии |
| 10 | `table_players.state` — `JOINED \| READY \| LEFT` | `LEFT` в базу не пишется: `leave()` удаляет строку. Check в БД значение всё ещё допускает |
| 11 | `rules_config` перечисляет `turnTimeoutSeconds`, `disconnectGraceSeconds`, `attackOrder`, `showRejectedAttempts`, `naves.finalCard` | `RulesConfigCodec` эти ключи **не читает**; таймауты берутся из `bardak.game.*` в `application.yml` |
| 12 | `card_assets.card_code` описан как «код карты движка», примеры `'Joker'`, `'back'` | Движок кодирует джокеров как `Joker-N`; `back` кодом движка не является. Схлопывание делает клиент |
| 13 | `users.username ... not null unique` | UNIQUE-ограничение **снято в V10**; уникальность держит `ux_users_username_lower` по `lower(username)` |
| 14 | Раздел «Открытые решения»: «мягкое удаление пользователей (`status = BLOCKED`)» — как открытый вопрос | Колонка и enum есть, но кода, выставляющего `BLOCKED`, **не найдено** ни в одном сервисе |

Не расхождение, но стоит знать: доковый DDL `deals.trump_suit varchar(16)` описывает
состояние **после** V6; V3 создавал `varchar(2)`.

---

## 8. Ловушки, важные для порта на Go

1. **`users.updated_at` не обновляется никогда.** В сущности колонка помечена
   `insertable = false`, но `updatable` остаётся по умолчанию `true`, а код в поле ничего
   не пишет — при `UPDATE` Hibernate записывает обратно то же значение, что прочитал.
   Триггера в базе нет. Значит `updated_at` всегда равен `created_at`. Если в Go это
   «починить», поведение изменится — сознательно или нет.
2. **`users.avatar_url` мёртвая.** Читается (`AuthDtos.UserView.avatarUrl`), но никогда не
   записывается: профиль умеет менять только `display_name` и `avatar` (эмодзи).
3. **Уникальность логина не ловится в коде.** Гонка двух регистраций отдаёт `500`, а не
   `409 USERNAME_TAKEN` — обработчика `DataIntegrityViolationException` в
   `ApiExceptionHandler` нет.
4. **Поиск по логину не попадает в индекс.** `findByUsernameIgnoreCase` даёт
   `upper(username) = upper(?)`, а индекс построен по `lower(username)`. В Go писать
   `lower(username) = lower($1)`.
5. **Порядок пары в `friendships` — строковый, а не `UUID.compareTo`.** Знаковое сравнение
   двух `long` противоположно побайтовому на половине идентификаторов. Проверка
   `friendships_ordered` это ловит — то есть ошибка проявится сразу, но 50 % операций.
6. **Создание стола не атомарно** (см. §6.2): самовызовы обходят прокси, компенсация
   ручная (`closeIfDeserted`).
7. **Запись хода — две транзакции** (события и снимок), см. §6.3.
8. **`save()` на присвоенных ключах — это `merge`, то есть SELECT + запись.** Для
   `match_snapshots` это молчаливая перезапись строки при повторе того же `seq`.
9. **Снимок пишется после каждого хода** — не «раз в N». Объём растёт как число ходов ×
   размер состояния раздачи; чистки старых снимков в коде нет.
10. **`FriendService.invite` шлёт по сокету внутри `readOnly`-транзакции** — соединение из
    пула держится на время сетевого вызова.
11. **`StatsService.of` считает всё на лету** по всем матчам игрока, без агрегатов и кэша.
    Индексов под это ровно два: `idx_match_players_user` и `idx_rating_history_user_time`.
12. **Лидерборд читает `user_rating` целиком** и сортирует — индекса по `rating` нет.
13. **Дублирующие индексы**: `idx_match_events_match (match_id, seq)` полностью покрыт
    уникальным `(match_id, seq)`. Переносить оба смысла нет.
14. **Ни одной пессимистичной блокировки.** Все конкурентные сценарии закрыты уникальными
    индексами + повтором, либо `@Version` на одном столе.
