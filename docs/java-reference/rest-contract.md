# Контракт REST API Java-бэкенда

Источник истины — код на коммите `575d81f4bd4ad092babf7f0d92933824079e9617`
(`back-bardak/src/main/java/kz/bardak`). Где `planning/05-api-contracts.md` расходится
с кодом — записано как в коде, расхождение отмечено отдельно (§9).

Всего в коде **37** методов-маппингов в 11 контроллерах. Пересчёт:

```sh
grep -rn "GetMapping\|PostMapping\|PatchMapping\|PutMapping\|DeleteMapping" \
  --include='*.java' back-bardak/src/main/java/kz/bardak | grep -v import | wc -l
```

| Контроллер | Базовый путь | Методов |
|---|---|---|
| `common/web/HealthController` | `/api` | 1 |
| `auth/api/AuthController` | `/api/auth` | 5 |
| `auth/api/ProfileController` | `/api/profile` | 3 |
| `lobby/api/TableController` | `/api/tables` | 7 |
| `lobby/api/CardSetController` | `/api` | 3 |
| `history/api/MatchHistoryController` | `/api/matches` | 3 |
| `rating/api/RatingController` | `/api/rating` | 5 |
| `rating/api/StatsController` | `/api/stats` | 2 |
| `social/api/FriendController` | `/api/friends` | 5 |
| `push/api/PushController` | `/api/push` | 3 |
| **Итого** | | **37** |

---

## 0. Общие правила сериализации — читать до всего остального

Три вещи, без которых Go-реализация не совпадёт с Java побайтно.

### 0.1. `null`-поля из ответов ВЫРЕЗАЮТСЯ

`application.yml`:

```yaml
spring:
  jackson:
    default-property-inclusion: non_null
```

Это глобально. Любое поле DTO со значением `null` **не появляется в JSON вообще**.
Не `"field": null`, а отсутствие ключа.

Практические следствия, которые ломают наивный порт:

- `GET /api/tables/current` у игрока, который нигде не сидит, отдаёт `{"inMatch":false}` —
  ни `table`, ни `mySeatNo` в теле нет.
- `UserView` без аватара — это `{"id":…,"username":…,"displayName":…}`, полей
  `avatarUrl`/`avatar` нет.
- `PlayerStats.empty()` отдаёт `{"matches":0,"wins":0,"losses":0,"dealsPlayed":0,
  "streak":{"kind":"NONE","length":0},"degrees":[]}` — `avgPlace`, `bestRating`,
  `worstRating` вырезаны.
- В `MatchSummary` пропадают `finishedAt`, `abortReason`, `myPlace`, `myRatingDelta`,
  когда они пусты.

Пустой список (`List.of()`) при этом **остаётся** — `non_null`, а не `non_empty`.

### 0.2. `ApiError` строже: `NON_EMPTY`

`common/web/ApiError` помечен `@JsonInclude(JsonInclude.Include.NON_EMPTY)`. Пустая карта
`details` и пустая строка не сериализуются. То есть обычная ошибка — это ровно
`{"code":…,"message":…,"traceId":…}`.

### 0.3. Числовые и строковые типы

- `UUID` в **ответах** непоследователен: часть DTO отдаёт `String`
  (`AuthDtos.UserView.id`, `LobbyDtos.TableView.id`/`hostUserId`/`cardSetId`/`themeId`,
  `LobbyDtos.SeatView.userId`, `FriendDtos.Friend.userId`, `CardSetView.id`,
  `TableThemeView.id`, `CardSetManifest.id`), часть — нативный `UUID`
  (`RatingDtos.*`, `HistoryDtos.*`). На проводе разницы нет (обе стороны — строка в
  каноническом нижнем регистре), но в Go типы полей надо повторять по этому списку,
  а не «все UUID одинаково».
- `BigDecimal` (`rating`, `avgPlace`, `ratingDelta`, …) Jackson пишет **числом без
  кавычек**, с тем масштабом, что лежит в БД/посчитан. `avgPlace` всегда `scale=2`
  (`HALF_UP`), например `2.33`.
- `Instant` при дефолтных настройках Spring Boot — строка ISO-8601 UTC
  (`2026-08-19T10:15:30.123456Z`); `WRITE_DATES_AS_TIMESTAMPS` нигде не включён.
- `int`/`long`/`boolean` — обычные JSON-примитивы, `null` быть не может.

---

## 1. Единый формат ошибки (`common/web/ApiExceptionHandler`)

`@RestControllerAdvice`, четыре обработчика. Тело всегда `ApiError`:

```json
{
  "code": "TABLE_FULL",
  "message": "За столом нет свободных мест",
  "traceId": "b1f2c3d4",
  "details": { "поле": "сообщение" }
}
```

| Поле | Тип | Nullable | Смысл |
|---|---|---|---|
| `code` | string | нет | Машиночитаемый, стабильный |
| `message` | string | нет | Для человека, может меняться |
| `traceId` | string | нет | **Первые 8 символов** случайного UUID (`UUID.randomUUID().toString().substring(0,8)`) |
| `details` | object | вырезается, если пусто | Подробности |

### Обработчики

| Исключение | Статус | `code` | `message` | `details` | Лог |
|---|---|---|---|---|---|
| `ApiException` | из самого исключения | из исключения | из исключения | из исключения | `log.info` «Запрос отклонён» |
| `MethodArgumentNotValidException` | **400** | `VALIDATION_FAILED` | `Проверьте заполнение полей` | `{имя поля: сообщение валидатора}`, порядок — `LinkedHashMap`, то есть порядок обхода `getFieldErrors()` | `log.info` |
| `NoResourceFoundException` | **404** | `NOT_FOUND` | `Такого адреса нет` | пусто | `log.info` |
| `Exception` (всё остальное) | **500** | `INTERNAL_ERROR` | `Что-то пошло не так` | пусто | `log.error` со стеком |

⚠️ **Текст исключения наружу не уходит никогда** — только заранее заготовленное сообщение.

### ⚠️ Ловушка: обработчик `Exception` перехватывает и «нормальные» ошибки Spring

`ApiExceptionHandler` **не наследует** `ResponseEntityExceptionHandler` и не объявляет
обработчиков для типовых исключений MVC. `ExceptionHandlerExceptionResolver` работает
раньше `DefaultHandlerExceptionResolver`, поэтому в `handleUnexpected` (500 `INTERNAL_ERROR`)
проваливаются, в частности:

- **невалидный UUID в пути** (`GET /api/tables/не-uuid`) →
  `MethodArgumentTypeMismatchException` → **500**, а не 400;
- **битый или отсутствующий JSON в теле** → `HttpMessageNotReadableException` → **500**;
- **неподдержанный метод** на существующем пути → `HttpRequestMethodNotSupportedException`
  → **500**, а не 405;
- **отсутствующий обязательный query-параметр** → **500**.

Это поведение Java-версии, а не идеал. Для Go: если решено чинить — фиксировать как
осознанное расхождение, иначе повторять 500.

### ⚠️ Ловушка: 401 от Spring Security НЕ в формате `ApiError`

Отказ по отсутствию/просроченности токена выдаёт `BearerTokenAuthenticationEntryPoint`
до того, как запрос дойдёт до контроллера. `AuthenticationEntryPoint` не переопределён
(`SecurityConfig` — `oauth2ResourceServer(oauth2 -> oauth2.jwt(jwt -> {}))`), поэтому ответ:

- статус **401**, **тело пустое**;
- заголовок `WWW-Authenticate: Bearer` (при истёкшем/битом токене — с
  `error="invalid_token", error_description=…`).

То же для 403 от `AccessDeniedHandler`. Клиент по коду `code` этот случай не отличит —
он смотрит на статус. Подтверждено тестом
`lobby/TableInviteLinkIT#shouldStillRequireATokenWhenTheFullTableViewIsAskedFor`.

### Каталог кодов, достижимых через REST

| `code` | Статус | Где бросается |
|---|---|---|
| `VALIDATION_FAILED` | 400 | `ApiExceptionHandler`, любой `@Valid` |
| `INVALID_RULES_CONFIG` | 400 | `TableController.asJson` |
| `INVALID_CREDENTIALS` | 400 | `AuthService.changePassword` (старый пароль не подошёл) |
| `INVALID_CREDENTIALS` | 401 | `AuthService.login` |
| `REFRESH_TOKEN_INVALID` | 401 | `RefreshTokenService`, `AuthService.refresh` |
| `INVALID_INVITE_CODE` | 403 | `AuthService.register` |
| `NOT_TABLE_HOST` | 403 | `LobbyService.close` |
| `NOT_SEASON_ADMIN` | 403 | `RatingService.closeAndOpen` |
| `NOT_FRIENDS` | 403 | `FriendService.invite` |
| `NOT_FOUND` | 404 | `ApiExceptionHandler`, неизвестный путь |
| `USER_NOT_FOUND` | 404 | `AuthService`, `RatingService.of`, `FriendService` |
| `TABLE_NOT_FOUND` | 404 | `LobbyService` |
| `CARD_SET_NOT_FOUND` | 404 | `CardSetController.manifest` |
| `MATCH_NOT_FOUND` | 404 | `MatchHistoryService.matchOf` |
| `NOT_FRIENDS` | 404 | `FriendService.pairOrFail` (пары нет) |
| `USERNAME_TAKEN` | 409 | `AuthService.register` |
| `MATCH_IN_PROGRESS` | 409 | `LobbyService.create/leave/close` |
| `TABLE_NOT_OPEN` | 409 | `LobbyService.join` |
| `ALREADY_AT_TABLE` | 409 | `LobbyService.join` |
| `TABLE_FULL` | 409 | `LobbyService.join` |
| `NOT_AT_TABLE` | 409 | `LobbyService.setReady` |
| `MATCH_NOT_FINISHED` | 409 | `MatchHistoryService.replay` |
| `CANNOT_FRIEND_SELF` | 409 | `FriendService.request` |
| `ALREADY_FRIENDS` | 409 | `FriendService.request` |
| `REQUEST_ALREADY_SENT` | 409 | `FriendService.request` |
| `NOT_YOUR_REQUEST` | 409 | `FriendService.accept` |
| `NO_DEFAULT` | 500 | `TableController.create` (нет набора/темы по умолчанию) |
| `INTERNAL_ERROR` | 500 | `ApiExceptionHandler` |

⚠️ **Один и тот же `code` с разными статусами**: `INVALID_CREDENTIALS` (400 и 401) и
`NOT_FRIENDS` (403 и 404). Go-реализация должна повторять пару «код + статус» по месту
броска, а не заводить одну константу на код.

Коды `MATCH_ALREADY_STARTED`, `NO_MATCH`, `TABLE_NOT_READY`, `NOT_A_PLAYER` через REST
недостижимы — они только в `game/ws/GameCommandHandler` и `game/runtime/MatchService`.

---

## 2. Открытые пути (без токена)

Дословно из `auth/SecurityConfig.securityFilterChain`, в порядке объявления
(первое совпадение выигрывает):

| Метод | Паттерн | Комментарий |
|---|---|---|
| `POST` | `/api/auth/register` | |
| `POST` | `/api/auth/login` | |
| `POST` | `/api/auth/refresh` | |
| `POST` | `/api/auth/logout` | ⚠️ см. ниже |
| любой | `/api/health` | |
| любой | `/actuator/**` | наружу открыты `health,info,metrics`; `health.show-details: always` |
| любой | `/assets/**` | картинки наборов карт и тем |
| `GET` | `/api/card-sets` | |
| `GET` | `/api/card-sets/**` | покрывает и `/api/card-sets/{id}/manifest` |
| `GET` | `/api/table-themes` | |
| `GET` | `/api/tables/invite/**` | единственная ручка столов без токена |
| любой | `/ws` | ⚠️ пропущено Spring Security намеренно; авторизует `WsTicketHandshakeInterceptor` по тикету (ADR-005) |
| `GET` | `/` , `/index.html`, `/app/**`, `/*.css`, `/*.js`, `/*.ico`, `/*.png`, `/*.svg`, `/manifest.webmanifest`, `/icons/**` | оболочка PWA |
| — | `anyRequest()` | `authenticated()` |

Прочие настройки цепочки:

- `csrf` — **выключен** (сессий нет, токен браузер сам не подставляет);
- `sessionManagement` — `STATELESS`;
- `oauth2ResourceServer().jwt()` — валидация Bearer-токена; свой парсер JWT не пишется.

### ⚠️ Ловушки открытых путей

1. **`POST /api/auth/logout` открыт.** Access-токен не нужен: кто угодно, знающий строку
   refresh-токена, может её погасить. Это осознанно (логаут должен работать и при
   протухшем access), но в Go повторять надо буквально.
2. **`GET /api/card-sets/**` открыт целиком**, а не только `/manifest`. Любой будущий
   подпуть под `/api/card-sets/` окажется публичным по умолчанию.
3. **`GET /api/tables/by-code/{code}` НЕ открыт** — только `/invite/{code}`. Полный вид
   стола со списком игроков остаётся за токеном.
4. **`GET /api/tables/current`** попадает под `anyRequest().authenticated()`, а не под
   `/api/tables/invite/**` — токен нужен.
5. На открытых ручках `@AuthenticationPrincipal Jwt jwt` был бы `null`; ни одна открытая
   ручка его не читает — это инвариант, который в Go легко случайно нарушить.

---

## 3. Формат JWT, refresh и ws-тикет

### 3.1. Access-токен (`auth/AccessTokenService`, `auth/SecurityConfig`)

- **Тип:** JWS, компактная сериализация (Nimbus, `NimbusJwtEncoder`).
- **Алгоритм:** `HS256` (`MacAlgorithm.HS256`). Заголовок собирается **явно**
  (`JwsHeader.with(MacAlgorithm.HS256)`) — по умолчанию Nimbus искал бы ключ под RS256.
- **Ключ:** `bardak.auth.jwt-secret` как есть, в UTF-8, `SecretKeySpec(..., "HmacSHA256")`.
  Валидация свойства требует **длину ≥ 32 символов** (`AuthProperties`, `MIN_SECRET_LENGTH`),
  иначе приложение не стартует. Дефолт в `application.yml` —
  `dev-only-secret-change-me-32-bytes-minimum!!` (только для локальной разработки).
- **Декодер:** `NimbusJwtDecoder.withSecretKey(...).macAlgorithm(HS256)` — то есть
  принимается **только HS256**.

Claims (ровно эти, других нет):

| Claim | Тип | Значение |
|---|---|---|
| `iss` | string | `"bardak"` |
| `iat` | number (epoch seconds) | `clock.instant()` |
| `exp` | number (epoch seconds) | `iat + bardak.auth.access-ttl` |
| `sub` | string | UUID пользователя |
| `username` | string | логин |
| `displayName` | string | имя за столом |

Ролей нет. `jti`, `aud`, `nbf` не выставляются.

⚠️ Всё, что читают контроллеры из токена, — это `jwt.getSubject()` (везде через
`UUID.fromString`) и `jwt.getClaimAsString("username")` (только `RatingController`,
для проверки права закрывать сезон).

⚠️ `UUID.fromString(jwt.getSubject())` не обёрнут — токен с несуществующим/битым `sub`
даёт `IllegalArgumentException` → **500 `INTERNAL_ERROR`**.

Сроки (`application.yml`): `access-ttl: 15m`, `refresh-ttl: 30d`, `ws-ticket-ttl: 30s`.
`expiresIn` в `TokenPair` — это `accessTtl.toSeconds()`, то есть **900**.

### 3.2. Refresh-токен (`auth/RefreshTokenService`, `auth/TokenSeriesRevoker`)

- **Не JWT.** 32 случайных байта из `SecureRandom`, `Base64.getUrlEncoder().withoutPadding()`
  → строка 43 символа. Подпись не нужна: токен всё равно проверяется по БД, потому что
  его надо уметь отзывать.
- **В БД лежит только хеш:** `Base64(SHA-256(token))` — **обычный** Base64 с паддингом
  (не url-safe!), поле `token_hash`. ⚠️ Два разных кодирования в одном классе: сам токен
  url-safe без паддинга, его хеш — стандартный с паддингом. Повторять точно, иначе
  выданные Java-версией токены перестанут находиться.
- Вместе с токеном сохраняется `User-Agent` (заголовок запроса, `required = false`).
- Срок: `issuedAt + refresh-ttl` (30 дней).

**Ротация** (`rotate`), при каждом `POST /api/auth/refresh`:

1. Найти по хешу; не нашли → 401 `REFRESH_TOKEN_INVALID`.
2. Токен **уже отозван** → это признак кражи: отзывается **вся серия пользователя**
   (`TokenSeriesRevoker.revokeAll`, `@Transactional(REQUIRES_NEW)` — отдельная транзакция,
   иначе отзыв откатился бы вместе с падающим запросом), пишется `log.warn`,
   затем 401 `REFRESH_TOKEN_INVALID`.
3. Токен просрочен → 401 `REFRESH_TOKEN_INVALID`.
4. Иначе старый помечается отозванным, выдаётся **новая пара** (новый access + новый
   refresh). Старый refresh больше не работает.

**Отзыв** (`revoke`, из `POST /api/auth/logout`) — «по возможности»: если токен не найден
или уже непригоден, ничего не делается и ответ всё равно **204**.

**`revokeAllOf`** — из смены пароля: гасит все refresh-токены игрока.
⚠️ Access-токены при этом **не** инвалидируются: текущая вкладка живёт до конца своего
15-минутного access-токена. Отзывного списка access-токенов в системе нет вообще.

### 3.3. Одноразовый WS-тикет (`auth/ws/WsTicketService`, ADR-005)

- Выдаётся только авторизованному: `POST /api/auth/ws-ticket` требует access-токена.
- 24 случайных байта, `Base64` url-safe без паддинга → 32 символа.
- Живёт `bardak.auth.ws-ticket-ttl` = **30 s**; `expiresIn` в ответе — это же в секундах.
- Хранилище — `ConcurrentHashMap` **в памяти процесса**. ⚠️ Тикет действует только
  на том инстансе, что его выдал; с двумя узлами без общего кеша рукопожатие отвалится.
- **Погашение атомарно** через `ConcurrentHashMap.remove` — двух одновременных
  подключений по одному тикету быть не может.
- Проверка срока делается **после** снятия: просроченный тикет всё равно удаляется.
- `@Scheduled(fixedDelayString = "PT1M")` `evictExpired()` чистит протухшие.
- Потребитель — `WsTicketHandshakeInterceptor`: читает **query-параметр `ticket`**
  (`wss://host/ws?ticket=…`), при неуспехе ставит **401** и обрывает рукопожатие до
  установления соединения; при успехе кладёт `userId` в атрибуты сессии под ключом
  `"userId"`.

---

## 4. Health

### `GET /api/health` — токен не нужен

Ответ 200, `Map<String,Object>` (не типизированный DTO):

```json
{
  "status": "UP",
  "version": "dev",
  "db": { "status": "UP", "version": "PostgreSQL 16.4", "migrations": 12 },
  "ts": 1755590400000
}
```

| Поле | Тип | Значение |
|---|---|---|
| `status` | string | всегда `"UP"` — константа, состояние БД сюда не влияет |
| `version` | string | `Package.getImplementationVersion()` или `"dev"`, если его нет |
| `db.status` | string | `"UP"` / `"DOWN"` |
| `db.version` | string | `select version()`, обрезанное **по первой запятой** (`split(",")[0]`); `"unknown"`, если запрос вернул null |
| `db.migrations` | number | `select count(*) from flyway_schema_history where success = true`; `0`, если null |
| `db.error` | string | **только при DOWN**: `e.getClass().getSimpleName()`, без текста |
| `ts` | number | `Instant.now().toEpochMilli()` |

⚠️ Ключи `db.version`/`db.migrations` при `DOWN` отсутствуют, а `db.error` — при `UP`;
это две разные формы одного поля. ⚠️ HTTP-статус **200 даже при `db.status = "DOWN"`**.

⚠️ `Map.of(...)` не гарантирует порядок ключей — в Go порядок можно выбрать любой.

---

## 5. Auth и профиль

### `POST /api/auth/register` — без токена

**Тело** `AuthDtos.RegisterRequest`:

| Поле | Тип | Валидация |
|---|---|---|
| `username` | string | `@NotBlank`, `@Size(min=3, max=32)` |
| `displayName` | string | `@NotBlank`, `@Size(min=2, max=64)` |
| `password` | string | `@NotBlank`, `@Size(min=8, max=128)` |
| `email` | string | `@Email`, `@Size(max=255)` — **необязательное**, `null` проходит |
| `inviteCode` | string | `@NotBlank` |

**Заголовок:** `User-Agent` (необязателен) — сохраняется в строке refresh-токена.

**Ответ 200** `TokenPair` (см. ниже). ⚠️ **200, не 201.**

**Ошибки:** 400 `VALIDATION_FAILED`; 403 `INVALID_INVITE_CODE`; 409 `USERNAME_TAKEN`.

**Порядок проверок важен:** сначала код приглашения, потом занятость логина.

**Проверка кода приглашения** (`AuthProperties.isInviteCodeValid`): вход `trim()`-ится
и сравнивается **без учёта регистра** с каждым из `bardak.auth.invite-codes`;
`null` → `false`. Дефолт списка — `bardak-2026`.

**Побочные эффекты:** строка в `users` (`id` = случайный UUID, `status = ACTIVE`,
`password_hash` = BCrypt с дефолтной стоимостью 10, `avatar`/`avatar_url` — `null`);
строка в `refresh_tokens`.

⚠️ Проверка занятости логина — `existsByUsernameIgnoreCase` + уникальный индекс.
Проверка и вставка не атомарны, поэтому правду держит **база**, а не проверка.

✅ **Исправлено 2026-08-19:** гонка двух одновременных регистраций одного логина теперь
даёт **409 `USERNAME_TAKEN`**, а не 500. Раньше `DataIntegrityViolationException`
проваливался в «что-то пошло не так». Закрыто тестом `ApiHardeningIT`.

### `POST /api/auth/login` — без токена

**Тело** `LoginRequest`: `username` `@NotBlank`, `password` `@NotBlank`.
**Заголовок:** `User-Agent` (необязателен).

**Ответ 200** `TokenPair`.

**Ошибки:** 400 `VALIDATION_FAILED`; **401 `INVALID_CREDENTIALS`** — один и тот же ответ
и на несуществующий логин, и на неверный пароль, и на неактивный статус
(`User.isActive()`, `status != ACTIVE`). Так сделано, чтобы по ответам не перебирались
имена зарегистрированных.

Логин ищется **без учёта регистра** (`findByUsernameIgnoreCase`, ADR-058).

**Побочный эффект:** новая строка в `refresh_tokens`. Старые сессии не трогаются.

### `POST /api/auth/refresh` — без токена

**Тело** `RefreshRequest`: `refreshToken` `@NotBlank`. **Заголовок:** `User-Agent`.

**Ответ 200** `TokenPair` — новые access **и** refresh.

**Ошибки:** 400 `VALIDATION_FAILED`; 401 `REFRESH_TOKEN_INVALID` (не найден / отозван /
просрочен / пользователь неактивен или удалён).

**Побочные эффекты:** старый refresh помечается отозванным, создаётся новый;
при предъявлении уже отозванного — отзыв **всей серии** пользователя (см. §3.2).

### `POST /api/auth/logout` — без токена

**Тело** `RefreshRequest`: `refreshToken` `@NotBlank`.
**Ответ 204**, тело пустое. Идемпотентно: неизвестный/уже отозванный токен → тоже 204.

### `POST /api/auth/ws-ticket` — **токен нужен**

Тела нет.

**Ответ 200** `AuthDtos.WsTicket`:

| Поле | Тип | Nullable |
|---|---|---|
| `ticket` | string (32 симв., base64url) | нет |
| `expiresIn` | number (секунды, = 30) | нет |

Побочный эффект — запись в in-memory карту тикетов.

### `TokenPair` и `UserView`

`AuthDtos.TokenPair`:

| Поле | Тип | Nullable |
|---|---|---|
| `accessToken` | string (JWT) | нет |
| `refreshToken` | string (43 симв.) | нет |
| `expiresIn` | number (секунды access-токена, = 900) | нет |
| `user` | `UserView` | нет |

`AuthDtos.UserView`:

| Поле | Тип | Nullable |
|---|---|---|
| `id` | string (UUID) | нет |
| `username` | string | нет |
| `displayName` | string | нет |
| `avatarUrl` | string | **да** → ключ вырезается |
| `avatar` | string (эмодзи, ≤8 симв.) | **да** → ключ вырезается |

⚠️ `avatarUrl` и `avatar` — два разных поля: первое досталось от старой схемы (ссылка),
второе — эмодзи (миграция V8). Пишет профиль только `avatar`; `avatarUrl` через REST
изменить нельзя.

### `GET /api/profile` — токен нужен

Ответ 200 `UserView` игрока из `sub`.
Ошибка: 404 `USER_NOT_FOUND`.

### `PATCH /api/profile` — токен нужен

**Тело** `UpdateProfileRequest`:

| Поле | Тип | Валидация |
|---|---|---|
| `displayName` | string | `@NotBlank`, `@Size(min=2, max=64)` |
| `avatar` | string | `@Size(max=8)`, **необязательное** |

**Обработка:** `displayName.trim()`; `avatar` → `null`, если он `null` или пустой/пробельный,
иначе `avatar.trim()`.

⚠️ `@Size(max=8)` считает **символы Java (UTF-16 code units)**, а не кодовые точки: эмодзи
с модификаторами (флаги, ZWJ-последовательности) съедают лимит быстрее, чем кажется.

**Ответ 200** обновлённый `UserView`. **Ошибки:** 400 `VALIDATION_FAILED`;
404 `USER_NOT_FOUND`. **Побочный эффект:** `UPDATE users SET display_name, avatar`.
Логин не меняется — по нему входят.

### `POST /api/profile/password` — токен нужен

**Тело** `ChangePasswordRequest`:

| Поле | Тип | Валидация |
|---|---|---|
| `currentPassword` | string | `@NotBlank` |
| `newPassword` | string | `@NotBlank`, `@Size(min=8, max=128)` |

**Ответ 204**, тело пустое.

**Ошибки:** 400 `VALIDATION_FAILED`; **400 `INVALID_CREDENTIALS`** — старый пароль
не подошёл (⚠️ здесь 400, а не 401, в отличие от логина); 404 `USER_NOT_FOUND`.

**Побочные эффекты:** новый BCrypt-хеш в `users`; **все** refresh-токены игрока
отзываются (`revokeAllOf`). Текущий access-токен продолжает работать до своего `exp`.

⚠️ Старый пароль спрашивается даже при живом токене: иначе оставленная открытой вкладка
позволяет запереть владельца снаружи.

---

## 6. Столы и каталоги

### `GET /api/tables` — токен нужен

Параметров нет. Возвращает `List<TableView>` — только `status = WAITING` **и**
`is_private = false`, сортировка `created_at DESC`.

⚠️ Приватные столы в списке не видны никогда — попасть на них можно только по коду.

### `POST /api/tables` — токен нужен

**Тело** `LobbyDtos.CreateTableRequest`:

| Поле | Тип | Валидация | Значение по умолчанию |
|---|---|---|---|
| `name` | string | `@NotBlank`, `@Size(min=2, max=64)` | — |
| `maxPlayers` | int | `@Min(2)`, `@Max(5)` | — (примитив: отсутствие поля = `0` → 400) |
| `cardSetId` | UUID | нет | `card_sets.is_default = true` |
| `themeId` | UUID | нет | `table_themes.is_default = true` |
| `rulesConfig` | object | нет | `{}` (сериализуется как есть в jsonb) |
| `isPrivate` | boolean | нет | `false` |

⚠️ Jackson маппит `isPrivate` из JSON-ключа **`isPrivate`** (record-компонент так и назван);
ключ `private` не подхватится.

**Ответ 200** `TableView` — уже с хозяином на месте 0. ⚠️ **200, не 201**; заголовка
`Location` нет.

**Ошибки:** 400 `VALIDATION_FAILED`; 400 `INVALID_RULES_CONFIG` (не сериализуется
`rulesConfig`); 409 `MATCH_IN_PROGRESS` (игрок сейчас в матче за другим столом);
409 `TABLE_FULL` / `ALREADY_AT_TABLE` (из внутреннего `join`);
500 `NO_DEFAULT` (не настроен набор карт или тема по умолчанию).

**Побочные эффекты, по порядку (`LobbyService.create`):**

1. Если игрок уже сидит за столом со статусом `≠ CLOSED`:
   - статус `IN_MATCH` → 409 `MATCH_IN_PROGRESS`, дальше ничего не происходит;
   - иначе строка `table_players` **удаляется**, и если стол опустел — он закрывается
     (`status = CLOSED`, `closed_at = now`).
2. `INSERT game_tables`: `id` — случайный UUID; `code` — 6 символов из алфавита
   `ABCDEFGHJKLMNPQRSTUVWXYZ23456789` (без похожих символов, код диктуют голосом),
   до 5 попыток подобрать незанятый, иначе `IllegalStateException` → 500 `INTERNAL_ERROR`;
   `status = WAITING`, `version = 0`.
3. `join(table, host)` — вставка в `table_players` на минимальное свободное место
   (нумерация **с 0**), до 5 попыток в отдельных транзакциях (`SeatAllocator`,
   `REQUIRES_NEW`).
4. Если посадка упала — стол сразу закрывается (`closeIfDeserted`) и ошибка пробрасывается.

⚠️ `create` **не** помечен `@Transactional` намеренно: после нарушения уникального индекса
в Postgres транзакцию продолжать нельзя, поэтому каждая попытка посадки — своя транзакция.

### `GET /api/tables/current` — токен нужен

**Ответ 200** `LobbyDtos.CurrentTableView`:

| Поле | Тип | Nullable |
|---|---|---|
| `table` | `TableView` | **да** → ключ вырезается |
| `inMatch` | boolean | нет (`false`, если стола нет) |
| `mySeatNo` | Integer | **да** → ключ вырезается |

«Текущий» = первая строка `table_players` этого игрока, чей стол не `CLOSED`.
`inMatch = (status == IN_MATCH)`.

⚠️ Ошибки «не найдено» здесь нет: игрок нигде не сидит → 200 и `{"inMatch":false}`.

### `GET /api/tables/{id}` — токен нужен

`id` — UUID в пути. Ответ 200 `TableView`. Ошибки: 404 `TABLE_NOT_FOUND`;
⚠️ невалидный UUID → **500 `INTERNAL_ERROR`** (см. §1).

### `GET /api/tables/by-code/{code}` — токен нужен

`code` — строка в пути; перед поиском **приводится к верхнему регистру**
(`LobbyService.byCode`), поэтому `abc123` и `ABC123` — один стол.
Ответ 200 `TableView`. Ошибка: 404 `TABLE_NOT_FOUND`.

### `GET /api/tables/invite/{code}` — **БЕЗ токена**

Единственная ручка столов без авторизации (ADR-063). Код так же поднимается в верхний
регистр.

**Ответ 200** `LobbyDtos.TableInviteView`:

| Поле | Тип | Nullable | Значение |
|---|---|---|---|
| `code` | string | нет | код стола (как в БД, в верхнем регистре) |
| `name` | string | нет | |
| `maxPlayers` | int | нет | |
| `seatsTaken` | int | нет | `count(table_players)` |
| `isPrivate` | boolean | нет | |
| `joinable` | boolean | нет | `status == WAITING && seatsTaken < maxPlayers` |

⚠️ Имён и идентификаторов игроков здесь **нет намеренно**: код короткий и живёт
в переписке, поэтому всё в этом ответе — публично.

**Ошибка:** 404 `TABLE_NOT_FOUND`.

### `DELETE /api/tables/{id}` — токен нужен

**Ответ 204**, тело пустое.

**Ошибки:** 404 `TABLE_NOT_FOUND`; 403 `NOT_TABLE_HOST` (закрывает только хозяин);
409 `MATCH_IN_PROGRESS`.

**Побочный эффект:** `status = CLOSED`, `closed_at = now`. Строки `table_players`
**не удаляются** — стол просто перестаёт считаться текущим.

### `TableView` и `SeatView`

`LobbyDtos.TableView`:

| Поле | Тип | Nullable |
|---|---|---|
| `id` | string (UUID) | нет |
| `code` | string | нет |
| `name` | string | нет |
| `hostUserId` | string (UUID) | нет |
| `maxPlayers` | int | нет |
| `status` | string — `WAITING` / `IN_MATCH` / `CLOSED` | нет |
| `cardSetId` | string (UUID) | нет |
| `themeId` | string (UUID) | нет |
| `isPrivate` | boolean | нет |
| `seats` | `List<SeatView>`, порядок по `seat_no` | нет (может быть пустым) |

⚠️ `rulesConfig` в `TableView` **не отдаётся** — при том что в запросе создания он есть.
Прочитать правила стола через REST нельзя.

`LobbyDtos.SeatView`:

| Поле | Тип | Значение |
|---|---|---|
| `seatNo` | int | с 0 |
| `userId` | string (UUID) | |
| `displayName` | string | из `users`; **`"—"`** (em-dash), если пользователь не найден |
| `ready` | boolean | `state == READY` (состояния: `JOINED`, `READY`, `LEFT`) |
| `online` | boolean | ⚠️ **захардкожено `true`** — реальное присутствие сюда пока не подключено |

### `GET /api/card-sets` — **БЕЗ токена**

Ответ 200 `List<CardSetView>`, сортировка по `name ASC`.

| Поле | Тип | Nullable |
|---|---|---|
| `id` | string (UUID) | нет |
| `code` | string | нет |
| `name` | string | нет |
| `description` | string | **да** |
| `version` | string | нет |
| `previewUrl` | string | **да** |
| `isDefault` | boolean | нет |

### `GET /api/card-sets/{id}/manifest` — **БЕЗ токена**

Ответ 200 `LobbyDtos.CardSetManifest`:

| Поле | Тип | Значение |
|---|---|---|
| `id` | string (UUID) | |
| `code` | string | |
| `version` | string | |
| `cards` | object `{cardCode: assetUrl}` | `LinkedHashMap`, порядок — `card_assets.ordinal ASC` |

⚠️ Порядок ключей в `cards` **значим** и должен воспроизводиться: это единственное, что
связывает код карты с картинкой (ADR-009); движок и протокол про URL не знают.

**Ошибка:** 404 `CARD_SET_NOT_FOUND`. Несуществующий набор при этом даёт 404, а вот
невалидный UUID — 500.

### `GET /api/table-themes` — **БЕЗ токена**

Ответ 200 `List<TableThemeView>`, сортировка по `name ASC`.

| Поле | Тип | Nullable |
|---|---|---|
| `id` | string (UUID) | нет |
| `code` | string | нет |
| `name` | string | нет |
| `feltColor` | string | **да** |
| `defaultBackCode` | string | **да** |
| `isDefault` | boolean | нет |

⚠️ Поле `background_url` в сущности `TableTheme` есть, но геттера и в DTO — нет; наружу
не отдаётся.

---

## 7. История матчей

Все три ручки требуют токен.

### `GET /api/matches` — токен нужен

**Query:**

| Параметр | Тип | Обязателен | По умолчанию |
|---|---|---|---|
| `userId` | UUID | нет | `sub` из токена |
| `status` | string | нет | без фильтра |

⚠️ **Чужую историю смотреть можно без ограничений** — `userId` берётся из запроса как есть,
проверки «это ты или твой друг» нет.

Фильтр `status` сравнивается **без учёта регистра** с именем enum
(`IN_PROGRESS`, `PAUSED`, `FINISHED`, `ABORTED`); неизвестное значение просто даёт
пустой список, а не ошибку.

**Ответ 200** `List<MatchSummary>`, порядок `started_at DESC`. Пустой список, если игрок
не сыграл ни одного матча.

### `GET /api/matches/{id}` — токен нужен

**Ответ 200** `HistoryDtos.MatchDetails`:

| Поле | Тип |
|---|---|
| `match` | `MatchSummary` (с точки зрения `sub` из токена) |
| `deals` | `List<DealSummary>`, порядок `deal_no ASC` |

**Ошибка:** 404 `MATCH_NOT_FOUND`.
⚠️ Проверки «спрашивающий играл в этом матче» нет: детали любого матча видны любому
авторизованному, только поля `myPlace`/`myRatingDelta` будут вырезаны.

### `GET /api/matches/{id}/replay` — токен нужен

**Ответ 200** `HistoryDtos.Replay`:

| Поле | Тип | Значение |
|---|---|---|
| `matchId` | UUID | |
| `status` | string | `FINISHED` или `ABORTED` |
| `mySeat` | int | место спрашивающего или **`-1`**, если он в матче не играл |
| `events` | `List<ReplayEvent>` | все события с `seq > 0`, порядок `seq ASC` |

**Ошибки:** 404 `MATCH_NOT_FOUND`; **409 `MATCH_NOT_FINISHED`** — если статус
`IN_PROGRESS` или `PAUSED` (реплей идущего матча = чтение партии из другого окна).

⚠️ **Фильтрация скрытой информации:** событие отдаётся, если
`privateToSeat == null || privateToSeat == mySeat` (`MatchEventRecord.isVisibleTo`).
Не игравший получает `mySeat = -1` и, соответственно, только публичные события.
Сырой `match_events` наружу не отдаётся никогда — по нему читаются чужие руки.

### DTO истории

`HistoryDtos.MatchSummary`:

| Поле | Тип | Nullable |
|---|---|---|
| `id` | UUID | нет |
| `tableId` | UUID | нет |
| `status` | string enum | нет |
| `startedAt` | Instant | нет |
| `finishedAt` | Instant | **да** |
| `playersCount` | int | нет |
| `dealsPlayed` | int | нет |
| `abortReason` | string | **да** |
| `ratingCounted` | boolean | нет — `status == FINISHED` |
| `myPlace` | Integer | **да** (спрашивающий не играл или место не проставлено) |
| `myRatingDelta` | BigDecimal | **да** |
| `players` | `List<PlayerResult>` | нет |

⚠️ `players` сортируется **по `place`, `null` в конец** — а не по `seatNo`, хотя из БД
читается по `seat_no`.

`HistoryDtos.PlayerResult`:

| Поле | Тип | Nullable |
|---|---|---|
| `userId` | UUID | нет |
| `displayName` | string | нет — **`"—"`**, если пользователь не найден |
| `seatNo` | int | нет |
| `place` | Integer | **да** |
| `navesLevel` | string | **да** |
| `lossType` | string (`LossDegree.name()`) | **да** |
| `ratingBefore` | BigDecimal | **да** |
| `ratingAfter` | BigDecimal | **да** |
| `ratingDelta` | BigDecimal | **да** |

`LossDegree` — `ROYAL`, `SUPER_MEGA_SUCK`, `SUPER_MEGA_FAIL`, `SUPER_FAIL`, `FAIL`
(порядок объявления = от самой тяжёлой к обычной).

`HistoryDtos.DealSummary`:

| Поле | Тип | Nullable |
|---|---|---|
| `dealNo` | int | нет |
| `trumpSuit` | string | нет |
| `loserSeat` | int | нет |
| `finishedAt` | Instant | **да** |
| `lastAttackCards` | `List<String>` | нет — распакованный jsonb-массив |
| `seats` | `List<DealSeatResult>`, порядок `seat_no ASC` | нет |

`HistoryDtos.DealSeatResult`:

| Поле | Тип | Nullable |
|---|---|---|
| `seatNo` | int | нет |
| `place` | Integer | **да** |
| `hungCards` | `List<String>` | нет |
| `navesLevelBefore` | string | **да** |
| `navesLevelAfter` | string | **да** |
| `levelChanges` | `List<LevelChangeView>` | нет |

`LevelChangeView`: `reason` (string), `amount` (int). Читается из jsonb-массива объектов
`{reason, amount}`; ⚠️ отсутствие любого из ключей даст NPE → 500.

`HistoryDtos.ReplayEvent`:

| Поле | Тип | Nullable |
|---|---|---|
| `seq` | int | нет |
| `dealNo` | Integer | **да** |
| `type` | string | нет |
| `actorSeat` | Integer | **да** |
| `payload` | произвольный JSON (`JsonNode`) | нет — отдаётся как есть, без обёртки |

⚠️ Если в БД лежит неразбираемый JSON — `IllegalStateException` → 500 `INTERNAL_ERROR`.

---

## 8. Рейтинг, статистика, друзья, push

### `GET /api/rating/me` — токен нужен

Ответ 200 `RatingDtos.RatingView` для `sub`.

### `GET /api/rating/users/{id}` — **токен нужен**

⚠️ Вопреки ожиданию «публичного профиля», ручка закрыта: она попадает под
`anyRequest().authenticated()`.

Ответ 200 `RatingView`. **Ошибка:** 404 `USER_NOT_FOUND`.

`RatingDtos.RatingView`:

| Поле | Тип | Значение |
|---|---|---|
| `userId` | UUID | нет |
| `displayName` | string | нет |
| `rating` | BigDecimal | из `user_rating`; **`1000`** (`UserRating.INITIAL`), если строки нет |
| `matchesPlayed` | int | **`0`**, если строки нет |
| `history` | `List<RatingPoint>` | порядок `created_at DESC`, может быть пустым |

⚠️ Не игравший — это **не ошибка**: 200 со стартовым рейтингом и пустой историей.

`RatingDtos.RatingPoint`: `matchId` (UUID), `ratingBefore` (BigDecimal),
`ratingAfter` (BigDecimal), `place` (int), `playersCount` (int), `playedAt` (Instant,
маппится из `RatingHistoryEntry.createdAt`). Все non-null.

### `GET /api/rating/top` — токен нужен

Ответ 200 `List<LeaderRow>`, сортировка `rating DESC`. Без пагинации и без лимита —
отдаётся вся таблица `user_rating`.

`LeaderRow`: `userId` (UUID), `displayName` (string, `"—"` если пользователь не найден),
`rating` (BigDecimal), `matchesPlayed` (int).

⚠️ N+1: имя каждого игрока читается отдельным `findById`.

### `GET /api/rating/seasons` — токен нужен

Ответ 200 `RatingDtos.SeasonsView`:

| Поле | Тип |
|---|---|
| `seasons` | `List<SeasonView>`, порядок `started_at DESC` |
| `canManage` | boolean — `bardak.rating.season-admins` содержит `username` из токена |

`SeasonView`: `id` (UUID), `name` (string), `startedAt` (Instant),
`closedAt` (Instant, **nullable** → вырезается), `open` (boolean).

⚠️ `canManage` считается по claim `username` **точным сравнением строк**
(`Set.copyOf(seasonAdmins).contains(username)`) — в отличие от логина при входе,
здесь регистр важен.

### `POST /api/rating/seasons` — токен нужен

**Тело** `CreateSeasonRequest`: `name` (string). ⚠️ **`@Valid` не стоит и валидации нет
вовсе.**

**Ответ 200** `SeasonView` нового сезона.

**Ошибки:** 403 `NOT_SEASON_ADMIN`; ⚠️ `name = null` или отсутствующее тело →
`NullPointerException` в `Season.open` / `HttpMessageNotReadableException` →
**500 `INTERNAL_ERROR`**.

**Побочные эффекты (одна транзакция):** у первого сезона с `closed_at IS NULL`
проставляется `closed_at = now`; вставляется новый сезон `started_at = now`,
`closed_at = null`. Открытый сезон существует всегда — закрытие и открытие одним
действием (ADR-037), иначе матчи между двумя вызовами осели бы вне сезонов.

### `GET /api/stats/me`, `GET /api/stats/users/{id}` — токен нужен

Обе отдают 200 `StatsDtos.PlayerStats`. Ошибок «нет такого игрока» **нет**: неизвестный
UUID даёт `PlayerStats.empty()`.

| Поле | Тип | Nullable | Как считается |
|---|---|---|---|
| `matches` | int | нет | строк `match_players` игрока **с непустым `place`** (отменённые не в счёт, §5.3) |
| `wins` | int | нет | `place == 1` |
| `losses` | int | нет | `lossType != null` |
| `avgPlace` | BigDecimal | **да** | сумма мест / число матчей, `scale=2`, `HALF_UP` |
| `dealsPlayed` | int | нет | сумма `deals_played` только по матчам в статусе `FINISHED` |
| `streak` | `Streak` | нет | см. ниже |
| `bestRating` | BigDecimal | **да** | `max(ratingAfter)` по истории рейтинга |
| `worstRating` | BigDecimal | **да** | `min(ratingAfter)` |
| `degrees` | `List<DegreeCount>` | нет | только степени с `count > 0`, порядок — объявления `LossDegree` (от тяжёлой к обычной) |

`Streak`: `kind` — `"WIN"` / `"LOSS"` / `"NONE"`, `length` — int.
⚠️ Серия считается **по местам** (`place == 1`), а не по знаку дельты рейтинга: в матче
на пятерых можно занять второе место и потерять очки. Считается по `rating_history`
(порядок `created_at DESC`) — то есть по матчам, попавшим в рейтинг, а не по `match_players`.

`DegreeCount`: `degree` (string, имя `LossDegree`), `count` (int).

Пустая статистика: `PlayerStats.empty()` = `(0, 0, 0, null, 0, Streak("NONE",0), null, null, [])`.

⚠️ Считается на лету по всем матчам игрока, без кэша и без пагинации.

### `GET /api/friends` — токен нужен

Ответ 200 `FriendDtos.FriendList`:

| Поле | Тип | Смысл |
|---|---|---|
| `friends` | `List<Friend>` | принятые |
| `incoming` | `List<Friend>` | заявки ко мне |
| `outgoing` | `List<Friend>` | мои заявки |

**Сортировка:** `friends` — сначала `online = true`, внутри по `displayName`
**без учёта регистра** (`String.CASE_INSENSITIVE_ORDER`). `incoming` и `outgoing`
**не сортируются** — порядок как из `findAllInvolving`.

`FriendDtos.Friend`:

| Поле | Тип | Nullable | Значение |
|---|---|---|---|
| `userId` | string (UUID) | нет | |
| `username` | string | нет | |
| `displayName` | string | нет | |
| `avatar` | string | **да** | эмодзи |
| `online` | boolean | нет | по **живому сокету** (`Presence`, in-memory), а не по «был недавно» (ADR-054) |
| `status` | string | нет | `PENDING` / `ACCEPTED` |
| `mine` | boolean | нет | заявку отправил спрашивающий (`requestedBy == viewer`) |

⚠️ Строка пары, у которой второй пользователь не найден в `users`, **молча пропускается**.

⚠️ **Ключ пары в БД — упорядоченная двойка** `(low_user_id, high_user_id)`, и порядок
считается **по канонической строке UUID** (`one.toString().compareTo(two.toString())`),
а НЕ через `UUID.compareTo`: последний сравнивает UUID как два **знаковых** `long`, тогда
как Postgres сравнивает побайтово, и на идентификаторах со старшим единичным битом эти
порядки противоположны — строка падала на проверке `low_user_id < high_user_id`.
В Go сравнивать надо байты (или каноническую строку), а не знаковые числа.
Кто кого позвал, хранится отдельно в `requested_by`.

### `POST /api/friends/requests` — токен нужен

**Тело** `AddFriendRequest`: `username` `@NotBlank`, `@Size(max=32)`.

**Ответ 200** `Friend`. **Ошибки:** 400 `VALIDATION_FAILED`;
404 `USER_NOT_FOUND`; 409 `CANNOT_FRIEND_SELF`; 409 `ALREADY_FRIENDS`;
409 `REQUEST_ALREADY_SENT`.

Логин `trim()`-ится и ищется **без учёта регистра** (ADR-058).

**Побочные эффекты:**
- пары нет → `INSERT friendships` со `status = PENDING`, `requested_by = я`;
- **есть встречная заявка (`canBeAcceptedBy(я)`) → это СОГЛАСИЕ**: пара переводится
  в `ACCEPTED`, а не заводится вторая заявка. ⚠️ Ключевая деталь: иначе двое, нажавшие
  «добавить» одновременно, остались бы с двумя висящими заявками и без дружбы.

### `POST /api/friends/{friendId}/accept` — токен нужен

`friendId` — UUID в пути, тела нет.

**Ответ 200** `Friend`. **Ошибки:** 404 `NOT_FRIENDS` (пары нет);
409 `NOT_YOUR_REQUEST` (заявку прислал не тебе).

⚠️ Повторный accept уже принятой пары — **не ошибка**, возвращается 200 с текущим
состоянием (идемпотентно).

**Побочный эффект:** `status = ACCEPTED`, `decided_at = now`.

### `DELETE /api/friends/{friendId}` — токен нужен

**Ответ 204**, тело пустое. **Ошибка:** 404 `NOT_FRIENDS`.

**Побочный эффект:** строка `friendships` **удаляется**. Отклонить заявку и удалить друга
— одна и та же операция; отказы нигде не хранятся.

### `POST /api/friends/{friendId}/invite` — токен нужен

**Тело** `InviteRequest`: `tableId` **string** `@NotBlank` (⚠️ не UUID — парсится вручную
`UUID.fromString`, поэтому мусор даёт **500 `INTERNAL_ERROR`**, а не 400).

**Ответ 200**, `Map`: `{"delivered": true|false}`.

`delivered` — дошло ли приглашение **прямо сейчас** по живому сокету. Приглашение нигде
не хранится (ADR-055).

**Ошибки:** 400 `VALIDATION_FAILED`; 404 `TABLE_NOT_FOUND`;
403 `NOT_FRIENDS` (звать можно только принятого друга — проверка на сервере, потому что
запрос приходит из сети, а не с экрана); 404 `USER_NOT_FOUND`.

**Побочные эффекты (в БД — никаких):**
- по всем живым сокетам друга уходит WS-конверт `type = "TABLE_INVITE"`,
  `tableId = <id стола>`, `payload = {fromName, tableId, tableName, tableCode}`;
- если не доставлено **и** push включён — уходит web-push `notifyInvite`.

### `GET /api/push/key` — **токен нужен**

⚠️ Хотя ключ публичный, ручка закрыта: под `permitAll` она не попадает.

**Ответ 200**, `Map`, две разные формы:

- уведомления настроены: `{"enabled": true, "publicKey": "<base64url>"}`;
- не настроены: `{"enabled": false}` — **без** ключа `publicKey`.

«Настроены» = `bardak.push.public-key` и `private-key` оба непустые. `enabled: false` —
не поломка: клиент по этому признаку просто не показывает кнопку подписки.

### `POST /api/push/subscriptions` — токен нужен

**Тело** `PushController.SubscribeRequest`:

| Поле | Тип | Валидация |
|---|---|---|
| `endpoint` | string | `@NotBlank` |
| `p256dh` | string | `@NotBlank` |
| `auth` | string | `@NotBlank` |

**Заголовок:** `User-Agent` (необязателен) — сохраняется в строке подписки.

**Ответ 204**, тело пустое. **Ошибка:** 400 `VALIDATION_FAILED`.

**Побочные эффекты:** ключ строки — **`endpoint`** (ADR-048), а не пара
«пользователь + устройство»:
- endpoint неизвестен → `INSERT push_subscriptions`;
- endpoint известен → `reassign(userId, p256dh, auth, userAgent)` — строка обновляется,
  **включая смену владельца**. Устройство могло перейти к другому игроку, и звонить на
  него прежнему нельзя.

Ключи `p256dh`/`auth` сервер не разбирает и не валидирует — только хранит.

### `DELETE /api/push/subscriptions` — токен нужен

⚠️ **DELETE с телом** — `@Valid @RequestBody UnsubscribeRequest`: `endpoint` `@NotBlank`.
Не query-параметр. Клиенты и прокси, режущие тело у DELETE, эту ручку сломают; в Go
контракт надо сохранить.

**Ответ 204**, тело пустое. **Ошибка:** 400 `VALIDATION_FAILED`.

**Побочный эффект:** `deleteByEndpoint(endpoint)` — идемпотентно, неизвестный endpoint
тоже 204.

⚠️ **Владелец подписки не проверяется**: любой авторизованный, знающий чужой `endpoint`,
может его отписать.

---

## 9. Расхождения кода с `planning/05-api-contracts.md`

| Что в доке | Что в коде |
|---|---|
| `GET /users/{id}` — «публичный профиль: имя, рейтинг, число матчей» | **Такой ручки нет.** Ближайшее — `GET /api/rating/users/{id}` и `GET /api/stats/users/{id}`, обе за токеном |
| `POST /auth/register` — `{username, displayName, password, email?}` | Есть **обязательное** пятое поле `inviteCode` (`@NotBlank`), без него 400; неверный код — 403 `INVALID_INVITE_CODE` |
| `/auth/register`, `/auth/login` → `{accessToken, refreshToken, user}` | Есть ещё `expiresIn` (секунды access-токена) |
| Про `/api/health` в доке ничего нет | Ручка есть, открыта без токена |
| `/replay` — «доступен для завершённых **или отменённых**» | В коде отказ по `IN_PROGRESS` и `PAUSED`; `FINISHED` и `ABORTED` разрешены — совпадает по смыслу, но условие в коде сформулировано «от обратного» |
| «Право закрыть сезон — ADR-043» в тексте, «ADR-037» в таблице | В коде javadoc ссылается на ADR-037; сам список — `bardak.rating.season-admins` |
| Пример ошибки с `details: {tableId: …}` | Ни один `ApiException` в коде **не** передаёт `details`; непустой `details` бывает только у `VALIDATION_FAILED` (карта полей) |
| `GET /tables` — «список открытых столов (`status=WAITING`)» | Дополнительно фильтруется `is_private = false` |
| Про `online` у `SeatView` в лобби | Захардкожено `true`; настоящее присутствие есть только у друзей |

---

## 10. Чек-лист для Go-реализации

1. Глобально вырезать `null`-поля из ответов; пустые списки — оставлять.
2. `ApiError` — вырезать и пустой `details`.
3. `traceId` — 8 символов, а не полный UUID.
4. Повторить пары «код + статус», включая двойные (`INVALID_CREDENTIALS` 400/401,
   `NOT_FRIENDS` 403/404).
5. Решить, воспроизводить ли 500 на битом UUID / битом JSON / неверном методе, или
   зафиксировать расхождение.
6. 401 — пустое тело и `WWW-Authenticate`, не `ApiError`.
7. `POST` на создание отдаёт **200**, не 201; `Location` нет.
8. Хеш refresh-токена — **стандартный** Base64 от SHA-256, сам токен — **url-safe без
   паддинга**. Не перепутать.
9. JWT: только HS256, claims ровно `iss/iat/exp/sub/username/displayName`.
10. Порядок ключей в `CardSetManifest.cards` — по `ordinal`.
11. `players` в `MatchSummary` сортируется по `place` (null в конец), не по `seatNo`.
12. Места за столом нумеруются **с 0**.
13. `DELETE /api/push/subscriptions` принимает тело.
14. Коды столов сравниваются в верхнем регистре; логины — без учёта регистра;
    `season-admins` — **с** учётом регистра.
15. Ключ пары друзей упорядочивается по канонической строке UUID (побайтово), не по
    знаковому сравнению.
