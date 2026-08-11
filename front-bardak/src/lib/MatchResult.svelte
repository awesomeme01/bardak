<script>
    import {table} from '../stores/table.svelte.js';
    import {degreeName, deltaText, levelName} from './naming.js';

    let {onClose} = $props();

    const players = $derived(table.result?.players ?? []);
</script>

<section class="card">
    <h2>Матч окончен</h2>
    <table class="result">
        <thead>
        <tr>
            <th>Место</th>
            <th>Игрок</th>
            <th>Навес</th>
            <th>Итог</th>
            <th>Рейтинг</th>
        </tr>
        </thead>
        <tbody>
        {#each players as player (player.userId)}
            <tr>
                <td>{player.place}</td>
                <td>{player.displayName}</td>
                <td>{levelName(player.navesLevel)}</td>
                <td>
                    {#if player.lossDegree}
                        <span class="badge badge-fail">{degreeName(player.lossDegree)}</span>
                    {:else}
                        —
                    {/if}
                </td>
                <td class:up={Number(player.ratingDelta) > 0} class:down={Number(player.ratingDelta) < 0}>
                    {deltaText(player.ratingDelta)}
                </td>
            </tr>
        {/each}
        </tbody>
    </table>
    <div class="row">
        <button type="button" onclick={onClose}>К столу</button>
    </div>
</section>

<style>
    .result {
        border-collapse: collapse;
        width: 100%;
    }

    .result th, .result td {
        padding: 0.4rem 0.6rem;
        text-align: left;
        border-bottom: 1px solid rgba(255, 255, 255, 0.12);
    }

    .up {
        color: #6fcf97;
    }

    .down {
        color: #eb5757;
    }
</style>
