<script>
    /**
     * Часы хода.
     *
     * ⭐ Остаток приходит с сервера при каждом снимке состояния, а между снимками мы просто
     * тикаем вниз. Считать самим от «тридцати» нельзя: по серверным часам за молчащего
     * ходят (§5.1), и разошедшийся счётчик выглядел бы как отобранный ход.
     */
    let {seconds = null, active = false} = $props();

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
</script>

{#if left !== null && left !== undefined}
    <span class="clock" class:urgent={active && left <= 10}>· {left} с</span>
{/if}

<style>
    .clock {
        color: var(--text-45);
        font-variant-numeric: tabular-nums;
    }

    .urgent {
        color: var(--red);
    }
</style>
