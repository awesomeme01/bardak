<script>
    import {login} from '../stores/auth.svelte.js';

    let {onRegister} = $props();

    let username = $state('');
    let password = $state('');
    let error = $state(null);
    let busy = $state(false);

    async function submit(event) {
        event.preventDefault();
        busy = true;
        error = null;
        try {
            await login(username, password);
        } catch (e) {
            // Сервер отвечает одинаково и на неверный пароль, и на несуществующий
            // логин — подсказывать «такого игрока нет» нельзя и на фронте.
            error = e.message;
        } finally {
            busy = false;
        }
    }
</script>

<section class="card">
    <h2>Вход</h2>
    <form onsubmit={submit}>
        <label>Логин<input bind:value={username} autocomplete="username" required></label>
        <label>Пароль<input type="password" bind:value={password} autocomplete="current-password" required></label>
        {#if error}<p class="badge badge-fail">{error}</p>{/if}
        <div class="row">
            <button type="submit" disabled={busy}>{busy ? 'Вхожу…' : 'Войти'}</button>
            <button type="button" onclick={onRegister}>Регистрация</button>
        </div>
    </form>
</section>
