<script>
    /**
     * Три карты веером на экране входа.
     *
     * ⭐ Картинки берутся из набора по умолчанию, а не зашиты: набор — сменный (ADR-009),
     * и заставка обязана меняться вместе с ним, иначе вход будет из другой игры.
     */
    import {onMount} from 'svelte';
    import {apiGet} from '../net/rest-client.js';

    let manifest = $state({});

    const FAN = [
        {code: '6-clubs', angle: -10, lift: 6, width: 64},
        {code: 'Joker', angle: 0, lift: 0, width: 70},
        {code: 'K-hearts', angle: 10, lift: 6, width: 64},
    ];

    onMount(async () => {
        try {
            const sets = await apiGet('/card-sets');
            const preferred = sets.find((set) => set.isDefault) ?? sets[0];
            manifest = (await apiGet(`/card-sets/${preferred.id}/manifest`)).cards ?? {};
        } catch {
            // Без картинок вход всё равно работает — просто без заставки.
        }
    });
</script>

<div class="fan">
    {#each FAN as card (card.code)}
        {#if manifest[card.code]}
            <img class="playing-card" src={manifest[card.code]} alt=""
                 style="width:{card.width}px; transform: rotate({card.angle}deg) translateY({card.lift}px)">
        {/if}
    {/each}
</div>

<style>
    .fan {
        display: flex;
        align-items: flex-end;
        justify-content: center;
        min-height: 101px;
    }

    .fan img + img {
        margin-left: -18px;
    }

    .fan img:nth-child(2) {
        z-index: 2;
    }
</style>
