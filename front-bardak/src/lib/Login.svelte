<script>
    import {login} from '../stores/auth.svelte.js';
    import CardFan from './CardFan.svelte';

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

<form class="screen" onsubmit={submit}>
    <div class="brand">
        <span class="mark">♠</span>
        <div>
            <div class="wordmark">БАРДАК</div>
            <div class="mono">игра для своих</div>
        </div>
    </div>

    <CardFan/>

    <div class="fields">
        <label class="field">
            <span class="label">Логин</span>
            <!-- ⚠️ Без этого телефон сам ставит заглавную первую букву, и вход с верным
                 паролем отвечал «неверный логин или пароль». -->
            <input bind:value={username} autocomplete="username" required
                   autocapitalize="none" autocorrect="off" spellcheck="false">
        </label>
        <label class="field">
            <span class="label">Пароль</span>
            <input type="password" bind:value={password} autocomplete="current-password" required>
        </label>
        {#if error}<p class="notice notice-fail">{error}</p>{/if}
    </div>

    <div class="actions">
        <button class="btn" type="submit" disabled={busy}>{busy ? 'Вхожу…' : 'Войти'}</button>
        <button class="btn-ghost" type="button" onclick={onRegister}>Регистрация по коду</button>
    </div>
</form>

<style>
    .screen {
        flex: 1;
        display: flex;
        flex-direction: column;
        gap: 30px;
        padding: 30px 22px 26px;
    }

    .brand {
        display: flex;
        align-items: center;
        gap: 11px;
    }

    .mark {
        width: 38px;
        height: 38px;
        border-radius: 11px;
        background: var(--gold-face);
        color: var(--gold-ink);
        display: flex;
        align-items: center;
        justify-content: center;
        font-size: 20px;
        font-weight: 800;
    }

    .wordmark {
        font-family: var(--display);
        font-weight: 600;
        font-size: 24px;
        letter-spacing: 0.04em;
        line-height: 1;
    }

    .fields {
        display: flex;
        flex-direction: column;
        gap: 14px;
    }

    .actions {
        margin-top: auto;
        display: flex;
        flex-direction: column;
        gap: 10px;
    }
</style>
