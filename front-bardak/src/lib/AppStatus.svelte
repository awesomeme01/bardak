<script>
    /**
     * Полоска состояния приложения: сеть, установка, готовое обновление.
     *
     * ⭐ Обновление посреди партии не применяется само. Перезагрузка стоит хода, а иногда
     * и партии: сокет рвётся, таймер хода идёт. Пока идёт матч — только предупреждение,
     * применяем, когда стол свободен.
     */
    import {applyUpdate, enablePush, installApp, pwa} from '../stores/pwa.svelte.js';
    import {session} from '../stores/auth.svelte.js';
    import {table} from '../stores/table.svelte.js';

    // ⭐ Подписка привязана к игроку и требует токена: незалогиненному эта кнопка
    // предлагает то, чего он сделать не может, и упирается в 401.
    const canSubscribe = $derived(session.status === 'authenticated' && !pwa.pushEnabled);

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

{#if pwa.pushError}
    <p class="badge badge-warn">{pwa.pushError}</p>
{/if}

{#if pwa.installPrompt || canSubscribe}
    <div class="row">
        {#if pwa.installPrompt}
            <button type="button" onclick={installApp}>Поставить на домашний экран</button>
        {/if}
        {#if canSubscribe}
            <!-- Разрешение спрашивается только по нажатию: см. stores/pwa.svelte.js. -->
            <button type="button" onclick={enablePush}>Уведомлять о моём ходе</button>
        {/if}
    </div>
{/if}
