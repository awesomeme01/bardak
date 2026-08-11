<script>
    import {register} from '../stores/auth.svelte.js';

    let {onDone} = $props();

    let form = $state({username: '', displayName: '', password: '', inviteCode: ''});
    let error = $state(null);
    let busy = $state(false);

    async function submit(event) {
        event.preventDefault();
        busy = true;
        error = null;
        try {
            await register(form);
        } catch (e) {
            error = e.message;
        } finally {
            busy = false;
        }
    }
</script>

<section class="card">
    <h2>Регистрация</h2>
    <!-- Регистрация закрытая: без кода приглашения не пустит (bardak.auth.invite-codes). -->
    <form onsubmit={submit}>
        <label>Логин<input bind:value={form.username} autocomplete="username" required minlength="3"></label>
        <label>Имя за столом<input bind:value={form.displayName} required minlength="2"></label>
        <label>Пароль<input type="password" bind:value={form.password} autocomplete="new-password" required minlength="8"></label>
        <label>Код приглашения<input bind:value={form.inviteCode} required></label>
        {#if error}<p class="badge badge-fail">{error}</p>{/if}
        <div class="row">
            <button type="submit" disabled={busy}>{busy ? 'Создаю…' : 'Создать'}</button>
            <button type="button" onclick={onDone}>Назад ко входу</button>
        </div>
    </form>
</section>
