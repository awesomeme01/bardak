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
    <p class="strip">Нет сети — ходы уйдут, как только связь вернётся</p>
{:else if tableLost}
    <p class="strip">Связь со столом потеряна — восстанавливаю</p>
{/if}

{#if pwa.updateReady && inMatch}
    <p class="strip quiet">Обновление готово — применим, когда матч закончится</p>
{/if}

{#if pwa.pushError}
    <p class="strip">{pwa.pushError}</p>
{/if}

<!-- ⭐ Установка и уведомления не должны занимать место в игре: это тонкая строка,
     а не панель поверх стола. -->
{#if (pwa.installPrompt || canSubscribe) && !inMatch}
    <div class="offers">
        {#if pwa.installPrompt}
            <button class="offer" type="button" onclick={installApp}>Поставить на телефон</button>
        {/if}
        {#if canSubscribe}
            <!-- Разрешение спрашивается только по нажатию: см. stores/pwa.svelte.js. -->
            <button class="offer" type="button" onclick={enablePush}>Уведомлять о моём ходе</button>
        {/if}
    </div>
{/if}

<style>
    .strip {
        margin: 0;
        padding: 9px 20px;
        background: rgba(240, 205, 138, 0.12);
        color: var(--gold);
        font: 500 12px var(--mono);
        text-align: center;
    }

    .strip.quiet {
        background: rgba(255, 255, 255, 0.06);
        color: var(--text-55);
    }

    .offers {
        /* Предложения — в самый низ экрана: это подсказки, а не то, ради чего пришли. */
        order: 99;
        margin-top: auto;
        display: flex;
        justify-content: center;
        gap: 14px;
        padding: 6px 20px 10px;
    }

    .offer {
        font: 500 11px var(--mono);
        color: var(--text-45);
        text-decoration: underline;
        text-underline-offset: 3px;
    }
</style>
