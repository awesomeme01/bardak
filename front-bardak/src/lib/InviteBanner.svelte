<script>
    /**
     * «Тебя зовут за стол» на экране входа и регистрации.
     *
     * ⭐ Смысл — объяснить, зачем регистрироваться, до того как человек увидит форму.
     * Без этого ссылка от друга открывает безымянный вход, и незнакомый с игрой уходит,
     * так и не поняв, куда попал.
     */
    import {onMount} from 'svelte';
    import {inviteLink, loadInviteTable} from '../stores/invite-link.svelte.js';

    onMount(loadInviteTable);
</script>

{#if inviteLink.code}
    <div class="invite-banner" class:dead={inviteLink.error || inviteLink.table?.joinable === false}>
        {#if inviteLink.table}
            <div class="title">Тебя зовут за стол «{inviteLink.table.name}»</div>
            <div class="mono sub">
                {inviteLink.table.seatsTaken} из {inviteLink.table.maxPlayers} мест занято
                <span class="sep">·</span> код {inviteLink.code}
            </div>
            {#if !inviteLink.table.joinable}
                <!-- ⚠️ Честно и сразу: иначе человек регистрируется и упирается в отказ. -->
                <div class="sub warn">Матч уже начался или мест не осталось — за этот стол не сесть</div>
            {:else}
                <div class="sub">Войди или заведи учётку — и сядешь за стол сразу</div>
            {/if}
        {:else if inviteLink.error}
            <div class="title">{inviteLink.error}</div>
            <div class="mono sub">код {inviteLink.code}</div>
        {:else}
            <div class="mono sub">Открываю приглашение…</div>
        {/if}
    </div>
{/if}

<style>
    .invite-banner {
        margin: 0 20px 14px;
        padding: 12px 16px;
        border-radius: 14px;
        border: 1px solid var(--gold-soft);
        background: rgba(240, 205, 138, 0.12);
        text-align: center;
    }

    .invite-banner.dead {
        border-color: var(--line-strong);
        background: rgba(255, 255, 255, 0.06);
    }

    .title {
        font-size: 14px;
        font-weight: 700;
        color: var(--gold);
    }

    .dead .title {
        color: var(--text-70);
    }

    .sub {
        margin-top: 4px;
        font-size: 11px;
        color: var(--text-55);
    }

    .warn {
        color: var(--seat-attack);
    }

    .sep {
        opacity: 0.4;
    }
</style>
