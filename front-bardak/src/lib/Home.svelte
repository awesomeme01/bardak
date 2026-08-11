<script>
    import {onMount} from 'svelte';
    import {loadProfile, logout} from '../stores/auth.svelte.js';
    import {leaveTable as forgetTable, lobby} from '../stores/lobby.svelte.js';
    import Lobby from './Lobby.svelte';
    import TableRoom from './TableRoom.svelte';
    import History from './History.svelte';

    let profile = $state(null);
    let error = $state(null);
    // Разделов два, и переключаются они кнопкой: роутер ради этого не нужен.
    let tab = $state('play');

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
        <button type="button" onclick={() => (tab = 'play')} disabled={tab === 'play'}>Игра</button>
        <button type="button" onclick={() => (tab = 'history')} disabled={tab === 'history'}>История</button>
        <button type="button" onclick={logout}>Выйти</button>
    </div>
</section>

{#if tab === 'history'}
    <History/>
{:else if lobby.current}
    <TableRoom info={lobby.current} onExit={forgetTable}/>
{:else}
    <Lobby onEnter={() => {}}/>
{/if}
