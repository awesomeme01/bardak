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
    import {anchorPoint} from './motion.svelte.js';

    /**
     * ⭐ `size` — диаметр аватара, а не «компактный да/нет». Мест за столом от одного до
     * четырёх, и каждому составу в макетах свой размер: вдвоём соперник крупный и сидит
     * напротив, впятером все мельчают, иначе не помещаются по ширине.
     */
    let {seat, size = 56, active = false, defending = false, decision = null,
         taking = false, hangCta = null, onHang = null} = $props();

    const avatarSize = $derived(size);
    const hungWidth = $derived(Math.round(size * 0.62));
    const hiddenWidth = $derived(Math.round(size * 0.45));

    /** Роль в раздаче: она красит кольцо и не зависит от того, чьего хода ждут. */
    const tone = $derived(!seat.inDeal ? null : defending ? 'defend' : active ? 'attack' : null);

    /** Порядковые для тех, кто уже вышел. Дальше третьего в раздаче не бывает — пятеро максимум. */
    const ORDINALS = ['первым', 'вторым', 'третьим', 'четвёртым'];

    /**
     * ⭐ Вышедший подписывается не «вышел», а каким по счёту. Порядок выхода — не мелочь:
     * первый получает −1 по шкале (§0.1), то есть отыгрывает ступень назад, и за столом
     * это считают. «Вышел» без номера прятало главное.
     */
    const exit = $derived.by(() => {
        if (seat.inDeal || !seat.exitPlace) {
            return null;
        }
        const ordinal = ORDINALS[seat.exitPlace - 1] ?? `${seat.exitPlace}-м`;
        return {
            badge: `${seat.exitPlace}-й`,
            text: seat.exitPlace === 1 ? `вышел ${ordinal} · −1` : `вышел ${ordinal}`,
            first: seat.exitPlace === 1,
        };
    });

    /**
     * ⚠️ Пас важнее очереди. Право подкидывать остаётся за спасовавшим до конца раунда,
     * и без этой проверки он подписан «ходит» ровно тогда, когда ходить уже не может.
     */
    const status = $derived.by(() => {
        if (!seat.inDeal) {
            return exit ? null : {text: 'вышел', tone: 'out'};
        }
        // ⭐ «Поднял» держится всё время, пока стол докидывают: это не мелькнувшее
        // событие, а положение дел — человек уже забирает, и подкидывают именно ему.
        if (taking) {
            return {text: 'поднял', tone: 'take'};
        }
        if (seat.passed) {
            return {text: 'пас', tone: 'out'};
        }
        if (defending) {
            return {text: 'отбивается', tone: 'defend'};
        }
        if (active) {
            return {text: 'ходит', tone: 'attack'};
        }
        return null;
    });
</script>

<div class="seat" class:dim={!seat.inDeal}>
    <div class="head">
        <span class="anchor" use:anchorPoint={`seat-${seat.seatNo}`}></span>
        <Avatar userId={seat.userId} size={avatarSize} {tone} pulse={active && !seat.passed}/>
        {#if exit}
            <span class="place" class:first={exit.first}>{exit.badge}</span>
        {/if}
        {#if decision && decision.text !== status?.text}
            <!-- Что сосед только что решил: снимок состояния этого не расскажет. -->
            <span class="decision" class:take={decision.tone === 'take'}
                  class:pass={decision.tone === 'pass'} class:defend={decision.tone === 'defend'}>
                {decision.text}
            </span>
        {/if}
        {#if seat.hasHiddenCard}
            <!-- Скрытая карта соседа: факт есть, содержимое не знает никто (§1.8). -->
            <span class="hidden-card"><Card faceDown width={hiddenWidth}/></span>
        {:else}
            <span class="hidden-card taken mono">взял</span>
        {/if}
    </div>

    <div class="name">{seat.displayName}</div>

    {#if exit}
        <div class="exit-note mono" class:first={exit.first}>{exit.text}</div>
    {/if}

    <div class="hand-count">
        <span class="backs" aria-hidden="true">
            <Card faceDown width={17} style="position:absolute; left:0; top:2px; transform:rotate(-11deg)"/>
            <Card faceDown width={17} style="position:absolute; left:6px; top:0"/>
            <Card faceDown width={17} style="position:absolute; left:12px; top:2px; transform:rotate(11deg)"/>
        </span>
        <span class="mono count">{seat.cardsCount}</span>
    </div>

    <!-- Навесы соседа открыты всем: без них не понять, кто близок к джокеру (§2.3). -->
    <div class="hung-zone" use:anchorPoint={`hung-${seat.seatNo}`}>
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
    </div>

    <!--
      ⭐ Кнопка навеса стоит у слота жертвы, а не в общем ряду действий: навешивают
      конкретному человеку, и промахнуться местом нельзя. Пульсирует, потому что окно
      навеса короткое и его легко проглядеть.
    -->
    {#if hangCta}
        <button class="hang-cta" type="button" onclick={onHang}>{hangCta} →</button>
    {/if}

    {#if status}
        <span class="status" class:attack={status.tone === 'attack'}
              class:defend={status.tone === 'defend'} class:out={status.tone === 'out'}
              class:take={status.tone === 'take'}>
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

    .status.attack {
        background: rgba(232, 98, 108, 0.16);
        color: var(--seat-attack);
    }

    .status.defend {
        background: rgba(127, 216, 166, 0.16);
        color: var(--seat-defend);
    }

    .status.take {
        background: rgba(240, 205, 138, 0.18);
        color: var(--gold);
    }

    .status.out {
        opacity: 0.7;
    }

    /** Каким по счёту вышел. Первому — золото: он единственный, кто получил −1. */
    .place {
        position: absolute;
        right: -8px;
        top: -6px;
        height: 22px;
        padding: 0 8px;
        border-radius: 11px;
        background: rgba(255, 255, 255, 0.14);
        color: var(--text);
        display: flex;
        align-items: center;
        font: 700 11px var(--mono);
    }

    .place.first {
        background: var(--gold-face);
        color: var(--gold-ink);
    }

    .exit-note {
        font-size: 10px;
        color: var(--text-45);
    }

    .exit-note.first {
        color: var(--green);
    }

    .hang-cta {
        margin-top: 2px;
        padding: 6px 12px;
        border-radius: 999px;
        background: var(--gold-face);
        color: var(--gold-ink);
        font-size: 12px;
        font-weight: 800;
        white-space: nowrap;
        box-shadow: 0 8px 20px rgba(201, 154, 78, 0.45);
        animation: cta-pulse 1.6s ease-in-out infinite;
    }

    @keyframes cta-pulse {
        0%, 100% {
            transform: scale(1);
        }
        50% {
            transform: scale(1.06);
        }
    }

    /* Точка вылета карт этого места. Размер нулевой — она только координата. */
    .anchor {
        position: absolute;
        left: 50%;
        top: 50%;
        width: 1px;
        height: 1px;
    }

    .hung-zone {
        display: flex;
        justify-content: center;
    }

    /**
     * ⭐ Решение соседа висит поверх аватара и само гаснет. Это сообщение о случившемся,
     * поэтому оно не занимает места в раскладке — иначе стол дёргался бы на каждый ход.
     */
    .decision {
        position: absolute;
        left: 50%;
        top: -9px;
        transform: translateX(-50%);
        padding: 2px 8px;
        border-radius: 8px;
        white-space: nowrap;
        font: 700 10px var(--mono);
        letter-spacing: 0.06em;
        background: var(--seat-attack);
        color: #1a0d0e;
        box-shadow: 0 4px 12px rgba(0, 0, 0, 0.45);
        animation: decision-in 0.22s ease both;
    }

    .decision.defend {
        background: var(--seat-defend);
        color: #0c1f16;
    }

    .decision.take {
        background: var(--gold);
        color: var(--gold-ink);
    }

    .decision.pass {
        background: rgba(255, 255, 255, 0.82);
        color: #191410;
    }

    @keyframes decision-in {
        from {
            opacity: 0;
            transform: translateX(-50%) translateY(5px);
        }
        to {
            opacity: 1;
            transform: translateX(-50%) translateY(0);
        }
    }
</style>
