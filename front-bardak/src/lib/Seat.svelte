<script>
    /**
     * Место соседа за столом.
     *
     * ⭐ Рука соседа всегда выглядит одинаково — три рубашки веером, а число карт стоит
     * цифрой рядом. Показывать её «настоящей длины» нельзя: чужих карт на устройстве нет
     * вовсе, а веер из девяти рубашек соврал бы про то, чего мы не знаем.
     */
    import Avatar from './Avatar.svelte';
    import Card from './Card.svelte';

    let {seat, compact = false, active = false, defending = false} = $props();

    const avatarSize = $derived(compact ? 46 : 56);
    const hungWidth = $derived(compact ? 30 : 34);

    const status = $derived.by(() => {
        if (!seat.inDeal) {
            return {text: 'вышел', tone: 'out'};
        }
        if (defending) {
            return {text: 'отбивается', tone: 'plain'};
        }
        if (active) {
            return {text: 'ходит', tone: 'turn'};
        }
        if (seat.passed) {
            return {text: 'пас', tone: 'out'};
        }
        return null;
    });
</script>

<div class="seat" class:dim={!seat.inDeal}>
    <div class="head">
        <Avatar userId={seat.userId} size={avatarSize} {active} ring={active}/>
        {#if seat.hasHiddenCard}
            <!-- Скрытая карта соседа: факт есть, содержимое не знает никто (§1.8). -->
            <span class="hidden-card"><Card faceDown width={compact ? 22 : 25}/></span>
        {:else}
            <span class="hidden-card taken mono">взял</span>
        {/if}
    </div>

    <div class="name" class:gold={active}>{seat.displayName}</div>

    <div class="hand-count">
        <span class="backs" aria-hidden="true">
            <Card faceDown width={17} style="position:absolute; left:0; top:2px; transform:rotate(-11deg)"/>
            <Card faceDown width={17} style="position:absolute; left:6px; top:0"/>
            <Card faceDown width={17} style="position:absolute; left:12px; top:2px; transform:rotate(11deg)"/>
        </span>
        <span class="mono count">{seat.cardsCount}</span>
    </div>

    <!-- Навесы соседа открыты всем: без них не понять, кто близок к джокеру (§2.3). -->
    {#if seat.hung.length}
        <div class="hung">
            {#each seat.hung as code, index (code)}
                <Card {code} width={index === seat.hung.length - 1 ? hungWidth + 6 : hungWidth}
                      dimmed={index !== seat.hung.length - 1}
                      style={index < seat.hung.length - 1 ? 'margin-right:-17px' : ''}/>
            {/each}
        </div>
    {:else}
        <div class="card-slot flying" class:gold={seat.nextIsJoker} style="width:{hungWidth + 4}px">
            <span class="rank">{seat.nextIsJoker ? '🃏' : seat.nextNavesRank ?? '6'}</span>
            <span class="flies">летит</span>
        </div>
    {/if}

    {#if status}
        <span class="status" class:turn={status.tone === 'turn'} class:out={status.tone === 'out'}>
            {status.text}
        </span>
    {/if}
</div>

<style>
    .seat {
        display: flex;
        flex-direction: column;
        align-items: center;
        gap: 5px;
        width: 104px;
    }

    .seat.dim {
        opacity: 0.55;
    }

    .head {
        position: relative;
    }

    .hidden-card {
        position: absolute;
        left: -16px;
        bottom: -8px;
    }

    .hidden-card :global(.playing-card) {
        border-radius: 3px;
        outline: 1px dashed var(--gold-soft);
        outline-offset: 1px;
    }

    .taken {
        width: 24px;
        height: 35px;
        border-radius: 3px;
        border: 1px dashed rgba(255, 255, 255, 0.3);
        display: flex;
        align-items: center;
        justify-content: center;
        transform: rotate(-14deg);
        font-size: 7px;
        color: var(--text-45);
    }

    .name {
        margin-top: 4px;
        font-size: 12px;
        font-weight: 700;
        max-width: 104px;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    .hand-count {
        display: flex;
        align-items: center;
        gap: 6px;
    }

    .backs {
        position: relative;
        display: block;
        width: 30px;
        height: 26px;
    }

    .count {
        font-size: 12px;
        color: var(--text-70);
    }

    .hung {
        display: flex;
        align-items: flex-end;
        margin-top: 3px;
    }

    .flying {
        margin-top: 3px;
    }

    .rank {
        font-size: 13px;
    }

    .flies {
        font-size: 7px;
        letter-spacing: 0.06em;
    }

    .status {
        height: 22px;
        padding: 0 9px;
        border-radius: 8px;
        background: rgba(255, 255, 255, 0.08);
        font: 500 10px var(--mono);
        letter-spacing: 0.06em;
        color: var(--text-55);
        display: inline-flex;
        align-items: center;
    }

    .status.turn {
        background: rgba(240, 205, 138, 0.16);
        color: var(--gold);
    }

    .status.out {
        opacity: 0.7;
    }
</style>
