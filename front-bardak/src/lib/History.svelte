<script>
    import {onMount} from 'svelte';
    import {closeReplay, history, loadHistory, loadReplay, openMatch} from '../stores/history.svelte.js';
    import RatingChart from './RatingChart.svelte';
    import {degreeName, deltaText, levelName, reasonName, suitName} from './naming.js';

    onMount(loadHistory);

    function when(iso) {
        return iso ? new Date(iso).toLocaleString('ru-RU') : '—';
    }
</script>

<section class="block-card">
    <span class="label">Рейтинг</span>
    {#if history.rating}
        <p><strong>{Number(history.rating.rating).toFixed(0)}</strong> · матчей: {history.rating.matchesPlayed}</p>
        <RatingChart points={history.rating.history}/>
    {:else}
        <p>Загружаю…</p>
    {/if}
</section>

<section class="block-card">
    <span class="label">История матчей</span>
    {#if history.error}
        <p class="notice notice-fail">{history.error}</p>
    {/if}
    {#if history.loading}
        <p>Загружаю…</p>
    {:else if !history.matches.length}
        <p>Сыгранных матчей пока нет.</p>
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
                        <span class="pill">отменён</span>
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
                            {#if history.replay}
                                <button type="button" onclick={closeReplay}>Скрыть реплей</button>
                            {:else}
                                <button type="button" onclick={() => loadReplay(match.id)}>Реплей</button>
                            {/if}
                        </div>

                        {#if history.replay}
                            <!-- ⭐ Реплей приходит уже отфильтрованным: чужого в нём нет. -->
                            <ol class="replay">
                                {#each history.replay.events as event (event.seq)}
                                    <li>
                                        <code>{event.seq}</code> {event.type}
                                        {#if event.payload?.cardCode}· {event.payload.cardCode}{/if}
                                        {#if event.payload?.seatNo !== undefined}· место {event.payload.seatNo + 1}{/if}
                                    </li>
                                {/each}
                            </ol>
                        {/if}
                    </div>
                {/if}
            </li>
        {/each}
    </ul>
</section>

<style>
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
