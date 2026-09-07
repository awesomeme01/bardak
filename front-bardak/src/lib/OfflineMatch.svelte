<script>
    /**
     * Запись партии, сыгранной за настоящим столом.
     *
     * ⭐ Стоит рядом с созданием стола, а не в истории: и то и другое — «завести партию»,
     * и человек, севший играть вживую, ищет это там же, где обычно начинает игру.
     *
     * ⚠️ Шкала исходов приходит С СЕРВЕРА, а не выписана здесь. Она — часть правил игры
     * и участвует в расчёте рейтинга; копия в разметке разошлась бы с расчётом на первой
     * же правке шкалы, и заметили бы это по кривому рейтингу, а не по ошибке.
     */
    import {onMount} from 'svelte';
    import {apiGet, apiPost} from '../net/rest-client.js';
    import {friends, loadFriends} from '../stores/friends.svelte.js';
    import {profile, loadRating} from '../stores/profile.svelte.js';
    import {outcomeName} from './naming.js';
    import Avatar from './Avatar.svelte';

    let {onDone} = $props();

    let outcomes = $state([]);
    let error = $state(null);
    let busy = $state(false);
    let saved = $state(null);

    /** Кто играл: идентификатор -> исход. Себя добавляем сразу — свою партию и пишем. */
    let picked = $state(new Map());
    let playedAt = $state(today());

    function today() {
        return new Date().toISOString().slice(0, 10);
    }

    onMount(async () => {
        loadFriends().catch(() => null);
        try {
            outcomes = (await apiGet('/offline/outcomes')).map((row) => row.code);
        } catch (e) {
            error = e.message;
        }
        if (profile.user) {
            picked = new Map([[profile.user.id, defaultOutcome()]]);
        }
    });

    /** По умолчанию — середина шкалы: любой выбор здесь был бы подсказкой, а не фактом. */
    function defaultOutcome() {
        return 'Jk';
    }

    function toggle(userId) {
        const next = new Map(picked);
        next.has(userId) ? next.delete(userId) : next.set(userId, defaultOutcome());
        picked = next;
    }

    function setOutcome(userId, code) {
        const next = new Map(picked);
        next.set(userId, code);
        picked = next;
    }

    function nameOf(userId) {
        if (userId === profile.user?.id) {
            return profile.user.displayName;
        }
        return friends.list.find((f) => f.userId === userId)?.displayName ?? 'Игрок';
    }

    const chosen = $derived([...picked.keys()]);
    const enough = $derived(chosen.length >= 2);

    async function submit(event) {
        event.preventDefault();
        if (busy || !enough) {
            return;
        }
        busy = true;
        error = null;
        try {
            const body = {
                // ⚠️ Дата уходит на конце дня по местному времени: партию записывают
                // вечером того же дня, и полночь UTC отправила бы её во вчера.
                playedAt: new Date(`${playedAt}T20:00:00`).toISOString(),
                players: chosen.map((userId) => ({userId, outcome: picked.get(userId)})),
            };
            const result = await apiPost('/offline/matches', body);
            saved = result.ratingChanges ?? [];
            await loadRating();
        } catch (e) {
            error = e.message;
        } finally {
            busy = false;
        }
    }

    function delta(value) {
        const number = Number(value);
        return number > 0 ? `+${number.toFixed(1)}` : number.toFixed(1);
    }
</script>

{#if saved}
    <!--
      ⭐ Итог показывается прямо здесь, а не «партия записана». Смысл записи — в том,
      как она сдвинула рейтинг; без этого непонятно, зачем было записывать.
    -->
    <div class="card sheet">
        <span class="label">Партия записана</span>
        {#each saved as change (change.userId)}
            <div class="result-line">
                <span class="place mono">{change.place}</span>
                <Avatar userId={change.userId} size={28}/>
                <span class="grow who-name">{nameOf(change.userId)}</span>
                <span class="mono d" class:up={Number(change.ratingDelta) > 0}>
                    {delta(change.ratingDelta)}
                </span>
            </div>
        {/each}
        <button class="btn" type="button" onclick={onDone}>Готово</button>
    </div>
{:else}
    <form class="card sheet" onsubmit={submit}>
        <span class="label">Партия за настоящим столом</span>

        {#if error}<p class="notice notice-fail">{error}</p>{/if}

        <label class="when">
            <span class="label">Когда играли</span>
            <input type="date" bind:value={playedAt} max={today()}>
        </label>

        <!--
          ⚠️ В состав идут только друзья — так решает и сервер. Партию никто не
          подтверждает, и без этого можно было бы приписать разгромный проигрыш кому угодно.
        -->
        <span class="label">Кто играл</span>
        <div class="picks">
            {#each friends.list as person (person.userId)}
                <button type="button" class="pick" class:chosen={picked.has(person.userId)}
                        onclick={() => toggle(person.userId)}>
                    <Avatar userId={person.userId} size={26}/>
                    <span class="pick-name">{person.displayName}</span>
                </button>
            {:else}
                <p class="muted">Сначала добавь друзей — записать партию можно только с ними.</p>
            {/each}
        </div>

        {#if chosen.length}
            <span class="label">Кто чем закончил</span>
            <div class="rows">
                {#each chosen as userId (userId)}
                    <div class="row-line">
                        <Avatar userId={userId} size={26}/>
                        <span class="grow who-name">{nameOf(userId)}</span>
                        <select value={picked.get(userId)}
                                onchange={(e) => setOutcome(userId, e.currentTarget.value)}>
                            {#each outcomes as code (code)}
                                <option value={code}>{outcomeName(code)}</option>
                            {/each}
                        </select>
                    </div>
                {/each}
            </div>
        {/if}

        <p class="mono hint">
            Счёт навесов у такой партии не ведётся — за столом его никто не записывает.
            Места и рейтинг считаются как в онлайне.
        </p>

        <div class="row-line">
            <button class="btn grow" type="submit" disabled={busy || !enough}>
                {busy ? 'Записываю…' : 'Записать'}
            </button>
            <button class="btn-ghost" type="button" onclick={onDone}>Отмена</button>
        </div>
    </form>
{/if}

<style>
    .sheet {
        display: flex;
        flex-direction: column;
        gap: 10px;
    }

    .when {
        display: flex;
        flex-direction: column;
        gap: 6px;
    }

    .picks {
        display: flex;
        flex-wrap: wrap;
        gap: 8px;
    }

    .pick {
        display: flex;
        align-items: center;
        gap: 7px;
        padding: 5px 10px 5px 6px;
        border: 1px solid var(--line);
        border-radius: 999px;
        background: none;
        color: inherit;
    }

    .pick.chosen {
        border-color: var(--gold-soft);
        background: rgba(240, 205, 138, 0.12);
    }

    .pick-name {
        font-size: 13px;
    }

    .rows {
        display: flex;
        flex-direction: column;
        gap: 8px;
    }

    .row-line {
        display: flex;
        align-items: center;
        gap: 10px;
    }

    .who-name {
        font-size: 14px;
        font-weight: 600;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    .grow {
        flex: 1;
        min-width: 0;
    }

    .result-line {
        display: flex;
        align-items: center;
        gap: 10px;
    }

    .place {
        width: 20px;
        text-align: center;
        color: var(--text-55);
    }

    .d {
        font-weight: 700;
        color: var(--red);
        font-variant-numeric: tabular-nums;
    }

    .d.up {
        color: var(--green);
    }

    .hint {
        margin: 0;
        font-size: 11px;
        color: var(--text-55);
        line-height: 1.45;
    }
</style>
