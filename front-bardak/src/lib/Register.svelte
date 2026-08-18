<script>
    import {register} from '../stores/auth.svelte.js';
    import CodeBoxes from './CodeBoxes.svelte';

    let {onDone} = $props();

    let form = $state({username: '', displayName: '', password: '', inviteCode: ''});
    let error = $state(null);
    let busy = $state(false);

    /**
     * Длина кода приглашения. Ровно столько клеточек, сколько символов в коде: пустые
     * клетки в конце читались как «ты чего-то не дописал», хотя код уже введён целиком.
     */
    const INVITE_LENGTH = 11;

    /** Кнопка ждёт, пока заполнено всё: отправлять заведомо неполную форму незачем. */
    const ready = $derived(form.username.trim().length >= 3
        && form.displayName.trim().length >= 2
        && form.password.length >= 8
        && form.inviteCode.trim().length === INVITE_LENGTH);

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

<!-- Регистрация закрытая: без кода приглашения не пустит (bardak.auth.invite-codes). -->
<form class="screen" onsubmit={submit}>
    <div class="head">
        <button class="icon-btn" type="button" onclick={onDone} aria-label="Назад ко входу">←</button>
        <h1>Регистрация</h1>
    </div>
    <p class="muted">Клуб закрытый — нужен код от того, кто уже за столом.</p>

    <div class="fields">
        <label class="field">
            <span class="label">Логин</span>
            <!-- ⚠️ Автозаглавная буква отключена: логин ищется без учёта регистра, но
                 показывать «Shabdan» тому, кто набрал «shabdan», всё равно незачем. -->
            <input bind:value={form.username} autocomplete="username" required minlength="3"
                   maxlength="32" autocapitalize="none" autocorrect="off" spellcheck="false">
        </label>
        <label class="field">
            <span class="label">Имя за столом</span>
            <input bind:value={form.displayName} required minlength="2">
        </label>
        <label class="field">
            <span class="label">Пароль</span>
            <input type="password" bind:value={form.password} autocomplete="new-password"
                   required minlength="8">
        </label>
    </div>

    <div class="code">
        <span class="label">Код приглашения</span>
        <CodeBoxes bind:value={form.inviteCode} length={INVITE_LENGTH} editable/>
    </div>

    {#if error}<p class="notice notice-fail">{error}</p>{/if}

    <div class="actions">
        <button class="btn" type="submit" disabled={busy || !ready}>{busy ? 'Создаю…' : 'Создать'}</button>
        <button class="btn-ghost" type="button" onclick={onDone}>Ко входу</button>
    </div>
</form>

<style>
    .screen {
        flex: 1;
        display: flex;
        flex-direction: column;
        gap: 18px;
        padding: 22px 22px 26px;
    }

    .head {
        display: flex;
        align-items: center;
        gap: 12px;
    }

    .fields {
        display: flex;
        flex-direction: column;
        gap: 14px;
    }

    .code {
        display: flex;
        flex-direction: column;
        gap: 9px;
    }

    .actions {
        margin-top: auto;
        display: flex;
        gap: 10px;
    }

    .actions :global(.btn) {
        flex: 2;
    }

    .actions :global(.btn-ghost) {
        flex: 1;
        height: 58px;
    }
</style>
