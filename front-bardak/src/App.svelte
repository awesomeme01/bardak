<script>
    import {onMount} from 'svelte';
    import {restoreSession, retrySession, session} from './stores/auth.svelte.js';
    import {table} from './stores/table.svelte.js';
    import Login from './lib/Login.svelte';
    import Register from './lib/Register.svelte';
    import Home from './lib/Home.svelte';
    import AppStatus from './lib/AppStatus.svelte';

    // Роутера нет: экранов три, и переключаются они состоянием сессии.
    let screen = $state('login');

    onMount(restoreSession);

    // ⭐ Фон красит <body>, а не экран: у стола сукно, у итога — красное зарево, и полоса
    // безопасной зоны телефона должна быть того же цвета, иначе она выдаёт «страницу».
    $effect(() => {
        document.body.classList.toggle('at-table', Boolean(table.game) && !table.result);
        document.body.classList.toggle('match-over', Boolean(table.result));
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
        <Register onDone={() => (screen = 'login')}/>
    {:else}
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
