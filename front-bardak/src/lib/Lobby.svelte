<script>
    import {onMount} from 'svelte';
    import {createTable, lobby, loadTables, openByCode, openTable} from '../stores/lobby.svelte.js';
    import CodeBoxes from './CodeBoxes.svelte';

    let {onEnter} = $props();

    let error = $state(null);
    let sheet = $state(null);        // 'create' | 'code' | null — что открыто поверх списка
    let name = $state('Стол на вечер');
    let maxPlayers = $state(4);
    let isPrivate = $state(false);
    let code = $state('');

    onMount(refresh);

    export async function refresh() {
        error = null;
        try {
            await loadTables();
        } catch (e) {
            error = e.message;
        }
    }

    async function run(action) {
        error = null;
        try {
            onEnter(await action());
        } catch (e) {
            error = e.message;
        }
    }

    const create = (event) => {
        event.preventDefault();
        run(() => createTable(name, maxPlayers, isPrivate));
    };

    const enterByCode = (event) => {
        event.preventDefault();
        run(() => openByCode(code));
    };
</script>

<div class="screen">
    {#if error}<p class="notice notice-fail">{error}</p>{/if}

    {#if lobby.current}
        <!-- ⭐ Свой стол — первым и золотым: если матч идёт, вернуться надо немедленно. -->
        <div class="card card-gold">
            <div class="row spread">
                <span class="label gold">Ты за столом</span>
                <span class="mono">{lobby.inMatch ? 'матч идёт' : 'ждём игроков'}</span>
            </div>
            <div class="row spread bottom">
                <div>
                    <div class="table-name">{lobby.current.name}</div>
                    <div class="mono">{lobby.current.seats.length} из {lobby.current.maxPlayers}</div>
                </div>
                <button class="btn btn-back" type="button"
                        onclick={() => onEnter(lobby.current)}>Вернуться</button>
            </div>
        </div>
    {/if}

    <div class="row spread">
        <span class="label">Открытые столы</span>
        <span class="mono">{lobby.tables.length}</span>
    </div>

    <div class="tables">
        {#each lobby.tables as item (item.id)}
            <div class="card table-row">
                <div class="grow">
                    <div class="table-name">{item.name}</div>
                    <div class="mono">{item.seats.length} из {item.maxPlayers}
                        {#if item.isPrivate}· по коду{/if}</div>
                </div>
                <div class="dots" aria-hidden="true">
                    {#each Array(item.maxPlayers) as unused, seat (seat)}
                        <span class="dot" class:taken={seat < item.seats.length}></span>
                    {/each}
                </div>
                <button class="btn-small" type="button" onclick={() => run(() => openTable(item.id))}>
                    Сесть
                </button>
            </div>
        {:else}
            <p class="muted empty">Открытых столов нет — создай первый.</p>
        {/each}
    </div>

    {#if sheet === 'create'}
        <form class="card sheet" onsubmit={create}>
            <span class="label">Новый стол</span>
            <input bind:value={name} required maxlength="64" placeholder="Название">
            <div class="seats-choice">
                {#each [2, 3, 4, 5] as count (count)}
                    <button type="button" class="seat-btn" class:chosen={maxPlayers === count}
                            onclick={() => (maxPlayers = count)}>{count}</button>
                {/each}
            </div>
            <label class="inline">
                <input type="checkbox" bind:checked={isPrivate}>
                <span>Приватный — только по коду</span>
            </label>
            <div class="row">
                <button class="btn grow" type="submit">Создать</button>
                <button class="btn-ghost" type="button" onclick={() => (sheet = null)}>Отмена</button>
            </div>
        </form>
    {:else if sheet === 'code'}
        <form class="card sheet" onsubmit={enterByCode}>
            <span class="label">Код от друга — шесть символов</span>
            <CodeBoxes bind:value={code} length={6} editable/>
            <div class="row">
                <button class="btn grow" type="submit">Войти</button>
                <button class="btn-ghost" type="button" onclick={() => (sheet = null)}>Отмена</button>
            </div>
        </form>
    {:else}
        <button class="card by-code" type="button" onclick={() => (sheet = 'code')}>
            <span>
                <span class="table-name">Войти по коду</span>
                <span class="mono block">6 символов от друга</span>
            </span>
            <span class="mono">→</span>
        </button>
    {/if}
</div>

<div class="bottom-bar">
    <button class="btn grow" type="button" onclick={() => (sheet = sheet === 'create' ? null : 'create')}>
        Создать стол
    </button>
</div>

<style>
    .screen {
        flex: 1;
        display: flex;
        flex-direction: column;
        gap: 14px;
        padding: 16px 20px 0;
        overflow-y: auto;
    }

    .spread {
        justify-content: space-between;
    }

    .bottom {
        align-items: flex-end;
        margin-top: 9px;
    }

    .grow {
        flex: 1;
        min-width: 0;
    }

    .table-name {
        font-size: 16px;
        font-weight: 700;
    }

    .block {
        display: block;
        margin-top: 4px;
    }

    .tables {
        display: flex;
        flex-direction: column;
        gap: 10px;
    }

    .table-row {
        display: flex;
        align-items: center;
        gap: 12px;
    }

    .dots {
        display: flex;
        gap: 3px;
        flex: none;
    }

    .dot {
        width: 8px;
        height: 8px;
        border-radius: 50%;
        background: rgba(255, 255, 255, 0.16);
    }

    .dot.taken {
        background: var(--gold);
    }

    .btn-back {
        height: 44px;
        padding: 0 20px;
        border-radius: 13px;
        font-size: 15px;
    }

    .by-code {
        display: flex;
        align-items: center;
        justify-content: space-between;
        border-style: dashed;
        color: var(--text-70);
        text-align: left;
    }

    .sheet {
        display: flex;
        flex-direction: column;
        gap: 12px;
    }

    .seats-choice {
        display: flex;
        gap: 8px;
    }

    .seat-btn {
        flex: 1;
        height: 48px;
        border-radius: 13px;
        border: 1px solid var(--line-strong);
        font-family: var(--mono);
        font-size: 16px;
        color: var(--text-70);
    }

    .seat-btn.chosen {
        border-color: var(--gold-soft);
        background: rgba(240, 205, 138, 0.1);
        color: var(--gold);
    }

    .inline {
        display: flex;
        align-items: center;
        gap: 10px;
        font-size: 14px;
        color: var(--text-70);
    }

    .inline input {
        width: 20px;
        height: 20px;
    }

    .empty {
        padding: 8px 2px;
    }

    .bottom-bar {
        flex: none;
        padding: 14px 20px 24px;
        display: flex;
        gap: 10px;
    }

    /* На широком экране список столов и формы стоят рядом, а не друг под другом. */
    @media (min-width: 900px) {
        .screen {
            display: grid;
            grid-template-columns: minmax(0, 1fr) 360px;
            grid-template-areas:
                'current side'
                'title   side'
                'tables  side'
                'bycode  side';
            gap: 16px 28px;
            align-content: start;
            padding: 24px 32px 0;
        }

        .card-gold {
            grid-area: current;
        }

        .tables {
            grid-area: tables;
        }

        .by-code, .sheet {
            grid-area: side;
            align-self: start;
        }

        .bottom-bar {
            max-width: 360px;
            margin-left: auto;
            padding: 14px 32px 28px;
        }
    }
</style>
