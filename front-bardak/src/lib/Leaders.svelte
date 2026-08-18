<script>
    /**
     * Таблица лидеров и сезоны.
     *
     * ⭐ Один экран на два вопроса — «кто сильнее» и «за какой отрезок». Порознь они
     * бессмысленны: рейтинг без сезона не сказать за что, а сезон без таблицы — просто дата.
     *
     * ⚠️ Право закрыть сезон приходит с сервера (`canManage`), а не выводится на клиенте.
     * Список ведущих живёт в настройках рейтинга (ADR-037), и гадать о нём по логину
     * значило бы показывать кнопку тем, кому она откажет.
     */
    import {onMount} from 'svelte';
    import {apiGet, apiPost} from '../net/rest-client.js';
    import Avatar from './Avatar.svelte';
    import {profile} from '../stores/profile.svelte.js';

    let {onBack, onPlayer = null} = $props();

    let rows = $state([]);
    let seasons = $state([]);
    let canManage = $state(false);
    let error = $state(null);
    let loading = $state(true);

    /** Форма нового сезона видна только ведущему и только по нажатию. */
    let closing = $state(false);
    let nextName = $state('');
    let busy = $state(false);

    onMount(load);

    async function load() {
        loading = true;
        error = null;
        try {
            const [top, seasonsView] = await Promise.all([
                apiGet('/rating/top'),
                apiGet('/rating/seasons'),
            ]);
            rows = top;
            seasons = seasonsView.seasons;
            canManage = seasonsView.canManage;
        } catch (e) {
            error = e.message;
        } finally {
            loading = false;
        }
    }

    const openSeason = $derived(seasons.find((season) => season.open) ?? null);

    /** ⭐ Своя строка подсвечивается: в таблице на два экрана себя иначе не найти. */
    const myId = $derived(profile.user?.id ?? null);

    function medal(index) {
        return ['🥇', '🥈', '🥉'][index] ?? null;
    }

    function when(iso) {
        return iso ? new Date(iso).toLocaleDateString('ru-RU',
            {day: 'numeric', month: 'short', year: 'numeric'}) : '';
    }

    async function closeSeason(event) {
        event.preventDefault();
        if (busy) {
            return;
        }
        busy = true;
        error = null;
        try {
            await apiPost('/rating/seasons', {name: nextName.trim()});
            nextName = '';
            closing = false;
            await load();
        } catch (e) {
            error = e.message;
        } finally {
            busy = false;
        }
    }
</script>

<div class="screen">
    <div class="head">
        <button class="icon-btn" type="button" onclick={onBack} aria-label="Назад">←</button>
        <h1>Таблица</h1>
        {#if openSeason}
            <span class="season-pill mono">сезон «{openSeason.name}»</span>
        {/if}
    </div>

    {#if error}<p class="notice notice-fail">{error}</p>{/if}

    {#if loading}
        <p class="muted centered">Считаю…</p>
    {:else}
        <div class="rows">
            {#each rows as row, index (row.userId)}
                <button class="card row" class:mine={row.userId === myId}
                        type="button" disabled={!onPlayer || row.userId === myId}
                        onclick={() => onPlayer?.(row.userId, row.displayName)}>
                    <span class="place mono">{medal(index) ?? index + 1}</span>
                    <Avatar userId={row.userId} size={36}/>
                    <span class="who">
                        <span class="name">{row.displayName}{#if row.userId === myId}<span class="you"> · ты</span>{/if}</span>
                        <span class="mono sub">{row.matchesPlayed}
                            {row.matchesPlayed === 1 ? 'матч' : row.matchesPlayed < 5 ? 'матча' : 'матчей'}</span>
                    </span>
                    <span class="rating mono">{Math.round(row.rating)}</span>
                </button>
            {:else}
                <!-- ⭐ Пусто — не ошибка: до первого доигранного матча рейтинга ещё ни у кого нет. -->
                <p class="muted empty">Ещё никто не доиграл ни одного матча — сыграйте первый.</p>
            {/each}
        </div>

        <div class="seasons">
            <div class="row-line">
                <span class="label">Сезоны</span>
                {#if canManage && !closing}
                    <button class="btn-small" type="button" onclick={() => (closing = true)}>
                        Закрыть сезон
                    </button>
                {/if}
            </div>

            {#if closing}
                <!-- ⚠️ Действие необратимое и одно на двоих: закрывает текущий и открывает
                     следующий. Поэтому имя следующего спрашиваем явно, а не подставляем. -->
                <form class="card sheet" onsubmit={closeSeason}>
                    <span class="label">Имя следующего сезона</span>
                    <input bind:value={nextName} required maxlength="64" placeholder="Осень 2026">
                    <p class="mono warn">Текущий сезон закроется, рейтинги останутся как есть.</p>
                    <div class="row-line">
                        <button class="btn grow" type="submit" disabled={busy || !nextName.trim()}>
                            {busy ? 'Закрываю…' : 'Закрыть и открыть новый'}
                        </button>
                        <button class="btn-ghost" type="button" onclick={() => (closing = false)}>Отмена</button>
                    </div>
                </form>
            {/if}

            {#each seasons as season (season.id)}
                <div class="card season" class:open={season.open}>
                    <span class="grow">
                        <span class="name">{season.name}</span>
                        <span class="mono sub">
                            {when(season.startedAt)}{season.closedAt ? ` — ${when(season.closedAt)}` : ''}
                        </span>
                    </span>
                    {#if season.open}<span class="pill-open mono">идёт</span>{/if}
                </div>
            {:else}
                <p class="muted empty">Сезонов ещё не заводили.</p>
            {/each}
        </div>
    {/if}
</div>

<style>
    .screen {
        flex: 1;
        display: flex;
        flex-direction: column;
        gap: 14px;
        padding: 16px 20px 24px;
        overflow-y: auto;
    }

    .head {
        display: flex;
        align-items: center;
        gap: 12px;
    }

    h1 {
        margin: 0;
        font-size: 18px;
    }

    .season-pill {
        margin-left: auto;
        font-size: 11px;
        color: var(--gold);
        padding: 4px 10px;
        border-radius: 999px;
        background: rgba(240, 205, 138, 0.12);
    }

    .rows {
        display: flex;
        flex-direction: column;
        gap: 8px;
    }

    .row {
        display: flex;
        align-items: center;
        gap: 12px;
        text-align: left;
        width: 100%;
    }

    .row:disabled {
        cursor: default;
    }

    .row.mine {
        border-color: var(--gold-soft);
        background: rgba(240, 205, 138, 0.1);
    }

    .place {
        width: 28px;
        text-align: center;
        font-size: 15px;
        color: var(--text-55);
        flex: none;
    }

    .who {
        flex: 1;
        min-width: 0;
        display: flex;
        flex-direction: column;
    }

    .name {
        font-size: 14px;
        font-weight: 700;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    .you {
        color: var(--gold);
        font-weight: 500;
    }

    .sub {
        font-size: 11px;
        color: var(--text-45);
    }

    .rating {
        font-size: 16px;
        font-weight: 700;
        color: var(--gold);
        flex: none;
    }

    .seasons {
        display: flex;
        flex-direction: column;
        gap: 8px;
        padding-top: 8px;
        border-top: 1px solid var(--line);
    }

    .row-line {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: 10px;
    }

    .season {
        display: flex;
        align-items: center;
        gap: 10px;
    }

    .season.open {
        border-color: var(--gold-soft);
    }

    .grow {
        flex: 1;
        min-width: 0;
        display: flex;
        flex-direction: column;
    }

    .pill-open {
        font-size: 10px;
        color: var(--gold);
        padding: 3px 9px;
        border-radius: 999px;
        background: rgba(240, 205, 138, 0.14);
    }

    .sheet {
        display: flex;
        flex-direction: column;
        gap: 10px;
    }

    .warn {
        margin: 0;
        font-size: 11px;
        color: var(--text-55);
    }

    .empty {
        padding: 12px 0;
        text-align: center;
    }

    .centered {
        text-align: center;
        padding: 20px 0;
    }
</style>
