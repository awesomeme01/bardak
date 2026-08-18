<script>
    /**
     * Друзья: кто в сети, кого позвать, кто позвал тебя.
     *
     * ⭐ Экран построен вокруг «кто сейчас здесь». Игра собирается на пару вечеров, и
     * единственный вопрос, с которым сюда заходят, — с кем можно сесть прямо сейчас.
     * Поэтому онлайн наверху, а кнопка «за стол» стоит у самого имени.
     */
    import {onMount} from 'svelte';
    import Avatar from './Avatar.svelte';
    import {acceptFriend, addFriend, friends, inviteFriend, loadFriends, removeFriend}
        from '../stores/friends.svelte.js';
    import {lobby} from '../stores/lobby.svelte.js';

    let {onBack, onPlayer = null} = $props();

    let username = $state('');
    let busy = $state(false);

    onMount(loadFriends);

    /** Звать можно только за свой стол, и только пока он ждёт игроков. */
    const myTable = $derived(lobby.current);
    const canInvite = $derived(Boolean(myTable) && !lobby.inMatch);

    async function submit(event) {
        event.preventDefault();
        if (busy || !username.trim()) {
            return;
        }
        busy = true;
        const ok = await addFriend(username.trim());
        if (ok) {
            username = '';
        }
        busy = false;
    }
</script>

<div class="screen">
    <div class="head">
        <button class="icon-btn" type="button" onclick={onBack} aria-label="Назад">←</button>
        <h1>Друзья</h1>
    </div>

    <!-- ⚠️ «Позвал» / «его нет в сети» показывает Home, а не этот экран: звать друзей
         можно и при создании стола, а оттуда игрока сразу уводит за стол — и ответ,
         дошёл ли оклик, он бы никогда не увидел. -->
    {#if friends.error}<p class="notice notice-fail">{friends.error}</p>{/if}

    <form class="card add" onsubmit={submit}>
        <span class="label">Позвать в друзья по логину</span>
        <div class="row">
            <input bind:value={username} placeholder="логин" maxlength="32"
                   autocapitalize="none" autocorrect="off" spellcheck="false">
            <button class="btn-small" type="submit" disabled={busy || !username.trim()}>
                {busy ? '…' : 'Позвать'}
            </button>
        </div>
    </form>

    {#if friends.incoming.length}
        <div class="block">
            <span class="label">Тебя зовут в друзья</span>
            {#each friends.incoming as person (person.userId)}
                <div class="card person">
                    <Avatar userId={person.userId} avatar={person.avatar} size={38}/>
                    <div class="grow">
                        <div class="name">{person.displayName}</div>
                        <div class="mono">@{person.username}</div>
                    </div>
                    <button class="btn-small" type="button"
                            onclick={() => acceptFriend(person.userId)}>Принять</button>
                    <button class="btn-ghost small" type="button"
                            onclick={() => removeFriend(person.userId)}>Нет</button>
                </div>
            {/each}
        </div>
    {/if}

    <div class="block">
        <span class="label">Друзья {#if friends.list.length}· {friends.list.length}{/if}</span>

        {#each friends.list as person (person.userId)}
            <div class="card person">
                <span class="dot" class:online={person.online}
                      title={person.online ? 'в сети' : 'не в сети'}></span>
                <!-- ⭐ По имени открывается его статистика: «кто это вообще» — первый
                     вопрос к списку друзей, и отвечать на него должен сам список. -->
                <button class="grow who" type="button" disabled={!onPlayer}
                        onclick={() => onPlayer?.(person.userId, person.displayName)}>
                    <Avatar userId={person.userId} avatar={person.avatar} size={38}/>
                    <span class="grow left">
                        <span class="name">{person.displayName}</span>
                        <span class="mono block">{person.online ? 'в сети' : `@${person.username}`}</span>
                    </span>
                </button>
                {#if canInvite}
                    <button class="btn-small" type="button"
                            onclick={() => inviteFriend(person.userId, myTable.id, person.displayName)}>
                        За стол
                    </button>
                {/if}
                <button class="btn-ghost small" type="button"
                        onclick={() => removeFriend(person.userId)} aria-label="Убрать из друзей">×</button>
            </div>
        {:else}
            <!-- Пустой список объясняет, что тут будет и как это получить. -->
            <p class="muted">
                Друзей пока нет. Позови по логину — и увидишь, кто из них сейчас в сети,
                а одной кнопкой позовёшь за свой стол.
            </p>
        {/each}

        {#if friends.list.length && !canInvite}
            <p class="mono hint">
                {#if lobby.inMatch}
                    Идёт матч — позвать за стол можно будет после него.
                {:else}
                    Сядь за стол, и рядом с друзьями появится кнопка «За стол».
                {/if}
            </p>
        {/if}
    </div>

    {#if friends.outgoing.length}
        <div class="block">
            <span class="label">Ждут ответа</span>
            {#each friends.outgoing as person (person.userId)}
                <div class="card person muted-row">
                    <Avatar userId={person.userId} avatar={person.avatar} size={38}/>
                    <div class="grow">
                        <div class="name">{person.displayName}</div>
                        <div class="mono">заявка отправлена</div>
                    </div>
                    <button class="btn-ghost small" type="button"
                            onclick={() => removeFriend(person.userId)}>Отменить</button>
                </div>
            {/each}
        </div>
    {/if}
</div>

<style>
    /* Имя друга — кнопка, но выглядеть должно строкой списка, а не кнопкой. */
    .who {
        display: flex;
        align-items: center;
        gap: 10px;
        background: none;
        border: none;
        padding: 0;
        text-align: left;
    }

    .who:disabled {
        cursor: default;
    }

    .left {
        display: flex;
        flex-direction: column;
        min-width: 0;
    }

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
        gap: 10px;
    }

    .add {
        display: flex;
        flex-direction: column;
        gap: 10px;
    }

    .row {
        display: flex;
        gap: 10px;
        align-items: center;
    }

    .row input {
        flex: 1;
        min-width: 0;
    }

    .person {
        display: flex;
        align-items: center;
        gap: 11px;
    }

    .muted-row {
        opacity: 0.7;
    }

    .grow {
        flex: 1;
        min-width: 0;
    }

    .name {
        font-size: 15px;
        font-weight: 700;
    }

    /* ⭐ Точка, а не подпись: онлайн ищут глазами, а не читают. */
    .dot {
        width: 9px;
        height: 9px;
        border-radius: 50%;
        background: rgba(255, 255, 255, 0.18);
        flex: none;
    }

    .dot.online {
        background: var(--green);
        box-shadow: 0 0 0 3px rgba(127, 216, 166, 0.2);
    }

    .small {
        height: 36px;
        padding: 0 12px;
        font-size: 13px;
    }

    .hint {
        color: var(--text-45);
    }
</style>
