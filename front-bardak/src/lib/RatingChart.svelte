<script>
    /**
     * График рейтинга. Рисуется одной ломаной в SVG: библиотека графиков ради одной
     * линии — это сотни килобайт и чужие стили, а точек здесь десятки.
     */
    let {points = []} = $props();

    const WIDTH = 320;
    const HEIGHT = 96;
    const PADDING = 4;

    // История приходит сверху вниз (новое первым), а график читается слева направо.
    const values = $derived([...points].reverse().map((point) => Number(point.ratingAfter)));
    const min = $derived(values.length ? Math.min(...values) : 0);
    const max = $derived(values.length ? Math.max(...values) : 0);

    const coordinates = $derived(values.map((value, index) => {
        const x = values.length === 1
            ? WIDTH / 2
            : PADDING + (index * (WIDTH - 2 * PADDING)) / (values.length - 1);
        // Плоская история — прямая посередине, иначе делили бы на ноль.
        const span = max - min;
        const y = span === 0
            ? HEIGHT / 2
            : HEIGHT - PADDING - ((value - min) * (HEIGHT - 2 * PADDING)) / span;
        return {x, y};
    }));

    const line = $derived(coordinates.map(({x, y}) => `${x.toFixed(1)},${y.toFixed(1)}`).join(' '));
</script>

{#if values.length}
    <svg viewBox="0 0 {WIDTH} {HEIGHT}" class="chart" role="img" aria-label="График рейтинга">
        <polyline points={line}/>
        <!-- Один матч ломаной не рисуется: точку надо показать, иначе график пуст. -->
        {#each coordinates as point}
            <circle cx={point.x} cy={point.y} r="2.5"/>
        {/each}
    </svg>
    <p class="range">от {min.toFixed(0)} до {max.toFixed(0)} за {values.length} матч(ей)</p>
{:else}
    <!-- Пустой график — не пустое место: рамка показывает, что тут появится линия. -->
    <div class="chart empty" role="img" aria-label="График рейтинга пока пуст">
        <span class="mono">рейтинг по матчам появится после первой партии</span>
    </div>
{/if}

<style>
    .chart {
        width: 100%;
        max-width: 320px;
        height: 96px;
    }

    .empty {
        display: flex;
        align-items: center;
        justify-content: center;
        padding: 0 14px;
        border: 1px dashed rgba(255, 255, 255, 0.16);
        border-radius: 12px;
        text-align: center;
        font-size: 10px;
        letter-spacing: 0.05em;
        color: var(--text-45);
    }

    polyline {
        fill: none;
        stroke: #6fcf97;
        stroke-width: 2;
        stroke-linejoin: round;
    }

    circle {
        fill: #6fcf97;
    }

    .range {
        opacity: 0.7;
        font-size: 0.85rem;
    }
</style>
