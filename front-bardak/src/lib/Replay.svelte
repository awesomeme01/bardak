<script>
    /**
     * Реплей матча.
     *
     * ⭐ Партия не показывается списком событий. `CARD_ATTACKED / PASSED / ROUND_BEATEN` —
     * это протокол; человек приходит сюда посмотреть, как всё было, и хочет читать фразами:
     * «кладёт 7♦», «бьёт 7♦ картой 9♦», «забирает стол — 6 карт».
     *
     * ⭐ Реплей идёт сам, как запись, а не листается кнопкой. Партию пересматривают, чтобы
     * пережить момент заново, — а для разбора рядом есть перемотка и шаг за шагом.
     *
     * ⚠️ Что видно в реплее, решает сервер: он отдаёт события уже отфильтрованными под
     * место смотрящего (§1.8). Чужая скрытая карта не приедет сюда даже в прошедшем матче.
     */
    import {onDestroy} from 'svelte';
    import {replayLine, suitName} from './naming.js';

    let {replay, details, onClose} = $props();

    /** Скорости показа: обычная, вдвое быстрее, вчетверо. */
    const SPEEDS = [1, 2, 4];
    const STEP_MS = 900;

    let dealIndex = $state(0);
    let step = $state(0);
    let playing = $state(false);
    let speed = $state(1);
    let timer = null;

    /** Имя по месту: без него реплей читается как «место 2 бьёт место 0». */
    const nameOf = $derived.by(() => {
        const seats = new Map((details?.match?.players ?? []).map((p) => [p.seatNo, p.displayName]));
        return (seat) => seats.get(seat) ?? `место ${seat}`;
    });

    /** Раздачи в порядке номера; события без раздачи (старт матча) идут в первую. */
    const deals = $derived.by(() => {
        const byDeal = new Map();
        for (const event of replay?.events ?? []) {
            const no = event.dealNo ?? 1;
            if (!byDeal.has(no)) {
                byDeal.set(no, []);
            }
            byDeal.get(no).push(event);
        }
        return [...byDeal.entries()]
            .sort((left, right) => left[0] - right[0])
            .map(([dealNo, events]) => ({dealNo, events}));
    });

    const deal = $derived(deals[dealIndex] ?? {dealNo: 0, events: []});
    const total = $derived(deal.events.length);

    /** Козырь раздачи берём из разбора — в событиях он появляется только при смене. */
    const trump = $derived((details?.deals ?? [])
        .find((row) => row.dealNo === deal.dealNo)?.trumpSuit ?? null);

    /** Показанные шаги: до текущего включительно. Дальше — ещё не случилось. */
    const shown = $derived(deal.events.slice(0, step + 1));

    $effect(() => {
        // Смена раздачи начинает показ заново: продолжать с чужого номера шага бессмысленно.
        dealIndex;
        step = 0;
        playing = false;
    });

    $effect(() => {
        clearInterval(timer);
        if (!playing) {
            return;
        }
        timer = setInterval(() => {
            if (step + 1 >= total) {
                playing = false;
                return;
            }
            step++;
        }, STEP_MS / speed);
        return () => clearInterval(timer);
    });

    onDestroy(() => clearInterval(timer));

    function toggle() {
        if (step + 1 >= total) {
            step = 0;
        }
        playing = !playing;
    }

    function cycleSpeed() {
        speed = SPEEDS[(SPEEDS.indexOf(speed) + 1) % SPEEDS.length];
    }
</script>

<div class="replay screen">
    <div class="head">
        <button class="icon-btn" type="button" onclick={onClose} aria-label="Закрыть реплей">←</button>
        <div class="grow">
            <div class="title">Реплей · раздача {deal.dealNo}</div>
            <div class="mono sub">
                {#if trump}козырь {suitName(trump)}<span class="sep">·</span>{/if}
                {total} {total === 1 ? 'событие' : 'событий'}
            </div>
        </div>
    </div>

    {#if deals.length > 1}
        <!-- Раздачи переключаются вручную: матч из двадцати раздач подряд никто не смотрит. -->
        <div class="deals">
            {#each deals as item, index (item.dealNo)}
                <button class="deal-btn" class:chosen={index === dealIndex} type="button"
                        onclick={() => (dealIndex = index)}>{item.dealNo}</button>
            {/each}
        </div>
    {/if}

    <div class="transport">
        <button class="rewind" type="button" onclick={() => { step = 0; playing = false; }}
                title="К началу раздачи" aria-label="К началу">⏮</button>
        <button class="play" type="button" onclick={toggle}
                aria-label={playing ? 'Пауза' : 'Играть'}>{playing ? '❚❚' : '▶'}</button>
        <div class="grow">
            <!-- Полоса — и показ хода, и перемотка: разбирают партию именно по ней. -->
            <input class="scrub" type="range" min="0" max={Math.max(0, total - 1)}
                   bind:value={step} oninput={() => (playing = false)}
                   aria-label="Перемотка реплея">
            <div class="mono line">
                <span>ход {Math.min(step + 1, total)} из {total}</span>
                <button class="speed" type="button" onclick={cycleSpeed}>×{speed}</button>
            </div>
        </div>
    </div>

    <ol class="steps">
        {#each shown as event, index (event.seq)}
            <li class="step" class:now={index === step}>
                <span class="dot mono">{index + 1}</span>
                <div class="body">
                    <div class="who">{nameOf(event.actorSeat)}</div>
                    <div class="what">{replayLine(event, nameOf)}</div>
                </div>
            </li>
        {/each}
        {#if !total}
            <p class="muted">В этой раздаче событий не записано.</p>
        {/if}
    </ol>
</div>

<style>
    /* Экран целиком: список шагов прокручивается внутри, шапка и перемотка стоят. */
    .replay {
        flex: 1;
        display: flex;
        flex-direction: column;
        gap: 12px;
        min-height: 0;
        padding: 16px 20px 20px;
        overflow: hidden;
    }

    .head {
        display: flex;
        align-items: center;
        gap: 12px;
    }

    .grow {
        flex: 1;
        min-width: 0;
    }

    .title {
        font-size: 16px;
        font-weight: 700;
    }

    .sub {
        margin-top: 3px;
        display: flex;
        align-items: center;
        gap: 7px;
    }

    .sep {
        opacity: 0.4;
    }

    .deals {
        display: flex;
        gap: 6px;
        flex-wrap: wrap;
    }

    .deal-btn {
        min-width: 32px;
        height: 32px;
        border-radius: 10px;
        border: 1px solid var(--line-strong);
        background: var(--surface);
        color: var(--text-55);
        font-family: var(--mono);
        font-size: 12px;
    }

    .deal-btn.chosen {
        border-color: var(--gold-soft);
        background: rgba(240, 205, 138, 0.12);
        color: var(--gold);
    }

    .transport {
        display: flex;
        align-items: center;
        gap: 12px;
        padding-bottom: 12px;
        border-bottom: 1px solid var(--line);
    }

    .rewind {
        width: 36px;
        height: 40px;
        border-radius: 12px;
        border: 1px solid var(--line-strong);
        background: var(--surface);
        color: var(--text-55);
        font-size: 13px;
        flex: none;
    }

    .play {
        width: 40px;
        height: 40px;
        border-radius: 12px;
        background: var(--gold-face);
        color: var(--gold-ink);
        font-size: 14px;
        font-weight: 800;
        flex: none;
    }

    .scrub {
        width: 100%;
        accent-color: var(--gold);
    }

    .line {
        margin-top: 4px;
        display: flex;
        align-items: center;
        justify-content: space-between;
    }

    .speed {
        font-family: var(--mono);
        font-size: 11px;
        color: var(--text-55);
        padding: 2px 8px;
        border-radius: 8px;
        border: 1px solid var(--line-strong);
    }

    .steps {
        list-style: none;
        margin: 0;
        padding: 0;
        display: flex;
        flex-direction: column;
        gap: 10px;
        overflow-y: auto;
    }

    /* ⭐ Прошедшие шаги гаснут, текущий — в полную силу: видно, где мы сейчас. */
    .step {
        display: flex;
        gap: 12px;
        opacity: 0.5;
    }

    .step.now {
        opacity: 1;
    }

    .dot {
        width: 26px;
        height: 26px;
        border-radius: 50%;
        border: 1px solid var(--line-strong);
        display: flex;
        align-items: center;
        justify-content: center;
        font-size: 11px;
        flex: none;
    }

    .step.now .dot {
        border-color: var(--gold-soft);
        background: rgba(240, 205, 138, 0.12);
        color: var(--gold);
    }

    .who {
        font-size: 14px;
        font-weight: 700;
    }

    .what {
        margin-top: 3px;
        font-size: 13px;
        line-height: 1.5;
        color: var(--text-70);
    }
</style>
