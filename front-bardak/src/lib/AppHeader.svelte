<script>
    /**
     * Компактная шапка вместо панели «Профиль».
     *
     * ⭐ Профиль занимал треть экрана на телефоне и не помогал игре. Здесь ровно то, что
     * человек хочет знать между партиями: кто он и какой у него рейтинг.
     */
    import {profile} from '../stores/profile.svelte.js';
    import Avatar from './Avatar.svelte';

    let {onRefresh = null, onHistory = null, onProfile = null, onStats = null,
        onFriends = null, onLeaders = null} = $props();
</script>

<header class="bar">
    <button class="who" type="button" onclick={onProfile}>
        <Avatar userId={profile.user?.id} avatar={profile.user?.avatar} size={40} active/>
        <div>
            <div class="name">{profile.user?.displayName ?? '…'}</div>
            <div class="mono">матчей {profile.matches}</div>
        </div>
    </button>
    <!--
      ⭐ Рейтинг — отдельная кнопка, и ведёт она в ТАБЛИЦУ, а не в профиль. Своё число
      без чужих ничего не значит: первый же вопрос к рейтингу — «а у остальных сколько».
      Раньше он был частью кнопки профиля, и попасть из него в таблицу можно было только
      через отдельный значок, о котором никто не догадывался.
    -->
    <button class="elo" type="button" onclick={onLeaders} aria-label="Таблица рейтинга">
        <span class="elo-value">{profile.rating ?? '—'}</span>
        <span class="elo-label mono">эло</span>
    </button>
    <div class="row">
        {#if onRefresh}
            <button class="icon-btn" type="button" onclick={onRefresh} aria-label="Обновить">↻</button>
        {/if}
        {#if onFriends}
            <button class="icon-btn" type="button" onclick={onFriends} aria-label="Друзья">👥</button>
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

    /* Рейтинг читается на бегу, поэтому он крупный и золотой, а подпись — мелкая. */
    .elo {
        display: flex;
        flex-direction: column;
        align-items: center;
        gap: 1px;
        flex: none;
        margin-left: auto;
        padding: 4px 12px;
        border: 1px solid var(--gold-soft);
        border-radius: 12px;
        background: rgba(240, 205, 138, 0.1);
        color: inherit;
    }

    .elo-value {
        font-family: var(--display);
        font-size: 19px;
        font-weight: 600;
        line-height: 1;
        color: var(--gold);
        font-variant-numeric: tabular-nums;
    }

    .elo-label {
        font-size: 9px;
        letter-spacing: 0.08em;
        text-transform: uppercase;
    }

    .mono {
        margin-top: 3px;
    }
</style>
