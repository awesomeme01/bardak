<script>
    /**
     * Компактная шапка вместо панели «Профиль».
     *
     * ⭐ Профиль занимал треть экрана на телефоне и не помогал игре. Здесь ровно то, что
     * человек хочет знать между партиями: кто он и какой у него рейтинг.
     */
    import {logout} from '../stores/auth.svelte.js';
    import {profile} from '../stores/profile.svelte.js';
    import Avatar from './Avatar.svelte';

    let {onRefresh = null, onHistory = null} = $props();
</script>

<header class="bar">
    <div class="who">
        <Avatar userId={profile.user?.id} size={40} active/>
        <div>
            <div class="name">{profile.user?.displayName ?? '…'}</div>
            <div class="mono">
                рейтинг <span class="gold">{profile.rating ?? '—'}</span> · матчей {profile.matches}
            </div>
        </div>
    </div>
    <div class="row">
        {#if onRefresh}
            <button class="icon-btn" type="button" onclick={onRefresh} aria-label="Обновить">↻</button>
        {/if}
        {#if onHistory}
            <button class="icon-btn" type="button" onclick={onHistory} aria-label="История">≡</button>
        {/if}
        <button class="icon-btn" type="button" onclick={logout} aria-label="Выйти">⏻</button>
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

    .who {
        display: flex;
        align-items: center;
        gap: 11px;
        min-width: 0;
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
