<script>
    /**
     * Полоска состояния приложения: сеть, установка, готовое обновление.
     *
     * ⭐ Обновление посреди партии не применяется само. Перезагрузка стоит хода, а иногда
     * и партии: сокет рвётся, таймер хода идёт. Пока идёт матч — только предупреждение,
     * применяем, когда стол свободен.
     */
    import {applyUpdate, installApp, pwa} from '../stores/pwa.svelte.js';
    import {table} from '../stores/table.svelte.js';

    const inMatch = $derived(table.game !== null && table.result === null);

    /**
     * ⭐ Связь со столом — не то же самое, что `navigator.onLine`.
     *
     * Браузер считает себя онлайн, пока есть сеть, даже если сервер лёг: за столом это
     * выглядит как «игра замерла», и игрок жмёт кнопки в пустоту. Показываем именно
     * состояние сокета.
     */
    const LOST = ['closed', 'error', 'offline', 'reconnecting', 'connecting'];
    const tableLost = $derived(table.info !== null && LOST.includes(table.status));

    $effect(() => {
        if (pwa.updateReady && !inMatch) {
            applyUpdate();
        }
    });
</script>

{#if !pwa.online}
    <p class="badge badge-warn">Нет сети — ходы уйдут, как только связь вернётся</p>
{:else if tableLost}
    <p class="badge badge-warn">Связь со столом потеряна — восстанавливаю</p>
{/if}

{#if pwa.updateReady && inMatch}
    <p class="badge badge-wait">Обновление готово — применим, когда матч закончится</p>
{/if}

{#if pwa.installPrompt}
    <div class="row">
        <button type="button" onclick={installApp}>Поставить на домашний экран</button>
    </div>
{/if}
