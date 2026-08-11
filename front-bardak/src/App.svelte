<script>
    import {onMount} from 'svelte';
    import {restoreSession, session} from './stores/auth.svelte.js';
    import Login from './lib/Login.svelte';
    import Register from './lib/Register.svelte';
    import Home from './lib/Home.svelte';

    // Роутер здесь не нужен: экранов три, и переключаются они состоянием сессии.
    let screen = $state('login');

    onMount(restoreSession);
</script>

<header>
    <h1>Bardak</h1>
    <p class="stage">M3 — лобби и столы</p>
</header>

<main>
    {#if session.status === 'unknown'}
        <section class="card"><p>Восстанавливаю сессию…</p></section>
    {:else if session.status === 'authenticated'}
        <Home/>
    {:else if screen === 'register'}
        <Register onDone={() => (screen = 'login')}/>
    {:else}
        <Login onRegister={() => (screen = 'register')}/>
    {/if}
</main>
