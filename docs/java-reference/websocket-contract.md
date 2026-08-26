# Контракт WebSocket — как он реализован в Java

Документ описывает **фактическое поведение эталонного Java-бэкенда**, чтобы Go-реализация
повторила его без догадок. Источник истины — код, а не `planning/05-api-contracts.md`:
плановый документ местами описывает протокол, которого в коде нет. Все такие расхождения
отмечены явно (раздел «Расхождения с `05-api-contracts.md`»).

Разобранные файлы:

- `back-bardak/src/main/java/kz/bardak/game/ws/` — `Envelope`, `WebSocketConfig`,
  `EchoWebSocketHandler`, `GameCommandHandler`
- `back-bardak/src/main/java/kz/bardak/lobby/ws/LobbyCommandHandler.java`
- `back-bardak/src/main/java/kz/bardak/auth/ws/` — `WsTicketHandshakeInterceptor`, `WsTicketService`
- `back-bardak/src/main/java/kz/bardak/game/protocol/` — `GameProtocol`, `PlayerViewDto`,
  `CardCodec`, `MatchStateCodec`
- `back-bardak/src/main/java/kz/bardak/game/runtime/` — `TableRuntime`, `TableRegistry`,
  `TurnClock`, `TimeoutPolicy`, `GameProperties`, `MatchSession`, `MatchService`
- `back-bardak/src/main/java/kz/bardak/history/MatchLog.java`
- `back-bardak/src/main/java/kz/bardak/social/Presence.java`, `social/TableInvites.java`
- `back-bardak/src/main/resources/application.yml`

---

## 1. URL и рукопожатие

### 1.1 Эндпоинт

Один-единственный: **`/ws`** (`WebSocketConfig.registerWebSocketHandlers`). Сырой WebSocket
+ JSON, **без STOMP и без SockJS** (ADR-002): нужны персональные проекции состояния —
каждому игроку своё сообщение, а модель STOMP «топик → все получают одно и то же» работает
против этого.

Полный адрес: `ws(s)://host/ws?ticket=<одноразовый тикет>`.

Сокет — **на приложение, а не на стол** (ADR-053). Он открывается после входа и живёт до
выхода; стол на него подписывается и отписывается, но им не владеет. Отсюда следует, что
одно соединение может по очереди работать с разными столами, а присутствие игрока «в сети»
считается по живому сокету (`Presence`, ADR-054).

### 1.2 Одноразовый тикет (ADR-005)

Схема ровно такая:

1. Клиент, уже имея access-JWT, делает `POST /api/auth/ws-ticket` (с `Authorization: Bearer`).
   Ответ: `{"ticket": "<url-safe base64, 24 случайных байта>", "expiresIn": <секунды>}`.
2. Клиент открывает `ws://host/ws?ticket=<ticket>`.
3. `WsTicketHandshakeInterceptor.beforeHandshake` читает **query-параметр `ticket`**
   (`servletRequest.getParameter("ticket")`) и гасит его через `WsTicketService.consume`.

Свойства тикета (`WsTicketService`):

| Свойство | Значение в коде |
|---|---|
| Длина | 24 случайных байта из `SecureRandom`, `Base64.getUrlEncoder().withoutPadding()` |
| TTL | `bardak.auth.ws-ticket-ttl`, по умолчанию **30s** |
| Хранилище | `ConcurrentHashMap` в памяти инстанса |
| Погашение | `map.remove(ticket)` — атомарно, поэтому двух подключений по одному тикету быть не может |
| Проверка срока | после `remove`: `expiresAt.isAfter(now)`; просроченный тикет тоже удаляется |
| Уборка | `@Scheduled(fixedDelayString = "PT1M")` вычищает протухшие |

⚠️ **Ловушка для Go:** реестр тикетов однонодовый. Тикет действует только на том инстансе,
который его выдал. Это осознанный долг (общий кеш — на M8, вместе с Redis).

⚠️ **Читается только query-параметр.** `getParameter` в сервлете смотрит и в query string,
и в теле формы, но у WS-рукопожатия тела нет, так что фактически это query. Заголовка
`Authorization` обработчик не смотрит **вообще**.

### 1.3 Что происходит при неудаче

`beforeHandshake` возвращает `false` и ставит **HTTP 401 Unauthorized** — соединение не
устанавливается. Одинаково для трёх случаев: тикета нет, тикет выдуман/просрочен, тикет уже
погашен. Тела ответа нет, кода ошибки в JSON нет — только статус (подтверждено `WsHandshakeIT`:
`connect(null)`, `connect("not-a-real-ticket")` и повторное использование тикета — все
`hasMessageContaining("401")`).

⭐ Проверка идёт **до** установления соединения, поэтому неавторизованный клиент не занимает
ни сессию, ни память.

### 1.4 Origin

`WebSocketConfig` настраивает:

- `setAllowedOrigins(bardak.ws.allowed-origins)` — по умолчанию
  `http://localhost:8088,http://localhost:5173`;
- `setAllowedOriginPatterns(bardak.ws.allowed-origin-patterns)` — только если после
  отбрасывания пустых строк список непуст. По умолчанию
  `http://192.168.*.*:8088,http://10.*.*.*:8088,http://172.16.*.*:8088`.

⭐ Шаблоны нужны для игры по локальной сети: адрес машины меняется от роутера к роутеру.
Шаблоны ограничены частными диапазонами, `*` не используется намеренно — браузер шлёт
`Origin` при рукопожатии, и открытый список означал бы, что сокет может открыть любая
страница в интернете.

Spring Security при этом `/ws` пропускает (`.requestMatchers("/ws").permitAll()`) — вся
авторизация в интерсепторе.

### 1.5 Атрибуты сессии и обёртка

- В атрибуты сессии кладётся `userId` (ключ `WsTicketHandshakeInterceptor.USER_ID_ATTRIBUTE`
  == строка `"userId"`), тип — `java.util.UUID`. Анонимных соединений не существует.
- `afterConnectionEstablished` оборачивает сессию в `ConcurrentWebSocketSessionDecorator`
  с `SEND_TIME_LIMIT_MS = 10_000` и `SEND_BUFFER_LIMIT = 512 * 1024` байт. Причина:
  `WebSocketSession` не потокобезопасна, а писать в неё будут поток стола и планировщик
  таймеров. Лимит буфера — защита от медленного клиента.
- Соединение регистрируется в `Presence` (по `userId` + `sessionId`); у одного игрока
  может быть несколько соединений (второе устройство, соседняя вкладка), и сообщения
  `Presence.send` уходят **во все сразу**.

### 1.6 Первое сообщение сервера — `CONNECTED`

Сразу после установления соединения сервер шлёт **только этому сокету**:

```json
{"v":1,"type":"CONNECTED","ts":1730000000000,
 "payload":{"sessionId":"<id сессии Spring>","protocolVersion":1}}
```

`id`, `tableId`, `seq` отсутствуют (см. правило `NON_NULL` ниже).

---

## 2. Конверт сообщения

`kz.bardak.game.ws.Envelope` — Java-record, **одинаковый в обе стороны**:

```java
record Envelope(Integer v, String id, String type, String tableId,
                Integer seq, Long ts, JsonNode payload)
```

| Поле | Тип | Смысл | Когда `null`/отсутствует |
|---|---|---|---|
| `v` | int | Версия протокола. Константа `Envelope.PROTOCOL_VERSION = 1` | Никогда у сервера. У клиента `null` → `PROTOCOL_VERSION_UNSUPPORTED` |
| `id` | string | Идентификатор сообщения. У команд — идемпотентность и корреляция ответа | У **всех** событий сервера `id == null`, **кроме** `ERROR`, `PONG` и `ECHO`, где он равен `id` вызвавшей команды |
| `type` | string | Тип команды (client→server) или события (server→client) | Никогда. Пустой/`null` у клиента → `TYPE_REQUIRED` |
| `tableId` | string (UUID) | Стол, к которому относится сообщение; по нему маршрутизация | `null` у `CONNECTED`, у `ERROR` из `EchoWebSocketHandler`, у `PONG`, если команда пришла без `tableId` |
| `seq` | int | **Только у игровых событий**: сквозной номер по матчу | `null` у `STATE_SYNC`, `CONNECTED`, `PONG`, `ERROR`, `ECHO`, `MATCH_OVER`, `MATCH_PAUSED`, `MATCH_RESUMED`, `MATCH_ABORTED`, `TURN_TIMEOUT`, `TABLE_INVITE` и **у всех событий лобби** |
| `ts` | long | `Instant.now().toEpochMilli()` отправителя | Никогда у сервера |
| `payload` | JSON | Полезная нагрузка, форма зависит от `type` | `null` у `PONG`; у `MATCH_RESUMED` — пустой объект `{}` |

### 2.1 ⭐ Сериализация: `NON_NULL` — поля не приходят как `null`, они **отсутствуют**

Два независимых механизма, оба важны для Go:

1. `@JsonInclude(JsonInclude.Include.NON_NULL)` на самом `Envelope` — `null`-поля конверта
   **выпадают из JSON целиком**. `PONG` выглядит так:
   `{"v":1,"id":"c-7","type":"PONG","tableId":"...","ts":...}` — ключа `payload` нет.
2. `spring.jackson.default-property-inclusion: non_null` в `application.yml` — то же правило
   применяется ко **всем** POJO, включая `PlayerViewDto`. В `STATE_SYNC` отсутствуют, а не
   равны `null`: `trumpSuit`, `trumpCard`, `protectedSuit`, `hangingVictimSeat`,
   `turnSecondsLeft`, `nextNavesRank`, `exitPlace`, `defend` внутри `table[]`.

⚠️ **Исключение:** узлы, собранные руками через `ObjectNode.put(key, (String) null)`,
дают явный `NullNode` и **остаются в JSON как `null`**. Это `lossDegree` в `MATCH_OVER`
и `payload` внутри `ECHO`. То есть в Go нельзя ставить `omitempty` тотально — надо
различать «поле DTO» (выпадает) и «поле собранного вручную узла» (остаётся `null`).

⚠️ Числа `0` и `false` **не выпадают** — `NON_NULL`, а не `NON_DEFAULT`. `mySeat: 0`,
`deckLeft: 0`, `passed: false` присутствуют всегда. В Go — указатели там, где в Java
`Integer`/`String`, и обычные значения там, где `int`/`boolean`.

### 2.2 Разбор входящего конверта

- `objectMapper.readValue(text, Envelope.class)`. `ObjectMapper` — дефолтный от Spring Boot,
  своего бина в проекте нет, значит `FAIL_ON_UNKNOWN_PROPERTIES = false`: **неизвестные поля
  входящего конверта молча игнорируются**.
- Ошибка разбора → `ERROR` с кодом `BAD_ENVELOPE` («Сообщение не разобрано как конверт
  протокола»), `id` в этом ответе — `null` (конверт не разобран, `id` взять неоткуда).
- Обрабатываются только **текстовые** кадры (`TextWebSocketHandler`). Бинарных обработчиков нет.
- Никаких ограничений на размер сообщения и никакого rate-limit в коде **не найдено**
  (в `05-api-contracts.md` они заявлены — см. расхождения).

---

## 3. Версия протокола

- Константа: `Envelope.PROTOCOL_VERSION = 1`. Ломающее изменение → `+1`.
- Проверка (`EchoWebSocketHandler.handleTextMessage`), сразу после разбора:

```java
if (incoming.v() == null || incoming.v() != Envelope.PROTOCOL_VERSION) {
    send(out, error(incoming.id(), "PROTOCOL_VERSION_UNSUPPORTED",
            "Поддерживается версия протокола " + Envelope.PROTOCOL_VERSION));
    return;
}
```

- Условие — **строгое равенство**, а не «>=». Ни старая, ни новая версия не принимается.
  Множественных поддерживаемых версий в коде нет.
- Ответ: `ERROR` с `payload.code = "PROTOCOL_VERSION_UNSUPPORTED"`, `id` = `id` команды,
  `tableId` = `null` (жёстко, даже если команда пришла со столом). Соединение **не рвётся**.
- Проверка идёт **до** `PING`: даже heartbeat с чужой версией получит `ERROR`, а не `PONG`.

---

## 4. Порядок диспетчеризации в `EchoWebSocketHandler`

Строго сверху вниз, первое совпадение выигрывает:

1. разбор конверта → `BAD_ENVELOPE`;
2. версия → `PROTOCOL_VERSION_UNSUPPORTED`;
3. `type == null || isBlank` → `TYPE_REQUIRED`;
4. `type == "PING"` → `PONG`, возврат;
5. `gameCommands.handles(type)` → запомнить `tableId` в `sessionTables`, отдать в
   `GameCommandHandler`;
6. `lobbyCommands.handles(type)` → то же, отдать в `LobbyCommandHandler`;
7. иначе — **эхо**: `ECHO` с `payload = {"echoOf": <type>, "payload": <исходный payload>}`.

⚠️ **Ловушка №1 (значимая).** В шагах 5 и 6 выполняется
`sessionTables.put(sessionId, UUID.fromString(incoming.tableId()))` **без try/catch**.
Если `tableId` — непустая, но не-UUID строка (например `""` или `"abc"`), `UUID.fromString`
бросает `IllegalArgumentException` прямо из `handleTextMessage`. Spring оборачивает
хендлер в `ExceptionWebSocketHandlerDecorator`, который на исключении **закрывает сессию**
(`CloseStatus.SERVER_ERROR`, код 1011). То есть:

- `tableId` отсутствует (`null`) → доходит до хендлера команд → аккуратный
  `ERROR / TABLE_ID_INVALID`;
- `tableId` кривой (не-UUID) → **разрыв соединения**, никакого `ERROR` клиент не получит.

Код `TABLE_ID_INVALID` в `GameCommandHandler`/`LobbyCommandHandler` практически достижим
только для `tableId == null`. В Go это стоит починить осознанно (валидировать UUID на входе
и отвечать `TABLE_ID_INVALID`), но факт расхождения с Java надо зафиксировать.

⚠️ **Ловушка №2.** `sessionTables` — это `Map<sessionId, UUID>` c одним значением:
запоминается **последний** стол, к которому обращалось соединение. При обрыве отписка идёт
только от него. Если клиент успел поработать с двумя столами, от первого он не отпишется.

---

## 5. Команды client → server

### 5.1 Общие правила

| Уровень | Команды | Кто маршрутизирует |
|---|---|---|
| Транспорт | `PING` | `EchoWebSocketHandler` |
| Игра | `MATCH_START`, `PLAY_CARD`, `PASS`, `TAKE`, `TRANSFER`, `HANG_CARD`, `HANG_SKIP`, `CHOOSE_TRUMP`, `REVEAL_FACE_DOWN`, `STATE_REQUEST`, `RESYNC`, `MATCH_LEAVE` | `GameCommandHandler.COMMANDS` |
| Лобби | `TABLE_JOIN`, `TABLE_LEAVE`, `TABLE_READY` | `LobbyCommandHandler.COMMANDS` |
| Прочее | любой другой `type` | эхо |

Все игровые и лобби-команды исполняются **на потоке стола** (ADR-007): `handle()` только
проверяет `tableId`, берёт `TableRuntime` из `TableRegistry` и делает `runtime.submit(...)`.
Один стол — одна очередь, один поток `table-<uuid>`, порядок выполнения равен порядку
постановки. Гонок между командами одного стола не существует по построению.

Ответ на команду асинхронный: он приходит тем же сокетом, но уже с потока стола.
Синхронного ack/response нет.

### 5.2 Игровые команды

Общие для всех: `tableId` обязателен и валиден; отправитель обязан сидеть за столом
(`session.seatOf(userId)`), иначе `NOT_A_PLAYER`.

| Тип | Payload | Кто вправе слать | Что делает | Ответ |
|---|---|---|---|---|
| `MATCH_START` | — | любой подключённый (⚠️ **проверки «это хост» нет**) | `MatchService.start`, `results.startMatch`, `runtime.subscribe(отправителя)` | `broadcast(runtime, session, List.of())` → каждому подписчику **`STATE_SYNC`**, затем перезапуск часов |
| `PLAY_CARD` | `{cardCode}` или `{cardCode, targetCardCode}` | игрок за столом | `targetCardCode` отсутствует → `DealCommand.Attack`; присутствует → `DealCommand.Defend` | события + `STATE_SYNC` (см. §7) либо `ERROR` |
| `PASS` | — | игрок за столом | `DealCommand.Pass` | то же |
| `TAKE` | — | игрок за столом | `DealCommand.Take` | то же |
| `TRANSFER` | `{cardCode}` | игрок за столом | `DealCommand.Transfer` | то же |
| `HANG_CARD` | `{cardCode}` | игрок за столом | `DealCommand.HangCard` — ⚠️ **жертву сервер определяет сам**, `targetSeat` в payload не читается | то же |
| `HANG_SKIP` | — | игрок за столом | `DealCommand.HangSkip` | то же |
| `CHOOSE_TRUMP` | `{suit}` | игрок за столом | `Suit.valueOf(value.toUpperCase(Locale.ROOT))` — принимает `HEARTS`/`hearts` | то же |
| `REVEAL_FACE_DOWN` | — или `{targetCardCode}` | игрок за столом | без цели → `RevealFaceDown`; с целью → `RevealFaceDownToDefend` | то же |
| `STATE_REQUEST` | — | любой подключённый | `runtime.subscribe`, отдать снимок, и если отправитель сидит за столом — `resumeAfterReconnect` | `STATE_SYNC` лично + (если игрок) `MATCH_RESUMED` всем |
| `RESYNC` | `{lastSeq}` | любой подключённый | `runtime.subscribe`, догон по логу, снимок, `resumeAfterReconnect` | пропущенные события + `STATE_SYNC` лично + `MATCH_RESUMED` всем |
| `MATCH_LEAVE` | — | только игрок за столом (`NOT_AT_TABLE` иначе) | отмена матча целиком | `MATCH_ABORTED` всем подписчикам стола |

⭐ **`PLAY_CARD` — одна команда, два смысла.** Защита обязана указывать цель: без неё при
нескольких картах на столе невозможно зафиксировать, что чем отбито. В движке это **две
разные команды** (`Attack` и `Defend`), а не одна с необязательным полем.

⚠️ `card(payload, "cardCode")` бросает `IllegalArgumentException` («Не указана карта:
cardCode»), если поле отсутствует → `ERROR / BAD_COMMAND`. То же для `suit`
(«Не указана масть») и для нераспознанного кода карты в `CardCodec.decode`.

⚠️ `HANG_CARD` в `05-api-contracts.md` описан с `targetSeat` — **в коде этого поля нет**,
жертву знает движок из состояния окна навеса.

### 5.3 Команды лобби

| Тип | Payload | Кто вправе | Что делает | Что рассылается |
|---|---|---|---|---|
| `TABLE_JOIN` | — | любой подключённый | `lobby.join(tableId, userId)`, затем `runtime.subscribe(userId, sender)` | `PLAYER_JOINED` **всем подписчикам стола** |
| `TABLE_LEAVE` | — | сидящий за столом | `lobby.leave`, затем `registry.unsubscribe` | `PLAYER_LEFT` всем (⭐ **до** отписки, поэтому уходящий своё событие ещё получает) |
| `TABLE_READY` | `{ready}` (по умолчанию `true`, если payload отсутствует) | сидящий за столом | `lobby.setReady` | `PLAYER_READY` всем |

Payload события лобби (`LobbyCommandHandler.payload`):
`{"userId": "...", "displayName": "..." (только если пользователь найден),
"seatNo": n, "ready": bool}` — `seatNo` и `ready` присутствуют только у `PLAYER_JOINED`
и `PLAYER_READY`; у `PLAYER_LEFT` и `PLAYER_OFFLINE` их нет.

⭐ **Подписка на события стола происходит только в четырёх местах:** `TABLE_JOIN`,
`MATCH_START`, `RESYNC`, `STATE_REQUEST`. Игрок, приславший `PLAY_CARD` без предварительной
подписки, получит только прямые ответы через `sender` (то есть `ERROR`), но **не** получит
`STATE_SYNC` — тот идёт через `runtime.sendTo`, а слушателя нет. Это надо воспроизвести
или сознательно изменить.

### 5.4 Коды отказов

**Транспортные (`EchoWebSocketHandler`, `tableId` в конверте всегда `null`):**

| Код | Когда |
|---|---|
| `BAD_ENVELOPE` | JSON не разобрался в конверт |
| `PROTOCOL_VERSION_UNSUPPORTED` | `v == null` или `v != 1` |
| `TYPE_REQUIRED` | `type` пустой или отсутствует |

**Уровня хендлера (`tableId` — из команды):**

| Код | Источник | Когда |
|---|---|---|
| `TABLE_ID_INVALID` | оба хендлера | `tableId == null`/пустой (см. ловушку §4) |
| `NO_MATCH` | `GameCommandHandler` | за столом матч не идёт (все команды кроме `MATCH_START`) |
| `NOT_AT_TABLE` | `GameCommandHandler` | `MATCH_LEAVE` от того, кто не играет |
| `NOT_A_PLAYER` | `GameCommandHandler` | игровой ход от того, кто не за столом |
| `BAD_COMMAND` | `GameCommandHandler` | любой `IllegalArgumentException`: нет `cardCode`/`suit`, кривой код карты, неизвестный тип, а также **`viewFor` для не-игрока** |
| `INTERNAL_ERROR` | оба хендлера | любой прочий `RuntimeException` (в лог — `log.error`) |
| `UNKNOWN_COMMAND` | `LobbyCommandHandler` | недостижимо: `handles()` отсеивает раньше |

**Из `ApiException` доменных сервисов** (код берётся из `e.code()`, сообщение — из
`e.getMessage()`):

- `MatchService`: `MATCH_ALREADY_STARTED`, `TABLE_NOT_READY`
- `LobbyService`: `TABLE_NOT_FOUND`, `TABLE_NOT_OPEN`, `TABLE_FULL`, `ALREADY_AT_TABLE`,
  `NOT_AT_TABLE`, `NOT_TABLE_HOST`, `MATCH_IN_PROGRESS`

**Из движка (`RejectionReason`)** — код `ERROR` равен `reason.name()`, сообщение всегда
буквально `"Ход отклонён"`:

`NOT_YOUR_TURN`, `CARD_NOT_IN_HAND`, `FACE_DOWN_CARD_NOT_PLAYABLE`, `ATTACK_LIMIT_REACHED`,
`DEFENDER_HAS_TOO_FEW_CARDS`, `RANK_NOT_ON_TABLE`, `TARGET_NOT_ON_TABLE`,
`TARGET_ALREADY_BEATEN`, `CARD_DOES_NOT_BEAT`, `DEFENDER_ALREADY_TOOK`, `TRANSFERS_DISABLED`,
`TRANSFER_AFTER_FIRST_BEAT`, `TRANSFER_RANK_MISMATCH`, `NEXT_PLAYER_HAS_TOO_FEW_CARDS`,
`NAVES_DISABLED`, `CANNOT_HANG_ON_SELF`, `CARD_NOT_ON_NAVES_SCALE`, `NOT_IN_HANGING_WINDOW`,
`TRUMP_NOT_IN_DISPUTE`, `TRUMP_NOT_CHOSEN_YET`, `NOTHING_TO_TAKE`, `MUST_REVEAL_FACE_DOWN`.

Форма `ERROR`:

```json
{"v":1,"id":"<id команды>","type":"ERROR","tableId":"<или отсутствует>",
 "ts":...,"payload":{"code":"NOT_YOUR_TURN","message":"Ход отклонён"}}
```

⭐ Отклонённая команда **не меняет состояние и не рвёт соединение**, но всё равно пишется
в лог матча как `MOVE_REJECTED` и **сжигает один номер `seq`** (§8).

---

## 6. События server → client

### 6.1 Каталог

Колонка «Кому» — фактический механизм доставки:

- **лично** — `sender.accept(...)`, то есть в тот сокет, откуда пришла команда;
- **всем подписчикам** — `runtime.broadcast(...)`: всем, кто в `listeners` этого стола,
  включая наблюдателей, не сидящих за столом;
- **персонально каждому игроку** — `runtime.sendTo(player, ...)` в цикле по
  `session.players()`: **только игроки матча**, наблюдатели не получают ничего.

| Тип | Payload | Кому | Есть `seq`? |
|---|---|---|---|
| `CONNECTED` | `{sessionId, protocolVersion}` | лично, сразу после handshake | нет |
| `PONG` | нет (`payload` отсутствует) | лично, `id` = `id` PING'а | нет |
| `ECHO` | `{echoOf, payload}` | лично | нет |
| `ERROR` | `{code, message}` | лично, `id` = `id` команды | нет |
| `STATE_SYNC` | `PlayerViewDto` (§6.3) | персонально каждому игроку | **нет** |
| `PLAYER_JOINED` | `{userId, displayName?, seatNo, ready}` | всем подписчикам | нет |
| `PLAYER_LEFT` | `{userId, displayName?}` | всем подписчикам | нет |
| `PLAYER_READY` | `{userId, displayName?, seatNo, ready}` | всем подписчикам | нет |
| `PLAYER_OFFLINE` | `{userId, displayName?}` | всем подписчикам (при обрыве) | нет |
| игровые события (§6.2) | зависит от типа | персонально каждому игроку, **с фильтром видимости** | **да** |
| `TURN_TIMEOUT` | `{seatNo}` | всем подписчикам | нет |
| `MATCH_PAUSED` | `{userId, turnMillisLeft, graceSeconds}` | всем подписчикам | нет |
| `MATCH_RESUMED` | `{}` (пустой объект) | всем подписчикам | нет |
| `MATCH_ABORTED` | `{userId, reason:"PLAYER_LEFT"}` (уход) или `{userId}` (не вернулся) | всем подписчикам | нет |
| `MATCH_OVER` | `{matchId, dealsPlayed, players[], lastAttackCards[]}` | всем подписчикам | нет |
| `TABLE_INVITE` | `{fromName, tableId, tableName, tableCode}` | во **все сокеты приглашённого** через `Presence.send`, вне стола | нет |
| `MOVE_REJECTED` | `{command, reason}` | **только через `RESYNC`**, только автору попытки | да |

⚠️ `MOVE_REJECTED` в реальном времени **не рассылается**: он существует только записью в
логе (`MatchLog.appendRejected` с `visibleToSeat = actorSeat`) и всплывает при догоне.
Событие `ATTEMPT_REJECTED`, обещанное планом, в коде **не найдено**.

### 6.2 Игровые события: типы и payload

Имя типа выводится механически из имени Java-record: `GameProtocol.eventType()` переводит
`CamelCase` → `SCREAMING_SNAKE_CASE` (`CardAttacked` → `CARD_ATTACKED`). Payload собирает
`GameProtocol.toEventPayload()` — `LinkedHashMap`, где **`seatNo` всегда первый ключ**.

| Тип | Payload | Видимость |
|---|---|---|
| `CARD_ATTACKED` | `{seatNo, cardCode}` | всем |
| `CARD_DEFENDED` | `{seatNo, cardCode, targetCardCode}` | всем |
| `ATTACK_TRANSFERRED` | `{seatNo, cardCode, toSeatNo}` | всем |
| `FACE_DOWN_REVEALED` | `{seatNo, cardCode}` | ⭐ **только владельцу** (`privateToSeat = seatNo`) |
| `PASSED` | `{seatNo}` | всем |
| `ATTACK_RIGHT_MOVED` | `{seatNo}` | всем |
| `ROUND_BEATEN` | `{seatNo, count}` — только количество, не карты | всем |
| `TAKE_ANNOUNCED` | `{seatNo}` | всем |
| `CARDS_TAKEN` | `{seatNo, count}` | всем |
| `CARDS_DRAWN` | `{seatNo, count}` | всем |
| `PLAYER_LEFT_DEAL` | `{seatNo}` | всем |
| `HIDDEN_TRUMP_REVEALED` | `{seatNo, cardCode}` | ⭐ **всем**, с мастью и номиналом |
| `TRUMP_CHANGED` | `{seatNo, suit}` | всем |
| `TRUMP_CHOSEN` | `{seatNo, suit}` | всем |
| `HANGING_WINDOW_OPENED` | `{seatNo}` | всем |
| `CARD_HUNG` | `{seatNo, cardCode, victimSeat}` | всем |
| `NAVES_LEVEL_CHANGED` | `{seatNo, level}` | всем |
| `DICE_ROLLED` | `{seatNo, participants:[int]}` | всем |
| `HANGING_WINDOW_CLOSED` | `{seatNo}` | всем |
| `DEAL_FINISHED` | `{seatNo}` — ⚠️ и только | всем |

⭐ **Приватность ровно одна.** `DealEvent.privateToSeat()` по умолчанию пуст; единственная
реализация, возвращающая место, — `FaceDownRevealed`. Скрытая карта уходит в руку владельца
и дальше играется как обычная; соперники узнают об этом только по изменившемуся снимку
(флага `hasHiddenCard` больше нет, `cardsCount` на единицу больше). Потайной козырь (§1.9
правил) устроен наоборот — он публичный, потому что меняет козырь всему столу.

Коды карт (`CardCodec`, ADR-009 — неизменяемый контракт): `6-diamonds`, `10-hearts`,
`A-spades`, `Joker-1`. Ранги: `6 7 8 9 10 J Q K A`. Масти в коде карты — **в нижнем
регистре** (`diamonds`, `hearts`, `spades`, `clubs`), а в поле `suit` событий и в
`trumpSuit` снимка — **в верхнем** (`DIAMONDS`, `HEARTS`, `SPADES`, `CLUBS`). Это разные
представления в одном протоколе, не опечатка.

### 6.3 `STATE_SYNC` — персональная проекция

Строится `GameProtocol.toDto(PlayerView, ...)` → `PlayerViewDto` → `valueToTree`.
⭐ Фильтрации здесь **нет**: чужих карт нет физически уже в `PlayerView` (ADR-026).
«Фильтровать было бы поздно и опасно».

```json
{"v":1,"type":"STATE_SYNC","tableId":"<uuid>","ts":...,
 "payload":{
   "tableId":"<uuid>", "dealNo":3, "phase":"DEFEND",
   "trumpSuit":"HEARTS", "trumpCard":"6-hearts", "protectedSuit":"SPADES",
   "deckLeft":12, "discardCount":20,
   "myHand":["A-spades","7-hearts"], "iHaveHiddenCard":true, "mySeat":2,
   "table":[{"attack":"10-diamonds"}],
   "players":[{"seatNo":0,"userId":"...","displayName":"...","cardsCount":6,
               "hasHiddenCard":false,"hung":["6-clubs"],"navesLevel":1,
               "nextNavesRank":"7","nextIsJoker":false,"passed":true,
               "inDeal":true,"stepsToJoker":8}],
   "attackerSeat":1, "defenderSeat":2, "canAttackSeat":1,
   "hangingVictimSeat":null, "turnSecondsLeft":27,
   "availableActions":[{"type":"PLAY_CARD","payload":{"cardCode":"7-hearts",
                        "targetCardCode":"10-diamonds"}},
                       {"type":"TAKE","payload":{}}]}}
```

Поля верхнего уровня:

| Поле | Тип | Примечание |
|---|---|---|
| `tableId` | string | дублирует конверт |
| `dealNo` | int | номер раздачи |
| `phase` | string | `DEALING`,`DICE`,`ATTACK`,`DEFEND`,`TAKING`,`HANGING`,`REFILL`,`DEAL_OVER` |
| `trumpSuit` | string? | отсутствует, пока козырь разыгрывают костью |
| `trumpCard` | string? | козырная карта из-под колоды — видна всем; отсутствует, когда её забрали или козырь назван костью |
| `protectedSuit` | string? | считает сервер; шлётся отдельно, потому что козырь может смениться посреди раздачи |
| `deckLeft`, `discardCount` | int | только количества, карты не отдаются |
| `myHand` | string[] | своя рука; **скрытой карты в ней нет даже у владельца** |
| `iHaveHiddenCard` | bool | только факт |
| `mySeat` | int | место смотрящего |
| `table` | `[{attack, defend?}]` | `defend` отсутствует у неотбитой |
| `players` | см. ниже | все места, включая своё |
| `attackerSeat` | int | ⚠️ это `PlayerView.roundStarterSeat` — **кто начал раунд** |
| `defenderSeat` | int | кто отбивается |
| `canAttackSeat` | int | у кого право положить карту сейчас |
| `hangingVictimSeat` | int? | кому навешивают; отсутствует, если окна нет |
| `turnSecondsLeft` | int? | ⭐ **остаток в секундах**, а не дедлайн |
| `availableActions` | `[{type, payload}]` | форма совпадает с командой, которую можно прислать обратно |

`players[]` (`SeatStateDto`): `seatNo`, `userId`, `displayName` (`"—"`, если пользователь не
найден), `cardsCount`, `hasHiddenCard`, `hung[]`, `navesLevel` (**int**, не строка ранга),
`nextNavesRank` (код ранга, `null`→отсутствует), `nextIsJoker`, `passed`, `inDeal`,
`exitPlace` (int?, отсутствует пока играет), `stepsToJoker`.

⭐ **`turnSecondsLeft` отдаёт сервер**, а не клиент отсчитывает от своей догадки: по этим же
часам сервер сходит за молчащего, и расхождение выглядело бы как отобранный ход. Значение —
`ceil(remaining_ms / 1000)`; отсутствует, когда часы не идут (ждать некого, матч на паузе
или `autoMoveOnTimeout = false` — см. §10).

⭐ `availableActions` считает сервер (ADR-003): фронт правил не знает. Формат элемента —
`{"type": "...", "payload": {...}}`, где `payload` — пустой объект для `PASS`/`TAKE`/
`HANG_SKIP`/`REVEAL_FACE_DOWN` без цели.

⚠️ `stateSync` вызывает `session.viewFor(userId)`, который для не-игрока бросает
`IllegalArgumentException`. Значит **`STATE_REQUEST` от наблюдателя** (не сидящего за столом)
вернёт `ERROR / BAD_COMMAND`, а не снимок. Наблюдателей протокол фактически не поддерживает.

### 6.4 `MATCH_OVER`

```json
{"matchId":"...","dealsPlayed":7,
 "players":[{"userId":"...","seatNo":0,"displayName":"...","place":1,
             "navesLevel":"K","lossDegree":null,"ratingBefore":1000.00,
             "ratingAfter":1012.50,"ratingDelta":12.50}],
 "lastAttackCards":["8-hearts","8-clubs"]}
```

Типы берутся из `MatchResultService.RatingChange(UUID userId, int place, String navesLevel,
LossDegree lossDegree, BigDecimal before, BigDecimal after)`:

- `seatNo` = `-1`, если игрок уже не находится в `session.players()`;
- ⚠️ **`navesLevel` здесь — строка** (код ранга, `"K"`), в отличие от `STATE_SYNC`, где то же
  по названию поле — **число**. Это несогласованность самого протокола, её надо повторить
  как есть или менять сознательно на обеих сторонах;
- `ratingBefore` / `ratingAfter` / `ratingDelta` — `BigDecimal`, то есть JSON-числа
  **с дробной частью** (`1000.00`). В Go нельзя принимать их как `int`;
- `lossDegree` — `null` или имя enum-константы; ⭐ **`null` здесь остаётся в JSON**
  (узел собран через `ObjectNode.put`, а не через DTO).

⭐ `lastAttackCards` нужны для различения степеней проигрыша по восьмёркам (§0.3 правил);
карты были на столе у всех на виду, скрывать нечего.

---

## 7. ⭐ Порядок отправки: разбор `broadcast()`

Это ключевой раздел. Клиент опирается на порядок.

```java
private void broadcast(TableRuntime runtime, MatchSession session, List<DealEvent> events) {
    final int firstSeq = session.lastSeq() - events.size() + 1;
    for (final UUID player : session.players()) {
        int seq = firstSeq;
        for (final DealEvent event : events) {
            if (isVisibleTo(event, session, player)) {
                runtime.sendTo(player, serialize(eventMessage(session, event, seq)));
            }
            seq++;
        }
        runtime.sendTo(player, serialize(stateSync(session, player)));
    }
}

private boolean isVisibleTo(DealEvent event, MatchSession session, UUID player) {
    return event.privateToSeat()
            .map(seat -> session.seatOf(player).filter(own -> own == seat).isPresent())
            .orElse(true);
}
```

Что из этого следует — по пунктам, все существенные:

1. **Внешний цикл — по игрокам, внутренний — по событиям.** Это не «сначала все события
   всем, потом все снимки». Реальный порядок записи в сокеты:
   `игрок0: e1,e2,STATE_SYNC → игрок1: e1,e2,STATE_SYNC → игрок2: …`.
   Для отдельно взятого клиента это неотличимо от «события, потом снимок», но при
   реализации на Go важно, что порядок между **разными** игроками именно такой.

2. **События уходят ПЕРЕД `STATE_SYNC`.** Причина в комментарии кода: «сначала события —
   что произошло, потом снимок — как теперь. Иначе клиент увидит новое состояние раньше
   причины и не сможет его анимировать». Клиент анимирует по событию, а состояние берёт
   из снимка — снимок обязан прийти последним, иначе анимация сыграет из уже нового
   состояния.

3. **Только игроки матча.** Цикл идёт по `session.players()` — список мест матча,
   зафиксированный на старте. Подписчики-наблюдатели через `broadcast()` не получают
   **ничего**: ни событий, ни снимков. Им достаются только сообщения через
   `runtime.broadcast` (`MATCH_OVER`, `MATCH_PAUSED`, `MATCH_RESUMED`, `MATCH_ABORTED`,
   `TURN_TIMEOUT`, события лобби).

4. **`sendTo` тихо теряет сообщение**, если у игрока нет живого слушателя
   (`listeners.get(userId) == null`). Отвалившийся сокет не роняет рассылку остальным
   (`deliver` ловит `RuntimeException` и пишет в debug).

5. **`seq` инкрементируется независимо от видимости.** Счётчик `seq++` стоит **вне** `if`.
   Это значит, что игрок, которому событие не видно, увидит **дыру** в нумерации: `…,5,7,…`.
   См. §8 — это имеет прямые последствия.

6. **Порядок вызова из `execute()`** для обычного хода:
   `matchLog.append` → `dealsPlayed` → `recordFinishedDeals` → `saveSnapshot` →
   **`broadcast`** → `restartTurnClock` → `finishIfOver`.
   ⭐ Сначала лог, потом рассылка (ADR-004): иначе после падения между ними клиенты видели
   бы ход, которого в истории нет.
   Отсюда: `MATCH_OVER` приходит **после** событий и снимков закрывающего хода.

7. **В `applyTimeout` порядок другой:** `TURN_TIMEOUT` (через `runtime.broadcast`, без `seq`)
   уходит **перед** `broadcast(events)`. То есть последовательность:
   `TURN_TIMEOUT → события → STATE_SYNC → (MATCH_OVER)`.

8. **Рассылка идёт с потока стола**, поэтому порядок сообщений тот же, что и порядок команд.
   Никакой отдельной синхронизации в Go не нужно, если сохранить модель «стол = актор с
   очередью» (ADR-007).

---

## 8. Нумерация `seq`

- Счётчик живёт в `MatchSession.lastSeq` — обычный `int`, потому что доступ только с потока
  стола.
- **Сквозной по матчу**, а не по раздаче: «счётчик у клиента один», переход между раздачами
  не требует особой обработки.
- Начинается с **1**: первый вызов `session.nextSeq()` при `lastSeq == 0` даёт 1.
- `matchLog.append(matchId, firstSeq, dealNo, events)` присваивает номера подряд и
  возвращает номер последнего; он и записывается в `lastSeq`.
- Ход, породивший `N` событий, занимает `N` номеров. `broadcast` вычисляет
  `firstSeq = lastSeq - events.size() + 1` — то есть **номера присваиваются по факту записи
  в лог**, а не заново.
- **Отклонённый ход тоже сжигает номер:** `matchLog.appendRejected(..., session.nextSeq(), ...)`
  и следом `session.lastSeq(session.nextSeq())`. Но никакого сообщения с этим `seq` клиентам
  **не уходит**.
- `seq` есть **только у игровых событий**. У `STATE_SYNC` его нет — снимок не является
  точкой в потоке событий.
- После рестарта сервера `lastSeq` восстанавливается из `seq` последнего снимка
  (`MatchService.restore` → `session.lastSeq(snapshot.seq())`).

### ⚠️ Ловушка: дыры в `seq` — норма, а клиент их за норму не считает

Дыра возникает штатно в двух случаях: приватное `FACE_DOWN_REVEALED` у чужого игрока и
любой отклонённый ход. Фронт (`front-bardak/src/stores/table.svelte.js`) при этом делает:

```js
if (envelope.seq > table.lastSeq + 1) wsSend('RESYNC', {lastSeq: table.lastSeq}, ...);
table.lastSeq = Math.max(table.lastSeq, envelope.seq);
```

То есть каждая штатная дыра провоцирует лишний `RESYNC`, а догон возвращает уже применённые
события заново. Функционально это переживается (снимок всё равно приходит последним и
приводит клиента к истине), но в Go стоит либо не жечь номера, либо явно задокументировать
для клиента, что дыра ≠ потеря. **Как есть в Java — жжём.**

---

## 9. Идемпотентность команд по `id`

### 9.1 Как работает

```java
if (session.alreadyApplied(command.id())) {
    sender.accept(serialize(stateSync(session, userId)));
    return;
}
...
final MatchResult result = session.apply(move);
session.remember(command.id());
```

- Хранилище — `LinkedHashSet<String> appliedCommands` в `MatchSession`, окно
  `REMEMBERED_COMMANDS = 200`; при переполнении вытесняются самые старые.
- `alreadyApplied(null) == false`, `remember(null)` — no-op. **Команда без `id` никогда не
  дедуплицируется.**
- Повтор **не применяется**, но клиенту всё равно уходит `STATE_SYNC` лично — «иначе он
  останется в неведении, дошла команда или нет». Подтверждено тестом
  `ResyncIT.shouldApplyTheMoveOnceWhenTheSameCommandIsSentTwice`: после повтора на столе
  одна карта, но снимок пришёл.

### 9.2 Что дедуплицируется, а что нет

Проверка стоит **после** ветвлений `MATCH_START`, `MATCH_LEAVE`, `RESYNC`, `STATE_REQUEST` —
эти четыре команды **не идемпотентны** и выполняются каждый раз.

### 9.3 ⚠️ Отклонённая команда тоже запоминается

`session.remember(command.id())` вызывается **до** проверки `result instanceof Rejected`.
Значит повторная отправка команды, которую движок отклонил, вернёт не `ERROR`, а
`STATE_SYNC`. Клиент, переотправляющий ход после разрыва, не узнает причину отказа со
второй попытки. Поведение надо повторить или изменить осознанно.

### 9.4 Почему это вообще нужно — ADR-052

Клиент переотправляет команду после разрыва, не зная, дошла ли она. Без дедупликации ход
применился бы дважды: карта ушла бы со стола дважды или пас закрыл бы чужой раунд.

⭐ **Требование к формату `id`, вытекающее из ADR-052:** идентификатор — случайный префикс
экземпляра клиента **плюс** счётчик, а не просто счётчик. Простой счётчик `c-1, c-2, …`
обнулялся при перезагрузке страницы; после обновления первый же ход снова назывался `c-1`,
сервер узнавал в нём уже применённую команду, молча возвращал состояние — карта
развыбиралась, кнопка исчезала, стол не двигался. Со стороны игрока это выглядело как
**«кнопки не реагируют»**. `crypto.randomUUID` не подошёл: он доступен только в защищённом
контексте, а по локальной сети игра открывается по обычному http.

Фактический формат в текущем клиенте:
`${Date.now().toString(36)}${Math.random().toString(36).slice(2,8)}-${++counter}`.

Сервер формат **не проверяет** — для него `id` просто строка. Но окно в 200 идентификаторов
рассчитано именно на такой, уникальный в пределах запуска клиента, `id`.

---

## 10. Heartbeat, RESYNC и восстановление после обрыва

### 10.1 PING/PONG

Серверная сторона тривиальна и живёт до всякой игровой логики:

```java
if ("PING".equals(incoming.type())) {
    send(out, Envelope.event("PONG", incoming.id(), incoming.tableId(), null));
    return;
}
```

- `PONG` возвращает `id` из `PING` и `tableId` из `PING` (обычно отсутствует).
- `payload` — `null`, то есть ключа в JSON нет.
- ⭐ Прикладной heartbeat нужен, потому что TCP-таймауты измеряются минутами, а ход длится
  30 секунд: «висящее, но мёртвое» соединение иначе не отличить от живого.
- ⚠️ **Сервер сам `PING` не инициирует и молчание клиента не отслеживает.** Никакого
  idle-таймаута в коде нет. Пауза матча запускается не «по отсутствию активности», а по
  фактическому закрытию сокета (`afterConnectionClosed`). Заявленное в плане «сервер, не
  получая активности, переводит матч в PAUSED» — **не реализовано**.

Клиентская сторона (для справки, `ws-client.js`): `PING` каждые **20 с**, два подряд
пропущенных `PONG` → клиент сам закрывает сокет и идёт в реконнект.

### 10.2 Обрыв соединения — что делает сервер

`EchoWebSocketHandler.afterConnectionClosed`:

1. `sessions.remove(sessionId)`;
2. `presence.disconnected(userId, sessionId)` — игрок перестаёт считаться онлайн, когда
   закрылся его **последний** сокет;
3. `sessionTables.remove(sessionId)` → если стол был, то
   `gameCommands.onDisconnect(tableId, userId)` и следом
   `lobbyCommands.onDisconnect(tableId, userId)`.

`GameCommandHandler.onDisconnect` (только если матч идёт **и** пользователь в нём играет):
на потоке стола —

- `clock.pause(tableId)` → таймер хода **останавливается**, остаток запоминается;
- `runtime.broadcast(MATCH_PAUSED {userId, turnMillisLeft, graceSeconds})`;
- `turnNotifier.pausedFor(...)` — push пропавшему (окно тишины здесь не применяется);
- `clock.scheduleAbort(tableId, disconnectGrace, …)` — таймер отмены матча.

`LobbyCommandHandler.onDisconnect`: `registry.unsubscribe(tableId, userId)` +
`runtime.broadcast(PLAYER_OFFLINE {userId, displayName?})`. Обе задачи идут в одну очередь
стола, поэтому порядок сообщений: **`MATCH_PAUSED`, затем `PLAYER_OFFLINE`**.

⚠️ **Ловушка (важная для Go).** `TableRegistry.unsubscribe` выгружает стол, когда у него не
осталось слушателей: `tables.remove(...)` + `runtime.close()` (`queue.shutdownNow()`).
Вызывается это изнутри задачи, исполняемой самим этим потоком стола. Если отключился
последний участник, `TableRuntime` умирает, а запланированный `scheduleAbort` через минуту
попробует `runtime.submit(...)` на закрытой очереди → `RejectedExecutionException`, который
`TurnClock.runSafely` проглотит в лог. Итог: **матч останется `IN_PROGRESS`,
`lobby.finishMatch` не будет вызван, стол не вернётся в лобби.** Это выглядит как дефект,
а не как решение; в Go его воспроизводить не стоит, но знать про него нужно — сценарий
«все вышли одновременно» ведёт себя не так, как «вышел один».

### 10.3 Возвращение: `RESYNC` и `STATE_REQUEST`

`RESYNC` (`payload: {lastSeq: n}`, при отсутствии payload читается как `0`):

```java
runtime.subscribe(userId, sender);
resync(session, userId, sender, command);       // события + снимок
if (session.seatOf(userId).isPresent()) {
    resumeAfterReconnect(runtime, session, userId);
}
```

`resync()` строго по порядку:

1. если отправитель сидит за столом — по одному сообщению на каждое событие из
   `matchLog.since(matchId, lastSeq, seat)`. ⭐ **Уже отфильтрованные по видимости**:
   фильтр идёт по записанному вместе с событием `visibleToSeat`, а не по пересчёту правил —
   «иначе правило жило бы в двух местах и однажды разошлось». Сырой лог наружу не отдаётся
   никогда, в нём лежит скрытая информация;
2. **всегда** — `STATE_SYNC` лично. «Так клиент сходится, даже если пропустил больше, чем
   помнит сервер».

Форма догоняемого события: `v=1`, `id=null`, `type` = тип из лога (включая `MOVE_REJECTED`),
`tableId`, `seq` из лога, свежий `ts`, `payload` — распарсенный JSON из лога (при ошибке
разбора — пустой объект `{}`, молча).

⚠️ Дельты как таковой нет: сервер отдаёт **все** видимые события с `seq > lastSeq`, сколько
бы их ни было. Ветка «дыра большая → только снимок» из плана в коде отсутствует — снимок
приходит всегда, дополнительно к событиям.

`resumeAfterReconnect` (общий для `RESYNC` и `STATE_REQUEST`, только для играющих):

- `clock.cancelAbort(tableId)` — отмена таймера отмены матча;
- `turnNotifier.present(userId)` — следующий ход снова достоин push'а;
- `clock.resume(tableId)` — продолжить **с остатка**, если пауза была;
- `runtime.broadcast(MATCH_RESUMED {})` всем подписчикам.

⚠️ `MATCH_RESUMED` рассылается **при каждом** `RESYNC`/`STATE_REQUEST` от играющего, даже
если паузы не было (`clock.resume` при отсутствии паузы — no-op, а событие всё равно
уходит). Клиент обязан быть к этому готов.

`STATE_REQUEST` — то же самое минус догон по логу: `subscribe` → `STATE_SYNC` лично →
`resumeAfterReconnect`.

### 10.4 Полный цикл переподключения (как его делает эталонный клиент)

```
разрыв
  → backoff 1s,2s,4s,…,30s с джиттером ±30%
  → POST /api/auth/ws-ticket  (новый тикет ОБЯЗАТЕЛЕН на каждое подключение)
  → WebSocket /ws?ticket=…
  → CONNECTED
  → RESYNC {lastSeq}          (только если соединение уже было — при первом входе догонять нечего)
  → отложенные команды с прежними id (TTL 30 с = таймауту хода; старше — выбрасываются)
```

Обязанности клиента, вытекающие из серверного контракта:
отслеживать `seq` и просить `RESYNC` при дыре; не применять события с `seq <= lastAppliedSeq`;
переотправлять неподтверждённые команды **с тем же `id`**.

---

## 11. Таймауты хода и пауза матча

### 11.1 Настройки (`bardak.game`, `GameProperties`)

| Ключ | Дефолт в `application.yml` | Дефолт в коде | Смысл |
|---|---|---|---|
| `turn-timeout` | `30s` | `30s` | сколько даётся на ход |
| `auto-move-on-timeout` | `${BARDAK_AUTO_MOVE:false}` | `false` | ⭐ **ходить ли за молчащего вообще** |
| `disconnect-grace` | `60s` | `60s` | сколько ждать вернувшегося до отмены матча |

⭐ **По умолчанию сервер за игрока не ходит.** «Игра для своих, и отобранный ход раздражает
сильнее, чем сосед, отошедший за чаем». Часы включаются настройкой — там, где за столом
сидят чужие.

### 11.2 `restartTurnClock` — вызывается после каждого применённого хода

```java
if (session.state().isOver())                 { clock.cancel(tableId); return; }
var onTheClock = TimeoutPolicy.seatOnTheClock(session.state().deal());
if (onTheClock.isEmpty())                     { clock.cancel(tableId); return; }
callToTable(runtime, session, onTheClock.get());          // push тому, кого нет за столом
if (!properties.autoMoveOnTimeout())          { clock.cancel(tableId); return; }
clock.start(tableId, properties.turnTimeout(),
            () -> runtime.submit(() -> applyTimeout(runtime, session)));
```

⚠️ **Прямое следствие для протокола:** при `auto-move-on-timeout: false` (дефолт!)
`clock` не запущен, значит `clock.remaining()` пуст, значит **`turnSecondsLeft` в
`STATE_SYNC` отсутствует всегда**. Клиент не должен считать это ошибкой — это штатная
конфигурация «ход ждёт своего хозяина сколько угодно».

`callToTable` вызывается **независимо** от `autoMoveOnTimeout`: тому, чей ход, уходит push,
если его нет среди `runtime.subscribers()`. ⭐ «Нет за столом» определяется по подписке на
события стола, а не по наличию сокета вообще: игрок мог открыть приложение и уйти в другой
стол или в историю. Push уходит с чужого потока — на потоке стола ждать ответа push-сервиса
нельзя (ADR-007). Окно тишины на игрока — `bardak.push.quiet-for`, по умолчанию 2 минуты.

### 11.3 Кто «на часах» и что сервер делает за него (`TimeoutPolicy`)

| Фаза | Кто на часах | Автодействие |
|---|---|---|
| `ATTACK`, `TAKING` | `attackRightSeat` | `Pass` |
| `DEFEND` | `defenderSeat` | `Take`, а при пустом столе — `Pass` (брать нечего) |
| `DICE` | выбирающий масть, иначе `attackRightSeat` | `ChooseTrump` самой многочисленной мастью на руках |
| `HANGING` | первое место текущего шага, ещё не решившее | `HangSkip` |
| прочие | никто — таймер не нужен | — |

⭐ Всегда выбирается **самое безобидное** действие: сервер никогда не решает за человека,
какой картой ходить. Единственное исключение — бросок кости: там выбора нет по сути, и
пассивность не должна лишать права (ADR-030).

### 11.4 Срабатывание таймера — `applyTimeout`

⭐ Срабатывание не делает ничего игрового само: оно кладёт команду в очередь стола (ADR-007).
Иначе автодействие пришло бы с чужого потока и могло бы пересечься с настоящим ходом
игрока, успевшего в последнюю секунду.

Последовательность внутри `applyTimeout` (только если результат `Applied`):
`matchLog.append` → `dealsPlayed` → `recordFinishedDeals` → `saveSnapshot` →
**`TURN_TIMEOUT {seatNo}` всем подписчикам** → `broadcast(события + снимки)` →
`restartTurnClock` → `finishIfOver`.

`TURN_TIMEOUT` не имеет `seq` и рассылается через `runtime.broadcast`, то есть его получают
и наблюдатели.

### 11.5 Пауза при обрыве (`TurnClock`)

⭐ Таймер **приостанавливается, а не перезапускается** (§5.2 правил): игрок, у которого
оставалось три секунды, после возвращения получает свои три секунды, а не полные тридцать.
Приостанавливаемый таймер — не то же самое, что перезапускаемый, и именно это различие
правила требуют явно.

- Остаток снимается **с самого `ScheduledFuture`** (`getDelay(MILLISECONDS)`), а не из своей
  копии времени: «два счётчика одного и того же неизбежно разъезжаются, и клиент увидел бы
  не то, по чему сервер на самом деле сходит за игрока».
- `pause()` возвращает остаток → он же уезжает в `MATCH_PAUSED.turnMillisLeft`
  (в **миллисекундах**, в отличие от `turnSecondsLeft` в снимке — в секундах).
- Если таймера не было (в частности, при `autoMoveOnTimeout = false`), `pause()` вернёт
  `Duration.ZERO` → `turnMillisLeft: 0`.
- `remaining()` пуст, когда часы не идут: «либо ждать некого, либо матч на паузе».
- `resume()` при отсутствии паузы — no-op.
- Отдельный набор таймеров `aborts` — `scheduleAbort` / `cancelAbort`.

### 11.6 Конец матча по паузе — `abort`

Игрок не вернулся за `disconnect-grace`:

`clock.cancel` → `matchLog.abort(matchId, "Игрок не вернулся за отведённое время")` →
`matches.finish(tableId)` → `lobby.finishMatch(tableId)` →
`MATCH_ABORTED {userId}` всем подписчикам.

⚠️ Комментарий в коде отдельно подчёркивает, что `lobby.finishMatch` здесь обязателен:
без него отменённый матч оставлял стол в состоянии `IN_MATCH` навсегда — сесть за него
нельзя, начать новый нельзя, и в лобби он до конца дней показывал «матч идёт». Штатное
завершение это делало, а отмена — нет. **Повторить в Go обязательно.**

### 11.7 Добровольный выход — `MATCH_LEAVE` / `leaveMatch`

⚠️ Отличается от пропажи со связи тем, что ждать некого: человек ушёл сознательно, и держать
остальных минуту на паузе незачем. Матч отменяется сразу.

`clock.cancel` + `clock.cancelAbort` → `matchLog.abort(matchId, "Игрок вышел из матча")` →
`matches.finish` → `lobby.finishMatch` → `MATCH_ABORTED {userId, reason:"PLAYER_LEFT"}`
всем → `lobby.leave(tableId, userId)` → `registry.unsubscribe(tableId, userId)`.

⭐ Тихо освободить своё место нельзя: движок продолжал бы ждать ушедшего, а на освободившийся
стул сел бы посторонний. Отменённый матч в рейтинг не идёт ни для кого (§5.3 правил) — уйти
из проигранной партии, «чтобы не засчиталась», всё равно не выйдет.

### 11.8 Штатное завершение — `finishIfOver`

⭐ Итог и рейтинг пишет `MatchResultService.finishMatch` **одной транзакцией**, и только
после её успеха стол объявляется свободным (`matches.finish` + `lobby.finishMatch`). Иначе
стол успел бы открыться для нового матча, а результат прошлого — не записаться.
Последним уходит `MATCH_OVER` всем подписчикам.

---

## 12. Модель конкурентности (что обязана повторить Go-реализация)

| Механизм | Java | Зачем |
|---|---|---|
| Стол — актор | `TableRuntime` = `newSingleThreadExecutor`, поток `table-<uuid>`, daemon | ADR-007: до пяти источников команд плюс таймер, очередь даёт линейный порядок бесплатно; движок остаётся однопоточным |
| Подписки | `ConcurrentHashMap<UUID, Consumer<String>>` | **одна подписка на игрока**: при переподключении новая заменяет старую, и в мёртвый сокет уже никто не пишет |
| Жизненный цикл стола | поднимается по первому обращению `runtimeFor`, выгружается, когда `listeners` опустел | не держать поток на пустой стол |
| Сессия WS | `ConcurrentWebSocketSessionDecorator(10s, 512KB)` | `WebSocketSession` не потокобезопасна |
| Присутствие | `Presence`: `userId → {sessionId → sender}` | у игрока бывает несколько соединений; онлайн заканчивается на последнем |
| Матч | `MatchSession` **намеренно не потокобезопасен** | живёт на потоке своего стола |

⚠️ На потоке стола нельзя делать долгое — стол «залипнет» для всех за ним. Push поэтому
уходит с чужого потока.

⚠️ Реестры (`WsTicketService`, `Presence`, `TableRegistry`, `MatchService.sessions`) —
**в памяти одного узла**. Второй инстанс сломает и тикеты, и присутствие, и маршрутизацию.
Записанный долг M8 (Redis), а не недосмотр.

---

## 13. Восстановление после рестарта сервера

Не часть проводного протокола, но влияет на то, что клиент увидит после реконнекта.

`MatchService.find(tableId)` при промахе по памяти вызывает `restore(tableId)`:
`matchLog.activeMatchFor(tableId)` → `matchLog.latestSnapshot(matchId)` →
`MatchStateCodec.decode` → новая `MatchSession` с `lastSeq = snapshot.seq()`.

- ⭐ Места берутся из `match_players`, **а не из лобби**: лобби живёт своей жизнью, а матч
  раздан по местам, зафиксированным на старте, и в снимке места — это индексы. Возьми их из
  лобби — и после рестарта игрок получил бы **чужую руку**, причём молча: расклад выглядел
  бы правдоподобно. Есть fallback на лобби для матчей, начатых до появления `match_players`,
  с `log.warn`.
- Снимок пишется **после каждого хода**, а не «раз в N событий»: движок применяет команды,
  а не события, проиграть лог поверх старого снимка нечем.
- ⚠️ `appliedCommands` (окно идемпотентности) **не восстанавливается** — после рестарта
  сервера переотправленная команда применится повторно.
- `MatchStateCodec` — снимок состояния в JSON и обратно, **не часть WS-протокола**: наружу
  он не уходит никогда, только в таблицу снимков. Написан руками, а не аннотациями Jackson,
  потому что `game.rules` не должен знать ни про Spring, ни про JSON, ни про базу.

---

## 14. Расхождения с `planning/05-api-contracts.md`

Плановый документ описывает более богатый протокол, чем реализован. Ориентироваться при
портировании — **на код**.

**Команд, заявленных в плане, но отсутствующих в коде:**
`ROLL_DICE`, `HANG_CLAIM`. (`ROLL_DICE` не нужен — кость бросается автоматически, ADR-030;
спор за навес разрешается внутри движка.)

**Команд, есть в коде и нет в плановой таблице команд:** `MATCH_START`, `STATE_REQUEST`.

**Отличия payload команд:**

| Команда | План | Код |
|---|---|---|
| `HANG_CARD` | `{cardCode, targetSeat}` | `{cardCode}` — жертву определяет сервер |
| `TABLE_READY` | `{ready}` | `{ready}`, но при отсутствии payload — `true` |

**События, заявленные в плане, которых в коде нет вообще:**
`MATCH_STARTED`, `DEAL_STARTED`, `DICE_ROLL_REQUIRED`, `CARDS_DEALT`, `TURN_CHANGED`,
`CARD_PLAYED`, `PLAYER_PASSED`, `ATTEMPT_REJECTED`, `TRICK_RESOLVED`, `HIDDEN_CARD_REVEALED`,
`PLAYER_OUT`, `HANGING_STARTED`, `HANGING_SKIPPED`, `JOKER_HUNG`, `ACK`.

Их роль исполняют либо события движка с другими именами (`CARD_ATTACKED`/`CARD_DEFENDED`
вместо `CARD_PLAYED`, `PASSED` вместо `PLAYER_PASSED`, `ROUND_BEATEN`/`CARDS_TAKEN` вместо
`TRICK_RESOLVED`, `FACE_DOWN_REVEALED` вместо `HIDDEN_CARD_REVEALED`, `PLAYER_LEFT_DEAL`
вместо `PLAYER_OUT`, `HANGING_WINDOW_OPENED`/`_CLOSED` вместо `HANGING_STARTED`/`SKIPPED`),
либо просто `STATE_SYNC`. Отдельного `ACK` нет — подтверждением служит `STATE_SYNC`, а
`ERROR` несёт `id` команды.

**События, которых нет в плане, но они есть в коде:**
`CONNECTED`, `ECHO`, `PLAYER_OFFLINE`, `TURN_TIMEOUT`, `ATTACK_RIGHT_MOVED`,
`TAKE_ANNOUNCED`, `CARDS_DRAWN`, `HIDDEN_TRUMP_REVEALED`, `TRUMP_CHANGED`, `TRUMP_CHOSEN`,
`NAVES_LEVEL_CHANGED`, `DICE_ROLLED`, `MOVE_REJECTED` (только через `RESYNC`).

**Отличия payload событий:**

| Событие | План | Код |
|---|---|---|
| `DEAL_FINISHED` | богатый объект: `results[]`, `levelChanges[]`, `losers[]` | **`{seatNo}` и всё** |
| `MATCH_OVER` | `losers[]`, `mainLoserSeat`, `places[]`, `ratingChanges[]` | `{matchId, dealsPlayed, players[], lastAttackCards[]}` |
| `MATCH_PAUSED` | `{reason, waitingForSeat, resumeDeadlineTs}` | `{userId, turnMillisLeft, graceSeconds}` |
| `MATCH_ABORTED` | `{reason}` | `{userId}` или `{userId, reason:"PLAYER_LEFT"}` |
| `CARD_HUNG` | `{fromSeat, toSeat, cardCode, navesLevelAfter}` | `{seatNo, cardCode, victimSeat}` |
| `DICE_ROLLED` | `{seatNo, value}` | `{seatNo, participants:[…]}` — значения кости в протокол не уходят |

**Отличия `STATE_SYNC`:**

| Поле плана | Реальность |
|---|---|
| `matchId`, `seq`, `matchStatus` | **отсутствуют** |
| `anyTrickBeaten`, `maxAttackThisRound` | **отсутствуют** |
| `online` в `players[]` | **отсутствует** |
| `turnDeadlineTs` (абсолютное время) | `turnSecondsLeft` (остаток в секундах) |
| `navesLevel: "K"` (код ранга) | в `STATE_SYNC` — `navesLevel: 3` (**число**); в `MATCH_OVER` — строка `"K"` |
| `canHangSeat` в фазе HANGING | `hangingVictimSeat` |
| `availableActions: [{type, cardCode, …}]` | `availableActions: [{type, payload:{…}}]` — параметры **вложены** в `payload` |
| — | добавлены `hasHiddenCard`, `nextIsJoker`, `inDeal`, `exitPlace`, `stepsToJoker`, `tableId`, `dealNo` |

**Прочее, заявленное и не реализованное:**

- rate limit на команды — **не найдено в коде**;
- ограничение размера сообщения — **не найдено** (есть только лимит буфера исходящих, 512 КБ);
- «сервер, не получая активности, переводит матч в PAUSED» — **не реализовано**, пауза только
  по закрытию сокета;
- «сервер какое-то время поддерживает обе версии» — **не реализовано**, проверка строгая;
- ветка ресинка «дыра большая → только снимок» — **не реализована**, всегда события + снимок.

---

## 15. Чек-лист для Go-реализации

1. `/ws?ticket=…`, тикет из `POST /api/auth/ws-ticket`, 24 байта, TTL 30 с, гасится
   атомарно, отказ = HTTP 401 **до** апгрейда.
2. Конверт `{v,id,type,tableId,seq,ts,payload}` с `omitempty`-семантикой для `id`, `tableId`,
   `seq`, `payload`; `v` строго `== 1`, иначе `PROTOCOL_VERSION_UNSUPPORTED`.
3. Сразу после апгрейда — `CONNECTED {sessionId, protocolVersion}`.
4. `PING` → `PONG` с тем же `id`, до всякой игровой логики. Сервер сам не пингует.
5. Один стол = одна очередь/горутина; подписка одна на игрока и заменяется при реконнекте.
6. ⭐ Порядок рассылки хода: **для каждого игрока** — сначала видимые ему события с `seq`,
   потом его персональный `STATE_SYNC`. Никогда наоборот.
7. Фильтр видимости — только `FACE_DOWN_REVEALED` приватен; `seq` инкрементируется и для
   отфильтрованных событий.
8. `seq` сквозной по матчу, с 1, присваивается при записи в лог; лог пишется **до** рассылки.
9. Дедупликация по `id` в окне ~200, ответ на повтор — `STATE_SYNC`. Не распространяется на
   `MATCH_START`/`MATCH_LEAVE`/`RESYNC`/`STATE_REQUEST`.
10. `RESYNC {lastSeq}` → видимые события из лога (уже с записанной видимостью) → `STATE_SYNC`
    → `MATCH_RESUMED` всем.
11. Таймер хода приостанавливается при обрыве и продолжается с остатка; `turnSecondsLeft`
    считает сервер; при `autoMoveOnTimeout=false` часов нет вовсе.
12. `disconnect-grace` истёк → `MATCH_ABORTED` **и обязательно** возврат стола в лобби.
