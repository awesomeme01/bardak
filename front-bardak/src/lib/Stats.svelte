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

    /**
     * ⭐ Экран один на себя и на чужого игрока. Отдельный «почти такой же» разошёлся бы
     * с этим на первой же правке: показатели те же, сервер считает их одинаково, и
     * различие сводится к тому, чей идентификатор в пути.
     *
     * @param userId  чужой игрок; пусто — свои показатели
     */
    let {onBack, userId = null, name = null} = $props();

    let stats = $state(null);
    let rating = $state(null);
    let error = $state(null);

    const mine = $derived(!userId);

    onMount(async () => {
        try {
            const [statsPath, ratingPath] = userId
                ? [`/stats/users/${userId}`, `/rating/users/${userId}`]
                : ['/stats/me', '/rating/me'];
            [stats, rating] = await Promise.all([apiGet(statsPath), apiGet(ratingPath)]);
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
        <h1>{mine ? 'Статистика' : (name ?? rating?.displayName ?? 'Игрок')}</h1>
        {#if !mine}<span class="whose mono">чужие показатели</span>{/if}
    </div>

    {#if error}
        <p class="notice notice-fail">{error}</p>
    {:else if !stats}
        <p class="muted">Считаю…</p>
    {:else}
        <!--
          ⭐ Пустая статистика показывается целиком, а не заменяется одной строкой. Иначе
          новичок не знает, что вообще считается: он видит «матчей нет» и уходит. Числа
          стоят нулями — и сразу видно, за чем следить.
        -->
        {#if stats.matches === 0}
            <p class="card muted empty-note">
                {#if mine}
                    Здесь появятся числа, как только сыграешь первый матч. Считается всё сразу:
                    победы, среднее место, раздачи и рейтинг по матчам.
                {:else}
                    Этот игрок ещё не доиграл ни одного матча.
                {/if}
            </p>
        {/if}

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
                <span class="value">{stats.matches ? Number(stats.avgPlace).toFixed(2) : '—'}</span>
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

        <div class="card block">
            <!-- ⭐ Степени идут от самой тяжёлой к обычной — так они и объявлены (§0.3). -->
            <span class="label">Чем заканчивались проигрыши</span>
            {#each stats.degrees as row (row.degree)}
                <div class="line">
                    <span class="degree">{degreeName(row.degree)}</span>
                    <span class="mono count">{row.count}</span>
                </div>
            {:else}
                <p class="muted">Пока ни одного проигрыша — тут будет видно, насколько тяжёлого.</p>
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

    .empty-note {
        line-height: 1.5;
        font-size: 14px;
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
