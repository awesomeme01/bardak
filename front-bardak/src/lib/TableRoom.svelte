<script>
    import {onDestroy, onMount} from 'svelte';
    import GameTable from './GameTable.svelte';
    import MatchResult from './MatchResult.svelte';
    import {detachTable, enterTable, leaveTable, setReady, startMatch, table} from '../stores/table.svelte.js';

    let {info, onExit} = $props();

    let ready = $state(false);


    // ⭐ Считаем по живому состоянию из стора, а не по снимку из REST: снимок сделан
    // в момент входа и про соседа, севшего секунду назад, ничего не знает.
    const seats = $derived(table.info?.seats ?? info.seats ?? []);
    const everyoneReady = $derived(seats.length >= 2 && seats.every((seat) => seat.ready));

    onMount(() => enterTable(info));
    // Экран закрылся — соединение остаётся, если идёт матч (см. detachTable).
    onDestroy(() => detachTable());

    function toggleReady() {
        ready = !ready;
        setReady(ready);
    }

    function exit() {
        leaveTable();
        onExit();
    }

    /** Свернуть: место и соединение остаются, игрок вернётся сюда сам. */
    function minimize() {
        onExit();
    }
</script>

{#if table.notice}
    <p class="badge badge-warn">{table.notice}</p>
{/if}

{#if table.result}
    <MatchResult onClose={() => (table.result = null)}/>
    <div class="row"><button type="button" onclick={exit}>Выйти из-за стола</button></div>
{:else if table.game}
    <GameTable/>
    <div class="row">
        <button type="button" onclick={minimize}>Свернуть — место останется за тобой</button>
    </div>
{:else}
    <section class="card">
        <h2>{info.name}</h2>
        <p>
            Код приглашения: <code>{info.code}</code> ·
            <span class="badge {table.status === 'open' ? 'badge-ok' : 'badge-wait'}">{table.status}</span>
        </p>

        <ul class="seats">
            {#each Array(info.maxPlayers) as _, seatNo}
                {@const seat = seats.find((s) => s.seatNo === seatNo)}
                <li>
                    <span class="seat-no">{seatNo + 1}</span>
                    {#if seat}
                        <span class:offline={!seat.online}>{seat.displayName}</span>
                        <span class="badge {seat.ready ? 'badge-ok' : 'badge-wait'}">
                            {seat.ready ? 'готов' : 'ждёт'}
                        </span>
                    {:else}
                        <span class="empty">свободно</span>
                    {/if}
                </li>
            {/each}
        </ul>

        <div class="row">
            <button type="button" onclick={toggleReady}>{ready ? 'Я не готов' : 'Я готов'}</button>
            <button type="button" onclick={startMatch} disabled={!everyoneReady}>Начать матч</button>
            <button type="button" onclick={exit}>Выйти из-за стола</button>
        </div>
    </section>
{/if}
