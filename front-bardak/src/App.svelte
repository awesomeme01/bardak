<script>
    import {onMount} from 'svelte';
    import {restoreSession, retrySession, session} from './stores/auth.svelte.js';
    import {table} from './stores/table.svelte.js';
    import {feltOf, lobby, openTable, restoreTable} from './stores/lobby.svelte.js';
    import {inviteLink, readTableFromUrl} from './stores/invite-link.svelte.js';
    import Login from './lib/Login.svelte';
    import Register from './lib/Register.svelte';
    import Home from './lib/Home.svelte';
    import AppStatus from './lib/AppStatus.svelte';
    import InviteBanner from './lib/InviteBanner.svelte';

    // Роутера нет: экранов три, и переключаются они состоянием сессии.
    // ⭐ Пришедшему по ссылке сразу показываем регистрацию: учётки у него, скорее всего,
    // ещё нет, а вход отсюда в один переход.
    let screen = $state(inviteLink.code ? 'register' : 'login');

    onMount(restoreSession);

    /**
     * Клик по уведомлению доводит до СТОЛА, а не до лобби.
     *
     * ⚠️ Service worker слал сообщение `OPEN_TABLE` с самого начала, но слушателя у него
     * не было ни одного: человек нажимал «твой ход», вкладка всплывала — и оставалась там,
     * где он её бросил, хоть в истории матчей. Уведомление доводило до приложения
     * и бросало на полпути.
     *
     * ⭐ Не удаётся открыть названный стол (устарел, место уже занято) — садимся туда,
     * где игрок сидит по мнению СЕРВЕРА. Он про это знает точнее, чем старое уведомление.
     */
    onMount(() => {
        const fromUrl = readTableFromUrl();
        if (fromUrl) {
            openTable(fromUrl).catch(() => restoreTable().catch(() => {}));
        }

        if (!navigator.serviceWorker) {
            return;
        }
        const onMessage = (event) => {
            if (event.data?.type !== 'OPEN_TABLE') {
                return;
            }
            const tableId = event.data.tableId;
            const opened = tableId ? openTable(tableId) : Promise.reject(new Error('стол не назван'));
            opened.catch(() => restoreTable().catch(() => {}));
        };
        navigator.serviceWorker.addEventListener('message', onMessage);
        return () => navigator.serviceWorker.removeEventListener('message', onMessage);
    });

    // ⭐ Фон красит <body>, а не экран: у стола сукно, у итога — красное зарево, и полоса
    // безопасной зоны телефона должна быть того же цвета, иначе она выдаёт «страницу».
    $effect(() => {
        document.body.classList.toggle('at-table', Boolean(table.game) && !table.result);
        document.body.classList.toggle('match-over', Boolean(table.result));
    });

    /**
     * Сукно выбранной темы.
     *
     * ⚠️ Тема по умолчанию своим цветом НЕ красится: в макете сукно — подобранный руками
     * трёхточечный градиент, а `feltColor` в базе плоский и заметно ярче. Подставлять его
     * ради формальной «поддержки тем» значило бы испортить вид стола. Красятся только
     * НЕ дефолтные темы, где другого источника вида нет: плоский цвет разводится в тот же
     * градиент, чтобы стол остался столом, а не залитым прямоугольником.
     */
    $effect(() => {
        const felt = feltOf(lobby.current?.themeId);
        if (felt) {
            document.body.style.setProperty('--felt',
                `radial-gradient(120% 74% at 50% 30%, ${felt} 0%,`
                + ` color-mix(in srgb, ${felt} 62%, #0e1714) 46%, #0e1714 100%)`);
        } else {
            document.body.style.removeProperty('--felt');
        }
    });
</script>

<div class="app">
    <AppStatus/>

    {#if session.status === 'unknown'}
        <p class="centered muted">Восстанавливаю сессию…</p>
    {:else if session.status === 'offline'}
        <div class="centered">
            <h2>Нет связи с сервером</h2>
            <p class="muted">Вход сохранён — как только сеть вернётся, игра продолжится.</p>
            <button class="btn" type="button" onclick={retrySession}>Попробовать снова</button>
        </div>
    {:else if session.status === 'authenticated'}
        <Home/>
    {:else if screen === 'register'}
        <InviteBanner/>
        <Register onDone={() => (screen = 'login')}/>
    {:else}
        <InviteBanner/>
        <Login onRegister={() => (screen = 'register')}/>
    {/if}
</div>

<style>
    .centered {
        flex: 1;
        display: flex;
        flex-direction: column;
        align-items: center;
        justify-content: center;
        gap: 14px;
        padding: 24px;
        text-align: center;
    }
</style>
