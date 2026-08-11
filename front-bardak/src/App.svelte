<script>
    import {onMount} from 'svelte';
    import {restoreSession, retrySession, session} from './stores/auth.svelte.js';
    import Login from './lib/Login.svelte';
    import Register from './lib/Register.svelte';
    import Home from './lib/Home.svelte';
    import AppStatus from './lib/AppStatus.svelte';

    // Роутер здесь не нужен: экранов три, и переключаются они состоянием сессии.
    let screen = $state('login');

    onMount(restoreSession);
</script>

<header>
    <h1>Bardak</h1>
    <p class="stage">M7 — приложение на телефоне</p>
</header>

<main>
    <AppStatus/>
    {#if session.status === 'unknown'}
        <section class="card"><p>Восстанавливаю сессию…</p></section>
    {:else if session.status === 'offline'}
        <section class="card">
            <h2>Нет связи с сервером</h2>
            <p>Вход сохранён — как только сеть вернётся, игра продолжится.</p>
            <div class="row"><button type="button" onclick={retrySession}>Попробовать снова</button></div>
        </section>
    {:else if session.status === 'authenticated'}
        <Home/>
    {:else if screen === 'register'}
        <Register onDone={() => (screen = 'login')}/>
    {:else}
        <Login onRegister={() => (screen = 'register')}/>
    {/if}
</main>
