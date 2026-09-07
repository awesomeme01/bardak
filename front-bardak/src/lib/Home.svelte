<script>
    /**
     * Что показывать вошедшему: стол, лобби или разборы.
     *
     * Роутера нет намеренно — экранов пять, и переключаются они состоянием, а не адресом.
     *
     * ⭐ Стол и лобби — разные экраны, а не одно «play». Пока они были одним, сидящий за
     * столом не мог посмотреть лобби вообще: любой возврат «в меню» приводил обратно
     * за стол. Теперь у игрока всегда есть оба пути — и в меню, и обратно к своей партии.
     */
    import {onMount} from 'svelte';
    import {loadProfile} from '../stores/profile.svelte.js';
    import {leaveTable as forgetTable, lobby, loadThemes, restoreTable} from '../stores/lobby.svelte.js';
    import {table} from '../stores/table.svelte.js';
    import {openConnection} from '../stores/connection.svelte.js';
    import {dismissInvite, friends, invite} from '../stores/friends.svelte.js';
    import {closeReplay, history, loadReplay} from '../stores/history.svelte.js';
    import {openByCode} from '../stores/lobby.svelte.js';
    import {consumeInvite, forgetInvite, inviteLink, loadInviteTable}
        from '../stores/invite-link.svelte.js';
    import AppHeader from './AppHeader.svelte';
    import Lobby from './Lobby.svelte';
    import TableRoom from './TableRoom.svelte';
    import Friends from './Friends.svelte';
    import History from './History.svelte';
    import Replay from './Replay.svelte';
    import Profile from './Profile.svelte';
    import Stats from './Stats.svelte';
    import Leaders from './Leaders.svelte';
    import Changelog from './Changelog.svelte';
    import VersionLine from './VersionLine.svelte';
    import {go, replace, route, screenOf} from '../stores/route.svelte.js';

    import Icon from './Icon.svelte';
    /**
     * ⭐ Экран берётся из АДРЕСА, а не из своей переменной. Отсюда всё остальное:
     * «назад» в браузере работает сам собой, перезагрузка возвращает туда же, где был,
     * а ссылкой на раздел можно поделиться.
     */
    const view = $derived(screenOf(route.path));
    const screen = $derived(view.screen === 'player' ? 'stats' : view.screen);

    /** Чужой профиль: экран статистики тот же, отличается только идентификатором. */
    const viewingId = $derived(view.screen === 'player' ? view.param : null);

    /**
     * Имя того, чей профиль открыли.
     *
     * ⚠️ Живёт отдельно от адреса и намеренно: по прямой ссылке имени взять неоткуда,
     * и экран статистики обязан открываться без него. Клик из таблицы или из друзей
     * имя знает — тогда шапка не мигает «загрузкой» на ровном месте.
     */
    let playerName = $state(null);

    function showPlayer(id, name) {
        playerName = name;
        go(`/player/${id}`);
    }

    /**
     * ⭐ «Назад» — это назад по истории браузера, а не «на экран, откуда пришли».
     * Раньше приложение помнило источник перехода само; теперь помнить нечего —
     * этим занимается браузер, и одинаково для своей кнопки и для системной.
     */
    function closePlayer() {
        window.history.back();
    }

    let error = $state(null);

    /** Стол из ссылки, когда игрок уже сидит за другим: ждёт его решения. */
    let pendingLink = $state(null);

    onMount(async () => {
        try {
            await loadProfile();
            // ⚠️ Темы нужны раньше лобби: по ссылке-приглашению игрок попадает сразу
            // за стол, минуя лобби, — а сукно красить уже надо.
            loadThemes().catch(() => null);
            // ⭐ Сокет открывается сразу после входа, а не при посадке за стол: иначе
            // друзья не видят игрока в сети, а приглашение доставлять некуда.
            await openConnection();
            // Возвращаемся за стол сами: место осталось за игроком, даже если вкладку закрыли.
            await restoreTable();
            if (lobby.current) {
                // ⚠️ Адрес главнее: перезагрузка на /history обязана оставить в истории,
                // даже если человек сидит за столом. Сажаем сами только с корня —
                // то есть когда раздел не выбран вовсе.
                if (route.path === '/') {
                    replace('/table');
                }
                // ⚠️ Уже сидим за столом — ссылка не утаскивает с текущей партии сама.
                // Но и молчать нельзя: человек нажал ссылку и вправе знать, что она
                // дошла. Показываем оклик и даём решить.
                await offerInviteLink();
                return;
            }
            if (!lobby.current && screenOf(route.path).screen === 'table') {
                replace('/');
            }
            await followInviteLink();
        } catch (e) {
            error = e.message;
        }
    });

    /**
     * Пришёл по ссылке — сажаем за стол сам, без лобби и без ввода кода.
     *
     * ⭐ Ради этого всё и затевалось: ссылка должна доводить до места, а не до главного
     * экрана с предложением поискать стол самому. Код мог пролежать в переписке неделю,
     * поэтому неудача здесь — обычное дело, а не ошибка приложения.
     */
    /**
     * Ссылка пришла к тому, кто уже за столом.
     *
     * ⚠️ Раньше здесь было молчание: код тихо оставался в памяти, экран не менялся, и
     * со стороны это выглядело как «ссылка не работает». Молчаливый отказ хуже отказа.
     */
    async function offerInviteLink() {
        if (!inviteLink.code) {
            return;
        }
        const table = await loadInviteTable();
        if (table) {
            pendingLink = table;
        }
    }

    /** Перейти по ссылке, встав из-за текущего стола. */
    async function acceptInviteLink() {
        const code = consumeInvite();
        pendingLink = null;
        try {
            const info = await openByCode(code);
            if (info) {
                replace('/table');
            }
        } catch {
            error = 'Стол по ссылке не открылся — возможно, его уже закрыли';
        }
    }

    function dismissInviteLink() {
        pendingLink = null;
        forgetInvite();
    }

    async function followInviteLink() {
        const code = consumeInvite();
        if (!code) {
            return;
        }
        try {
            const info = await openByCode(code);
            if (info) {
                replace('/table');
            }
        } catch {
            error = 'Стол по ссылке не открылся — возможно, его уже закрыли';
        }
    }

    /** Стол показываем, только если игрок за ним действительно сидит. */
    const atTable = $derived(screen === 'table' && lobby.current !== null);

    function toLobby() {
        go('/');
    }

    function toTable() {
        go('/table');
    }

    function leftTable() {
        forgetTable();
        go('/');
    }

    /**
     * Реплей — свой адрес `/history/<матч>`, а не состояние внутри истории.
     *
     * ⭐ Так «назад» из реплея возвращает к списку матчей, а не выбрасывает из игры,
     * и ссылку на разбор партии можно кинуть тому, с кем играл.
     */
    let openedReplay = $state(null);
    $effect(() => {
        const wanted = view.screen === 'history' ? view.param : null;
        if (wanted && openedReplay !== wanted) {
            openedReplay = wanted;
            loadReplay(wanted).catch((e) => (error = e.message));
        }
        if (!wanted && openedReplay) {
            openedReplay = null;
            closeReplay();
        }
    });
</script>

{#if !atTable}
    <AppHeader onHistory={() => go(screen === 'history' ? '/' : '/history')}
               onProfile={() => go('/profile')}
               onFriends={() => go('/friends')}
               onStats={() => { playerName = null; go('/stats'); }}
               onLeaders={() => go('/leaders')}/>
{/if}

{#if error}<p class="notice notice-fail top">{error}</p>{/if}

<!--
  ⭐ Ответ на приглашение — «позвал» или «его нет в сети» — живёт на уровне приложения,
  а не на экране друзей. Позвать можно и при создании стола, а оттуда игрока сразу уводит
  за стол: на своём экране эта полоса гасла бы, так и не показавшись.
-->
{#if friends.notice}<p class="notice top">{friends.notice}</p>{/if}

<!-- ⭐ Пришёл по ссылке, а сам уже за столом: ссылка сработала, решает игрок. -->
{#if pendingLink}
    <div class="invite">
        <div class="grow">
            <div class="invite-title">Ссылка зовёт за стол «{pendingLink.name}»</div>
            <div class="mono">
                {pendingLink.seatsTaken} из {pendingLink.maxPlayers} мест
                {#if !pendingLink.joinable}· мест нет{/if}
            </div>
        </div>
        {#if pendingLink.joinable}
            <button class="btn-small" type="button" onclick={acceptInviteLink}>Перейти</button>
        {/if}
        <button class="btn-ghost invite-no" type="button" onclick={dismissInviteLink}>Остаться</button>
    </div>
{/if}

<!-- Приглашение от друга: оклик, который надо услышать сразу, где бы игрок ни находился. -->
{#if invite.from}
    <div class="invite">
        <div class="grow">
            <div class="invite-title">{invite.from} зовёт за стол</div>
            <div class="mono">«{invite.tableName}» · код {invite.tableCode}</div>
        </div>
        <button class="btn-small" type="button" onclick={async () => {
            const info = await openByCode(invite.tableCode);
            dismissInvite();
            if (info) {
                toTable();
            }
        }}>Сесть</button>
        <button class="btn-ghost invite-no" type="button" onclick={dismissInvite}>Позже</button>
    </div>
{/if}

<!--
  ⭐ Полоса навигации на каждом экране, кроме самого стола: уйти в меню и вернуться
  к своей партии должно быть можно откуда угодно. За столом её нет намеренно — там
  экран занят игрой, а выход живёт в строке раздачи.

  ⚠️ Кнопка «назад» на всё приложение ОДНА, и живёт она здесь. Раньше рядом с ней
  каждый экран рисовал ещё и свою — со стрелкой без подписи, — и получалось две кнопки
  подряд, делающие одно и то же. Здесь она к тому же знает, куда возвращать: из чужого
  профиля — назад по истории, откуда пришли, а не в лобби через голову.
-->
{#if !atTable && screen !== 'lobby'}
    <nav class="screen-nav">
        <button class="nav-btn" type="button" onclick={viewingId ? closePlayer : toLobby}>
            <Icon name="back" size={16}/>
            <span>{viewingId ? 'Назад' : 'В главное меню'}</span>
        </button>
    </nav>
{/if}

{#if !atTable && lobby.current}
    <!-- Матч идёт, а игрок ушёл смотреть разбор: зовём обратно, иначе партия отменится. -->
    <button class="back-to-table" type="button" onclick={toTable}>
        ← Вернуться за стол «{lobby.current.name}»
        {#if table.game}<span class="pill pill-turn">матч идёт</span>{/if}
    </button>
{/if}

{#if screen === 'profile'}
    <Profile/>
{:else if screen === 'stats'}
    <Stats onPlayer={showPlayer} userId={viewingId} name={playerName}/>
{:else if screen === 'leaders'}
    <Leaders onPlayer={showPlayer}/>
{:else if screen === 'changelog'}
    <Changelog/>
{:else if screen === 'friends'}
    <Friends onPlayer={showPlayer}/>
{:else if screen === 'history'}
    <!--
      ⭐ Реплей занимает экран целиком, а не раскрывается внутри карточки матча: смотреть
      партию и одновременно листать список — разные занятия, и в одной колонке им тесно.
    -->
    {#if history.replay}
        <Replay replay={history.replay} details={history.details} onClose={() => go('/history')}/>
    {:else}
        <History onReplay={(matchId) => go(`/history/${matchId}`)}/>
    {/if}
{:else if atTable}
    <TableRoom info={lobby.current} onExit={leftTable} onMenu={toLobby}
               onHistory={() => go('/history')}/>
{:else}
    <Lobby onEnter={toTable}/>
    <!-- ⭐ Версия внизу главного: отсюда её и спрашивают, когда что-то «не работает». -->
    <VersionLine/>
{/if}

<style>
    .top {
        margin: 12px 20px 0;
    }

    .screen-nav {
        margin: 12px 20px 0;
        display: flex;
        gap: 10px;
    }

    .nav-btn {
        display: inline-flex;
        align-items: center;
        gap: 7px;
        padding: 9px 14px;
        border-radius: 12px;
        border: 1px solid var(--line-strong);
        background: var(--surface);
        color: var(--text-70);
        font-size: 14px;
    }

    .invite {
        margin: 12px 20px 0;
        padding: 12px 16px;
        border-radius: 14px;
        border: 1px solid var(--gold-soft);
        background: rgba(240, 205, 138, 0.12);
        display: flex;
        align-items: center;
        gap: 10px;
    }

    .invite-title {
        font-size: 15px;
        font-weight: 700;
        color: var(--gold);
    }

    .invite-no {
        height: 36px;
        padding: 0 12px;
        font-size: 13px;
    }

    .back-to-table {
        margin: 12px 20px 0;
        padding: 12px 16px;
        border-radius: 14px;
        border: 1px solid var(--gold-soft);
        background: rgba(240, 205, 138, 0.08);
        color: var(--gold);
        font-size: 14px;
        text-align: left;
        display: flex;
        align-items: center;
        gap: 10px;
    }
</style>
