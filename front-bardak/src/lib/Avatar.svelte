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

    /**
     * ⭐ `tone` — роль в раздаче, а не «чей ход»: атака красная, защита зелёная, ждущие
     * без цвета вовсе. Цвет роли важнее подсветки очереди — за столом сначала ищут глазами
     * «кто на кого», и только потом «кто думает».
     */
    let {userId = '', avatar = null, size = 52, active = false, tone = null, pulse = false} = $props();

    const face = $derived(avatarOf(userId, avatar));
</script>

<span class="avatar" class:active class:pulse
      class:attack={tone === 'attack'} class:defend={tone === 'defend'}
      style="width:{size}px; height:{size}px; font-size:{Math.round(size * 0.5)}px">
    {face}
</span>

<style>
    .avatar {
        /* Якорь для кольца пульса — оно лежит поверх аватара и размеров не меняет. */
        position: relative;
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

    /* Кто нападает. Красный здесь не «опасность», а сторона — как повязка на рукаве. */
    .attack {
        border-color: var(--seat-attack);
        box-shadow: 0 0 0 3px rgba(232, 98, 108, 0.16);
    }

    .defend {
        border-color: var(--seat-defend);
        box-shadow: 0 0 0 3px rgba(127, 216, 166, 0.16);
    }

    /**
     * Пульс достаётся тому, кого сейчас ждут, — поверх цвета его роли.
     *
     * ⚠️ Кольцо — отдельный псевдоэлемент, а не растущая тень самого аватара. Тень
     * пришлось бы перерисовывать каждый кадр; у псевдоэлемента меняются только
     * `transform` и `opacity`, и слой остаётся тем же (см. `pulse-ring` в styles.css).
     */
    .pulse::after {
        content: '';
        position: absolute;
        inset: -2px;
        border-radius: 50%;
        border: 3px solid var(--gold);
        /* Базово кольца нет: движение целиком задают кадры (важно для reduced-motion). */
        opacity: 0;
        animation: pulse-ring 1.9s ease-in-out infinite;
        pointer-events: none;
    }

    .pulse.attack::after {
        border-color: var(--seat-attack);
    }

    .pulse.defend::after {
        border-color: var(--seat-defend);
    }
</style>
