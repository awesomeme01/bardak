<script>
    /**
     * Место соседа за столом — одна ячейка рейки.
     *
     * ⭐ Соперник занимает РОВНО высоту рейки и ни пикселем больше: аватар, имя, строка
     * статуса. Всё остальное — бейджами поверх аватара, а не новыми строками. Раньше
     * место росло на четыре-пять строк (аватар · имя · веер со счётом · слот навеса ·
     * пилюля решения), и при пятерых рейка съедала пол-экрана, а стол упирался в руку.
     *
     * ⭐ Рука соседа не рисуется веером вовсе — только счётчик: чужих карт на устройстве
     * нет физически, и веер из девяти рубашек соврал бы про то, чего мы не знаем.
     */
    import Avatar from './Avatar.svelte';
    import Card from './Card.svelte';
    import {anchorPoint} from './motion.svelte.js';

    /**
     * ⭐ `size` — диаметр аватара: вдвоём соперник крупный и сидит напротив, впятером
     * все мельчают, иначе четверо не встают в один ряд.
     */
    let {seat, size = 60, active = false, defending = false, decision = null,
         taking = false, hangCta = null, onHang = null} = $props();

    /** Бейджи считаются от аватара: пропорции макета сохраняются на всех составах. */
    const badgeHeight = $derived(Math.round(size * 0.32));
    const navesWidth = $derived(Math.round(size * 0.32));
    const navesHeight = $derived(Math.round(size * 0.42));
    const backWidth = $derived(Math.round(size * 0.14));

    /** Роль в раздаче: она красит кольцо и не зависит от того, чьего хода ждут. */
    const tone = $derived(!seat.inDeal ? null : defending ? 'defend' : active ? 'attack' : null);

    /** Порядковые для тех, кто уже вышел. Дальше третьего в раздаче не бывает — пятеро максимум. */
    const ORDINALS = ['первым', 'вторым', 'третьим', 'четвёртым'];

    /**
     * Одна строка под именем на все случаи.
     *
     * ⚠️ Пас важнее очереди: право подкидывать остаётся за спасовавшим до конца раунда,
     * и без этой проверки он подписан «ходит» ровно тогда, когда ходить уже не может.
     *
     * ⭐ Решение соседа («перевёл», «бито») показывается ЗДЕСЬ же, вытесняя статус
     * на несколько секунд. Отдельной пилюлей оно добавляло пятую строку и толкало стол
     * вниз на разную величину — при разных составах по-разному.
     */
    const status = $derived.by(() => {
        if (!seat.inDeal) {
            if (!seat.exitPlace) {
                return {text: 'вышел', tone: 'out'};
            }
            const ordinal = ORDINALS[seat.exitPlace - 1] ?? `${seat.exitPlace}-м`;
            return {
                text: seat.exitPlace === 1 ? `вышел ${ordinal} · −1` : `вышел ${ordinal}`,
                tone: seat.exitPlace === 1 ? 'first' : 'out',
            };
        }
        if (decision) {
            return {text: decision.text, tone: decision.tone === 'plain' ? null : decision.tone};
        }
        // ⭐ «Поднял» держится всё время, пока стол докидывают: это не мелькнувшее
        // событие, а положение дел — человек уже забирает, и подкидывают именно ему.
        if (taking) {
            return {text: 'поднял', tone: 'take'};
        }
        if (seat.passed) {
            return {text: 'пас', tone: 'out'};
        }
        if (defending) {
            return {text: 'отбивается', tone: 'defend'};
        }
        if (active) {
            return {text: 'ходит', tone: 'attack'};
        }
        return null;
    });

    /** Что летит соседу следующим: шкала навесов и есть счёт в игре (ADR-017). */
    const flying = $derived(seat.nextIsJoker ? '🃏' : seat.nextNavesRank ?? '6');
</script>

<div class="seat" class:dim={!seat.inDeal} style="--avatar:{size}px">
    <div class="head">
        <span class="anchor" use:anchorPoint={`seat-${seat.seatNo}`}></span>
        <Avatar userId={seat.userId} {size} {tone} pulse={active && !seat.passed}/>

        <!-- Счёт карт: бейдж на аватаре, а не строка под ним. -->
        <span class="count mono" style="height:{badgeHeight}px">
            <Card faceDown width={backWidth}/>
            {seat.cardsCount}
        </span>

        <!--
          ⭐ Навесы соседа сведены к ОДНОМУ бейджу: «что летит следующим». Стопка карт
          показывала, сколько навесили, а в игре считают, сколько осталось до джокера
          (ADR-017) — и это ровно один символ вместо ряда картинок.
        -->
        <span class="naves mono" class:hung={seat.hung.length > 0} class:gold={seat.nextIsJoker}
              style="width:{navesWidth}px; height:{navesHeight}px"
              use:anchorPoint={`hung-${seat.seatNo}`}>{flying}</span>

        <!--
          ⭐ Кнопка навеса висит ПОВЕРХ имени, а не отдельной строкой: окно навеса
          короткое, кнопка нужна у жертвы, но высоту рейки менять нельзя — иначе стол
          дёргается вверх-вниз в самый неподходящий момент.
        -->
        {#if hangCta}
            <button class="hang-cta mono" type="button" onclick={onHang}>{hangCta}</button>
        {/if}
    </div>

    <div class="name" class:defending>{seat.displayName}</div>

    <div class="status mono" class:attack={status?.tone === 'attack'}
         class:defend={status?.tone === 'defend'} class:out={status?.tone === 'out'}
         class:take={status?.tone === 'take'} class:first={status?.tone === 'first'}>
        {status?.text ?? ''}
    </div>
</div>

<style>
    /**
     * ⚠️ Высота ячейки задана содержимым, но содержимое строго ограничено тремя
     * элементами: аватар, имя, статус. Добавить сюда четвёртую строку — значит сломать
     * рейку при пяти игроках. Всё новое вешается бейджем на аватар.
     */
    .seat {
        display: flex;
        flex-direction: column;
        align-items: center;
        min-width: 0;
        flex: 1 1 0;
    }

    .seat.dim {
        opacity: 0.5;
    }

    .head {
        position: relative;
        width: var(--avatar);
        height: var(--avatar);
    }

    .anchor {
        position: absolute;
        left: 50%;
        top: 50%;
    }

    .count {
        position: absolute;
        right: -7px;
        top: -3px;
        padding: 0 5px;
        border-radius: 10px;
        background: rgba(8, 12, 10, 0.92);
        border: 1px solid rgba(255, 255, 255, 0.2);
        display: flex;
        align-items: center;
        gap: 4px;
        font-size: 10px;
        font-weight: 700;
        line-height: 1;
    }

    .count :global(.playing-card) {
        border-radius: 1px;
    }

    .naves {
        position: absolute;
        left: -7px;
        bottom: -3px;
        border-radius: 3px;
        border: 1px dashed rgba(240, 205, 138, 0.5);
        background: rgba(8, 12, 10, 0.75);
        display: flex;
        align-items: center;
        justify-content: center;
        font-size: 9px;
        color: var(--gold);
        line-height: 1;
    }

    /* Сплошная рамка — уже навешено; пунктир — ещё летит. */
    .naves.hung {
        border-style: solid;
        border-color: rgba(240, 205, 138, 0.75);
    }

    .naves.gold {
        border-color: var(--gold);
        box-shadow: 0 0 0 2px rgba(240, 205, 138, 0.18);
    }

    .hang-cta {
        position: absolute;
        left: 50%;
        top: calc(100% - 2px);
        transform: translateX(-50%);
        z-index: 6;
        white-space: nowrap;
        padding: 3px 8px;
        border-radius: 9px;
        border: 1px solid var(--gold);
        background: rgba(20, 14, 8, 0.95);
        color: var(--gold);
        font-size: 10px;
        font-weight: 700;
        cursor: pointer;
        animation: hang-pulse 1.4s ease-in-out infinite;
    }

    @keyframes hang-pulse {
        0%, 100% { box-shadow: 0 0 0 0 rgba(240, 205, 138, 0); }
        50% { box-shadow: 0 0 0 6px rgba(240, 205, 138, 0.2); }
    }

    @media (prefers-reduced-motion: reduce) {
        .hang-cta { animation: none; }
    }

    .name {
        margin-top: 6px;
        font-size: 11.5px;
        font-weight: 700;
        max-width: 100%;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    .name.defending {
        color: #f0959d;
    }

    /**
     * ⚠️ Строка статуса занимает место ВСЕГДА, даже пустая: иначе появление «пас»
     * толкает вниз всю рейку, а вместе с ней и стол.
     */
    .status {
        margin-top: 2px;
        height: 12px;
        font-size: 9px;
        letter-spacing: 0.06em;
        color: rgba(242, 240, 234, 0.4);
        white-space: nowrap;
    }

    .status.attack { color: var(--gold); }
    .status.defend { color: #f0959d; }
    .status.take { color: #7fd8a6; }
    .status.first { color: #7fd8a6; }
</style>
