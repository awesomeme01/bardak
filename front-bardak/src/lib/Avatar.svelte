<script>
    /**
     * Аватар игрока.
     *
     * ⭐ Картинок у игроков нет и заводить их незачем: за столом сидят свои, и узнают друг
     * друга по имени. Но одинаковые кружки читаются хуже разных, поэтому каждому достаётся
     * своя мордочка — выведенная из его идентификатора, то есть постоянная. Сегодня лис —
     * значит, и завтра лис.
     */
    import {avatarOf} from './naming.js';

    let {userId = '', size = 52, active = false, ring = false} = $props();

    const face = $derived(avatarOf(userId));
</script>

<span class="avatar" class:active class:ring
      style="width:{size}px; height:{size}px; font-size:{Math.round(size * 0.5)}px">
    {face}
</span>

<style>
    .avatar {
        border-radius: 50%;
        background: radial-gradient(60% 60% at 35% 30%, #4a453b, #2b271f);
        border: 2px solid rgba(255, 255, 255, 0.16);
        display: inline-flex;
        align-items: center;
        justify-content: center;
        flex: none;
        line-height: 1;
    }

    /* Чей ход — тот и подсвечен: это первое, что ищут глазами. */
    .active {
        border-color: var(--gold);
    }

    .ring {
        animation: turn-ring 1.9s ease-in-out infinite;
    }
</style>
