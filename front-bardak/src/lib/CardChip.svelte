<script>
    /**
     * Мини-карта для значков: не уменьшенная картинка, а нарисованная заново.
     *
     * ⭐ Настоящая карта в двадцать пикселей нечитаема — номинал на ней живёт в углу
     * и превращается в точку. Здесь номинал и масть занимают всю площадь, поэтому
     * «что висит» читается даже на аватаре соперника.
     *
     * ⚠️ Это НЕ замена карте: в руке, на столе и в навесе внизу экрана лежат настоящие
     * картинки набора (ADR-009). Фишка нужна ровно там, где места нет.
     */
    import {shortCard} from './naming.js';

    let {code, width = 26} = $props();

    /** «6♦» → ранг и масть по отдельности: масть красим, ранг делаем крупным. */
    const parts = $derived.by(() => {
        const short = shortCard(code);
        if (short === '🃏') {
            return {rank: '🃏', suit: '', red: false};
        }
        const suit = short.slice(-1);
        return {rank: short.slice(0, -1), suit, red: suit === '♦' || suit === '♥'};
    });
</script>

<span class="chip" class:red={parts.red} class:joker={parts.rank === '🃏'}
      style="width:{width}px; height:{Math.round(width * 1.452)}px; font-size:{Math.round(width * 0.46)}px">
    <span class="rank">{parts.rank}</span>
    {#if parts.suit}<span class="suit">{parts.suit}</span>{/if}
</span>

<style>
    .chip {
        display: inline-flex;
        flex-direction: column;
        align-items: center;
        justify-content: center;
        line-height: 1;
        border-radius: 3px;
        background: #f4f1ea;
        color: #191410;
        box-shadow: 0 2px 6px rgba(0, 0, 0, 0.45);
        font-family: var(--body);
        font-weight: 800;
        flex: none;
    }

    .chip.red {
        color: #c1121f;
    }

    /* Джокер: золотая фишка — его ни с чем не спутать, и он же конец игры. */
    .chip.joker {
        background: linear-gradient(160deg, #f5d79b, #c99a4e);
        color: #191410;
    }

    .suit {
        font-size: 0.8em;
        margin-top: 1px;
    }
</style>
