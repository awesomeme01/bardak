<script>
    import Card from './Card.svelte';
    import {play, table} from '../stores/table.svelte.js';

    const game = $derived(table.game);

    /**
     * Разрешённые ходы, разложенные по смыслу.
     *
     * ⭐ Всё это приходит с сервера: клиент не решает, что чем бьётся и что можно навесить.
     * Он только раскладывает список по кнопкам.
     */
    const actions = $derived.by(() => {
        const list = game?.availableActions ?? [];
        return {
            attacks: list.filter((a) => a.type === 'PLAY_CARD' && !a.payload.targetCardCode),
            defends: list.filter((a) => a.type === 'PLAY_CARD' && a.payload.targetCardCode),
            transfers: list.filter((a) => a.type === 'TRANSFER'),
            hangs: list.filter((a) => a.type === 'HANG_CARD'),
            simple: list.filter((a) => ['PASS', 'TAKE', 'HANG_SKIP', 'REVEAL_FACE_DOWN'].includes(a.type)),
            trumps: list.filter((a) => a.type === 'CHOOSE_TRUMP'),
        };
    });

    let selectedCard = $state(null);

    /** Карты, которыми можно накрыть выбранную цель. */
    const targetsFor = $derived.by(() => {
        if (!selectedCard) {
            return [];
        }
        return actions.defends
            .filter((a) => a.payload.cardCode === selectedCard)
            .map((a) => a.payload.targetCardCode);
    });

    function clickHand(code) {
        const attack = actions.attacks.find((a) => a.payload.cardCode === code);
        const transfer = actions.transfers.find((a) => a.payload.cardCode === code);
        const hang = actions.hangs.find((a) => a.payload.cardCode === code);
        const canDefend = actions.defends.some((a) => a.payload.cardCode === code);

        if (canDefend) {
            // Защита обязана указать цель (§2.1) — поэтому сначала карта, потом карта на столе.
            selectedCard = selectedCard === code ? null : code;
            return;
        }
        if (hang) {
            play(hang);
        } else if (attack) {
            play(attack);
        } else if (transfer) {
            play(transfer);
        }
        selectedCard = null;
    }

    function clickTarget(attackCode) {
        const action = actions.defends.find(
            (a) => a.payload.cardCode === selectedCard && a.payload.targetCardCode === attackCode);
        if (action) {
            play(action);
            selectedCard = null;
        }
    }

    const label = {
        PASS: 'Пас', TAKE: 'Беру', HANG_SKIP: 'Пропустить навес',
        REVEAL_FACE_DOWN: 'Вскрыть скрытую',
    };
    const suitName = {DIAMONDS: '♦ бубны', HEARTS: '♥ черви', SPADES: '♠ пики', CLUBS: '♣ трефы'};
</script>

<section class="card">
    <h2>Раздача {game.dealNo} · {game.phase}</h2>
    <p>
        Козырь: {game.trumpSuit ? suitName[game.trumpSuit] : 'разыгрывается'} ·
        защищённая масть: {game.protectedSuit ? suitName[game.protectedSuit] : '—'} ·
        в колоде {game.deckLeft}
    </p>

    <ul class="seats">
        {#each game.players as seat (seat.seatNo)}
            <li class:current={seat.seatNo === game.canAttackSeat || seat.seatNo === game.defenderSeat}>
                <span class="seat-no">{seat.seatNo + 1}</span>
                <span>{seat.displayName}</span>
                <span class="muted">{seat.cardsCount} карт{seat.hasHiddenCard ? ' + скрытая' : ''}</span>
                <!-- Слот навесов открыт всем: без него не понять, кто близок к джокеру (§2.3). -->
                <span class="naves">
                    {#each seat.hung as hung}<Card code={hung} small/>{/each}
                    <span class="muted">
                        {seat.nextIsJoker ? 'летит джокер' : `летит ${seat.nextNavesRank ?? '—'}`}
                    </span>
                </span>
                {#if seat.seatNo === game.defenderSeat}<span class="badge badge-wait">отбивается</span>{/if}
                {#if seat.seatNo === game.canAttackSeat}<span class="badge badge-ok">ходит</span>{/if}
            </li>
        {/each}
    </ul>
</section>

<section class="card">
    <h2>Стол</h2>
    {#if game.table.length === 0}
        <p class="muted">Пусто</p>
    {:else}
        <div class="table-slots">
            {#each game.table as slot (slot.attack)}
                <div class="slot" class:targetable={targetsFor.includes(slot.attack)}>
                    <Card code={slot.attack}
                          onclick={targetsFor.includes(slot.attack) ? () => clickTarget(slot.attack) : null}/>
                    {#if slot.defend}<Card code={slot.defend} small/>{/if}
                </div>
            {/each}
        </div>
    {/if}
</section>

<section class="card">
    <h2>Рука</h2>
    <div class="hand">
        {#each game.myHand as code (code)}
            <Card {code} selected={selectedCard === code} onclick={() => clickHand(code)}/>
        {/each}
        {#if game.iHaveHiddenCard}
            <!-- Свою скрытую карту не видит даже владелец (§1.8) — только рубашку. -->
            <Card code="back" faceDown/>
        {/if}
    </div>

    {#if selectedCard}
        <p class="muted">Выбрана {selectedCard} — укажи, какую карту бьёшь</p>
    {/if}

    <div class="row">
        {#each actions.simple as action}
            <button type="button" onclick={() => play(action)}>{label[action.type] ?? action.type}</button>
        {/each}
        {#each actions.trumps as action}
            <button type="button" onclick={() => play(action)}>
                Козырь: {suitName[action.payload.suit]}
            </button>
        {/each}
    </div>

    {#if game.availableActions.length === 0}
        <p class="muted">Сейчас ход не за тобой</p>
    {/if}
</section>
