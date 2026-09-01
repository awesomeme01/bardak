<script>
    /**
     * Кольцо времени вокруг аватара: сколько осталось на ход.
     *
     * ⭐ Остаток приходит с сервера при каждом снимке, а между снимками кольцо тикает
     * само. Считать целиком у себя нельзя: по СЕРВЕРНЫМ часам за молчащего ходят (§5.1),
     * и разошедшийся счётчик выглядел бы как отобранный ход.
     *
     * ⚠️ Кольцо появляется, только когда часы реально идут. При `autoMove = false` сервер
     * не отдаёт остаток вовсе — и рисовать нечего: ход ждёт хозяина сколько угодно.
     */
    let {seconds = null, size = 60, total = 30} = $props();

    let left = $state(null);

    $effect(() => {
        left = seconds;
        if (seconds === null || seconds === undefined) {
            return;
        }
        const timer = setInterval(() => {
            left = Math.max(0, (left ?? 0) - 1);
        }, 1000);
        return () => clearInterval(timer);
    });

    /** Полная длина — чтобы кольцо не «прыгало», если сервер даст больше ожидаемого. */
    const span = $derived(Math.max(total, seconds ?? 0, 1));

    const radius = $derived(size / 2 - 2);
    const circumference = $derived(2 * Math.PI * radius);
    const offset = $derived(circumference * (1 - Math.max(0, left ?? 0) / span));

    /** Три состояния: спокойное, «пора», «вот-вот». */
    const alarm = $derived(left !== null && left <= 5);
    const warn = $derived(left !== null && left <= 20 && !alarm);
</script>

{#if left !== null && left !== undefined}
    <svg class="ring" class:warn class:alarm width={size} height={size} viewBox="0 0 {size} {size}"
         aria-hidden="true">
        <!-- Дорожка: без неё кольцо на тёмном сукне читается как случайная дуга. -->
        <circle class="track" cx={size / 2} cy={size / 2} r={radius}/>
        <circle class="progress" cx={size / 2} cy={size / 2} r={radius}
                stroke-dasharray={circumference} stroke-dashoffset={offset}/>
    </svg>
{/if}

<style>
    /**
     * ⚠️ Кольцо лежит ПОВЕРХ аватара и ничего не двигает: место соперника ужато
     * до рейки, и лишний пиксель высоты сдвинул бы весь стол.
     */
    .ring {
        position: absolute;
        left: 50%;
        top: 50%;
        transform: translate(-50%, -50%) rotate(-90deg);
        pointer-events: none;
        z-index: 5;
    }

    .track {
        fill: none;
        stroke: rgba(255, 255, 255, 0.1);
        stroke-width: 3;
    }

    /* ⭐ Переход по длине, а не перерисовка: секунда убывает плавно, без рывка. */
    .progress {
        fill: none;
        stroke: var(--gold);
        stroke-width: 3;
        stroke-linecap: round;
        transition: stroke-dashoffset 0.9s linear, stroke 0.3s ease;
    }

    .warn .progress {
        stroke: #f0a94e;
        animation: ring-warn 1.1s ease-in-out infinite;
    }

    .alarm .progress {
        stroke: var(--red);
        animation: ring-alarm 0.45s ease-in-out infinite;
    }

    @keyframes ring-warn {
        0%, 100% { opacity: 1; }
        50% { opacity: 0.45; }
    }

    /* Под самый конец мигание частое: это последнее предупреждение, а не украшение. */
    @keyframes ring-alarm {
        0%, 100% { opacity: 1; }
        50% { opacity: 0.25; }
    }

    @media (prefers-reduced-motion: reduce) {
        .warn .progress,
        .alarm .progress {
            animation: none;
        }
    }
</style>
