<script>
    /**
     * Профиль: имя за столом, мордочка, пароль.
     *
     * ⭐ Логин не меняется. По нему входят, и его знают соседи; имя же — то, что видно
     * в игре, и его хочется поправить, не заводя новую учётку.
     */
    import {apiGet, apiPatch, apiPost} from '../net/rest-client.js';
    import {logout} from '../stores/auth.svelte.js';
    import {loadProfile, profile} from '../stores/profile.svelte.js';
    import {FACES, avatarOf} from './naming.js';
    import {enablePush, pwa} from '../stores/pwa.svelte.js';

    let {onBack} = $props();

    let displayName = $state(profile.user?.displayName ?? '');
    let avatar = $state(profile.user?.avatar ?? '');
    let saved = $state(false);
    let error = $state(null);

    let currentPassword = $state('');
    let newPassword = $state('');
    let passwordDone = $state(false);

    const chosen = $derived(avatar || avatarOf(profile.user?.id));

    async function save(event) {
        event.preventDefault();
        error = null;
        try {
            await apiPatch('/profile', {displayName, avatar: avatar || null});
            await loadProfile();
            saved = true;
            setTimeout(() => (saved = false), 2500);
        } catch (e) {
            error = e.message;
        }
    }

    async function changePassword(event) {
        event.preventDefault();
        error = null;
        try {
            await apiPost('/profile/password', {currentPassword, newPassword});
            // ⚠️ Сервер гасит все входы: текущая вкладка доживает до конца access-токена.
            passwordDone = true;
            currentPassword = '';
            newPassword = '';
        } catch (e) {
            error = e.message;
        }
    }
</script>

<div class="screen">
    <div class="head">
        <button class="icon-btn" type="button" onclick={onBack} aria-label="Назад">←</button>
        <h1>Профиль</h1>
    </div>

    {#if error}<p class="notice notice-fail">{error}</p>{/if}

    <form class="card block" onsubmit={save}>
        <span class="label">Как тебя видят за столом</span>
        <div class="row">
            <span class="chosen">{chosen}</span>
            <label class="field grow">
                <span class="label">Имя</span>
                <input bind:value={displayName} required minlength="2" maxlength="64">
            </label>
        </div>

        <span class="label">Мордочка</span>
        <div class="faces">
            {#each FACES as face (face)}
                <button type="button" class="face" class:picked={avatar === face}
                        onclick={() => (avatar = avatar === face ? '' : face)}>{face}</button>
            {/each}
        </div>
        <p class="mono">Пусто — мордочка выводится из твоего кода игрока.</p>

        <button class="btn" type="submit">{saved ? 'Сохранено' : 'Сохранить'}</button>
    </form>

    <form class="card block" onsubmit={changePassword}>
        <span class="label">Пароль</span>
        {#if passwordDone}
            <p class="notice">Пароль сменён. На других устройствах придётся войти заново.</p>
        {/if}
        <label class="field">
            <span class="label">Текущий</span>
            <input type="password" bind:value={currentPassword} autocomplete="current-password" required>
        </label>
        <label class="field">
            <span class="label">Новый — от 8 символов</span>
            <input type="password" bind:value={newPassword} autocomplete="new-password"
                   required minlength="8">
        </label>
        <!-- ⚠️ Старый пароль спрашивается даже при живом токене: иначе оставленная
             открытой вкладка позволяет запереть владельца снаружи. -->
        <button class="btn-ghost" type="submit">Сменить пароль</button>
    </form>

    <div class="card block">
        <span class="label">Устройство</span>
        <div class="line mono">
            <span>Уведомления «твой ход»</span>
            {#if pwa.pushEnabled}
                <span class="pill pill-ready">включены</span>
            {:else}
                <button class="btn-small" type="button" onclick={enablePush}>Включить</button>
            {/if}
        </div>
        {#if pwa.pushError}<p class="mono">{pwa.pushError}</p>{/if}
        <div class="line mono">
            <span>Логин</span>
            <span>@{profile.user?.username}</span>
        </div>
        <button class="btn-ghost" type="button" onclick={logout}>Выйти из аккаунта</button>
    </div>
</div>

<style>
    .screen {
        flex: 1;
        display: flex;
        flex-direction: column;
        gap: 14px;
        padding: 16px 20px 28px;
        overflow-y: auto;
    }

    .head {
        display: flex;
        align-items: center;
        gap: 12px;
    }

    .block {
        display: flex;
        flex-direction: column;
        gap: 12px;
    }

    .grow {
        flex: 1;
        min-width: 0;
    }

    .chosen {
        width: 56px;
        height: 56px;
        border-radius: 50%;
        background: radial-gradient(60% 60% at 35% 30%, #4a453b, #2b271f);
        border: 2px solid var(--gold-soft);
        display: flex;
        align-items: center;
        justify-content: center;
        font-size: 28px;
        flex: none;
    }

    .faces {
        display: grid;
        grid-template-columns: repeat(6, 1fr);
        gap: 8px;
    }

    .face {
        aspect-ratio: 1;
        border-radius: 12px;
        border: 1px solid var(--line);
        background: var(--surface);
        font-size: 22px;
    }

    .face.picked {
        border-color: var(--gold);
        background: rgba(240, 205, 138, 0.12);
    }

    .line {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: 12px;
        color: var(--text-55);
    }
</style>
