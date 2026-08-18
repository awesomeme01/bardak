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
    import {lobby} from '../stores/lobby.svelte.js';
    import {linkFor} from '../stores/invite-link.svelte.js';

    /**
     * ⚠️ Стрелка назад и «Выйти из-за стола» — разные действия, а не одно.
     *
     * Раньше обе освобождали место: заглянуть в лобби, не потеряв стол, было нельзя вовсе.
     * Теперь стрелка уводит в меню и место сохраняет, а встают только явной кнопкой.
     */
    let {onExit, onMenu = null, fallback = null} = $props();

    /** Что именно скопировали: 'code' | 'link' | null. Подтверждение показывает та кнопка,
     *  на которую нажали, — иначе «Скопировано» загорается сразу под обеими. */
    let copied = $state(/** @type {'code' | 'link' | null} */ (null));

    /**
     * ⭐ Стол берётся из стора, но до первого снимка его там нет: `enterTable` вызывается
     * в `onMount`, то есть уже после первой отрисовки. Без запасного значения экран падал
     * на пустом месте — и падал молча, оставляя пользователя с куском интерфейса.
     */
    /**
     * ⚠️ Состав берётся у лобби — оно единственный владелец мест (см. applyTableEvent).
     * `table.info` держит только то, за каким столом мы сидим; состав в нём был копией,
     * которая переставала обновляться.
     */
    const info = $derived(lobby.current ?? table.info ?? fallback);
    const seats = $derived(info?.seats ?? []);
    const mySeat = $derived(seats.find((seat) => seat.userId === profile.user?.id));
    const everyoneReady = $derived(seats.length >= 2 && seats.every((seat) => seat.ready));

    /** Ссылка ведёт прямо за этот стол — и того, у кого учётки ещё нет, тоже. */
    const link = $derived(linkFor(info.code));

    /**
     * ⚠️ `navigator.clipboard` есть не всегда: он требует защищённого контекста, а по
     * локальной сети мы ходим по обычному http. Поэтому есть запасной путь через
     * скрытое поле — иначе кнопка «Скопировать» молча не работала бы ровно там, где
     * ссылку и раздают.
     */
    async function copy(text, mark) {
        try {
            await navigator.clipboard.writeText(text);
        } catch {
            const field = document.createElement('textarea');
            field.value = text;
            field.setAttribute('readonly', '');
            field.style.position = 'fixed';
            field.style.opacity = '0';
            document.body.appendChild(field);
            field.select();
            try {
                document.execCommand('copy');
            } catch {
                return false;
            } finally {
                field.remove();
            }
        }
        copied = mark;
        setTimeout(() => (copied = null), 2000);
        return true;
    }

    const copyCode = () => copy(info.code, 'code');
    const copyLink = () => copy(link, 'link');

    async function share() {
        // ⭐ Отдаём ссылку, а не текст с кодом: по ссылке друг попадает за стол одним
        // нажатием, а код ему пришлось бы вводить руками в приложении, которого у него нет.
        if (navigator.share) {
            await navigator.share({title: 'Бардак', text: `Стол «${info.name}» в бардаке`, url: link})
                .catch(() => null);
            return;
        }
        await copyLink();
    }
</script>

<header class="bar">
    <button class="icon-btn" type="button" onclick={onMenu ?? onExit}
            aria-label="В главное меню — место останется за тобой">←</button>
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
                {copied === 'code' ? 'Скопировано' : 'Скопировать код'}
            </button>
        </div>

        <!--
          ⭐ Ссылка — главный способ позвать: она доводит до места сама, и человеку без
          учётки тоже. Код рядом остаётся для тех, кто уже в игре и кому проще ввести
          шесть символов, чем искать сообщение.
        -->
        <div class="link-row">
            <span class="label">Ссылка — работает и для тех, кто ещё не играл</span>
            <code class="link">{link}</code>
            <div class="row center">
                <button class="btn-small gold" type="button" onclick={copyLink}>
                    {copied === 'link' ? 'Ссылка скопирована' : 'Скопировать ссылку'}
                </button>
                <button class="btn-small" type="button" onclick={share}>Поделиться</button>
            </div>
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

    .link-row {
        width: 100%;
        display: flex;
        flex-direction: column;
        align-items: center;
        gap: 8px;
        padding-top: 12px;
        border-top: 1px solid var(--line);
    }

    /* ⚠️ Адрес по локальной сети длинный и без пробелов — рвём его принудительно,
       иначе он растягивает экран и возвращает горизонтальную прокрутку. */
    .link {
        width: 100%;
        padding: 8px 10px;
        border-radius: 10px;
        background: rgba(255, 255, 255, 0.06);
        font: 500 11px var(--mono);
        color: var(--text-70);
        word-break: break-all;
        line-height: 1.4;
    }

    .btn-small.gold {
        background: var(--gold-face);
        color: var(--gold-ink);
        font-weight: 700;
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
