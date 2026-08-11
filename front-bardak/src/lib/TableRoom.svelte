<script>
    import {onDestroy, onMount} from 'svelte';
    import {applyTableEvent, leaveTable, lobby} from '../stores/lobby.svelte.js';
    import {WsClient} from '../net/ws-client.js';

    let {onExit} = $props();

    let status = $state('idle');
    let ready = $state(false);
    let client = null;

    const table = $derived(lobby.current);
    const everyoneReady = $derived(
        table && table.seats.length >= 2 && table.seats.every((seat) => seat.ready));

    onMount(() => {
        client = new WsClient({
            onStatus: (next) => (status = next),
            onEvent: applyTableEvent,
        });
        client.connect().then(() => client.send('TABLE_JOIN', {}, table.id));
    });

    onDestroy(() => client?.close());

    function toggleReady() {
        ready = !ready;
        client?.send('TABLE_READY', {ready}, table.id);
    }

    function exit() {
        client?.send('TABLE_LEAVE', {}, table.id);
        client?.close();
        leaveTable();
        onExit();
    }
</script>

<section class="card">
    <h2>{table.name}</h2>
    <p>Код приглашения: <code>{table.code}</code> · <span class="badge {status === 'open' ? 'badge-ok' : 'badge-wait'}">{status}</span></p>

    <ul class="seats">
        {#each Array(table.maxPlayers) as _, seatNo}
            {@const seat = table.seats.find((candidate) => candidate.seatNo === seatNo)}
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

    {#if everyoneReady}
        <p class="badge badge-ok">Все готовы — можно начинать (матч подключим на M4)</p>
    {/if}

    <div class="row">
        <button type="button" onclick={toggleReady}>{ready ? 'Я не готов' : 'Я готов'}</button>
        <button type="button" onclick={exit}>Выйти из-за стола</button>
    </div>
</section>
