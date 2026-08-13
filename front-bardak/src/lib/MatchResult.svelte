<script>
    /**
     * Итог матча.
     *
     * ⭐ Экран строится вокруг степени проигрыша, а не вокруг таблицы мест. Степень — это
     * то, что игроки пересказывают друг другу и скриншотят; «Королевский» должен выглядеть
     * приговором, а не строкой отчёта.
     */
    import Card from './Card.svelte';
    import Avatar from './Avatar.svelte';
    import {table} from '../stores/table.svelte.js';
    import {profile} from '../stores/profile.svelte.js';
    import {degreeName, deltaText, levelName} from './naming.js';

    let {onClose, onHistory} = $props();

    const players = $derived(table.result?.players ?? []);
    const lastAttack = $derived(table.result?.lastAttackCards ?? []);
    const eights = $derived(lastAttack.filter((code) => code.startsWith('8-')).length);
    const mine = $derived(players.find((player) => player.userId === profile.user?.id));
    const loser = $derived(players.find((player) => player.lossDegree));

    /** Пояснение к степени: почему именно эта, а не соседняя (§0.3). */
    const WHY = {
        ROYAL: 'Джокер, проигранная раздача и ровно четыре восьмёрки в последней атаке. Реже не бывает.',
        SUPER_MEGA_SUCK: 'Джокер, проигранная раздача и восьмёрки в последней атаке.',
        SUPER_MEGA_FAIL: 'Джокер навесили картой, и раздача тоже проиграна.',
        SUPER_FAIL: 'В навесе был туз, а проигранная раздача добавила ступень.',
        FAIL: 'Джокер в навесе, но раздачу игрок не проиграл.',
    };

    const iLost = $derived(Boolean(mine?.lossDegree));
</script>

<div class="screen" class:royal={loser?.lossDegree === 'ROYAL'}>
    <div class="head">
        <div class="label">Матч окончен</div>
        {#if loser}
            <span class="joker"><Card code="Joker-1" width={86}/></span>
            <h1 class="degree">{degreeName(loser.lossDegree)}</h1>
            <p class="why">{WHY[loser.lossDegree] ?? ''}</p>
            <p class="who muted">
                {iLost ? 'Это ты.' : `${loser.displayName} — до джокера доехал он.`}
            </p>
        {:else}
            <h1 class="degree">Матч окончен</h1>
        {/if}
    </div>

    {#if lastAttack.length}
        <!-- ⭐ Восьмёрки в последней атаке — это и есть разница между «Королевским»
             и «Супер-мега-саком» (§0.3). Показываем карты, а не пересказ. -->
        <div class="last-attack" class:royal-box={loser?.lossDegree === 'ROYAL'}>
            <div class="label">Последняя атака{#if eights} · восьмёрки{/if}</div>
            <div class="attack-cards">
                {#each lastAttack as code (code)}
                    <Card {code} width={52}/>
                {/each}
            </div>
            {#if eights === 4}
                <p class="mono note">четыре восьмёрки — дальше некуда</p>
            {:else if eights > 0}
                <p class="mono note">{eights === 1 ? 'одна восьмёрка' : `${eights} восьмёрки`} —
                    не четыре, до «Королевского» не дотянул</p>
            {/if}
        </div>
    {/if}

    <div class="places">
        {#each players as player (player.userId)}
            <div class="card place" class:mine={player.userId === profile.user?.id}>
                <span class="rank">{player.place}</span>
                <Avatar userId={player.userId} size={38}/>
                <div class="grow">
                    <div class="name">{player.displayName}</div>
                    <div class="mono">навес: {levelName(player.navesLevel)}</div>
                </div>
                <span class="delta" class:up={Number(player.ratingDelta) > 0}
                      class:down={Number(player.ratingDelta) < 0}>
                    {deltaText(player.ratingDelta)}
                </span>
            </div>
        {/each}
    </div>

    <div class="bottom-bar">
        <button class="btn grow" type="button" onclick={onClose}>Ещё матч</button>
        <button class="btn-ghost" type="button" onclick={onHistory}>Разбор</button>
    </div>
</div>

<style>
    .screen {
        flex: 1;
        display: flex;
        flex-direction: column;
        gap: 18px;
        padding: 22px 20px 0;
        overflow-y: auto;
    }

    /* Королевский — раз в сто партий: даёт золотой отсвет поверх красного. */
    .royal::before {
        content: '';
        position: fixed;
        inset: 0 0 auto;
        height: 340px;
        background: conic-gradient(from 210deg at 50% -10%, rgba(240, 205, 138, 0.14),
                                   transparent 40%, rgba(240, 205, 138, 0.1) 70%, transparent);
        pointer-events: none;
    }

    .head {
        text-align: center;
        display: flex;
        flex-direction: column;
        align-items: center;
        gap: 12px;
    }

    .joker :global(.playing-card) {
        border-radius: 10px;
        box-shadow: 0 18px 40px rgba(0, 0, 0, 0.7), 0 0 0 2px var(--gold-soft);
    }

    .degree {
        font-size: 34px;
        color: var(--red);
    }

    .why {
        font-size: 13px;
        line-height: 1.55;
        color: var(--text-55);
        text-wrap: pretty;
    }

    .who {
        font-size: 13px;
    }

    .last-attack {
        padding: 14px 16px;
        border-radius: 16px;
        border: 1px solid rgba(232, 85, 95, 0.4);
        background: rgba(232, 85, 95, 0.1);
        display: flex;
        flex-direction: column;
        gap: 11px;
        align-items: center;
    }

    .last-attack .label {
        color: #ff9ba2;
    }

    .royal-box {
        border-color: var(--gold-soft);
        background: rgba(240, 205, 138, 0.1);
    }

    .royal-box .label {
        color: var(--gold);
    }

    .attack-cards {
        display: flex;
        gap: 8px;
        flex-wrap: wrap;
        justify-content: center;
    }

    .note {
        text-align: center;
    }

    .places {
        display: flex;
        flex-direction: column;
        gap: 10px;
    }

    .place {
        display: flex;
        align-items: center;
        gap: 12px;
    }

    .place.mine {
        border-color: var(--gold-soft);
    }

    .rank {
        width: 22px;
        font-family: var(--mono);
        font-size: 15px;
        color: var(--text-45);
    }

    .grow {
        flex: 1;
        min-width: 0;
    }

    .name {
        font-size: 15px;
        font-weight: 700;
    }

    .delta {
        font-family: var(--mono);
        font-size: 14px;
        color: var(--text-55);
    }

    .delta.up {
        color: var(--green);
    }

    .delta.down {
        color: #ff6b75;
    }

    .bottom-bar {
        margin-top: auto;
        padding: 16px 0 24px;
        display: flex;
        gap: 10px;
    }

    .bottom-bar .grow {
        flex: 2;
    }

    .bottom-bar :global(.btn-ghost) {
        flex: 1;
        height: 58px;
    }
</style>
