# Семантика рантайма и параллелизма (Java → Go)

Источник истины — код `back-bardak/src/main/java/kz/bardak/game/runtime/` и вызывающий его
`game/ws/`, `lobby/ws/`. Всё, что ниже, проверено по коду на текущей рабочей копии; расхождения
с `planning/05-api-contracts.md` и `planning/09-decisions.md` вынесены отдельным разделом.

Разобранные файлы:

| Файл | Роль |
|---|---|
| `runtime/TableRuntime.java` | очередь и поток одного стола, реестр слушателей, рассылка |
| `runtime/TableRegistry.java` | карта живых столов, подъём и выгрузка |
| `runtime/MatchService.java` | старт матча, хранение сессий в памяти, подъём из снимка |
| `runtime/MatchSession.java` | состояние идущего матча, места, дедупликация команд |
| `runtime/TurnClock.java` | таймер хода (с паузой) и таймер отмены матча |
| `runtime/TimeoutPolicy.java` | кто на часах и что сервер делает за молчащего |
| `runtime/GameProperties.java` | `bardak.game.*`: таймаут хода, автоход, окно возврата |
| `runtime/RulesConfigCodec.java` | разбор `rules_config` стола в `RulesConfig` |
| `ws/GameCommandHandler.java` | вся оркестрация: команды, таймеры, пауза, отмена, ресинк |
| `ws/EchoWebSocketHandler.java` | сокет, маршрутизация конверта, обрыв связи |
| `lobby/ws/LobbyCommandHandler.java` | команды лобби на той же очереди стола |

---

## 1. Модель параллелизма

### 1.1. Что чем сериализуется

`TableRuntime` — один стол, одна очередь, один поток:

```java
this.queue = Executors.newSingleThreadExecutor(runnable -> {
    final Thread thread = new Thread(runnable, "table-" + tableId);
    thread.setDaemon(true);
    return thread;
});
```

Всё, что меняет состояние стола, проходит через `runtime.submit(...)`. Постановщиков четыре:

1. игровые команды — `GameCommandHandler.handle` (поток WebSocket-соединения);
2. команды лобби — `LobbyCommandHandler.handle` (тот же поток);
3. обрыв связи — `GameCommandHandler.onDisconnect` и `LobbyCommandHandler.onDisconnect`
   (поток закрытия сокета);
4. таймеры — `TurnClock` (единственный поток `turn-clock`), который в `onExpiry` **ничего
   игрового не делает**, а только кладёт задачу в очередь стола.

`submit` ловит любой `RuntimeException` из задачи и логирует — упавшая команда не убивает поток
стола и не останавливает очередь.

### 1.2. Что гарантируется

- **Линейный порядок** внутри одного стола: FIFO однопоточного `ThreadPoolExecutor`. Порядок
  выполнения = порядок постановки. Команда игрока и срабатывание таймера никогда не выполняются
  одновременно.
- **Однопоточность движка**: `MatchSession`, `MatchEngine`, `DealState` — без синхронизации
  вообще. В javadoc `MatchSession` записано прямо: «Объект не потокобезопасен намеренно: он
  живёт на потоке своего стола (ADR-007)». `lastSeq` — обычный `int` по той же причине.
- **Порядок сообщений совпадает с порядком команд**: рассылка (`broadcast`, `sendTo`) идёт с
  потока стола, синхронно внутри той же задачи.
- **Порядок внутри одного применённого хода**: сначала запись в лог (`matchLog.append`), потом
  снимок, потом события, потом персональный `STATE_SYNC`. Комментарий в коде: «Сначала лог,
  потом рассылка (ADR-004): иначе после падения между ними клиенты видели бы ход, которого
  в истории нет».

### 1.3. Что НЕ гарантируется (важно для переноса)

- **Очередь неограниченная.** `newSingleThreadExecutor` — это `LinkedBlockingQueue` без предела.
  Backpressure нет: клиент может забить очередь стола. В Go нужен буферизованный канал
  с осознанным поведением при переполнении (в Java его нет — это не «фича, которую надо
  повторить», а дыра, которую можно закрыть).
- **Отправка в сокет блокирует поток стола.** `TableRuntime.deliver` вызывает
  `listener.accept(message)`, а это `WebSocketSession.sendMessage` — синхронный ввод-вывод.
  Смягчено только `ConcurrentWebSocketSessionDecorator` с лимитами из `EchoWebSocketHandler`:
  буфер 512 КиБ (`SEND_BUFFER_LIMIT`), лимит времени отправки 10 000 мс (`SEND_TIME_LIMIT_MS`);
  при превышении декоратор закрывает сессию. То есть медленный клиент способен придержать стол
  до нескольких секунд. Это и есть «⚠️ на потоке стола нельзя делать долгое» из javadoc.
- **Работа с БД идёт на потоке стола.** `matchLog.append`, `snapshot`, `dealsPlayed`,
  `dealHistory.record`, `results.finishMatch`, `lobby.*` — всё синхронно внутри задачи стола.
  Наблюдаемое свойство «пока БД тупит, стол стоит» сохраняется как есть.
- **Не всё, что трогает матч, идёт через очередь.** См. ловушки §8.2 и §8.3.

### 1.4. Реестр слушателей

`TableRuntime.listeners` — `ConcurrentHashMap<UUID, Consumer<String>>`, **один слушатель
на игрока**, не на соединение. Мутируется он фактически только с потока стола (`subscribe`
вызывается внутри `execute`, `unsubscribe` — внутри задач стола), но конкурентная карта
оставлена как страховка; `subscribers()` отдаёт копию.

---

## 2. Жизненный цикл стола

### 2.1. Подъём

`TableRegistry.runtimeFor(tableId)` — `computeIfAbsent`: стол поднимается **по первой любой
команде с этим `tableId`**, игровой или лоббийной. Отдельного «создания стола» в рантайме нет;
существование стола в БД при этом не проверяется — `runtimeFor` создаст очередь под любой
корректный UUID, а «стола нет» вылезет уже из `lobby.byId(...)` внутри задачи.

### 2.2. Выгрузка

Единственный путь — `TableRegistry.unsubscribe(tableId, userId)`:

```java
runtime.unsubscribe(userId);
if (!runtime.hasListeners()) {
    tables.remove(tableId, runtime);
    runtime.close();      // queue.shutdownNow(); listeners.clear();
}
```

Вызывается из трёх мест, и все три — на потоке стола: `LobbyCommandHandler` на `TABLE_LEAVE`
и на `onDisconnect`, `GameCommandHandler.leaveMatch`. Стол выгружается, когда ушёл последний
подписчик.

**Выгрузка стола не убивает матч.** `MatchService.sessions` — отдельная карта, из неё сессия
удаляется только в `finish(tableId)` (конец матча, `MATCH_LEAVE`, отмена по невозврату).
После выгрузки следующая команда поднимет **новый** `TableRuntime` с пустым списком
слушателей, а `matches.find(tableId)` вернёт ту же живую сессию.

### 2.3. Подъём матча из снимка после рестарта

`MatchService.find` = `sessions.get(tableId)` **or** `restore(tableId)`. `restore` читает
`matchLog.activeMatchFor(tableId)` + `latestSnapshot(...)`, разбирает `rules_config` стола,
восстанавливает состояние кодеком и кладёт в `sessions`. То есть любая команда за столом
(включая `STATE_REQUEST`) после перезапуска сервера воскрешает матч прозрачно для игроков.

⭐ Места берутся из `match_players` (`findByMatchIdOrderBySeatNo`), а **не** из лобби. Если
записей нет (матчи, начатые до появления таблицы) — фолбэк на порядок мест лобби с `WARN`.
Комментарий объясняет цену ошибки: «Возьми их из лобби — и после рестарта игрок получил бы
чужую руку, причём молча».

### 2.4. Старт матча

`MatchService.start(tableId)`:

- `MATCH_ALREADY_STARTED` (409), если сессия уже есть;
- `TABLE_NOT_READY` (409), если `lobby.isReadyToStart` ложно;
- `matchSeed = secureRandom.nextLong()` — дальше всё случайное выводится из него;
- `matchLog.startMatch(tableId, seats, seed, table.rulesConfig())` — снимок правил обязателен;
- `lobby.startMatch(tableId)`.

Порядок мест берётся из лобби **один раз здесь** и фиксируется на весь матч.

### 2.5. Конец матча

`GameCommandHandler.finishIfOver`: `results.finishMatch(...)` (итог + рейтинг одной
транзакцией) → `matches.finish(tableId)` → `lobby.finishMatch(tableId)` → `runtime.broadcast`
`MATCH_OVER`. Порядок принципиален: стол объявляется свободным только после успешной записи
итога. Таймер к этому моменту уже снят — `restartTurnClock` вызывается раньше и на
`state().isOver()` делает `clock.cancel`.

---

## 3. Часы хода

### 3.1. Устройство `TurnClock`

Один общий `ScheduledExecutorService` на **один** поток (`turn-clock`, daemon) на все столы.
Три карты, ключ везде — `tableId`:

- `pending: UUID → Pending(future, onExpiry)` — идущий таймер хода;
- `paused: UUID → Paused(remaining, onExpiry)` — остановленный таймер и его остаток;
- `aborts: UUID → ScheduledFuture` — таймер отмены матча.

| Метод | Что делает |
|---|---|
| `start(table, timeout, onExpiry)` | `cancel(table)` (снимает и `pending`, и `paused`), затем ставит новый |
| `pause(table)` | снимает `pending`, кладёт остаток в `paused`, **возвращает остаток** |
| `resume(table)` | берёт `paused` и планирует **остаток**; нет паузы — no-op |
| `cancel(table)` | снимает `pending` и стирает `paused` |
| `remaining(table)` | остаток по `pending`; пусто, если часов нет или матч на паузе |
| `scheduleAbort/cancelAbort` | независимый таймер отмены |

⭐ Остаток снимается **с самого задания** (`future.getDelay(MILLISECONDS)`), а не считается по
своей копии времени — иначе два счётчика одного и того же разъедутся, и клиент увидит не то,
по чему сервер реально сходит.

⭐ Срабатывание таймера сначала делает `pending.remove(tableId)`, затем `runSafely(onExpiry)`.
`onExpiry` в `GameCommandHandler` — это `() -> runtime.submit(() -> applyTimeout(...))`,
то есть решение принимается на потоке стола, а не на `turn-clock`.

### 3.2. Кто на часах — `TimeoutPolicy.seatOnTheClock(deal)`

| Фаза | Место на часах |
|---|---|
| `ATTACK`, `TAKING` | `deal.attackRightSeat()` |
| `DEFEND` | `deal.defenderSeat()` |
| `DICE` | `hiddenTrumpAwaitingSuit().chooserSeat()`, иначе `attackRightSeat()` |
| `HANGING` | первое место текущей ступени окна, которого нет в `decided()` |
| `DEALING`, `REFILL`, `DEAL_OVER` | пусто — часы не нужны |

### 3.3. Что происходит по истечении — `TimeoutPolicy.autoActionFor(deal)`

⭐ Всегда самое безобидное действие; движок никогда не решает за человека, какой картой ходить.

| Фаза | Автодействие |
|---|---|
| `ATTACK`, `TAKING` | `Pass(seat)` |
| `DEFEND` | `Take(seat)`, но если `deal.table().isEmpty()` — `Pass(seat)` (правило пустого стола) |
| `HANGING` | `HangSkip(seat)` |
| `DICE` | `ChooseTrump(seat, richestSuit)` — единственное исключение: выбора по сути нет, и пассивность не должна лишать права (ADR-030) |

`richestSuit` = масть, которой у игрока больше всего среди `PipCard`. Тай-брейк:
`max` по количеству, затем по `ordinal` масти в **обратном** порядке — то есть при равенстве
побеждает масть с меньшим ординалом, `DIAMONDS` (порядок enum `Suit`: DIAMONDS, HEARTS, SPADES,
CLUBS). Если пиковых карт на руках нет вовсе (одни джокеры) — `Suit.values()[0]`, тоже
`DIAMONDS`.

### 3.4. Перезапуск часов — `GameCommandHandler.restartTurnClock`

Вызывается после каждого применённого хода (и после `MATCH_START`, и после автохода):

```
если state().isOver()            → clock.cancel, выход
если seatOnTheClock пусто        → clock.cancel, выход
callToTable(runtime, session, seat)          // push «твой ход», если игрока нет за столом
если !properties.autoMoveOnTimeout()  → clock.cancel, выход
clock.start(tableId, turnTimeout, () -> runtime.submit(() -> applyTimeout(...)))
```

Порядок здесь наблюдаем: **push «твой ход» уходит независимо от того, включены ли часы.**

`callToTable` считает «нет за столом» по `runtime.subscribers().contains(userId)` — по подписке
на события стола, а не по наличию сокета вообще: игрок мог открыть приложение и уйти в историю.
Сам push уходит через `TurnNotifier` с окном тишины `bardak.push.quiet-for` (по умолчанию 2m),
персональным на игрока.

### 3.5. Применение автохода — `applyTimeout`

```
autoActionFor(state.deal()) → если пусто, ничего не делаем
session.apply(auto)
если Applied:
    matchLog.append(...)  → lastSeq
    matchLog.dealsPlayed(...)
    recordFinishedDeals(...)
    saveSnapshot(...)
    broadcast TURN_TIMEOUT {seatNo}   ← всем подписчикам стола
    broadcast(события + персональные STATE_SYNC)  ← только игрокам матча
    restartTurnClock(...)
    finishIfOver(...)
```

⚠️ `TURN_TIMEOUT` уходит **до** событий и снимка. Команда с `commandId` тут не участвует —
автоход в `appliedCommands` не попадает.

---

## 4. ⚠️ Настройка автохода по таймауту

**Ключ:** `bardak.game.auto-move-on-timeout`, переменная окружения `BARDAK_AUTO_MOVE`.
**Значение по умолчанию: `false`** — и в `application.yml` (`${BARDAK_AUTO_MOVE:false}`),
и в `GameProperties.defaults()`.

```yaml
bardak:
  game:
    turn-timeout: 30s
    auto-move-on-timeout: ${BARDAK_AUTO_MOVE:false}
    disconnect-grace: 60s
```

`GameProperties` — record с `@ConfigurationProperties(prefix = "bardak.game")`, регистрируется
через `@EnableConfigurationProperties` в `auth/SecurityConfig.java`. Компактный конструктор
подставляет `30s` / `60s`, если значение не задано; у `boolean` такой защиты нет — отсутствие
ключа даёт `false` по правилам биндинга.

### Что происходит, когда автоход ВЫКЛЮЧЕН (значение по умолчанию)

1. `restartTurnClock` всё равно вызывает `callToTable` — push «твой ход» отсутствующему игроку
   уходит.
2. Затем `clock.cancel(tableId)` — **таймер хода не ставится вообще**.
3. `clock.remaining(tableId)` пусто → в `STATE_SYNC` поле `turnSecondsLeft` = `null`
   (`@JsonInclude(NON_NULL)` в Jackson-конфиге, `default-property-inclusion: non_null`) →
   клиенту нечего показывать в обратном отсчёте.
4. `TURN_TIMEOUT` не приходит никогда. Ход ждёт своего хозяина сколько угодно.

Это зафиксировано двумя интеграционными тестами: `TurnHoldsIT` (поведение по умолчанию —
`TURN_TIMEOUT` не приходит за 4 с при `turn-timeout=1s`, `turnSecondsLeft` отсутствует) и
`TimersIT` (тот же сервер с `auto-move-on-timeout=true` — `TURN_TIMEOUT` обязан прийти).
Обоснование — ADR-059: «игра для своих, отобранный ход раздражает сильнее, чем сосед,
отошедший за чаем».

⚠️ **Важно для Go:** выключенный автоход отключает часы **целиком**, а не только автодействие.
Соблазн «таймер тикает, но по истечении ничего не делаем» ломает наблюдаемое поведение —
клиент увидит обратный отсчёт там, где его быть не должно.

⚠️ Пауза при обрыве связи (`disconnect-grace`) от этой настройки **не зависит** и работает
всегда. С выключенным автоходом `clock.pause()` просто вернёт `Duration.ZERO`, и в
`MATCH_PAUSED` уедет `turnMillisLeft: 0`.

---

## 5. Пауза и отмена матча при обрыве связи

### 5.1. Пороги

| Что | Ключ | По умолчанию |
|---|---|---|
| Таймаут хода | `bardak.game.turn-timeout` | **30 s** |
| Окно возврата до отмены матча | `bardak.game.disconnect-grace` | **60 s** |
| Окно тишины push-уведомлений | `bardak.push.quiet-for` | 2 m |

### 5.2. Обрыв — `GameCommandHandler.onDisconnect(tableId, userId)`

Вызывается из `EchoWebSocketHandler.afterConnectionClosed`, если для сессии известен
`tableId` (он запоминается в `sessionTables` при первой же команде с непустым `tableId`).

```
matches.find(tableId)                  ← ⚠️ на потоке WS, не на потоке стола
если seatOf(userId) пусто → выход      ← за наблюдателя матч не встаёт
registry.find(tableId) → runtime.submit(...):
    left = clock.pause(tableId)
    broadcast MATCH_PAUSED {userId, turnMillisLeft, graceSeconds}
    turnNotifier.pausedFor(userId, tableName, tableId, graceSeconds)   ← push пропавшему
    clock.scheduleAbort(tableId, disconnectGrace, () -> runtime.submit(abort(...)))
```

`MATCH_PAUSED` уходит **всем подписчикам стола** (`runtime.broadcast`), включая, как правило,
самого пропавшего — его слушатель к этому моменту ещё не снят (см. §5.5).

Окно тишины к `pausedFor` **не применяется**: «пауза случается редко, а цена молчания —
отменённый матч у всех за столом».

### 5.3. Возврат — `resumeAfterReconnect`

Вызывается из обработки `RESYNC` и `STATE_REQUEST`, если отправитель сидит за столом:

```
clock.cancelAbort(tableId)
turnNotifier.present(userId)              ← сбрасывает окно тишины push
clock.resume(tableId)                     ← продолжает с ОСТАТКА; нет паузы → no-op
broadcast MATCH_RESUMED {}                ← всем подписчикам, payload пустой объект
```

⭐ Ключевое свойство: таймер **приостанавливается, а не перезапускается**. Игрок, у которого
оставалось три секунды, получает свои три секунды, а не полные тридцать. Пиновано юнит-тестом
`TurnClockTest.shouldKeepTheRemainderWhenTheTimerIsPausedAndResumed`.

### 5.4. Не вернулся — `abort`

```
clock.cancel(tableId)
matchLog.abort(matchId, "Игрок не вернулся за отведённое время")
matches.finish(tableId)
lobby.finishMatch(tableId)                ← ⚠️ обязательно, см. ниже
broadcast MATCH_ABORTED {userId}          ← поля reason НЕТ
```

⚠️ `lobby.finishMatch` в ветке отмены — исправленная регрессия, отмеченная и в коде,
и в `TimersIT`: без неё отменённый матч оставлял стол в статусе `IN_MATCH` навсегда — сесть
за него было нельзя, начать новый нельзя, и лобби до конца дней показывало «матч идёт».

Рейтинг при отмене **не трогается** — `results.finishMatch` вызывается только из `finishIfOver`.

Игроки при отмене со стула **не встают**: `lobby.leave` тут не вызывается (в отличие от
добровольного выхода), подписки остаются.

### 5.5. Добровольный выход — команда `MATCH_LEAVE`

Отличается от обрыва тем, что ждать некого: матч отменяется **сразу**, без паузы и без окна.

```
если seatOf(userId) пусто → ApiException NOT_AT_TABLE (409)
clock.cancel + clock.cancelAbort
matchLog.abort(matchId, "Игрок вышел из матча")
matches.finish(tableId); lobby.finishMatch(tableId)
broadcast MATCH_ABORTED {userId, reason: "PLAYER_LEFT"}
lobby.leave(tableId, userId)              ← ушедший освобождает место
registry.unsubscribe(tableId, userId)     ← и может выгрузить стол, если остался один
```

Тихо освободить своё место нельзя: «движок продолжал бы ждать ушедшего, а на освободившийся
стул сел бы посторонний».

### 5.6. Порядок при закрытии сокета

`EchoWebSocketHandler.afterConnectionClosed`:

```
sessions.remove(sessionId)
presence.disconnected(userId, sessionId)
tableId = sessionTables.remove(sessionId)
если tableId != null:
    gameCommands.onDisconnect(tableId, userId)     ← ставит задачу «пауза»
    lobbyCommands.onDisconnect(tableId, userId)    ← ставит задачу «отписать + PLAYER_OFFLINE»
```

Обе задачи попадают в очередь стола именно в этом порядке. Вторая делает
`registry.unsubscribe` и затем шлёт `PLAYER_OFFLINE` всем оставшимся.

---

## 6. Подписка соединений и две вкладки

### 6.1. Где происходит подписка

`runtime.subscribe(userId, sender)` вызывается на потоке стола в:

- `TABLE_JOIN` (лобби);
- `MATCH_START`, `RESYNC`, `STATE_REQUEST` (игра).

Обычные игровые команды (`PLAY_CARD`, `PASS`, …) **не подписывают** — клиент, приславший ход,
не подписавшись, получит только персональный ответ на свою команду, но не рассылку.

`sender` — это лямбда `event -> sendRaw(out, event)`, замкнутая на конкретный
`ConcurrentWebSocketSessionDecorator` конкретной сессии.

### 6.2. Отписка

- `TABLE_LEAVE` и `MATCH_LEAVE` — явно;
- обрыв сокета — через `LobbyCommandHandler.onDisconnect`;
- выгрузка стола (`TableRuntime.close`) чистит карту целиком.

### 6.3. ⚠️ Две вкладки одного игрока

`listeners` ключуется по `userId`, поэтому:

- вторая вкладка **вытесняет** первую: `listeners.put(userId, ...)` заменяет слушателя, и
  первая вкладка перестаёт получать события стола, оставаясь при этом открытой и «онлайн»;
- javadoc `subscribe` это и декларирует: «Один игрок — одна подписка: при переподключении новая
  заменяет старую, и в мёртвый сокет уже никто не пишет»;
- **но** закрытие первой (старой) вкладки вызывает `afterConnectionClosed` → `onDisconnect`
  с тем же `userId` → матч встаёт на паузу и снимается подписка **живой** второй вкладки.
  Оба обработчика работают с `userId`, номер сессии в них не передаётся.

Контраст: `social/Presence` устроен иначе — `Map<UUID, Map<sessionId, sender>>`, набор сессий
на игрока, и «онлайн заканчивается на последнем сокете». То есть в присутствии
многовкладочность учтена, в подписке на стол — нет.

---

## 7. Возврат игрока: `RESYNC` и `STATE_REQUEST`

Оба типа обрабатываются до проверки дедупликации и оба подписывают отправителя заново.

### 7.1. `RESYNC {lastSeq}`

```
runtime.subscribe(userId, sender)
seat = seatOf(userId) или -1
lastSeq = payload.lastSeq (0, если payload пуст)
если seat >= 0:
    для каждого события из matchLog.since(matchId, lastSeq, seat):
        отправителю конверт {v, id: null, type: событие, tableId, seq, ts: now, payload}
отправителю STATE_SYNC (полный персональный снимок)
если seat >= 0: resumeAfterReconnect(...)
```

⭐ `matchLog.since` фильтрует по видимости на уровне репозитория
(`event.isVisibleTo(seatNo)`): сырой лог содержит скрытую информацию и наружу не отдаётся
никогда. `ResyncIT` пиновал и обратный случай — отклонённая попытка (`MOVE_REJECTED`) видна
только своему автору, соседу при догоне не приходит.

⚠️ Ограничения по объёму догона нет: `lastSeq = 0` пришлёт **весь лог матча**, отфильтрованный
по месту. Ветки «дыра большая → только снимок» из `05-api-contracts.md` в коде нет.

⚠️ Догнанные события отправляются **напрямую в `sender`**, а не через `runtime.sendTo` — то есть
именно в тот сокет, который прислал `RESYNC`, даже если подписка уже принадлежит другому.

### 7.2. `STATE_REQUEST`

То же, но без досылки событий: подписка → один `STATE_SYNC` → `resumeAfterReconnect`,
если отправитель сидит за столом.

### 7.3. Дедупликация повторно отправленных команд

`MatchSession.appliedCommands` — `LinkedHashSet<String>` на последние
`REMEMBERED_COMMANDS = 200` идентификаторов, FIFO-вытеснение.

```java
if (session.alreadyApplied(command.id())) {
    sender.accept(serialize(stateSync(session, userId)));   // повтор не применяем,
    return;                                                 // но состояние отдаём
}
```

Тонкости, которые надо повторить точно:

- `commandId == null` **не запоминается** и потому никогда не дедуплицируется;
- `remember(command.id())` вызывается **сразу после `session.apply`, до разбора вердикта**, то
  есть отклонённая команда тоже «сгорает»: повторная отправка с тем же `id` вернёт `STATE_SYNC`,
  а не повторное `ERROR`;
- набор живёт в памяти сессии и **не переживает рестарт** — после подъёма из снимка
  переотправленная команда применится второй раз (лог/снимок дедупликацию не восстанавливают);
- `MATCH_START`, `MATCH_LEAVE`, `RESYNC`, `STATE_REQUEST` обрабатываются раньше проверки и
  не дедуплицируются вовсе.

### 7.4. Нумерация событий

Один сквозной `seq` по матчу (не по раздаче). В `broadcast`:

```java
final int firstSeq = session.lastSeq() - events.size() + 1;
```

Счётчик инкрементируется по **всем** событиям, а отправляются только видимые данному игроку.
⚠️ Значит **дыры в `seq` у клиента — норма**, а не сигнал потери: приватное событие соседа
съедает номер. Клиент, который на каждую дыру шлёт `RESYNC`, будет ресинкаться постоянно.
В `05-api-contracts.md` записано обратное («запрашивать RESYNC при обнаружении дыры») — при
переносе это надо учитывать как известное расхождение.

`STATE_SYNC`, `ERROR`, `MATCH_*`, `TURN_TIMEOUT` идут через `Envelope.event(...)` и `seq`
**не несут вообще** (`null`, вырезается `NON_NULL`).

---

## 8. Ловушки, которые легко воспроизвести неверно

### 8.1. Автоход может сходить «не за того»

`TurnClock` при срабатывании удаляет запись из `pending` **до** запуска `onExpiry`, а сам
`onExpiry` только ставит задачу в очередь стола. Если игрок успел сходить в последнюю секунду,
его команда уже в очереди раньше — она применится, `restartTurnClock` вызовет `clock.start`,
но `cancel` внутри уже не догонит **улетевшую** задачу таймаута. `applyTimeout` затем берёт
`TimeoutPolicy.autoActionFor(session.state().deal())` от **текущего** состояния и сходит за
того, кто на часах теперь — то есть за игрока, у которого ход только начался.

Ни номера места, ни поколения таймера в задаче нет — проверить, «тот ли это таймаут»,
невозможно. В Go это лечится токеном поколения (epoch) или тем, что таймер отменяется через
`context`/канал, читаемый на той же горутине стола. Наблюдаемое свойство «за игрока,
у которого ход только начался, сервер не ходит» в Java формально не гарантировано — при
переносе стоит его гарантировать, а не воспроизводить гонку.

### 8.2. `onDisconnect` читает матч с потока WS

`GameCommandHandler.onDisconnect` вызывает `matches.find(tableId)` и `session.seatOf(userId)`
**до** `runtime.submit`, то есть на потоке закрытия сокета. `MatchSession` объявлен
непотокобезопасным. Более того, `find` может пойти в ветку `restore(...)` — чтение из БД
и `sessions.put` — параллельно с потоком стола, который делает то же самое. Тогда возникнут
два разных `MatchSession` на один стол, и работа одного из них потеряется.
В Go: весь `onDisconnect` целиком должен идти в очередь стола.

### 8.3. Отмена матча может не наступить

`scheduleAbort` замыкается на конкретный `runtime`. Если после паузы стол опустеет
(`registry.unsubscribe` → `close()` → `queue.shutdownNow()`), сработавший таймер вызовет
`runtime.submit` на закрытой очереди → `RejectedExecutionException` → он поглощается
`TurnClock.runSafely` и превращается в строчку «Таймер стола {} упал» в логе. Матч останется
в `MatchService.sessions`, стол — в статусе `IN_MATCH`, отмена не произойдёт.

Ровно такой сценарий даёт стол на двоих, где после обрыва одного второй тоже уходит.

### 8.4. `TurnClock` ничего не знает о выгрузке стола

`TableRegistry.unsubscribe` не трогает `TurnClock`. Записи в `pending`/`paused`/`aborts`
остаются висеть до срабатывания (и, как в §8.3, срабатывают в пустоту). Явного
`clock.cancel(tableId)` при выгрузке стола в коде **не найдено**.

### 8.5. «Пауза» — только уведомление, а не блокировка

`MATCH_PAUSED` останавливает таймер и сообщает клиентам, но **не запрещает команды**.
Оставшиеся за столом игроки могут продолжать ходить: `execute` никакого флага паузы
не проверяет — такого флага в коде нет вообще. Первый же применённый ход вызовет
`restartTurnClock`, который либо `clock.cancel` (автоход выключен), либо `clock.start`
(включён) — и в обоих случаях **сохранённый остаток из `paused` будет стёрт**. Вернувшийся
игрок получит либо полный ход, либо часы без остатка.

### 8.6. `resumeAfterReconnect` вызывает кто угодно из-за стола

`STATE_REQUEST` или `RESYNC` от **любого** сидящего игрока — не обязательно от пропавшего —
делает `clock.cancelAbort` и рассылает `MATCH_RESUMED`. Достаточно соседу обновить вкладку,
и окно ожидания пропавшего сбрасывается: матч больше не будет отменён по невозврату, пока
кто-то не отвалится снова. Соответствия «кого ждали ↔ кто вернулся» в коде нет.

### 8.7. `MATCH_START` не подписывает остальных

`MATCH_START` подписывает только своего отправителя. Остальные должны быть подписаны раньше —
через `TABLE_JOIN`. Игрок, вошедший за стол по REST (если такой путь появится), стартового
`STATE_SYNC` не увидит — `broadcast` шлёт `runtime.sendTo(player, ...)`, а без слушателя это
тихий no-op.

### 8.8. `broadcast` — два разных смысла

- `TableRuntime.broadcast(message)` — **всем подписчикам** стола, включая наблюдателей
  (`MATCH_PAUSED`, `MATCH_RESUMED`, `MATCH_ABORTED`, `MATCH_OVER`, `TURN_TIMEOUT`,
  `PLAYER_*` из лобби);
- `GameCommandHandler.broadcast(runtime, session, events)` — **только `session.players()`**,
  каждому своё: сперва видимые ему события (по `event.privateToSeat()`), потом персональный
  `STATE_SYNC`. Наблюдатель, не сидящий за столом, снимков состояния не получает вовсе.

Порядок «сначала события, потом снимок» обязателен: «иначе клиент увидит новое состояние
раньше причины и не сможет его анимировать».

### 8.9. `RulesConfigCodec` глотает любую ошибку

Неразобранный `rules_config` не роняет матч, а тихо даёт `RulesConfig.defaults()` с `WARN`
в логе. Неизвестный ранг в шкале навесов бросает `IllegalArgumentException` — но он ловится
тем же `catch (RuntimeException ...)`, и результат тот же: играем по умолчанию. То же самое
происходит и при **восстановлении** матча из снимка, где `rules_config` берётся из текущего
состояния стола — при подъёме из снимка правила читаются из `table.rulesConfig()`, а не из
`rules_snapshot` матча (⚠️ расхождение с ADR-016, см. §10).

---

## 9. Что в Go придётся сделать иначе

| Java | Go | Что обязано сохраниться |
|---|---|---|
| `newSingleThreadExecutor` на стол | горутина на стол + канал команд | строгий FIFO для одного стола; движок остаётся однопоточным и без мьютексов |
| `ExecutorService.execute` | `select { case ch <- cmd: ... }` | порядок постановки = порядок выполнения; решить, что делать при переполнении (в Java очередь безграничная) |
| `queue.shutdownNow()` | `close(done)` / `cancel()` контекста стола | горутина завершается, ожидающие таймеры не пишут в мёртвый канал (в Java это `RejectedExecutionException`, см. §8.3) |
| `ScheduledExecutorService` + `ScheduledFuture.cancel(false)` | `time.Timer`/`time.AfterFunc` + `Stop()`, либо `time.After` в `select` горутины стола | «сработало» = «положили команду в очередь стола», а не «сделали ход из таймера» |
| `future.getDelay(MILLISECONDS)` для остатка | `deadline.Sub(time.Now())`, дедлайн хранится явно | остаток берётся из одного источника; двух счётчиков одного и того же быть не должно |
| `pause`/`resume` через сохранённый `Paused(remaining, onExpiry)` | сохранить `remaining` и пересоздать таймер | пауза **останавливает**, а не перезапускает: вернувшийся получает свой остаток |
| прерывание потока (`shutdownNow` ставит флаг) | `context.Context` с отменой | отмена не должна прерывать уже начатую запись в БД посреди «лог → снимок → рассылка» |
| `Consumer<String>` прямо в `WebSocketSession.sendMessage` (блокирующий I/O на потоке стола) | буферизованный канал на соединение + отдельная writer-горутина | одна медленная/мёртвая связь не рушит рассылку остальным (`deliver` в Java ловит исключение и продолжает) и не тормозит стол; выбрать явную политику при переполнении буфера (в Java — лимиты 512 КиБ / 10 с и закрытие сессии) |
| `ConcurrentHashMap<UUID, TableRuntime>` + `computeIfAbsent` | `sync.Map` или мьютекс + карта, либо горутина-реестр | стол поднимается по первой команде и выгружается на последнем отписавшемся — атомарно с проверкой «нет слушателей» |
| `ConcurrentHashMap<UUID, Consumer>` слушателей | карта под тем же мьютексом или владение горутиной стола | один слушатель на игрока, новый вытесняет старого (или — лучше — набор сессий, см. §6.3) |
| `try/catch RuntimeException` вокруг задачи стола | `defer recover()` в горутине стола | паника одной команды не убивает стол и не останавливает очередь |
| `LinkedHashSet` последних 200 `commandId` | срез + карта или маленький ring buffer | дедупликация с FIFO-вытеснением, ровно 200; `nil`/пустой id не дедуплицируется |
| `int lastSeq` без синхронизации | обычное поле структуры стола | доступ только из горутины стола |
| `SecureRandom.nextLong()` для seed матча | `crypto/rand` | seed берётся один раз при старте, дальше всё детерминировано |

### Наблюдаемые свойства, которые обязаны выжить

1. Команды одного стола применяются строго по одной и в порядке прихода; два хода никогда
   не пересекаются.
2. После каждого применённого хода каждый игрок получает **свои** события (отфильтрованные
   по `privateToSeat`) и **свой** `STATE_SYNC`, именно в этом порядке.
3. Запись в лог происходит **до** рассылки; снимок сохраняется после каждого применённого хода.
4. `seq` сквозной по матчу, дыры у клиента легальны.
5. Повтор команды с тем же `id` не применяется второй раз, но клиент получает актуальный
   `STATE_SYNC`.
6. При выключенном `auto-move-on-timeout` (по умолчанию) часы не идут вообще, `turnSecondsLeft`
   отсутствует, `TURN_TIMEOUT` не приходит; ход ждёт своего хозяина неограниченно.
7. При включённом — по истечении `turn-timeout` сервер делает **самое безобидное** действие
   (пас / взял / пропуск навеса / выбор козыря по кости), рассылает `TURN_TIMEOUT {seatNo}`,
   а затем обычную пару «события + снимок».
8. Обрыв связи сидящего игрока: таймер хода **останавливается** (не сбрасывается),
   всем `MATCH_PAUSED {userId, turnMillisLeft, graceSeconds}`, пропавшему — push.
9. Возврат в течение `disconnect-grace`: `MATCH_RESUMED`, отсчёт продолжается **с остатка**.
10. Невозврат: `MATCH_ABORTED {userId}`, матч выгружается, **стол возвращается в `WAITING`**,
    рейтинг не пересчитывается.
11. Добровольный `MATCH_LEAVE`: отмена немедленная, `MATCH_ABORTED {userId, reason:"PLAYER_LEFT"}`,
    ушедший встаёт с места.
12. Конец матча: сначала транзакция «итог + рейтинг», только потом стол объявляется свободным
    и уходит `MATCH_OVER`.
13. Матч живёт в памяти и поднимается из снимка при первой же команде после рестарта; места
    берутся из `match_players`, а не из лобби.
14. Стол в реестре поднимается по первой команде и выгружается, когда ушёл последний
    подписчик; матч при этом не умирает.

---

## 10. Расхождения кода с проектной документацией

| Тема | `planning/*` | Код |
|---|---|---|
| Payload `MATCH_PAUSED` | `{reason, waitingForSeat, resumeDeadlineTs}` (05, стр. 308, 492) | `{userId, turnMillisLeft, graceSeconds}` |
| Payload `MATCH_ABORTED` | `{reason}` (05, стр. 310, 496) | при невозврате — `{userId}` **без `reason`**; при `MATCH_LEAVE` — `{userId, reason:"PLAYER_LEFT"}` |
| Payload `MATCH_OVER` | `{losers, mainLoserSeat, places, navesLevels, ratingChanges}` | `{matchId, dealsPlayed, players[{userId, seatNo, displayName, place, navesLevel, lossDegree, ratingBefore, ratingAfter, ratingDelta}], lastAttackCards[]}` |
| Событие `ACK {commandId, accepted}` | описано (05, таблица событий) | **не найдено в коде** — не отправляется никогда |
| Событие `TURN_TIMEOUT` | в таблице событий отсутствует | отправляется всем подписчикам как `{seatNo}` |
| Команда `ROLL_DICE` | описана (05) | **не найдено в коде**: `GameProtocol.toCommand` её не знает, в `COMMANDS` её нет; кость бросается сервером, наружу торчит только `CHOOSE_TRUMP` (ADR-030) |
| Команда `HANG_CLAIM` | описана (05) | **не найдено в коде** |
| Команды `MATCH_START`, `STATE_REQUEST`, `MATCH_LEAVE`, `REVEAL_FACE_DOWN` | в таблице команд отсутствуют | есть в `COMMANDS` и обрабатываются |
| `RESYNC`: «дыра большая → полный `STATE_SYNC` вместо событий» | 05, стр. 508–511 | ветвления нет: сначала **все** события с `seq > lastSeq`, потом снимок всегда |
| «Клиент обязан запрашивать `RESYNC` при обнаружении дыры в `seq`» | 05, стр. 517 | дыры легальны из-за фильтрации по видимости (§7.4) |
| «Сервер, не получая активности, переводит матч в `PAUSED`» | 05, раздел Heartbeat | **не найдено в коде**: пауза наступает только по фактическому закрытию сокета; `PING`/`PONG` есть, но серверного детектора молчания нет |
| `turnTimeoutSeconds`, `disconnectGraceSeconds` живут в `rules_config` стола и копируются в `rules_snapshot` (ADR-016) | 09, ADR-016 | оба живут в `bardak.game.*` (глобальный конфиг приложения), в `RulesConfig`/`RulesConfigCodec` их **нет** |
| Восстановление матча играет по `rules_snapshot` матча | ADR-016, комментарий в `MatchSession` | `MatchService.restore` берёт правила из **текущего** `table.rulesConfig()`, а не из `rules_snapshot` записи матча |
| ADR-015: «`MATCH_ABORTED` с причиной» | 09, ADR-015 | причина пишется в БД (`matchLog.abort(matchId, "…")`), но в событие клиенту не попадает |

Отдельно: `GameProperties.defaults()` не вызывается **нигде** — ни в `main`, ни в тестах
(проверено грепом по всему репозиторию). Это чистая документация умолчаний; реально они
приходят из `application.yml` и из компактного конструктора record'а. В Go дублировать этот
мёртвый конструктор незачем — но значения `30s / false / 60s` должны совпасть.
