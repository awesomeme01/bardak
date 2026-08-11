<script>
    import {table} from '../stores/table.svelte.js';

    let {code, faceDown = false, small = false, onclick = null, selected = false} = $props();

    // Джокеры нумерованы (Joker-3), а картинка у них одна: номер нужен движку,
    // чтобы отличать карты, а манифесту — нет.
    const assetKey = $derived(code?.startsWith('Joker-') ? 'Joker' : code);
    const src = $derived(faceDown ? table.manifest['back'] : table.manifest[assetKey]);
</script>

{#if onclick}
    <button class="card-face {small ? 'small' : ''}" class:selected type="button" {onclick}
            title={code}>
        {#if src}<img {src} alt={code}>{:else}<span>{code}</span>{/if}
    </button>
{:else}
    <div class="card-face {small ? 'small' : ''}" title={faceDown ? 'рубашка' : code}>
        {#if src}<img {src} alt={faceDown ? '' : code}>{:else}<span>{faceDown ? '🂠' : code}</span>{/if}
    </div>
{/if}
