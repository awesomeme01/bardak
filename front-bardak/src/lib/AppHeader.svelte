<script>
    /**
     * Компактная шапка вместо панели «Профиль».
     *
     * ⭐ Профиль занимал треть экрана на телефоне и не помогал игре. Здесь ровно то, что
     * человек хочет знать между партиями: кто он и какой у него рейтинг.
     */
    import {profile} from '../stores/profile.svelte.js';
    import Avatar from './Avatar.svelte';

    let {onRefresh = null, onHistory = null, onProfile = null, onStats = null} = $props();
</script>

<header class="bar">
    <button class="who" type="button" onclick={onProfile}>
        <Avatar userId={profile.user?.id} avatar={profile.user?.avatar} size={40} active/>
        <div>
            <div class="name">{profile.user?.displayName ?? '…'}</div>
            <div class="mono">
                рейтинг <span class="gold">{profile.rating ?? '—'}</span> · матчей {profile.matches}
            </div>
        </div>
    </button>
    <div class="row">
        {#if onRefresh}
            <button class="icon-btn" type="button" onclick={onRefresh} aria-label="Обновить">↻</button>
        {/if}
        {#if onStats}
            <button class="icon-btn" type="button" onclick={onStats} aria-label="Статистика">📊</button>
        {/if}
        {#if onHistory}
            <button class="icon-btn" type="button" onclick={onHistory} aria-label="История">≡</button>
        {/if}
    </div>
</header>

<style>
    .bar {
        flex: none;
        padding: 10px 20px 14px;
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: 12px;
        border-bottom: 1px solid rgba(255, 255, 255, 0.07);
    }

    /* Шапка — вход в профиль: отдельной кнопки «настройки» на телефоне жалко места. */
    .who {
        display: flex;
        align-items: center;
        gap: 11px;
        min-width: 0;
        background: none;
        border: none;
        color: inherit;
        text-align: left;
        padding: 0;
    }

    .name {
        font-size: 15px;
        font-weight: 700;
        line-height: 1.1;
    }

    .mono {
        margin-top: 3px;
    }
</style>
