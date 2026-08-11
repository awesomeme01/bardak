<script>
    import {onMount} from 'svelte';
    import {loadProfile, logout, session} from '../stores/auth.svelte.js';
    import Connection from './Connection.svelte';

    let profile = $state(null);
    let error = $state(null);

    onMount(async () => {
        try {
            profile = await loadProfile();
        } catch (e) {
            error = e.message;
        }
    });
</script>

<section class="card">
    <h2>Профиль</h2>
    {#if error}
        <p class="badge badge-fail">{error}</p>
    {:else if profile}
        <p>За столом ты <strong>{profile.displayName}</strong> (@{profile.username})</p>
    {:else}
        <p>Загружаю…</p>
    {/if}
    <div class="row">
        <button type="button" onclick={logout}>Выйти</button>
    </div>
</section>

<Connection/>
