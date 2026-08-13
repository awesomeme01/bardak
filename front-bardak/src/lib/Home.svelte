<script>
    /**
     * Что показывать вошедшему: стол, лобби или историю.
     *
     * Роутера нет намеренно — экранов три, и переключаются они состоянием, а не адресом.
     */
    import {onMount} from 'svelte';
    import {loadProfile} from '../stores/profile.svelte.js';
    import {leaveTable as forgetTable, lobby, restoreTable} from '../stores/lobby.svelte.js';
    import {table} from '../stores/table.svelte.js';
    import AppHeader from './AppHeader.svelte';
    import Lobby from './Lobby.svelte';
    import TableRoom from './TableRoom.svelte';
    import History from './History.svelte';
    import Profile from './Profile.svelte';
    import Stats from './Stats.svelte';

    let tab = $state('play');
    let error = $state(null);
    let lobbyScreen = $state(null);

    onMount(async () => {
        try {
            await loadProfile();
            // Возвращаемся за стол сами: место осталось за игроком, даже если вкладку закрыли.
            await restoreTable();
        } catch (e) {
            error = e.message;
        }
    });

    const atTable = $derived(tab === 'play' && lobby.current !== null);
</script>

{#if !atTable && tab !== 'profile' && tab !== 'stats'}
    <AppHeader onRefresh={tab === 'play' ? () => lobbyScreen?.refresh() : null}
               onHistory={() => (tab = tab === 'history' ? 'play' : 'history')}
               onProfile={() => (tab = 'profile')}
               onStats={() => (tab = 'stats')}/>
{/if}

{#if error}<p class="notice notice-fail top">{error}</p>{/if}

{#if tab === 'profile'}
    <Profile onBack={() => (tab = 'play')}/>
{:else if tab === 'stats'}
    <Stats onBack={() => (tab = 'play')}/>
{:else if tab === 'history'}
    {#if lobby.current}
        <!-- ⭐ Матч идёт, а игрок ушёл в разбор: зовём обратно, иначе партия отменится по времени. -->
        <button class="back-to-table" type="button" onclick={() => (tab = 'play')}>
            ← Вернуться за стол «{lobby.current.name}»
            {#if table.game}<span class="pill pill-turn">матч идёт</span>{/if}
        </button>
    {/if}
    <History/>
{:else if lobby.current}
    <TableRoom info={lobby.current} onExit={forgetTable} onHistory={() => (tab = 'history')}/>
{:else}
    <Lobby bind:this={lobbyScreen} onEnter={() => {}}/>
{/if}

<style>
    .top {
        margin: 12px 20px 0;
    }

    .back-to-table {
        margin: 12px 20px 0;
        padding: 12px 16px;
        border-radius: 14px;
        border: 1px solid var(--gold-soft);
        background: rgba(240, 205, 138, 0.08);
        color: var(--gold);
        font-size: 14px;
        text-align: left;
        display: flex;
        align-items: center;
        gap: 10px;
    }
</style>
