<script>
    /**
     * Статистика игрока.
     *
     * ⭐ Числа считает сервер по истории матчей. Клиент их не выводит и не копит: два
     * источника одной правды однажды разъезжаются, и потом не понять, какому верить.
     */
    import {onMount} from 'svelte';
    import {apiGet} from '../net/rest-client.js';
    import RatingChart from './RatingChart.svelte';
    import {degreeName} from './naming.js';

    let {onBack} = $props();

    let stats = $state(null);
    let rating = $state(null);
    let error = $state(null);

    onMount(async () => {
        try {
            [stats, rating] = await Promise.all([apiGet('/stats/me'), apiGet('/rating/me')]);
        } catch (e) {
            error = e.message;
        }
    });

    const streak = $derived.by(() => {
        if (!stats?.streak || stats.streak.kind === 'NONE') {
            return null;
        }
        const won = stats.streak.kind === 'WIN';
        return {
            text: `${won ? 'W' : 'L'}${stats.streak.length}`,
            hint: won ? 'подряд выигранных' : 'подряд проигранных',
            won,
        };
    });
</script>

<div class="screen">
    <div class="head">
        <button class="icon-btn" type="button" onclick={onBack} aria-label="Назад">←</button>
        <h1>Статистика</h1>
    </div>

    {#if error}
        <p class="notice notice-fail">{error}</p>
    {:else if !stats}
        <p class="muted">Считаю…</p>
    {:else if stats.matches === 0}
        <p class="muted">Ещё ни одного сыгранного матча. Сядь за стол — и здесь появятся числа.</p>
    {:else}
        <div class="tiles">
            <div class="card tile">
                <span class="value">{rating ? Math.round(Number(rating.rating)) : '—'}</span>
                <span class="label">рейтинг</span>
            </div>
            <div class="card tile">
                <span class="value">{stats.matches}</span>
                <span class="label">матчей</span>
            </div>
            <div class="card tile">
                <span class="value green">{stats.wins}</span>
                <span class="label">побед</span>
            </div>
            <div class="card tile">
                <span class="value red">{stats.losses}</span>
                <span class="label">проигрышей</span>
            </div>
            <div class="card tile">
                <span class="value">{Number(stats.avgPlace).toFixed(2)}</span>
                <span class="label">среднее место</span>
            </div>
            <div class="card tile">
                <span class="value">{stats.dealsPlayed}</span>
                <span class="label">раздач</span>
            </div>
        </div>

        {#if streak}
            <div class="card streak" class:won={streak.won}>
                <span class="value">{streak.text}</span>
                <span class="mono">{streak.hint}</span>
            </div>
        {/if}

        <div class="card block">
            <span class="label">Рейтинг по матчам</span>
            <RatingChart points={rating?.history ?? []}/>
            <div class="line mono">
                <span>лучший</span>
                <span>{stats.bestRating ? Math.round(Number(stats.bestRating)) : '—'}</span>
            </div>
            <div class="line mono">
                <span>худший</span>
                <span>{stats.worstRating ? Math.round(Number(stats.worstRating)) : '—'}</span>
            </div>
        </div>

        {#if stats.degrees.length}
            <div class="card block">
                <!-- ⭐ Степени идут от самой тяжёлой к обычной — так они и объявлены (§0.3). -->
                <span class="label">Чем заканчивались проигрыши</span>
                {#each stats.degrees as row (row.degree)}
                    <div class="line">
                        <span class="degree">{degreeName(row.degree)}</span>
                        <span class="mono count">{row.count}</span>
                    </div>
                {/each}
            </div>
        {/if}
    {/if}
</div>

<style>
    .screen {
        flex: 1;
        display: flex;
        flex-direction: column;
        gap: 14px;
        padding: 16px 20px 28px;
        overflow-y: auto;
    }

    .head {
        display: flex;
        align-items: center;
        gap: 12px;
    }

    .tiles {
        display: grid;
        grid-template-columns: repeat(3, 1fr);
        gap: 10px;
    }

    .tile {
        display: flex;
        flex-direction: column;
        align-items: center;
        gap: 4px;
        padding: 14px 8px;
        text-align: center;
    }

    .value {
        font-family: var(--display);
        font-size: 26px;
        font-weight: 600;
        line-height: 1;
        font-variant-numeric: tabular-nums;
    }

    .green {
        color: var(--green);
    }

    .red {
        color: var(--red);
    }

    .streak {
        display: flex;
        align-items: center;
        gap: 12px;
        border-color: rgba(232, 132, 140, 0.35);
        background: rgba(232, 132, 140, 0.08);
    }

    .streak.won {
        border-color: rgba(127, 216, 166, 0.35);
        background: rgba(127, 216, 166, 0.08);
    }

    .block {
        display: flex;
        flex-direction: column;
        gap: 10px;
    }

    .line {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: 12px;
    }

    .degree {
        font-size: 14px;
        color: var(--red);
    }

    .count {
        font-size: 14px;
        color: var(--text);
    }

    @media (min-width: 900px) {
        .screen {
            padding: 24px 32px 32px;
        }

        .tiles {
            grid-template-columns: repeat(6, 1fr);
        }
    }
</style>
