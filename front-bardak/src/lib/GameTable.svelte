<script>
    /**
     * Игровой стол.
     *
     * ⭐ Ни одно правило здесь не воспроизводится: что можно сделать — приходит с сервера
     * списком (ADR-003). Экран только раскладывает этот список по кнопкам и подсвечивает
     * карты, которыми разрешено пойти.
     *
     * ⭐ Ход подтверждается кнопкой, а не совершается касанием карты. На телефоне промах
     * по карте стоит хода, а иногда и партии: сначала карта выбирается, потом главная
     * кнопка говорит, что именно с ней произойдёт.
     *
     * ⭐ Рука не подсказывает, чем можно пойти. Обведённой оказывалась половина карт, а
     * когда бить было нечем — гасла вся рука разом и выглядела сломанной. Подсказка
     * осталась там, где её ниоткуда не узнать: в окне навеса.
     */
    import {flip} from 'svelte/animate';
    import Card from './Card.svelte';
    import CardFlights from './CardFlights.svelte';
    import Seat from './Seat.svelte';
    import TurnClock from './TurnClock.svelte';
    import {play, table} from '../stores/table.svelte.js';
    import {connection} from '../stores/connection.svelte.js';
    import {TIMING, anchorPoint, flyFrom} from './motion.svelte.js';
    import {isRedSuit, suitGlyph} from './naming.js';

    let {onLeave = null, onMenu = null} = $props();

    const game = $derived(table.game);

    /**
     * ⚠️ Выход спрашивает подтверждение, и не из вежливости: уходящий отменяет партию
     * всем за столом. Случайное попадание по кнопке стоило бы чужой игры.
     */
    let leaveAsked = $state(false);

    const actions = $derived.by(() => {
        const list = game?.availableActions ?? [];
        return {
            attacks: list.filter((a) => a.type === 'PLAY_CARD' && !a.payload.targetCardCode),
            defends: list.filter((a) => a.type === 'PLAY_CARD' && a.payload.targetCardCode),
            transfers: list.filter((a) => a.type === 'TRANSFER'),
            hangs: list.filter((a) => a.type === 'HANG_CARD'),
            pass: list.find((a) => a.type === 'PASS'),
            take: list.find((a) => a.type === 'TAKE'),
            hangSkip: list.find((a) => a.type === 'HANG_SKIP'),
            reveal: list.find((a) => a.type === 'REVEAL_FACE_DOWN' && !a.payload.targetCardCode),
            trumps: list.filter((a) => a.type === 'CHOOSE_TRUMP'),
        };
    });

    /**
     * Карты, которыми можно навесить, — единственная подсветка, оставшаяся в руке.
     *
     * <p>Навес случается редко и по своим правилам шкалы: какой картой навешивают, игрок
     * не выведет ни из стола, ни из руки. Всё остальное он узнаёт, нажав на карту.
     */
    const hangable = $derived(new Set(actions.hangs.map((action) => action.payload.cardCode)));

    /** Идёт окно навеса — своё или чужое. */
    const hangingNow = $derived(game?.hangingVictimSeat !== null && game?.hangingVictimSeat !== undefined);

    let selected = $state(null);

    // Выбранная карта могла уйти из руки — например, за нас сходил таймер.
    $effect(() => {
        if (selected && !game?.myHand.includes(selected)) {
            selected = null;
        }
    });

    const targets = $derived(
        selected ? actions.defends.filter((a) => a.payload.cardCode === selected) : []);

    /** Что произойдёт по главной кнопке. Пусто — сейчас ход не за нами. */
    const primary = $derived.by(() => {
        if (actions.trumps.length) {
            return null;   // выбор козыря — отдельный ряд кнопок, там нечего подтверждать
        }
        if (selected) {
            const hang = actions.hangs.find((a) => a.payload.cardCode === selected);
            if (hang) {
                return {label: `Навесить ${short(selected)}`, action: hang};
            }
            const attack = actions.attacks.find((a) => a.payload.cardCode === selected);
            if (attack) {
                return {label: `Атаковать ${short(selected)}`, action: attack};
            }
            const transfer = actions.transfers.find((a) => a.payload.cardCode === selected);
            if (transfer) {
                return {label: `Перевести ${short(selected)}`, action: transfer, tone: 'blue'};
            }
            if (targets.length === 1) {
                return {label: `Отбиться ${short(selected)}`, action: targets[0]};
            }
            return null;   // целей несколько — пусть укажет, какую карту бьёт
        }
        if (actions.reveal) {
            return {label: 'Вскрыть скрытую', action: actions.reveal};
        }
        return null;
    });

    const myTurn = $derived((game?.availableActions ?? []).length > 0);
    const unbeaten = $derived((game?.table ?? []).filter((slot) => !slot.defend));
    const iDefend = $derived(game?.defenderSeat === game?.mySeat);

    /**
     * ⭐ «Беру» имеет смысл, только пока на столе есть неотбитое. Всё отбито — забирать
     * нечего и незачем: игрок унёс бы в руку и свою же защиту. Кнопка при этом остаётся
     * (правила её не запрещают), но перестаёт быть красной и звать нажать.
     */
    const takeMatters = $derived(unbeaten.length > 0);

    /** Карта выбрана, а сделать ею нечего — это надо сказать словами, а не молчать. */
    const selectedIsDead = $derived.by(() => {
        if (!selected || actions.trumps.length) {
            return false;
        }
        const usable = [actions.attacks, actions.transfers, actions.hangs]
            .some((list) => list.some((action) => action.payload.cardCode === selected));
        return !usable && targets.length === 0;
    });

    const prompt = $derived.by(() => {
        if (actions.trumps.length) {
            return 'Назови козырь';
        }
        if (hangingNow) {
            return game.hangingVictimSeat === game.mySeat ? 'Тебе навешивают' : 'Твой навес';
        }
        if (selected && targets.length > 1) {
            return 'Укажи, какую карту бьёшь';
        }
        if (iDefend && unbeaten.length === 1) {
            // ⭐ Карта названа прямо в подсказке: при одной неотбитой искать её глазами
            // на столе незачем, а на телефоне это ещё и лишнее движение.
            return `Отбей ${short(unbeaten[0].attack)}`;
        }
        if (iDefend && unbeaten.length) {
            return 'Отбивайся';
        }
        if (iDefend && game.table.length) {
            return 'Всё отбито — ход за соперником';
        }
        // ⚠️ Право подкидывать остаётся за спасовавшим до конца раунда: без проверки
        // на реальные ходы экран звал подкинуть того, кому нечем и некуда.
        if (game.canAttackSeat === game.mySeat && myTurn) {
            return game.table.length ? 'Подкидывай или пасуй' : 'Твоя атака';
        }
        return null;
    });

    /**
     * Соперники слева направо — по часовой стрелке от меня.
     *
     * ⭐ Я всегда внизу, поэтому следующий по ходу сосед сидит слева, а не справа: если
     * смотреть на стол как на циферблат, движение от шести часов по часовой идёт сначала
     * влево и вверх. Так порядок мест на экране совпадает с порядком хода в движке
     * ({@code nextActiveSeatAfter} — это следующий номер места по кругу).
     *
     * ⚠️ Раньше места шли просто по возрастанию номера. При моём месте 0 это совпадало
     * с правдой случайно, а на любом другом — расходилось: сосед, которому я передаю ход,
     * оказывался не там, где его ищут глазами.
     */
    const opponents = $derived.by(() => {
        const players = game?.players ?? [];
        const count = players.length;
        return players
            .filter((seat) => seat.seatNo !== game.mySeat)
            .sort((left, right) => clockwise(left.seatNo, count) - clockwise(right.seatNo, count));
    });

    function clockwise(seatNo, count) {
        return (seatNo - game.mySeat + count) % count;
    }
    const me = $derived((game?.players ?? []).find((seat) => seat.seatNo === game.mySeat));

    /** Размер аватара по числу соперников — по макетам составов 2→5. */
    const SEAT_SIZE = {1: 62, 2: 56, 3: 50, 4: 46};

    const seatSize = $derived(SEAT_SIZE[opponents.length] ?? 50);

    /** В колоде осталась одна карта — та самая, что сменит козырь всему столу (§1.9). */
    const lastIsHiddenTrump = $derived(game?.deckLeft === 1);

    /** Подпись кнопки навеса у жертвы. Пусто — навешивать сейчас нечем или некому. */
    function hangCtaFor(seat) {
        if (game.hangingVictimSeat !== seat.seatNo || !actions.hangs.length) {
            return null;
        }
        return `Навесить ${seat.nextIsJoker ? '🃏' : seat.nextNavesRank ?? ''}`.trim();
    }

    /**
     * ⭐ Кнопка у жертвы навешивает сразу, а не выбирает карту.
     *
     * Подтверждать нечего: и карта, и жертва названы прямо на кнопке, а других вариантов
     * навеса в этот момент не бывает. Раньше она лишь подсвечивала карту в руке, и ход
     * надо было добить нижней кнопкой — с большой рукой её уносило за край экрана,
     * и нажатие выглядело так, будто кнопка не работает.
     */
    function takeHangCard() {
        const hang = actions.hangs[0];
        if (hang) {
            run(hang);
        }
    }

    /**
     * ⚠️ Веер сжимается под ширину экрана.
     *
     * Забравший стол легко держит полтора десятка карт, а веер из восемнадцати штук с
     * постоянным нахлёстом шире любого экрана: крайние карты уезжают за край вместе с
     * возможностью ими пойти. Нахлёст считается так, чтобы рука всегда помещалась целиком.
     */
    let handWidth = $state(0);

    const cardWidth = $derived(handWidth >= 900 ? 96 : 70);

    const overlap = $derived.by(() => {
        const count = game?.myHand.length ?? 0;
        // Нахлёст «по умолчанию»: с такой рукой веер выглядит веером, а не стопкой.
        const cosy = cardWidth >= 96 ? 34 : 26;
        if (count < 2 || handWidth === 0) {
            return cosy;
        }
        // 16 — горизонтальные отступы самой руки, они в ширину веера не входят.
        const fits = (handWidth - 16 - cardWidth) / (count - 1);
        // ⚠️ Берём БОЛЬШИЙ из двух: уютный нахлёст — это минимум, а не потолок. Ограничив
        // его сверху, я оставил восемнадцать карт шире экрана — ровно ту поломку, из-за
        // которой до крайней карты было не дотянуться.
        // Полоска в 12px — это ровно угол с номиналом и мастью: меньше уже не карта.
        return Math.min(cardWidth - 12, Math.max(cosy, Math.ceil(cardWidth - fits)));
    });

    /** Короткая запись карты для кнопки: «6♣» вместо «6-clubs». */
    function short(code) {
        if (!code) {
            return '';
        }
        if (code.startsWith('Joker')) {
            return '🃏';
        }
        const [rank, suit] = code.split('-');
        const glyph = {diamonds: '♦', hearts: '♥', spades: '♠', clubs: '♣'}[suit] ?? '';
        return rank + glyph;
    }

    /**
     * ⭐ Выбрать можно любую карту, даже негодную. Запрет на нажатие раньше означал, что
     * карта молча не реагирует, и отличить «нельзя» от «не попал» было невозможно.
     * Теперь карта поднимается всегда, а кнопка внизу объясняет, что с ней будет.
     */
    function tapCard(code) {
        selected = selected === code ? null : code;
    }

    function tapTarget(attackCode) {
        const action = targets.find((a) => a.payload.targetCardCode === attackCode);
        if (action) {
            play(action);
            selected = null;
        }
    }

    function run(action) {
        play(action);
        selected = null;
    }
</script>

<div class="table-screen">
    <!--
      ⚠️ Состояние связи видно прямо за столом. Без него мёртвый сокет ничем себя не
      выдавал: экран прежний, карты на местах, а ходы уходят в никуда.
    -->
    {#if connection.status !== 'open'}
        <div class="link-state mono" class:lost={connection.status === 'unauthorized'}>
            {connection.status === 'unauthorized'
                ? 'Сессия потеряна — обнови страницу'
                : 'Связь со столом восстанавливается…'}
        </div>
    {/if}

    <div class="deal-line mono">
        <span>Раздача {game.dealNo}</span>
        <span class="sep">·</span>
        <span>козырь
            <span class="suit" class:red={isRedSuit(game.trumpSuit)}>
                {game.trumpSuit ? suitGlyph(game.trumpSuit) : '?'}
            </span>
        </span>
        {#if game.protectedSuit}
            <span class="sep">·</span>
            <span>защищена
                <span class="suit" class:red={isRedSuit(game.protectedSuit)}>{suitGlyph(game.protectedSuit)}</span>
            </span>
        {/if}

        {#if onMenu}
            <button class="leave-btn" type="button" onclick={onMenu}
                    title="В главное меню — место за столом останется за тобой">меню</button>
        {/if}

        {#if onLeave}
            <span class="leave">
                {#if leaveAsked}
                    <button class="leave-btn danger" type="button" onclick={onLeave}>
                        отменить матч
                    </button>
                    <button class="leave-btn" type="button" onclick={() => (leaveAsked = false)}>
                        играем
                    </button>
                {:else}
                    <button class="leave-btn" type="button" onclick={() => (leaveAsked = true)}
                            title="Выйти из-за стола — партия отменится у всех">выйти</button>
                {/if}
            </span>
        {/if}
    </div>

    <div class="opponents" class:tiers={opponents.length > 3} data-count={opponents.length}>
        {#each opponents as seat (seat.seatNo)}
            <Seat {seat} size={seatSize}
                  active={seat.seatNo === game.canAttackSeat}
                  defending={seat.seatNo === game.defenderSeat}
                  decision={table.decisions[seat.seatNo] ?? null}
                  taking={seat.seatNo === game.defenderSeat && game.phase === 'TAKING'}
                  hangCta={hangCtaFor(seat)} onHang={takeHangCard}/>
        {/each}
    </div>

    <div class="middle">
        <div class="deck" use:anchorPoint={'deck'}>
            {#if game.deckLeft > 0}
                <div class="deck-stack">
                    <!--
                      ⭐ Козырная карта лежит под колодой лицом вверх и берётся последней (§1.9).
                      Её показывают целиком, а не мастью: за столом козырь знают в лицо — «семёрка
                      червей», а не «черви». Самая нижняя карта — потайной козырь — по-прежнему
                      тайна для всех, её тут нет.
                    -->
                    {#if game.trumpCard}
                        <span class="trump-under">
                            <Card code={game.trumpCard} width={54}/>
                        </span>
                    {:else if game.trumpSuit}
                        <span class="trump-card" class:red={isRedSuit(game.trumpSuit)}>
                            {suitGlyph(game.trumpSuit)}
                        </span>
                    {/if}
                    <Card faceDown width={52} style="position:absolute; left:34px; top:2px"/>
                    <Card faceDown width={52} style="position:absolute; left:32px; top:0"/>
                </div>
                <div class="mono">Колода {game.deckLeft}</div>
            {:else if lastIsHiddenTrump}
                <!--
                  ⭐ Осталась одна карта — это потайной козырь (§1.9). Кто её возьмёт, тому
                  она уйдёт в руку, а козырь сменится её мастью со следующего раунда. Момент
                  редкий и переворачивающий раздачу, поэтому он объявлен, а не показан цифрой.
                -->
                <div class="trump-change">
                    <div class="tc-head mono">
                        Козырь
                        <span class="suit" class:red={isRedSuit(game.trumpSuit)}>
                            {game.trumpSuit ? suitGlyph(game.trumpSuit) : '?'}
                        </span>
                        <span class="tc-arrow">→ сменится</span>
                    </div>
                    <div class="tc-body">
                        <span class="tc-card"><Card faceDown width={52}/></span>
                        <span class="mono tc-note">потайной козырь · последний</span>
                    </div>
                </div>
            {:else}
                <div class="card-slot" style="width:52px"><span class="mono">пусто</span></div>
                <div class="mono">Колода 0</div>
            {/if}
        </div>

        <div class="board" use:anchorPoint={'board'}>
            {#if game.table.length === 0}
                <div class="card-slot gold" style="width:70px">
                    <span class="mono">брось</span><span class="mono">карту</span>
                </div>
            {:else}
                <div class="slots">
                    {#each game.table as slot (slot.attack)}
                        {@const canBeat = targets.some((a) => a.payload.targetCardCode === slot.attack)}
                        <!--
                          ⭐ Слот разъезжается плавно (animate:flip), а карта въезжает в него
                          из руки (use:flyFrom). Порядок важен: слот уже встал на новое место,
                          и карта летит именно туда, куда ляжет, а не в середину стола.
                        -->
                        <div class="slot" animate:flip={{duration: TIMING.move}}
                             use:anchorPoint={`slot-${slot.attack}`}>
                            <span use:flyFrom={{key: slot.attack}}>
                                <Card code={slot.attack} width={62} selected={canBeat}
                                      onclick={canBeat ? () => tapTarget(slot.attack) : null}/>
                            </span>
                            {#if slot.defend}
                                <span class="defence">
                                    <span use:flyFrom={{key: slot.defend}}>
                                        <Card code={slot.defend} width={62}/>
                                    </span>
                                </span>
                            {/if}
                        </div>
                    {/each}
                </div>
            {/if}

            {#if prompt}
                <div class="prompt">
                    <span class="gold">{prompt}</span>
                    <TurnClock seconds={game.turnSecondsLeft} active={myTurn}/>
                </div>
            {/if}
        </div>

        <div class="discard" use:anchorPoint={'discard'}>
            {#if game.discardCount > 0}
                <div class="pile">
                    <Card faceDown width={36} style="position:absolute; left:2px; top:4px; transform:rotate(-16deg); filter:brightness(.7)"/>
                    <Card faceDown width={36} style="position:absolute; left:10px; top:1px; transform:rotate(7deg); filter:brightness(.85)"/>
                    <Card faceDown width={36} style="position:absolute; left:6px; top:0; transform:rotate(-3deg)"/>
                </div>
            {:else}
                <div class="card-slot" style="width:40px"></div>
            {/if}
            <div class="mono">Бито {game.discardCount}</div>
        </div>
    </div>

    <div class="mine">
        <div class="my-hung" use:anchorPoint={`hung-${game.mySeat}`}>
            {#if me?.hung.length}
                <div class="hung-row">
                    {#each me.hung as code, index (code)}
                        <Card {code} width={index === me.hung.length - 1 ? 50 : 40}
                              dimmed={index !== me.hung.length - 1}
                              style={index < me.hung.length - 1 ? 'margin-right:-22px' : ''}/>
                    {/each}
                </div>
            {:else}
                <div class="card-slot" style="width:38px">
                    <span class="mono">{me?.nextIsJoker ? '🃏' : me?.nextNavesRank ?? '6'}</span>
                    <span class="flies mono">летит</span>
                </div>
            {/if}
            <!--
              ⭐ Считается не сколько навесили, а сколько осталось до джокера: шкала и есть
              счёт в игре (ADR-017), и «навесили 2» ничего не говорит о том, близко ли конец.
            -->
            <div class="mono hung-label">
                Мой навес<br>
                <span class="gold">
                    {me?.stepsToJoker ? `до джокера ${me.stepsToJoker}` : 'джокер висит'}
                </span>
            </div>
        </div>

        {#if game.iHaveHiddenCard}
            <!-- Свою скрытую карту не видит даже владелец (§1.8) — только рубашку. -->
            <span class="my-hidden"><Card faceDown width={38}/></span>
        {/if}
    </div>

    <!--
      ⭐ Веер живёт на внутреннем узле, а перестановка — на внешнем. Так `animate:flip`
      двигает карту по горизонтали, а поворот доезжает своим переходом: при добавлении
      карты соседние расходятся, а не перескакивают в новый угол.
    -->
    <div class="hand" style="--overlap:{overlap}px" use:anchorPoint={'hand'}
         bind:clientWidth={handWidth}>
        {#each game.myHand as code, index (code)}
            {@const middle = (game.myHand.length - 1) / 2}
            {@const tilt = Math.min(5, 60 / Math.max(1, game.myHand.length))}
            {@const offset = index - middle}
            <span class="hand-card" animate:flip={{duration: TIMING.move}}>
                <span class="fan" style="transform: rotate({offset * tilt}deg) translateY({Math.abs(offset) * 4}px)">
                    <span use:flyFrom={{key: code, pool: 'hand', delay: index * 40}}>
                        <Card {code} width={70} selected={selected === code}
                              playable={hangingNow && hangable.has(code)}
                              onclick={() => tapCard(code)}
                              style={selected === code ? 'transform: translateY(-18px)' : ''}/>
                    </span>
                </span>
            </span>
        {/each}
    </div>

    <div class="actions">
        {#if actions.trumps.length}
            {#each actions.trumps as action (action.payload.suit)}
                <button class="btn trump" type="button" onclick={() => run(action)}>
                    <span class="suit" class:red={isRedSuit(action.payload.suit)}>
                        {suitGlyph(action.payload.suit)}
                    </span>
                </button>
            {/each}
        {:else}
            {#if actions.take}
                <!-- Красным «Беру» зовёт только тогда, когда на столе есть что забирать. -->
                <button class="narrow" class:btn={takeMatters} class:btn-red={takeMatters}
                        class:btn-ghost={!takeMatters} type="button" onclick={() => run(actions.take)}
                        title={takeMatters ? 'Забрать стол' : 'Всё отбито — забирать нечего'}>Беру</button>
            {/if}
            {#if primary}
                <button class="btn wide" class:btn-blue={primary.tone === 'blue'} type="button"
                        onclick={() => run(primary.action)}>{primary.label}</button>
            {:else if selectedIsDead}
                <div class="waiting mono">{short(selected)} сейчас не сыграть</div>
            {:else if !myTurn}
                <div class="waiting mono">Ход соперника</div>
            {/if}
            {#if actions.pass}
                <button class="btn-ghost narrow" type="button" onclick={() => run(actions.pass)}>Пас</button>
            {/if}
            {#if actions.hangSkip}
                <button class="btn-ghost narrow" type="button" onclick={() => run(actions.hangSkip)}>Мимо</button>
            {/if}
        {/if}
    </div>
</div>

<CardFlights/>

<style>
    /* Стол занимает ровно доступную высоту и не прокручивается: всё видно сразу. */
    .table-screen {
        flex: 1 1 auto;
        display: flex;
        flex-direction: column;
        align-items: center;
        gap: 10px;
        padding: 10px 0 0;
        min-height: 0;
        overflow: hidden;
    }

    .deal-line {
        display: flex;
        align-items: center;
        gap: 8px;
        color: var(--text-55);
    }

    .sep {
        opacity: 0.4;
    }

    /* Выход живёт в строке раздачи, а не среди игровых кнопок: он не ход. */
    .leave {
        display: inline-flex;
        gap: 6px;
        margin-left: 10px;
    }

    .leave-btn {
        padding: 3px 9px;
        border-radius: 8px;
        border: 1px solid var(--line-strong);
        background: rgba(255, 255, 255, 0.05);
        font: 500 10px var(--mono);
        letter-spacing: 0.06em;
        color: var(--text-55);
    }

    .leave-btn.danger {
        border-color: rgba(232, 98, 108, 0.55);
        background: rgba(232, 98, 108, 0.16);
        color: var(--seat-attack);
    }

    .suit {
        font-size: 13px;
        color: var(--text);
    }

    .suit.red {
        color: #ff8d95;
    }

    .opponents {
        display: flex;
        justify-content: space-around;
        align-items: flex-start;
        gap: 4px;
        width: 100%;
        padding: 0 12px;
    }

    /* Ширина места по составу: чем больше соседей, тем теснее каждому (макет составов 2→5). */
    .opponents[data-count='2'] :global(.seat) {
        width: 150px;
    }

    .opponents[data-count='3'] :global(.seat) {
        width: 112px;
    }

    /**
     * ⚠️ Пятеро за столом: соперники в две ступени <b>по двое</b>.
     *
     * Одного `flex-wrap` мало — при четырёх местах по 96px трое влезали в первый ряд,
     * и стол получался «3 + 1». Ровно половина ширины на место делает ступени честными.
     */
    .tiers {
        flex-wrap: wrap;
        row-gap: 10px;
    }

    .tiers :global(.seat) {
        flex: 0 0 calc(50% - 4px);
        width: auto;
    }

    /* Верхняя пара стоит теснее нижней — стол читается как чаша, а не как таблица. */
    .tiers :global(.seat:nth-child(-n+2)) {
        padding: 0 26px;
    }

    /* ⚠️ `min-height` — пол, а не рост: середина обязана уметь сжиматься, иначе на
       невысоком окне стол выпирает за экран и уводит кнопки хода под нижний край. */
    .middle {
        flex: 1 1 auto;
        min-height: 0;
        width: 100%;
        display: grid;
        grid-template-columns: 84px 1fr 64px;
        align-items: center;
        gap: 4px;
        padding: 0 14px;
    }

    .deck, .discard {
        display: flex;
        flex-direction: column;
        align-items: center;
        gap: 8px;
    }

    .deck-stack {
        position: relative;
        width: 96px;
        height: 86px;
    }

    /**
     * Козырная карта лежит поперёк под колодой — как за настоящим столом.
     *
     * ⭐ Повёрнута так, чтобы наружу торчал именно угол с номиналом и мастью: козырь
     * должен читаться с одного взгляда, иначе показывать карту вместо масти незачем.
     */
    .trump-under {
        position: absolute;
        left: -12px;
        top: 20px;
        transform: rotate(-90deg);
        transform-origin: center center;
        z-index: 0;
    }

    .trump-under :global(.playing-card) {
        box-shadow: 0 6px 14px rgba(0, 0, 0, 0.5);
    }

    /* Козырь лежит под колодой боком — видно ровно тот край, что торчит слева. */
    .trump-card {
        position: absolute;
        left: -18px;
        top: 22px;
        width: 74px;
        height: 51px;
        border-radius: 5px;
        background: #f4f1ea;
        color: #191410;
        display: flex;
        align-items: center;
        justify-content: flex-start;
        padding-left: 7px;
        font-size: 24px;
        line-height: 1;
        box-shadow: 0 8px 18px rgba(0, 0, 0, 0.55);
    }

    .trump-card.red {
        color: #c02b36;
    }

    .pile {
        position: relative;
        width: 54px;
        height: 56px;
    }

    /* Панель смены козыря: золотая рамка, потому что это событие раздачи, а не её фон. */
    .trump-change {
        display: flex;
        flex-direction: column;
        align-items: center;
    }

    .tc-head {
        display: flex;
        align-items: center;
        gap: 6px;
        padding: 6px 11px;
        border: 1px solid var(--gold-soft);
        border-bottom: 0;
        border-radius: 11px 11px 0 0;
        background: linear-gradient(160deg, rgba(240, 205, 138, 0.22), rgba(201, 154, 78, 0.12));
        font-size: 10px;
        letter-spacing: 0.08em;
        text-transform: uppercase;
        color: var(--gold);
        white-space: nowrap;
    }

    .tc-arrow {
        color: var(--text-55);
        text-transform: none;
        letter-spacing: 0;
    }

    .tc-body {
        display: flex;
        flex-direction: column;
        align-items: center;
        gap: 6px;
        padding: 10px 14px;
        border: 1px solid var(--gold-soft);
        border-radius: 0 0 13px 13px;
        background: rgba(6, 9, 8, 0.5);
    }

    .tc-card :global(.playing-card) {
        outline: 1px dashed var(--gold-soft);
        outline-offset: 2px;
    }

    .tc-note {
        font-size: 8px;
        letter-spacing: 0.06em;
        text-transform: uppercase;
        color: var(--text-45);
        text-align: center;
    }

    .board {
        display: flex;
        flex-direction: column;
        align-items: center;
        gap: 12px;
    }

    .slots {
        display: flex;
        gap: 10px;
        flex-wrap: wrap;
        justify-content: center;
    }

    .slot {
        position: relative;
        width: 78px;
        height: 106px;
    }

    .prompt {
        display: flex;
        align-items: center;
        gap: 8px;
        font: 500 11px var(--mono);
        letter-spacing: 0.08em;
        text-transform: uppercase;
        white-space: nowrap;
    }

    .mine {
        width: 100%;
        display: flex;
        align-items: flex-end;
        justify-content: space-between;
        padding: 0 20px;
    }

    .my-hung {
        display: flex;
        align-items: flex-end;
        gap: 11px;
    }

    .hung-row {
        display: flex;
        align-items: flex-end;
    }

    .hung-label {
        line-height: 1.6;
        padding-bottom: 2px;
    }

    .flies {
        font-size: 7px;
    }

    .my-hidden :global(.playing-card) {
        outline: 1px dashed var(--gold-soft);
        outline-offset: 2px;
    }

    /* ⚠️ Ширина во всю строку обязательна: по ней считается нахлёст, а без неё контейнер
       сжимается по содержимому и «доступная ширина» всегда равна ширине веера. */
    .hand {
        display: flex;
        align-items: flex-end;
        justify-content: center;
        width: 100%;
        box-sizing: border-box;
        padding: 0 8px 6px;
        flex: none;
    }

    .hand-card {
        /* ⭐ Без этого карты сжимаются под ширину экрана — и веер получается из карт
           разного размера, будто часть колоды другая. */
        flex: none;
        display: block;
    }

    /* Нахлёст считает скрипт по числу карт — так рука влезает в экран любой длины. */
    .hand-card + .hand-card {
        margin-left: calc(-1 * var(--overlap, 26px));
    }

    /* Наклон карты в веере: он меняется при добавлении соседей, поэтому доезжает плавно. */
    .fan {
        display: block;
        transition: transform 0.34s cubic-bezier(0.22, 0.61, 0.25, 1);
    }

    /**
     * Отбившая карта ложится поверх атакующей со сдвигом — видно обе.
     *
     * ⭐ Ровно того же размера, что и нижняя, слегка повёрнутая и с тенью погуще. Карта
     * поверх, нарисованная мельче нижней и без тени, читается не как «легла сверху»,
     * а как ошибка вёрстки: настоящая карта, положенная на карту, не уменьшается.
     */
    .defence {
        position: absolute;
        left: 14px;
        top: 16px;
        transform: rotate(3.5deg);
        transform-origin: 20% 20%;
    }

    .defence :global(.playing-card) {
        box-shadow: -3px 10px 20px rgba(0, 0, 0, 0.6);
    }

    .actions {
        width: 100%;
        padding: 12px 14px calc(20px + env(safe-area-inset-bottom));
        display: flex;
        gap: 9px;
        background: linear-gradient(to top, rgba(6, 9, 8, 0.94) 55%, rgba(6, 9, 8, 0));
    }

    .wide {
        flex: 2;
    }

    .narrow {
        flex: 1;
        height: 58px;
        font-size: 15px;
    }

    .trump {
        flex: 1;
        font-size: 22px;
    }

    .waiting {
        flex: 2;
        height: 58px;
        display: flex;
        align-items: center;
        justify-content: center;
        color: var(--text-45);
    }

    /**
     * ⭐ Десктоп: соперники садятся дугой, как за настоящим столом, — крайние ниже средних.
     * Это не украшение: при пятерых ряд из одинаковых мест читается как список, а дуга —
     * как стол, за которым сидят напротив.
     */
    @media (min-width: 900px) {
        .table-screen {
            gap: 16px;
            padding-top: 18px;
        }

        .opponents {
            padding: 0 60px;
            justify-content: space-between;
            align-items: flex-start;
        }

        .opponents :global(.seat:first-child),
        .opponents :global(.seat:last-child) {
            transform: translateY(46px);
        }

        /* Двое за столом — соперник один и сидит строго напротив, без дуги. */
        .opponents:has(:global(.seat:only-child)) :global(.seat) {
            transform: none;
        }

        /**
         * ⭐ На широком экране ступеней нет: четверо садятся одной дугой. Две ступени —
         * вынужденная мера телефона, где четыре места просто не помещаются по ширине,
         * и переносить её на десктоп значит объяснять экраном то, чего он не требует.
         */
        .tiers {
            flex-wrap: nowrap;
            row-gap: 0;
        }

        .tiers :global(.seat) {
            flex: 0 0 auto;
            width: 104px;
            padding: 0;
        }

        .middle {
            grid-template-columns: 160px 1fr 140px;
            padding: 0 60px;
        }

        .hand :global(.card-button) {
            width: 96px !important;
        }

        .actions {
            max-width: 720px;
            margin: 0 auto;
            background: none;
        }

        .mine {
            padding: 0 60px;
        }
    }
</style>
