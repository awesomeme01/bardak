<script>
    /**
     * Стол: комната ожидания до матча, игровой экран во время него, итог после.
     *
     * ⭐ Соединение принадлежит стору, а не этому экрану: уход в историю посреди партии
     * не должен ставить её на паузу (ADR-049).
     */
    import {onDestroy, onMount} from 'svelte';
    import GameTable from './GameTable.svelte';
    import MatchResult from './MatchResult.svelte';
    import WaitingRoom from './WaitingRoom.svelte';
    import {detachTable, enterTable, leaveTable, table} from '../stores/table.svelte.js';

    let {info, onExit, onHistory} = $props();

    onMount(() => enterTable(info));
    // Экран закрылся — соединение остаётся, если идёт матч (см. detachTable).
    onDestroy(() => detachTable());

    function leave() {
        leaveTable();
        onExit();
    }
</script>

{#if table.notice}
    <p class="floating-notice notice">{table.notice}</p>
{/if}

{#if table.result}
    <MatchResult onClose={() => (table.result = null)} {onHistory}/>
{:else if table.game}
    <GameTable/>
{:else}
    <WaitingRoom onExit={leave} fallback={info}/>
{/if}

<style>
    /* Плашка висит поверх стола: она сообщает о случившемся, а не сдвигает игру. */
    .floating-notice {
        position: fixed;
        left: 50%;
        top: calc(10px + env(safe-area-inset-top));
        transform: translateX(-50%);
        z-index: 40;
        max-width: min(92vw, 420px);
        text-align: center;
        box-shadow: 0 10px 30px rgba(0, 0, 0, 0.5);
    }
</style>
