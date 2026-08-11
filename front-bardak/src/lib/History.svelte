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

<section class="card">
    <h2>Рейтинг</h2>
    {#if history.rating}
        <p><strong>{Number(history.rating.rating).toFixed(0)}</strong> · матчей: {history.rating.matchesPlayed}</p>
        <RatingChart points={history.rating.history}/>
    {:else}
        <p>Загружаю…</p>
    {/if}
</section>

<section class="card">
    <h2>История матчей</h2>
    {#if history.error}
        <p class="badge badge-fail">{history.error}</p>
    {/if}
    {#if history.loading}
        <p>Загружаю…</p>
    {:else if !history.matches.length}
        <p>Сыгранных матчей пока нет.</p>
    {/if}

    <ul class="matches">
        {#each history.matches as match (match.id)}
            <li>
                <button type="button" class="row-button" onclick={() => openMatch(match.id)}>
                    <span>{when(match.startedAt)}</span>
                    <span>{match.playersCount} игрока · раздач: {match.dealsPlayed}</span>
                    {#if match.status === 'ABORTED'}
                        <!-- Отменённый матч виден с причиной и не влияет на рейтинг (§5.3). -->
                        <span class="badge badge-warn">отменён: {match.abortReason ?? 'без причины'}</span>
                    {:else}
                        <span class="badge {match.myPlace === 1 ? 'badge-ok' : 'badge-wait'}">
                            место {match.myPlace ?? '—'}
                        </span>
                        <span>{deltaText(match.myRatingDelta)}</span>
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
    }

    .row-button {
        display: flex;
        gap: 0.75rem;
        align-items: center;
        width: 100%;
        text-align: left;
        background: none;
        border: none;
        border-bottom: 1px solid rgba(255, 255, 255, 0.12);
        padding: 0.5rem 0;
        color: inherit;
        cursor: pointer;
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
</style>
