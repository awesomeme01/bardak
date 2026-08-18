<script>
    import {onMount} from 'svelte';
    import {history, loadHistory, loadReplay, openMatch} from '../stores/history.svelte.js';
    import RatingChart from './RatingChart.svelte';
    import {degreeName, deltaText, levelName, reasonName, suitName} from './naming.js';

    onMount(loadHistory);

    /**
     * ⚠️ Число матчей в рейтинге и длина этого списка — разные величины, и раньше они
     * молча спорили на экране: сверху «матчей: 1», ниже три строки.
     *
     * Спор не в данных: отменённый матч в счёт не идёт (§5.3), но из истории не исчезает —
     * посмотреть, что там было, надо и по нему. Поэтому список сам говорит, сколько строк
     * засчитано, а сколько нет, и оба числа сходятся.
     */
    const counted = $derived(history.matches.filter((match) => match.status === 'FINISHED').length);
    const aborted = $derived(history.matches.filter((match) => match.status === 'ABORTED').length);
    const running = $derived(history.matches.filter((match) => match.status === 'IN_PROGRESS').length);

    function when(iso) {
        return iso ? new Date(iso).toLocaleString('ru-RU') : '—';
    }
</script>

<section class="block-card">
    <span class="label">Рейтинг</span>
    {#if history.rating}
        <p><strong>{Number(history.rating.rating).toFixed(0)}</strong> ·
            засчитанных матчей: {history.rating.matchesPlayed}</p>
        <RatingChart points={history.rating.history}/>
    {:else}
        <p>Загружаю…</p>
    {/if}
</section>

<section class="block-card">
    <span class="label">История матчей</span>
    {#if history.matches.length}
        <p class="mono tally">
            засчитано {counted}{#if running} · идёт {running}{/if}{#if aborted} · отменено {aborted}{/if}
            {#if aborted || running} — в рейтинг идут только законченные{/if}
        </p>
    {/if}
    {#if history.error}
        <p class="notice notice-fail">{history.error}</p>
    {/if}
    {#if history.loading}
        <p>Загружаю…</p>
    {:else if !history.matches.length}
        <!-- Пустая история объясняет, что в ней будет: иначе экран читается как поломка. -->
        <p class="muted">Сыгранных матчей пока нет. После первой партии здесь появится список —
            с местами, степенями проигрыша и разбором по раздачам.</p>
    {/if}

    <ul class="matches">
        {#each history.matches as match (match.id)}
            <li>
                <button type="button" class="card match" onclick={() => openMatch(match.id)}>
                    <span class="grow">
                        <span class="when">{when(match.startedAt)}</span>
                        <span class="mono block">{match.playersCount} игрока · раздач: {match.dealsPlayed}</span>
                    </span>
                    {#if match.status === 'ABORTED'}
                        <!-- Отменённый матч виден с причиной и не влияет на рейтинг (§5.3). -->
                        <span class="pill">отменён · не в счёт</span>
                    {:else if match.status === 'IN_PROGRESS'}
                        <!-- Идущий матч в истории уже есть, но места и рейтинга у него ещё нет. -->
                        <span class="pill pill-turn">идёт сейчас</span>
                    {:else}
                        <span class="pill" class:pill-ready={match.myPlace === 1}>
                            место {match.myPlace ?? '—'}
                        </span>
                        <span class="delta" class:up={Number(match.myRatingDelta) > 0}
                              class:down={Number(match.myRatingDelta) < 0}>
                            {deltaText(match.myRatingDelta)}
                        </span>
                    {/if}
                </button>

                {#if history.details?.match?.id === match.id}
                    <div class="details">
                        <ol class="players">
                            {#each history.details.match.players as player (player.userId)}
                                <li>
                                    {player.place ?? '—'}. {player.displayName} ·
                                    навес: {levelName(player.navesLevel)}
                                    {#if player.lossType}· <em>{degreeName(player.lossType)}</em>{/if}
                                    · {deltaText(player.ratingDelta)}
                                </li>
                            {/each}
                        </ol>

                        {#each history.details.deals as deal (deal.dealNo)}
                            <div class="deal">
                                <h4>Раздача {deal.dealNo} · козырь {suitName(deal.trumpSuit)}</h4>
                                {#each deal.seats as seat (seat.seatNo)}
                                    <p>
                                        место {seat.seatNo + 1}: {levelName(seat.navesLevelBefore)}
                                        → {levelName(seat.navesLevelAfter)}
                                        {#if seat.levelChanges.length}
                                            ({seat.levelChanges
                                            .map((change) => `${reasonName(change.reason)} ${change.amount > 0 ? '+' : ''}${change.amount}`)
                                            .join(', ')})
                                        {/if}
                                        {#if seat.hungCards.length}· навесили: {seat.hungCards.join(', ')}{/if}
                                    </p>
                                {/each}
                            </div>
                        {/each}

                        <div class="row">
                            <button class="btn-small" type="button"
                                    onclick={() => loadReplay(match.id)}>Смотреть реплей</button>
                        </div>

                    </div>
                {/if}
            </li>
        {/each}
    </ul>
</section>

<style>
    /* Сверка чисел: сколько строк в списке пошло в рейтинг, а сколько нет. */
    .tally {
        font-size: 11px;
        color: var(--text-45);
    }

    .matches {
        list-style: none;
        padding: 0;
        margin: 0;
        display: flex;
        flex-direction: column;
        gap: 10px;
    }

    .block-card {
        display: flex;
        flex-direction: column;
        gap: 12px;
        padding: 16px 20px 0;
    }

    .match {
        display: flex;
        align-items: center;
        gap: 10px;
        width: 100%;
        text-align: left;
        color: inherit;
    }

    .grow {
        flex: 1;
        min-width: 0;
    }

    .when {
        font-size: 14px;
        font-weight: 700;
    }

    .block {
        display: block;
        margin-top: 4px;
    }

    .delta {
        font-family: var(--mono);
        font-size: 13px;
        color: var(--text-55);
    }

    .delta.up {
        color: var(--green);
    }

    .delta.down {
        color: #ff6b75;
    }

    .details {
        padding: 0.5rem 0 1rem 1rem;
        font-size: 0.9rem;
    }

    .deal h4 {
        margin: 0.6rem 0 0.2rem;
    }

    .deal p {
        margin: 0.1rem 0;
        opacity: 0.85;
    }

    .replay {
        max-height: 16rem;
        overflow-y: auto;
        opacity: 0.85;
    }

    /* Десктоп: рейтинг и список матчей рядом — разбор раскрывается под списком. */
    @media (min-width: 900px) {
        .block-card {
            padding: 20px 32px 0;
        }

        .block-card:first-child {
            display: grid;
            grid-template-columns: 320px minmax(0, 1fr);
            gap: 24px;
            align-items: center;
        }
    }
</style>
