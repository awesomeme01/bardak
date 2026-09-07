<script>
    /**
     * Общая статистика: все игроки в одной таблице.
     *
     * ⭐ Личная статистика отвечает «как я играю», эта — «как мы играем». Своё среднее
     * место ничего не значит, пока не видно чужого, а «кто чаще всех доводит до
     * королевского» из одного профиля не узнать вовсе.
     *
     * ⚠️ Числа считает сервер одним запросом, включая среднее место: посчитай его здесь —
     * и округление разойдётся с тем, что показано в профиле.
     */
    import {onMount} from 'svelte';
    import {apiGet} from '../net/rest-client.js';
    import {profile} from '../stores/profile.svelte.js';
    import Avatar from './Avatar.svelte';

    let {onPlayer = null} = $props();

    let data = $state(null);
    let error = $state(null);
    let sort = $state('rating');

    onMount(async () => {
        try {
            data = await apiGet('/stats/overview');
        } catch (e) {
            error = e.message;
        }
    });

    /**
     * ⚠️ Порядок задаётся ЗДЕСЬ, а не пересчётом на сервере: сортировка — это способ
     * посмотреть на те же числа, а не другой запрос. Список короткий, сортировать его
     * заново в базе на каждое нажатие было бы расточительством.
     */
    const SORTS = [
        {key: 'rating', label: 'по эло', of: (p) => Number(p.rating ?? 0)},
        {key: 'matches', label: 'по матчам', of: (p) => p.matches},
        {key: 'wins', label: 'по победам', of: (p) => p.wins},
        // Меньше — лучше, поэтому знак обратный: сортировка всегда «сверху лучшие».
        {key: 'avgPlace', label: 'по месту', of: (p) => -Number(p.avgPlace ?? 99)},
        {key: 'royals', label: 'по королевским', of: (p) => p.royals},
    ];

    const rows = $derived.by(() => {
        if (!data) {
            return [];
        }
        const pick = SORTS.find((s) => s.key === sort) ?? SORTS[0];
        return [...data.players].sort((a, b) => pick.of(b) - pick.of(a));
    });

    const myId = $derived(profile.user?.id ?? null);

    /** Доля в процентах без дробей: на десятке матчей дробь только шумит. */
    function share(part, whole) {
        return whole ? `${Math.round((100 * part) / whole)}%` : '—';
    }

    function plural(count, one, few, many) {
        const tail = count % 10;
        const hundred = count % 100;
        if (tail === 1 && hundred !== 11) return one;
        if (tail >= 2 && tail <= 4 && (hundred < 12 || hundred > 14)) return few;
        return many;
    }
</script>

{#if error}
    <p class="notice notice-fail">{error}</p>
{:else if !data}
    <p class="muted centered">Считаю…</p>
{:else}
    <div class="totals card">
        <div class="total">
            <span class="value">{data.totals.matches}</span>
            <span class="label">{plural(data.totals.matches, 'матч', 'матча', 'матчей')}</span>
        </div>
        <div class="total">
            <span class="value">{data.totals.offline}</span>
            <span class="label">оффлайн</span>
        </div>
        <div class="total">
            <span class="value">{data.totals.deals}</span>
            <span class="label">раздач</span>
        </div>
        <div class="total">
            <span class="value">{data.totals.players}</span>
            <span class="label">игроков</span>
        </div>
    </div>

    <div class="sorts">
        {#each SORTS as option (option.key)}
            <button class="chip" class:on={sort === option.key} type="button"
                    onclick={() => (sort = option.key)}>{option.label}</button>
        {/each}
    </div>

    <div class="rows">
        {#each rows as row, index (row.userId)}
            <button class="card row" class:mine={row.userId === myId} type="button"
                    disabled={!onPlayer || row.userId === myId}
                    onclick={() => onPlayer?.(row.userId, row.displayName)}>
                <div class="head-line">
                    <span class="rank mono">{index + 1}</span>
                    <Avatar userId={row.userId} size={32}/>
                    <span class="who">
                        <span class="name">{row.displayName}{#if row.userId === myId}<span class="you"> · ты</span>{/if}</span>
                        <span class="mono sub">{row.matches}
                            {plural(row.matches, 'матч', 'матча', 'матчей')}</span>
                    </span>
                    <span class="elo mono">{row.rating ? Math.round(Number(row.rating)) : '—'}</span>
                </div>

                <div class="cells">
                    <span class="cell">
                        <span class="cell-value green">{row.wins}</span>
                        <span class="cell-label">побед · {share(row.wins, row.matches)}</span>
                    </span>
                    <span class="cell">
                        <span class="cell-value red">{row.losses}</span>
                        <span class="cell-label">проигр · {share(row.losses, row.matches)}</span>
                    </span>
                    <span class="cell">
                        <span class="cell-value">{row.avgPlace ? Number(row.avgPlace).toFixed(2) : '—'}</span>
                        <span class="cell-label">ср. место</span>
                    </span>
                    <span class="cell">
                        <span class="cell-value" class:red={row.royals > 0}>{row.royals}</span>
                        <span class="cell-label">королевских</span>
                    </span>
                    <span class="cell">
                        <span class="cell-value">{row.hung}</span>
                        <span class="cell-label">навесил</span>
                    </span>
                </div>
            </button>
        {:else}
            <p class="muted centered">Ещё никто не доиграл ни одного матча.</p>
        {/each}
    </div>

    <p class="mono foot">
        Навесы считаются только по онлайн-матчам: за настоящим столом их никто не
        записывает.
    </p>
{/if}

<style>
    .totals {
        display: grid;
        grid-template-columns: repeat(4, 1fr);
        gap: 8px;
    }

    .total {
        display: flex;
        flex-direction: column;
        align-items: center;
        gap: 3px;
        min-width: 0;
    }

    .value {
        font-family: var(--display);
        font-size: 21px;
        font-weight: 600;
        line-height: 1;
        font-variant-numeric: tabular-nums;
    }

    .label {
        font-size: 10px;
        color: var(--text-45);
        text-align: center;
    }

    .sorts {
        display: flex;
        flex-wrap: wrap;
        gap: 6px;
    }

    .chip {
        padding: 5px 11px;
        border: 1px solid var(--line);
        border-radius: 999px;
        background: none;
        color: var(--text-55);
        font: inherit;
        font-size: 12px;
    }

    .chip.on {
        border-color: var(--gold-soft);
        background: rgba(240, 205, 138, 0.12);
        color: var(--gold);
    }

    .rows {
        display: flex;
        flex-direction: column;
        gap: 8px;
    }

    /*
      ⚠️ align-items: stretch ОБЯЗАТЕЛЕН. Карточка — это <button>, а браузер задаёт
      кнопке align-items: center; в колоночном флексе это сжимает всё содержимое
      в узкий столбик по центру вместо строк во всю ширину.
    */
    .row {
        display: flex;
        flex-direction: column;
        align-items: stretch;
        gap: 10px;
        width: 100%;
        text-align: left;
    }

    /*
      ⚠️ Своя строка НЕ гаснет, хотя она и disabled: нажимать на себя незачем — свой
      профиль уже открыт, — но глобальное правило button:disabled гасит её до 40%,
      и подсветка «это ты» пропадала вместе с ней.
    */
    .row:disabled {
        opacity: 1;
        cursor: default;
    }

    .row.mine {
        border-color: var(--gold-soft);
        background: rgba(240, 205, 138, 0.1);
    }

    .head-line {
        display: flex;
        align-items: center;
        gap: 10px;
        min-width: 0;
    }

    .rank {
        width: 18px;
        flex: none;
        text-align: center;
        font-size: 12px;
        color: var(--text-30);
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

    .elo {
        flex: none;
        font-size: 16px;
        font-weight: 700;
        color: var(--gold);
        font-variant-numeric: tabular-nums;
    }

    /*
      ⚠️ Пять колонок в ряд на телефоне не помещаются — они переносятся сеткой,
      а не сжимаются: сжатая подпись «королевских» обрезалась бы на «королевс…».
    */
    .cells {
        display: grid;
        grid-template-columns: repeat(auto-fit, minmax(78px, 1fr));
        gap: 8px;
        padding-top: 9px;
        border-top: 1px solid var(--line);
    }

    .cell {
        display: flex;
        flex-direction: column;
        gap: 2px;
        min-width: 0;
    }

    .cell-value {
        font-size: 15px;
        font-weight: 700;
        font-variant-numeric: tabular-nums;
    }

    .cell-value.green {
        color: var(--green);
    }

    .cell-value.red {
        color: var(--red);
    }

    .cell-label {
        font-size: 10px;
        color: var(--text-45);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }

    .centered {
        text-align: center;
        padding: 20px 0;
    }

    .foot {
        margin: 0;
        font-size: 11px;
        color: var(--text-45);
        line-height: 1.45;
    }
</style>
