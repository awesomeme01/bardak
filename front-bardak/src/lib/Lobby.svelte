<script>
    import {onMount} from 'svelte';
    import {createTable, lobby, loadTables, openByCode, openTable} from '../stores/lobby.svelte.js';

    let {onEnter} = $props();

    let name = $state('Стол на вечер');
    let maxPlayers = $state(4);
    let isPrivate = $state(false);
    let code = $state('');
    let error = $state(null);

    onMount(refresh);

    async function refresh() {
        error = null;
        try {
            await loadTables();
        } catch (e) {
            error = e.message;
        }
    }

    async function create(event) {
        event.preventDefault();
        error = null;
        try {
            onEnter(await createTable(name, maxPlayers, isPrivate));
        } catch (e) {
            error = e.message;
        }
    }

    async function enterByCode(event) {
        event.preventDefault();
        error = null;
        try {
            onEnter(await openByCode(code));
        } catch (e) {
            error = e.message;
        }
    }

    async function enter(table) {
        onEnter(await openTable(table.id));
    }
</script>

<section class="card">
    <h2>Столы</h2>
    {#if error}<p class="badge badge-fail">{error}</p>{/if}
    {#if lobby.tables.length === 0}
        <p>Открытых столов нет — создай первый.</p>
    {:else}
        <ul class="tables">
            {#each lobby.tables as table (table.id)}
                <li>
                    <button type="button" onclick={() => enter(table)}>
                        {table.name} — {table.seats.length}/{table.maxPlayers}
                    </button>
                </li>
            {/each}
        </ul>
    {/if}
    <div class="row"><button type="button" onclick={refresh}>Обновить</button></div>
</section>

<section class="card">
    <h2>Новый стол</h2>
    <form onsubmit={create}>
        <label>Название<input bind:value={name} required maxlength="64"></label>
        <label>Мест
            <select bind:value={maxPlayers}>
                {#each [2, 3, 4, 5] as count}<option value={count}>{count}</option>{/each}
            </select>
        </label>
        <label class="inline"><input type="checkbox" bind:checked={isPrivate}> Приватный — только по коду</label>
        <div class="row"><button type="submit">Создать</button></div>
    </form>
</section>

<section class="card">
    <h2>Вход по коду</h2>
    <form onsubmit={enterByCode}>
        <label>Код приглашения<input bind:value={code} required maxlength="8" placeholder="ABC123"></label>
        <div class="row"><button type="submit">Войти</button></div>
    </form>
</section>
