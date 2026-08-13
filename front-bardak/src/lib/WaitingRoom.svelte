<script>
    /**
     * Комната ожидания: код приглашения, места, готовность.
     *
     * ⭐ Код — крупными клетками и с кнопкой «скопировать»: его диктуют голосом или кидают
     * в переписку, и оба способа должны работать без пересчёта символов пальцем по экрану.
     */
    import CodeBoxes from './CodeBoxes.svelte';
    import Avatar from './Avatar.svelte';
    import {setReady, startMatch, table} from '../stores/table.svelte.js';
    import {profile} from '../stores/profile.svelte.js';

    let {onExit, fallback = null} = $props();

    let copied = $state(false);

    /**
     * ⭐ Стол берётся из стора, но до первого снимка его там нет: `enterTable` вызывается
     * в `onMount`, то есть уже после первой отрисовки. Без запасного значения экран падал
     * на пустом месте — и падал молча, оставляя пользователя с куском интерфейса.
     */
    const info = $derived(table.info ?? fallback);
    const seats = $derived(info?.seats ?? []);
    const mySeat = $derived(seats.find((seat) => seat.userId === profile.user?.id));
    const everyoneReady = $derived(seats.length >= 2 && seats.every((seat) => seat.ready));

    async function copyCode() {
        try {
            await navigator.clipboard.writeText(info.code);
            copied = true;
            setTimeout(() => (copied = false), 2000);
        } catch {
            // Буфер обмена может быть закрыт настройками — код и так виден на экране.
        }
    }

    async function share() {
        const text = `Стол «${info.name}» в бардаке. Код: ${info.code}`;
        if (navigator.share) {
            await navigator.share({title: 'Бардак', text}).catch(() => null);
            return;
        }
        await navigator.clipboard.writeText(text).catch(() => null);
        copied = true;
        setTimeout(() => (copied = false), 2000);
    }
</script>

<header class="bar">
    <button class="icon-btn" type="button" onclick={onExit} aria-label="К списку столов">←</button>
    <div>
        <div class="name">{info.name}</div>
        <div class="mono">{seats.length} из {info.maxPlayers} · {info.isPrivate ? 'по коду' : 'открытый'}</div>
    </div>
</header>

<div class="screen">
    <div class="code-block">
        <span class="label">Код приглашения — диктуй голосом</span>
        <CodeBoxes value={info.code} length={info.code.length}/>
        <div class="row center">
            <button class="btn-small" type="button" onclick={copyCode}>
                {copied ? 'Скопировано' : 'Скопировать'}
            </button>
            <button class="btn-small" type="button" onclick={share}>Поделиться</button>
        </div>
    </div>

    <span class="label">Места</span>
    <div class="seats">
        {#each Array(info.maxPlayers) as unused, seatNo (seatNo)}
            {@const seat = seats.find((s) => s.seatNo === seatNo)}
            <div class="card seat" class:mine={seat && seat.userId === profile.user?.id}
                 class:empty={!seat}>
                {#if seat}
                    <Avatar userId={seat.userId} size={44} active={seat.ready}/>
                    <div class="grow">
                        <div class="name">
                            {seat.displayName}{#if seat.userId === profile.user?.id}<span class="muted"> · ты</span>{/if}
                        </div>
                        <div class="mono">место {seatNo + 1}</div>
                    </div>
                    <span class="pill" class:pill-ready={seat.ready}>{seat.ready ? 'готов' : 'ждёт'}</span>
                {:else}
                    <span class="hole" aria-hidden="true">+</span>
                    <div class="grow muted">свободно</div>
                {/if}
            </div>
        {/each}
    </div>

    <p class="hint mono">
        Матч стартует, когда готовы все. Шкала: 6 → джокер
    </p>
</div>

<div class="bottom-bar">
    {#if everyoneReady && mySeat?.ready}
        <button class="btn" type="button" onclick={startMatch}>Начать матч</button>
    {:else}
        <button class="btn" type="button" onclick={() => setReady(!mySeat?.ready)}>
            {mySeat?.ready ? 'Я передумал' : 'Я готов'}
        </button>
    {/if}
    <button class="btn-ghost" type="button" onclick={onExit}>Выйти из-за стола</button>
</div>

<style>
    .bar {
        flex: none;
        padding: 10px 20px 14px;
        display: flex;
        align-items: center;
        gap: 12px;
        border-bottom: 1px solid rgba(255, 255, 255, 0.07);
    }

    .name {
        font-size: 16px;
        font-weight: 700;
    }

    .screen {
        flex: 1;
        display: flex;
        flex-direction: column;
        gap: 12px;
        padding: 24px 20px 0;
        overflow-y: auto;
    }

    .code-block {
        display: flex;
        flex-direction: column;
        gap: 12px;
        align-items: center;
        text-align: center;
    }

    .code-block :global(.boxes) {
        width: 100%;
    }

    .center {
        justify-content: center;
    }

    .seats {
        display: flex;
        flex-direction: column;
        gap: 10px;
    }

    .seat {
        display: flex;
        align-items: center;
        gap: 12px;
    }

    .seat.mine {
        border-color: rgba(240, 205, 138, 0.4);
        background: rgba(240, 205, 138, 0.07);
    }

    .seat.empty {
        border-style: dashed;
    }

    .grow {
        flex: 1;
        min-width: 0;
    }

    .hole {
        width: 44px;
        height: 44px;
        border-radius: 50%;
        border: 1px dashed rgba(255, 255, 255, 0.25);
        display: flex;
        align-items: center;
        justify-content: center;
        color: var(--text-30);
        flex: none;
    }

    .hint {
        margin-top: 8px;
        padding: 13px 15px;
        border-radius: 14px;
        background: var(--surface);
        border: 1px solid rgba(255, 255, 255, 0.07);
        line-height: 1.7;
    }

    .bottom-bar {
        flex: none;
        padding: 14px 20px 24px;
        display: flex;
        flex-direction: column;
        gap: 9px;
    }
</style>
