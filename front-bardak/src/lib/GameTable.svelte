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
     */
    import Card from './Card.svelte';
    import Seat from './Seat.svelte';
    import TurnClock from './TurnClock.svelte';
    import {play, table} from '../stores/table.svelte.js';
    import {isRedSuit, suitGlyph} from './naming.js';

    const game = $derived(table.game);

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

    /** Карты руки, которыми сейчас разрешено пойти хоть как-нибудь. */
    const playable = $derived.by(() => {
        const codes = new Set();
        for (const list of [actions.attacks, actions.defends, actions.transfers, actions.hangs]) {
            list.forEach((action) => codes.add(action.payload.cardCode));
        }
        return codes;
    });

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

    const prompt = $derived.by(() => {
        if (actions.trumps.length) {
            return 'Назови козырь';
        }
        if (game.hangingVictimSeat !== null && game.hangingVictimSeat !== undefined) {
            return game.hangingVictimSeat === game.mySeat ? 'Тебе навешивают' : 'Твой навес';
        }
        if (selected && targets.length > 1) {
            return 'Укажи, какую карту бьёшь';
        }
        if (game.defenderSeat === game.mySeat && game.table.some((slot) => !slot.defend)) {
            return 'Отбивайся';
        }
        if (game.canAttackSeat === game.mySeat) {
            return game.table.length ? 'Подкидывай или пасуй' : 'Твоя атака';
        }
        return null;
    });

    const myTurn = $derived((game?.availableActions ?? []).length > 0);
    const opponents = $derived((game?.players ?? []).filter((seat) => seat.seatNo !== game.mySeat));
    const me = $derived((game?.players ?? []).find((seat) => seat.seatNo === game.mySeat));

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

    function tapCard(code) {
        if (!playable.has(code)) {
            return;
        }
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
    </div>

    <div class="opponents" class:two-rows={opponents.length > 3}>
        {#each opponents as seat (seat.seatNo)}
            <Seat {seat} compact={opponents.length > 2}
                  active={seat.seatNo === game.canAttackSeat}
                  defending={seat.seatNo === game.defenderSeat}/>
        {/each}
    </div>

    <div class="middle">
        <div class="deck">
            {#if game.deckLeft > 0}
                <div class="deck-stack">
                    <!--
                      Козырь лежит под колодой и берётся последним (§1.9). Сама карта клиенту
                      не приходит — только масть, поэтому показываем масть, а не выдуманную карту.
                    -->
                    {#if game.trumpSuit}
                        <span class="trump-card" class:red={isRedSuit(game.trumpSuit)}>
                            {suitGlyph(game.trumpSuit)}
                        </span>
                    {/if}
                    <Card faceDown width={52} style="position:absolute; left:20px; top:2px"/>
                    <Card faceDown width={52} style="position:absolute; left:18px; top:0"/>
                </div>
                <div class="mono">Колода {game.deckLeft}</div>
            {:else}
                <div class="card-slot" style="width:52px"><span class="mono">пусто</span></div>
                <div class="mono">Колода 0</div>
            {/if}
        </div>

        <div class="board">
            {#if game.table.length === 0}
                <div class="card-slot gold" style="width:70px">
                    <span class="mono">брось</span><span class="mono">карту</span>
                </div>
            {:else}
                <div class="slots">
                    {#each game.table as slot (slot.attack)}
                        {@const canBeat = targets.some((a) => a.payload.targetCardCode === slot.attack)}
                        <div class="slot">
                            <Card code={slot.attack} width={62} playable={canBeat}
                                  onclick={canBeat ? () => tapTarget(slot.attack) : null}/>
                            {#if slot.defend}
                                <Card code={slot.defend} width={62}
                                      style="position:absolute; left:14px; top:16px"/>
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

        <div class="discard">
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
        <div class="my-hung">
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
            <div class="mono hung-label">
                Мой навес<br>
                <span class="gold">{me?.hung.length ? `навесили ${me.hung.length}` : 'чисто'}</span>
            </div>
        </div>

        {#if game.iHaveHiddenCard}
            <!-- Свою скрытую карту не видит даже владелец (§1.8) — только рубашку. -->
            <span class="my-hidden"><Card faceDown width={38}/></span>
        {/if}
    </div>

    <div class="hand" style="--count:{game.myHand.length}">
        {#each game.myHand as code, index (code)}
            {@const middle = (game.myHand.length - 1) / 2}
            {@const offset = index - middle}
            <span class="hand-card" style="transform: rotate({offset * 5}deg) translateY({Math.abs(offset) * 4}px)">
                <Card {code} width={70} playable={playable.has(code)} dimmed={!playable.has(code)}
                      onclick={() => tapCard(code)}
                      style={selected === code ? 'transform: translateY(-18px)' : ''}/>
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
                <button class="btn btn-red narrow" type="button" onclick={() => run(actions.take)}>Беру</button>
            {/if}
            {#if primary}
                <button class="btn wide" class:btn-blue={primary.tone === 'blue'} type="button"
                        onclick={() => run(primary.action)}>{primary.label}</button>
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

<style>
    .table-screen {
        flex: 1;
        display: flex;
        flex-direction: column;
        align-items: center;
        gap: 10px;
        padding: 10px 0 0;
        min-height: 0;
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

    /* Пятеро за столом: соперники в две ступени, иначе они не помещаются по ширине. */
    .two-rows {
        flex-wrap: wrap;
        row-gap: 10px;
    }

    .two-rows :global(.seat) {
        width: 96px;
    }

    .middle {
        flex: 1;
        min-height: 168px;
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
        width: 78px;
        height: 86px;
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

    .hand {
        display: flex;
        align-items: flex-end;
        justify-content: center;
        padding: 0 8px 6px;
        min-height: 112px;
    }

    .hand-card {
        /* ⭐ Без этого карты сжимаются под ширину экрана — и веер получается из карт
           разного размера, будто часть колоды другая. */
        flex: none;
    }

    .hand-card + .hand-card {
        margin-left: -26px;
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

        .middle {
            grid-template-columns: 160px 1fr 140px;
            padding: 0 60px;
            min-height: 230px;
        }

        .hand {
            min-height: 150px;
        }

        .hand :global(.card-button) {
            width: 96px !important;
        }

        .hand-card + .hand-card {
            margin-left: -34px;
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
