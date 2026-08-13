<script>
    /**
     * Игральная карта.
     *
     * ⭐ Размер задаётся только шириной: высота выводится из пропорции набора (500×726),
     * заданной в `styles.css`. Иначе при смене набора карты поедут по всему приложению.
     */
    import {table} from '../stores/table.svelte.js';

    let {
        code = null,
        faceDown = false,
        width = 70,
        playable = false,
        dimmed = false,
        onclick = null,
        style = '',
        title = null,
    } = $props();

    // Джокеры нумерованы (Joker-3), а картинка у них одна: номер нужен движку,
    // чтобы отличать карты, а манифесту — нет.
    const assetKey = $derived(code?.startsWith('Joker-') ? 'Joker' : code);
    const src = $derived(faceDown ? table.manifest['back'] : table.manifest[assetKey]);
    const sizing = $derived(`width:${width}px; ${style}`);
</script>

{#if onclick}
    <button type="button" {onclick} class="card-button" style={sizing}
            title={title ?? code} aria-label={title ?? code}>
        <img class="playing-card" class:playable class:dimmed src={src} alt={code}>
    </button>
{:else if src}
    <img class="playing-card" class:playable class:dimmed src={src} style={sizing}
         alt={faceDown ? 'рубашка' : code} title={title ?? (faceDown ? 'рубашка' : code)}>
{:else}
    <!-- Манифест ещё не пришёл или набор без этой карты: показываем код, а не пустоту. -->
    <span class="card-slot" style={sizing}>{faceDown ? '🂠' : code}</span>
{/if}

<style>
    .card-button {
        padding: 0;
        border: none;
        background: none;
        line-height: 0;
        display: block;
        transition: transform 0.12s ease;
    }

    .card-button:active :global(.playing-card) {
        transform: translateY(-4px);
    }

    /* ⭐ Размер задан кнопке, и картинка обязана его слушаться: без этого она берёт
       собственные 500 пикселей и разносит вёрстку руки. */
    .card-button :global(.playing-card) {
        width: 100%;
        height: auto;
    }
</style>
