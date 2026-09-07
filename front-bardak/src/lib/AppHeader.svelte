<script>
    /**
     * Компактная шапка вместо панели «Профиль».
     *
     * ⭐ Профиль занимал треть экрана на телефоне и не помогал игре. Здесь ровно то, что
     * человек хочет знать между партиями: кто он и какой у него рейтинг.
     *
     * ⭐ На телефоне разделы уезжают под «три точки». Четыре значка в ряд съедали место
     * у имени, и длинное имя обрывалось на второй букве; к тому же значок без подписи
     * приходится угадывать. В меню у каждого пункта есть слово — угадывать нечего.
     * На широком экране места хватает, и значки остаются на виду.
     */
    import Avatar from './Avatar.svelte';
    import Icon from './Icon.svelte';
    import {profile} from '../stores/profile.svelte.js';

    let {onHistory = null, onProfile = null, onStats = null,
        onFriends = null, onLeaders = null} = $props();

    let open = $state(false);

    /** Пункты меню и значков — один список: разойтись они не должны. */
    const items = $derived([
        {key: 'friends', icon: 'friends', label: 'Друзья', run: onFriends},
        {key: 'stats', icon: 'stats', label: 'Статистика', run: onStats},
        {key: 'history', icon: 'history', label: 'История матчей', run: onHistory},
        {key: 'profile', icon: 'profile', label: 'Профиль', run: onProfile},
    ].filter((item) => item.run));

    function choose(item) {
        open = false;
        item.run();
    }

    /**
     * ⚠️ Меню закрывается по нажатию МИМО него, и слушатель висит на окне, а не на
     * подложке: подложка поверх экрана перехватывала бы прокрутку лобби на телефоне.
     */
    function closeOnOutside(event) {
        if (open && !event.target.closest('.menu-wrap')) {
            open = false;
        }
    }
</script>

<svelte:window onpointerdown={closeOnOutside}
               onkeydown={(e) => e.key === 'Escape' && (open = false)}/>

<header class="bar">
    <button class="who" type="button" onclick={onProfile}>
        <Avatar userId={profile.user?.id} avatar={profile.user?.avatar} size={40} active/>
        <span class="who-text">
            <span class="name">{profile.user?.displayName ?? '…'}</span>
            <span class="mono sub">матчей {profile.matches}</span>
        </span>
    </button>

    <!--
      ⭐ Рейтинг — отдельная кнопка, и ведёт она в ТАБЛИЦУ, а не в профиль. Своё число
      без чужих ничего не значит: первый же вопрос к рейтингу — «а у остальных сколько».
    -->
    <button class="elo" type="button" onclick={onLeaders} aria-label="Таблица рейтинга">
        <Icon name="trophy" size={13}/>
        <span class="elo-value">{profile.rating ?? '—'}</span>
    </button>

    <!-- Широкий экран: значки на виду. -->
    <div class="wide-row">
        {#each items as item (item.key)}
            <button class="icon-btn" type="button" onclick={item.run} aria-label={item.label}>
                <Icon name={item.icon}/>
            </button>
        {/each}
    </div>

    <!-- Телефон: те же разделы под «тремя точками», но со словами. -->
    <div class="menu-wrap">
        <button class="icon-btn" type="button" onclick={() => (open = !open)}
                aria-label="Разделы" aria-haspopup="menu" aria-expanded={open}>
            <Icon name="menu"/>
        </button>

        {#if open}
            <div class="menu" role="menu">
                {#each items as item (item.key)}
                    <button class="menu-item" type="button" role="menuitem"
                            onclick={() => choose(item)}>
                        <Icon name={item.icon} size={17}/>
                        <span>{item.label}</span>
                    </button>
                {/each}
            </div>
        {/if}
    </div>
</header>

<style>
    .bar {
        flex: none;
        padding: 10px 20px 14px;
        display: flex;
        align-items: center;
        gap: 10px;
        border-bottom: 1px solid rgba(255, 255, 255, 0.07);
    }

    /* Шапка — вход в профиль: отдельной кнопки «настройки» на телефоне жалко места. */
    .who {
        display: flex;
        align-items: center;
        gap: 11px;
        /* ⚠️ min-width: 0 обязателен: без него длинное имя распирает шапку, а не
           обрезается, и кнопки уезжают за край экрана. */
        min-width: 0;
        flex: 1;
        background: none;
        border: none;
        color: inherit;
        text-align: left;
        padding: 0;
    }

    .who-text {
        display: flex;
        flex-direction: column;
        min-width: 0;
    }

    .name {
        font-size: 15px;
        font-weight: 700;
        line-height: 1.15;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    .sub {
        margin-top: 2px;
        font-size: 11px;
        color: var(--text-45);
        white-space: nowrap;
    }

    /* Рейтинг читается на бегу: крупная золотая цифра, без подписи под ней. */
    .elo {
        display: inline-flex;
        align-items: center;
        gap: 5px;
        flex: none;
        padding: 6px 11px;
        border: 1px solid var(--gold-soft);
        border-radius: 999px;
        background: rgba(240, 205, 138, 0.1);
        color: var(--gold);
    }

    .elo-value {
        font-family: var(--display);
        font-size: 17px;
        font-weight: 600;
        line-height: 1;
        font-variant-numeric: tabular-nums;
    }

    .wide-row {
        display: none;
        gap: 8px;
    }

    .menu-wrap {
        position: relative;
        flex: none;
    }

    /*
      ⚠️ Меню висит НАД содержимым (position: absolute + z-index), а не раздвигает шапку:
      иначе при открытии весь экран дёргался бы вниз.
    */
    .menu {
        position: absolute;
        top: calc(100% + 8px);
        right: 0;
        z-index: 40;
        min-width: 194px;
        padding: 6px;
        display: flex;
        flex-direction: column;
        gap: 2px;
        border: 1px solid var(--line-strong);
        border-radius: 14px;
        background: var(--sheet, #1b2621);
        box-shadow: 0 18px 40px rgba(0, 0, 0, 0.55);
    }

    .menu-item {
        display: flex;
        align-items: center;
        gap: 10px;
        width: 100%;
        padding: 10px 12px;
        border: none;
        border-radius: 10px;
        background: none;
        color: var(--text);
        font: inherit;
        font-size: 14px;
        text-align: left;
    }

    .menu-item:active {
        background: rgba(255, 255, 255, 0.07);
    }

    @media (min-width: 620px) {
        .wide-row {
            display: flex;
        }

        .menu-wrap {
            display: none;
        }
    }
</style>
