<script>
    /**
     * Версия сборки внизу экрана.
     *
     * ⭐ Нужна не из любви к номерам: когда игрок говорит «у меня не работает», первый
     * вопрос — какая у него версия. Установленное на домашний экран приложение живёт
     * своей копией оболочки и может отстать на несколько выкаток, ничем этого не выдав.
     */
    import {onMount} from 'svelte';
    import {applyUpdate} from '../stores/pwa.svelte.js';
    import {go} from '../stores/route.svelte.js';

    /** Версия, вшитая в этот бандл при сборке (VITE_APP_VERSION в Dockerfile). */
    const mine = import.meta.env.VITE_APP_VERSION ?? 'dev';

    let server = $state(null);

    onMount(async () => {
        try {
            // ⚠️ Мимо кэша: устаревшая копия ответа health — ровно та ложь, которую
            // эта строка и должна ловить.
            const response = await fetch('/api/health', {cache: 'no-store'});
            server = (await response.json())?.version ?? null;
        } catch {
            // Нет сети — просто не показываем «вышла новая»: молчание честнее догадки.
        }
    });

    /**
     * ⚠️ Сравниваем, только когда обе версии настоящие. В дев-сборке `mine` равно
     * «dev», и без этой проверки строка вечно кричала бы про новую версию.
     */
    const stale = $derived(Boolean(server) && mine !== 'dev' && server !== mine);

    /**
     * Обновиться.
     *
     * ⭐ Если Service Worker уже скачал новую оболочку, применяем её; иначе просто
     * перезагружаемся — свежий индекс подтянет остальное.
     */
    function refresh() {
        applyUpdate();
        setTimeout(() => window.location.reload(), 150);
    }
</script>

<div class="version mono">
    <button class="link" type="button" onclick={() => go('/changelog')}
            title="Что менялось в игре">версия {mine}</button>
    {#if stale}
        <span class="dot">·</span>
        <button class="fresh" type="button" onclick={refresh}>вышла новая — обновить</button>
    {/if}
</div>

<style>
    .version {
        display: flex;
        align-items: center;
        justify-content: center;
        gap: 6px;
        padding: 10px 12px calc(10px + env(safe-area-inset-bottom));
        font-size: 10px;
        color: var(--text-30);
    }

    .link {
        background: none;
        border: 0;
        padding: 0;
        font: inherit;
        color: var(--text-45);
        text-decoration: underline;
        text-underline-offset: 3px;
        cursor: pointer;
    }

    /* Новая версия — золотом: это единственное, ради чего строку читают. */
    .fresh {
        background: none;
        border: 0;
        padding: 0;
        font: inherit;
        color: var(--gold);
        cursor: pointer;
        text-decoration: underline;
        text-underline-offset: 3px;
    }

    .dot {
        opacity: 0.4;
    }
</style>
