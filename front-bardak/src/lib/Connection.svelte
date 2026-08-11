<script>
    import {onDestroy, onMount} from 'svelte';
    import {WsClient} from '../net/ws-client.js';

    // Панель соединения из M1: она осталась полезной — по ней видно, что сокет живой
    // и что тикет выдаётся. На M3 сюда приедет лобби.
    let status = $state('idle');
    let log = $state([]);
    let client = null;

    onMount(() => {
        client = new WsClient({
            onStatus: (next) => (status = next),
            onEvent: (envelope) => (log = [...log.slice(-9), `${envelope.type}`]),
        });
        client.connect();
    });

    onDestroy(() => client?.close());
</script>

<section class="card">
    <h2>Соединение <code>/ws</code></h2>
    <div class="row">
        <span class="badge {status === 'open' ? 'badge-ok' : 'badge-wait'}">{status}</span>
        <button type="button" onclick={() => client?.send('PING', {})}>PING</button>
    </div>
    <pre>{log.join('\n')}</pre>
</section>
