<script>
    /**
     * Слой летящих карт поверх стола.
     *
     * ⭐ Слой ничего не решает: он рисует то, что положили в `flights`. Кто и почему летит,
     * знает только хранилище стола, которое читает события сервера.
     *
     * ⭐ `pointer-events: none` на всём слое обязателен: летящая карта не должна перехватить
     * нажатие по карте в руке, мимо которой пролетает.
     */
    import Card from './Card.svelte';
    import {TIMING, flights, spotlight} from './motion.svelte.js';

    const RATIO = 726 / 500;

    /**
     * Где и какого размера карта в текущей точке полёта.
     *
     * <p>Узел свёрстан размером точки прибытия, а размер вылета доигрывается масштабом:
     * менять `width` покадрово — это пересчёт вёрстки на каждый кадр, а `transform`
     * браузер уводит на видеокарту.
     */
    function frameOf(flight) {
        const point = flight.arrived ? flight.to : flight.from;
        const scale = (flight.arrived ? flight.toWidth : flight.fromWidth) / flight.toWidth;
        const height = flight.toWidth * RATIO;
        return `translate(${point.x - flight.toWidth / 2}px, ${point.y - height / 2}px)`
            + ` rotate(${flight.arrived ? flight.spin : 0}deg) scale(${scale})`;
    }
</script>

<div class="flights" aria-hidden="true">
    {#each flights as flight (flight.id)}
        <span class="flight"
              style="transform: {frameOf(flight)};
                     transition-duration: {TIMING.move}ms; transition-delay: {flight.delay}ms;">
            <Card code={flight.code} faceDown={flight.faceDown} width={flight.toWidth}/>
        </span>
    {/each}
</div>

{#if spotlight.card}
    <!-- Потайной козырь меняет козырь всему столу (§1.9) — его показывают, а не упоминают. -->
    <div class="spotlight" aria-live="polite">
        <div class="spot-card">
            <Card code={spotlight.card} width={128}/>
        </div>
        <span class="spot-label mono">потайной козырь</span>
    </div>
{/if}

<style>
    .flights {
        position: fixed;
        inset: 0;
        pointer-events: none;
        z-index: 40;
    }

    .flight {
        position: absolute;
        left: 0;
        top: 0;
        display: block;
        transform-origin: center center;
        transition-property: transform;
        /* Карту ведёт рука, а не пружина: разгон и мягкая остановка без отскока. */
        transition-timing-function: cubic-bezier(0.22, 0.61, 0.25, 1);
        will-change: transform;
    }

    /* Карта в воздухе отбрасывает тень длиннее лежащей — иначе она не «над столом». */
    .flight :global(.playing-card) {
        box-shadow: 0 18px 34px rgba(0, 0, 0, 0.62);
    }

    .spotlight {
        position: fixed;
        inset: 0;
        z-index: 50;
        pointer-events: none;
        display: flex;
        flex-direction: column;
        align-items: center;
        justify-content: center;
        gap: 14px;
        background: rgba(6, 9, 8, 0.55);
        animation: spot-veil 0.25s ease both;
    }

    /* Карта приходит рубашкой и переворачивается — иначе непонятно, что её вскрыли. */
    .spot-card {
        transform-style: preserve-3d;
        animation: spot-flip 0.62s cubic-bezier(0.3, 0.8, 0.3, 1) both;
    }

    .spot-label {
        font-size: 11px;
        letter-spacing: 0.14em;
        text-transform: uppercase;
        color: var(--gold);
        animation: spot-veil 0.3s 0.35s ease both;
    }

    @keyframes spot-veil {
        from {
            opacity: 0;
        }
        to {
            opacity: 1;
        }
    }

    @keyframes spot-flip {
        from {
            transform: perspective(900px) rotateY(-92deg) scale(0.72);
        }
        to {
            transform: perspective(900px) rotateY(0deg) scale(1);
        }
    }

    /* Кто просил не двигать — тому не двигаем: перелёт схлопывается в мгновенную смену. */
    @media (prefers-reduced-motion: reduce) {
        .flight {
            transition-duration: 1ms !important;
        }

        .spot-card,
        .spotlight,
        .spot-label {
            animation-duration: 1ms;
        }
    }
</style>
