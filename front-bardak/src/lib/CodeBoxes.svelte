<script>
    /**
     * Код по клеточкам.
     *
     * ⭐ Клеточки нужны не для красоты: код приглашения диктуют голосом, и разбитый
     * по символам он и читается, и произносится вслух без ошибок. Буквы в алфавите кода
     * подобраны без похожих начертаний — путать нечего.
     *
     * `editable` — код вводят (регистрация); без него код показывают (комната ожидания).
     */
    let {value = $bindable(''), length = 6, editable = false} = $props();

    const chars = $derived(Array.from({length}, (unused, index) => value[index] ?? ''));

    function onInput(event) {
        value = event.target.value.toUpperCase();
    }
</script>

{#if editable}
    <div class="boxes editable">
        <input class="hidden-input" value={value} oninput={onInput} maxlength={length}
               autocapitalize="characters" autocomplete="off" spellcheck="false"
               aria-label="Код приглашения">
        {#each chars as char, index (index)}
            <span class="box" class:filled={char !== ''}>{char}</span>
        {/each}
    </div>
{:else}
    <div class="boxes">
        {#each chars as char, index (index)}
            <span class="box filled">{char}</span>
        {/each}
    </div>
{/if}

<style>
    .boxes {
        display: flex;
        gap: 6px;
        position: relative;
    }

    .box {
        flex: 1;
        min-width: 0;
        height: 56px;
        border-radius: 13px;
        border: 1px solid var(--line-strong);
        display: flex;
        align-items: center;
        justify-content: center;
        font-family: var(--mono);
        font-size: 20px;
        color: var(--text-30);
    }

    .box.filled {
        border-color: var(--gold-soft);
        background: rgba(240, 205, 138, 0.08);
        color: var(--gold);
    }

    /* Поле лежит поверх клеточек прозрачным: печатает система, показываем мы. */
    .hidden-input {
        position: absolute;
        inset: 0;
        width: 100%;
        height: 100%;
        opacity: 0;
        border: none;
        background: none;
        padding: 0;
        font-size: 16px;
        z-index: 2;
    }

    .editable:focus-within .box {
        border-color: var(--gold-soft);
    }
</style>
