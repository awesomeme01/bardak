<script>
    /**
     * Игровой стол.
     *
     * ⭐ Ни одно правило здесь не воспроизводится: что можно сделать — приходит с сервера
     * списком (ADR-003). Экран только раскладывает этот список по кнопкам и подсвечивает
     * карты, которыми разрешено пойти.
     *
     * ⭐ Ход подтверждается кнопкой, а не совершается касанием карты. На телефоне промах
     * по карте стоит хода, а иногда и партии: сначала карта выбирается, потом главная
     * кнопка говорит, что именно с ней произойдёт.
     *
     * ⭐ Рука не подсказывает, чем можно пойти. Обведённой оказывалась половина карт, а
     * когда бить было нечем — гасла вся рука разом и выглядела сломанной. Подсказка
     * осталась там, где её ниоткуда не узнать: в окне навеса.
     */
    import {flip} from 'svelte/animate';
    import Card from './Card.svelte';
    import CardFlights from './CardFlights.svelte';
    import Seat from './Seat.svelte';
    import TurnClock from './TurnClock.svelte';
    import {play, table} from '../stores/table.svelte.js';
    import {connection} from '../stores/connection.svelte.js';
    import {TIMING, anchorPoint, flyFrom} from './motion.svelte.js';
    import {sound, toggleSound} from './sound.svelte.js';
    import {draggable, dropTargets} from './drag.svelte.js';
    import {isRedSuit, suitGlyph} from './naming.js';

    let {onLeave = null, onMenu = null} = $props();

    const game = $derived(table.game);

    /**
     * ⚠️ Выход спрашивает подтверждение, и не из вежливости: уходящий отменяет партию
     * всем за столом. Случайное попадание по кнопке стоило бы чужой игры.
     */
    let leaveAsked = $state(false);

    const actions = $derived.by(() => {
        const list = game?.availableActions ?? [];
        return {
            attacks: list.filter((a) => a.type === 'PLAY_CARD' && !a.payload.targetCardCode),
            defends: list.filter((a) => a.type === 'PLAY_CARD' && a.payload.targetCardCode),
            transfers: list.filter((a) => a.type === 'TRANSFER'),
            hangs: list.filter((a) => a.type === 'HANG_CARD'),
            pass: list.find((a) => a.type === 'PASS'),
            take: list.find((a) => a.type === 'TAKE'),
            hangSkip: list.find((a) => a.type === 'HANG_SKIP'),
            reveal: list.find((a) => a.type === 'REVEAL_FACE_DOWN' && !a.payload.targetCardCode),
            trumps: list.filter((a) => a.type === 'CHOOSE_TRUMP'),
        };
    });

    /**
     * Карты, которыми можно навесить, — единственная подсветка, оставшаяся в руке.
     *
     * <p>Навес случается редко и по своим правилам шкалы: какой картой навешивают, игрок
     * не выведет ни из стола, ни из руки. Всё остальное он узнаёт, нажав на карту.
     */
    const hangable = $derived(new Set(actions.hangs.map((action) => action.payload.cardCode)));

    /**
     * Действует ли правило отстающего (§2.3): у жертвы САМЫЙ НИЗКИЙ уровень среди тех,
     * кто ещё в раздаче, и он такой один. Тогда навесить может любой, а копии одного
     * ранга можно отдать пачкой — уровень всё равно поднимется на одну ступень.
     *
     * ⭐ Считается на клиенте, а не приходит с сервера: всё нужное уже есть в снимке
     * (уровни и «кто в раздаче»), и заводить ради этого поле в протоколе значило бы
     * держать один и тот же факт в двух местах. Сервер всё равно проверит сам.
     */
    const laggardRule = $derived.by(() => {
        if (!hangingNow) {
            return false;
        }
        const active = (game?.players ?? []).filter((player) => player.inDeal);
        const stepOf = (player) => (player.nextIsJoker
            ? RANK_ORDER.length
            : RANK_ORDER.indexOf(player.nextNavesRank ?? '6'));
        let lowest = Infinity;
        let onLowest = 0;
        let lowestSeat = null;
        for (const player of active) {
            const step = stepOf(player);
            if (step < lowest) {
                lowest = step;
                onLowest = 1;
                lowestSeat = player.seatNo;
            } else if (step === lowest) {
                onLowest++;
            }
        }
        return onLowest === 1 && lowestSeat === game.hangingVictimSeat;
    });

    /**
     * Отмеченные для навеса карты.
     *
     * ⚠️ Кнопка навеса раньше отправляла карту сразу, и при правиле отстающего это
     * означало «нажал — улетела одна», без возможности отдать больше или выбрать какие.
     * Теперь карты набираются нажатиями, а кнопка отправляет набор.
     */
    let hangPicks = $state(new Set());

    // Окно закрылось — набор ни к чему не относится.
    $effect(() => {
        if (!hangingNow && hangPicks.size) {
            hangPicks = new Set();
        }
    });

    /** Идёт окно навеса — своё или чужое. */
    const hangingNow = $derived(game?.hangingVictimSeat !== null && game?.hangingVictimSeat !== undefined);

    let selected = $state(null);

    // Выбранная карта могла уйти из руки — например, за нас сходил таймер.
    $effect(() => {
        if (selected && !game?.myHand.includes(selected)) {
            selected = null;
        }
    });

    const targets = $derived(
        selected ? actions.defends.filter((a) => a.payload.cardCode === selected) : []);

    /** Что произойдёт по главной кнопке. Пусто — сейчас ход не за нами. */
    const primary = $derived.by(() => {
        if (actions.trumps.length) {
            return null;   // выбор козыря — отдельный ряд кнопок, там нечего подтверждать
        }
        if (selected) {
            const hang = actions.hangs.find((a) => a.payload.cardCode === selected);
            if (hang) {
                return {label: `Навесить ${short(selected)}`, action: hang};
            }
            const attack = actions.attacks.find((a) => a.payload.cardCode === selected);
            if (attack) {
                return {label: `Атаковать ${short(selected)}`, action: attack};
            }
            const transfer = actions.transfers.find((a) => a.payload.cardCode === selected);
            if (transfer) {
                return {label: `Перевести ${short(selected)}`, action: transfer, tone: 'blue'};
            }
            if (targets.length === 1) {
                return {label: `Отбиться ${short(selected)}`, action: targets[0]};
            }
            return null;   // целей несколько — пусть укажет, какую карту бьёт
        }
        if (actions.reveal) {
            return {label: 'Вскрыть скрытую', action: actions.reveal};
        }
        return null;
    });

    const myTurn = $derived((game?.availableActions ?? []).length > 0);
    const unbeaten = $derived((game?.table ?? []).filter((slot) => !slot.defend));
    const iDefend = $derived(game?.defenderSeat === game?.mySeat);

    /**
     * ⭐ «Беру» имеет смысл, только пока на столе есть неотбитое. Всё отбито — забирать
     * нечего и незачем: игрок унёс бы в руку и свою же защиту. Кнопка при этом остаётся
     * (правила её не запрещают), но перестаёт быть красной и звать нажать.
     */
    const takeMatters = $derived(unbeaten.length > 0);

    /**
     * ⚠️ «Беру» при полностью отбитом столе — почти всегда мисклик, и цена ему —
     * унести в руку свои же отбитые карты. Живая партия: игрок отбил всё, включая
     * джокера, нажал «Беру» — и докинутого в добор короля крыть было уже нельзя.
     * Поэтому пустое взятие спрашивает второй раз; настоящее — уходит с первого.
     */
    let takeAsked = $state(false);
    $effect(() => {
        void game;          // любой новый снимок сбрасывает вопрос
        takeAsked = false;
    });

    function tapTake() {
        if (!takeMatters && !takeAsked) {
            takeAsked = true;
            return;
        }
        run(actions.take);
    }

    /** Карта выбрана, а сделать ею нечего — это надо сказать словами, а не молчать. */
    const selectedIsDead = $derived.by(() => {
        if (!selected || actions.trumps.length) {
            return false;
        }
        const usable = [actions.attacks, actions.transfers, actions.hangs]
            .some((list) => list.some((action) => action.payload.cardCode === selected));
        return !usable && targets.length === 0;
    });

    /**
     * ⭐ Кнопка ЗОВЁТ нажать (пульсирует), когда игра ждёт только её: противник всё
     * отбил — раунд закрывает пас нападающего, и без него стол стоит. «Беру» из зова
     * исключено намеренно: оно красное и нежелательное, подталкивать к нему нельзя.
     */
    const passUrges = $derived(Boolean(actions.pass) && unbeaten.length === 0 && !hangingNow);

    /** Вскрытие обязательно, когда других ходов нет (MUST_REVEAL_FACE_DOWN). */
    const revealUrges = $derived(Boolean(actions.reveal) && !actions.attacks.length
        && !actions.defends.length && !actions.transfers.length && !actions.hangs.length);

    const prompt = $derived.by(() => {
        if (actions.trumps.length) {
            return 'Назови козырь';
        }
        if (hangingNow) {
            return game.hangingVictimSeat === game.mySeat ? 'Тебе навешивают' : 'Твой навес';
        }
        if (selected && targets.length > 1) {
            return 'Укажи, какую карту бьёшь';
        }
        if (iDefend && unbeaten.length === 1) {
            // ⭐ Карта названа прямо в подсказке: при одной неотбитой искать её глазами
            // на столе незачем, а на телефоне это ещё и лишнее движение.
            return `Отбей ${short(unbeaten[0].attack)}`;
        }
        if (iDefend && unbeaten.length) {
            return 'Отбивайся';
        }
        if (iDefend && game.table.length) {
            return 'Всё отбито — ход за соперником';
        }
        // ⚠️ Право подкидывать остаётся за спасовавшим до конца раунда: без проверки
        // на реальные ходы экран звал подкинуть того, кому нечем и некуда.
        if (game.canAttackSeat === game.mySeat && myTurn) {
            return game.table.length ? 'Подкидывай или пасуй' : 'Твоя атака';
        }
        return null;
    });

    /**
     * Соперники слева направо — по часовой стрелке от меня.
     *
     * ⭐ Я всегда внизу, поэтому следующий по ходу сосед сидит слева, а не справа: если
     * смотреть на стол как на циферблат, движение от шести часов по часовой идёт сначала
     * влево и вверх. Так порядок мест на экране совпадает с порядком хода в движке
     * ({@code nextActiveSeatAfter} — это следующий номер места по кругу).
     *
     * ⚠️ Раньше места шли просто по возрастанию номера. При моём месте 0 это совпадало
     * с правдой случайно, а на любом другом — расходилось: сосед, которому я передаю ход,
     * оказывался не там, где его ищут глазами.
     */
    const opponents = $derived.by(() => {
        const players = game?.players ?? [];
        const count = players.length;
        return players
            .filter((seat) => seat.seatNo !== game.mySeat)
            .sort((left, right) => clockwise(left.seatNo, count) - clockwise(right.seatNo, count));
    });

    function clockwise(seatNo, count) {
        return (seatNo - game.mySeat + count) % count;
    }
    const me = $derived((game?.players ?? []).find((seat) => seat.seatNo === game.mySeat));

    /**
     * ⭐ Размеры стола зависят от СОСТАВА, а не от того, сколько влезло. В макете два
     * полюса — вдвоём крупно, впятером компактно, — и между ними разложены тройка
     * с четвёркой. Это не «подгон под экран»: чем меньше людей, тем больше места
     * каждому, и карты можно рисовать крупнее.
     */
    const SIZES = {
        1: {avatar: 82, stake: 60, hand: 78, pile: 76},
        2: {avatar: 72, stake: 58, hand: 76, pile: 74},
        3: {avatar: 66, stake: 56, hand: 72, pile: 72},
        4: {avatar: 60, stake: 54, hand: 70, pile: 72},
    };
    const sizes = $derived(SIZES[opponents.length] ?? SIZES[4]);

    /** В колоде осталась одна карта — та самая, что сменит козырь всему столу (§1.9). */
    const lastIsHiddenTrump = $derived(game?.deckLeft === 1);

    /** Подпись кнопки навеса у жертвы. Пусто — навешивать сейчас нечем или некому. */
    function hangCtaFor(seat) {
        if (game.hangingVictimSeat !== seat.seatNo || !actions.hangs.length) {
            return null;
        }
        const rank = seat.nextIsJoker ? '🃏' : seat.nextNavesRank ?? '';
        if (laggardRule && hangPicks.size > 1) {
            return `Навесить ${rank} · ${hangPicks.size}`.trim();
        }
        return `Навесить ${rank}`.trim();
    }

    /**
     * ⭐ Кнопка у жертвы навешивает сразу, а не выбирает карту.
     *
     * Подтверждать нечего: и карта, и жертва названы прямо на кнопке, а других вариантов
     * навеса в этот момент не бывает. Раньше она лишь подсвечивала карту в руке, и ход
     * надо было добить нижней кнопкой — с большой рукой её уносило за край экрана,
     * и нажатие выглядело так, будто кнопка не работает.
     */
    function takeHangCard() {
        const picked = [...hangPicks];
        if (picked.length) {
            // ⭐ Первая карта — обычное поле команды, остальные копии едут рядом:
            // так навес одной картой и навес пачкой остаются одной командой.
            const [first, ...rest] = picked;
            hangPicks = new Set();
            play({type: 'HANG_CARD', payload: {cardCode: first, moreCards: rest}});
            return;
        }
        const hang = actions.hangs[0];
        if (hang) {
            run(hang);
        }
    }

    /**
     * ⚠️ Веер сжимается под ширину экрана.
     *
     * Забравший стол легко держит полтора десятка карт, а веер из восемнадцати штук с
     * постоянным нахлёстом шире любого экрана: крайние карты уезжают за край вместе с
     * возможностью ими пойти. Нахлёст считается так, чтобы рука всегда помещалась целиком.
     */
    let handWidth = $state(0);
    let viewport = $state(typeof window !== 'undefined' ? window.innerWidth : 390);

    /**
     * ⭐ Сетка нарисована под 390px. На экранах уже — сжимаем ЦЕЛИКОМ и пропорционально,
     * а не переверстываем: полосы обязаны остаться на своих местах при любой ширине.
     * Шире 390 не растягиваем — карты размером с ладонь игру не улучшают.
     */
    const fit = $derived(Math.min(1, viewport / 390));
    const px = (size) => Math.round(size * fit);

    const avatarSize = $derived(px(sizes.avatar));
    const pileWidth = $derived(px(sizes.pile));
    const cardWidth = $derived(px(sizes.hand));

    /**
     * Карта на столе и слот под пару «атака + защита».
     *
     * ⚠️ Слот шире карты ровно на сдвиг защиты — иначе отбившая карта вылезает за
     * границу колонки и наезжает на соседний слот. Размеры выведены из макета
     * (карта 54 → слот 74, сдвиг 20/9), а не подобраны на глаз.
     */
    const stakeCard = $derived(px(sizes.stake));
    const stakeShiftX = $derived(Math.round(stakeCard * 0.37));
    const stakeShiftY = $derived(Math.round(stakeCard * 0.165));
    const slotWidth = $derived(stakeCard + stakeShiftX);
    const slotHeight = $derived(Math.round(stakeCard * 1.452) + stakeShiftY);

    /** Карта колоды и сброса: они мельче ставки, это фон, а не предмет разговора. */
    const pileCard = $derived(Math.round(stakeCard * 0.86));

    /**
     * ⭐ Кто на часах, когда ход не мой. Раньше это писалось НА СТОЛЕ поверх карт;
     * теперь — одной строкой в нижней панели, на месте кнопок, которых всё равно нет.
     */
    /** Моя собственная реплика: у своего места аватара на столе нет. */
    const myShout = $derived(table.shout?.seatNo === game?.mySeat ? table.shout.text : null);

    const onTheClock = $derived.by(() => {
        if (!game || myTurn) {
            return null;
        }
        const defending = game.phase === 'DEFEND' || game.phase === 'TAKING';
        const seatNo = defending ? game.defenderSeat : game.canAttackSeat;
        const seat = (game.players ?? []).find((player) => player.seatNo === seatNo);
        if (!seat || seatNo === game.mySeat) {
            return null;
        }
        return {name: seat.displayName, role: defending ? 'отбивается' : 'ходит', defending};
    });

    /**
     * Рука разложена по мастям, внутри масти — по возрастанию номинала.
     *
     * ⭐ Масти чередуются по цвету (♦ ♠ ♥ ♣), а не идут подряд красные и подряд чёрные:
     * в веере с нахлёстом видно только узкую полоску карты, и две соседние красные масти
     * сливаются в одно пятно. Чередование делает границу между мастями видимой.
     *
     * ⚠️ Сортируется ТОЛЬКО показ. Порядок в состоянии — дело сервера, и переставлять
     * его здесь нельзя: карта опознаётся по коду, а не по месту в списке.
     */
    const SUIT_ORDER = {diamonds: 0, spades: 1, hearts: 2, clubs: 3};
    const RANK_ORDER = ['6', '7', '8', '9', '10', 'J', 'Q', 'K', 'A'];

    function handOrder(code) {
        // Джокеры не принадлежат масти и живут в конце руки — как и на шкале навесов.
        if (code.startsWith('Joker')) {
            return [9, 0];
        }
        const [rank, suit] = code.split('-');
        return [SUIT_ORDER[suit] ?? 8, RANK_ORDER.indexOf(rank)];
    }

    const sortedHand = $derived.by(() => {
        const hand = [...(game?.myHand ?? [])];
        return hand.sort((left, right) => {
            const [leftSuit, leftRank] = handOrder(left);
            const [rightSuit, rightRank] = handOrder(right);
            return leftSuit - rightSuit || leftRank - rightRank;
        });
    });

    /** Наклон карты в веере: чем больше рука, тем мельче шаг. */
    const tiltStep = $derived(Math.min(5, 60 / Math.max(1, game?.myHand.length ?? 1)));

    const overlap = $derived.by(() => {
        const count = game?.myHand.length ?? 0;
        // Нахлёст «по умолчанию»: с такой рукой веер выглядит веером, а не стопкой.
        const cosy = cardWidth >= 96 ? 34 : 26;
        if (count < 2 || handWidth === 0) {
            return cosy;
        }
        /**
         * ⚠️ Поворот РАСШИРЯЕТ габарит, и расчёт обязан это учитывать. Крайняя карта
         * веера из восьми повёрнута на 17°, и её угол вылезает за край экрана —
         * веер помещался «по расчёту» и не помещался на экране. Считаем разлёт
         * от поворота и вычитаем его из доступной ширины.
         */
        const maxTilt = tiltStep * (count - 1) / 2;
        const bleed = Math.ceil(Math.sin(maxTilt * Math.PI / 180) * cardWidth * 1.452);
        // 16 — горизонтальные отступы самой руки, они в ширину веера не входят.
        const fits = (handWidth - 16 - 2 * bleed - cardWidth) / (count - 1);
        // ⚠️ Берём БОЛЬШИЙ из двух: уютный нахлёст — это минимум, а не потолок. Ограничив
        // его сверху, я оставил восемнадцать карт шире экрана — ровно ту поломку, из-за
        // которой до крайней карты было не дотянуться.
        // Полоска в 12px — это ровно угол с номиналом и мастью: меньше уже не карта.
        return Math.min(cardWidth - 12, Math.max(cosy, Math.ceil(cardWidth - fits)));
    });

    /** Короткая запись карты для кнопки: «6♣» вместо «6-clubs». */
    function short(code) {
        if (!code) {
            return '';
        }
        if (code.startsWith('Joker')) {
            return '🃏';
        }
        const [rank, suit] = code.split('-');
        const glyph = {diamonds: '♦', hearts: '♥', spades: '♠', clubs: '♣'}[suit] ?? '';
        return rank + glyph;
    }

    /**
     * ⭐ Выбрать можно любую карту, даже негодную. Запрет на нажатие раньше означал, что
     * карта молча не реагирует, и отличить «нельзя» от «не попал» было невозможно.
     * Теперь карта поднимается всегда, а кнопка внизу объясняет, что с ней будет.
     */
    /**
     * Карту донесли до цели.
     *
     * ⭐ Что именно произошло, решают `availableActions`, а не бросок: правил фронт
     * не знает (ADR-003). Перетаскивание лишь называет карту и место — дальше ищется
     * готовое действие, и если его нет, ничего не происходит.
     */
    function onDrop(code, target) {
        if (target === 'board') {
            const attack = actions.attacks.find((a) => a.payload.cardCode === code);
            if (attack) {
                selected = null;
                play(attack);
                return;
            }
            // ⚠️ Перевод — тоже «карта на стол»: с точки зрения руки жест тот же самый.
            const transfer = actions.transfers.find((a) => a.payload.cardCode === code);
            if (transfer) {
                selected = null;
                play(transfer);
            }
            return;
        }
        if (target.startsWith('slot:')) {
            const attackCode = target.slice(5);
            const defend = actions.defends.find((a) => a.payload.cardCode === code
                && a.payload.targetCardCode === attackCode);
            if (defend) {
                selected = null;
                play(defend);
            }
        }
    }

    /** Можно ли тащить эту карту: пустое перетаскивание только раздражает. */
    function isDraggable(code) {
        return actions.attacks.some((a) => a.payload.cardCode === code)
            || actions.transfers.some((a) => a.payload.cardCode === code)
            || actions.defends.some((a) => a.payload.cardCode === code);
    }

    /** Подсветка цели: годится ли сюда та карта, что сейчас в руке. */
    function dropAccepts(target) {
        const code = dropTargets.active;
        if (!code) {
            return false;
        }
        if (target === 'board') {
            return actions.attacks.some((a) => a.payload.cardCode === code)
                || actions.transfers.some((a) => a.payload.cardCode === code);
        }
        return actions.defends.some((a) => a.payload.cardCode === code
            && a.payload.targetCardCode === target.slice(5));
    }

    function tapCard(code) {
        // ⭐ При правиле отстающего карты НАБИРАЮТСЯ: нажатие добавляет или убирает
        // карту из набора, а отправляет его кнопка у жертвы.
        if (laggardRule && hangable.has(code)) {
            const next = new Set(hangPicks);
            next.has(code) ? next.delete(code) : next.add(code);
            hangPicks = next;
            return;
        }
        selected = selected === code ? null : code;
    }

    /**
     * Нажатие по карте, уже лежащей на столе.
     *
     * ⭐ Своя — забрать обратно, чужая — зафиксировать («Карте место!»). Одно и то же
     * движение, разный смысл: своё держишь, чужому не даёшь передумать.
     *
     * ⚠️ Отбить важнее: если этой картой сейчас можно побиться, нажатие означает защиту,
     * а не возню с чужой картой. Иначе выбранная в руке карта не находила бы цель.
     */
    function tapTableCard(code, by, pinned, canBeat) {
        if (canBeat) {
            tapTarget(code);
            return;
        }
        if (pinned) {
            return;   // зафиксированную не трогает уже никто
        }
        play({type: by === game.mySeat ? 'RECALL_CARD' : 'PIN_CARD',
            payload: {cardCode: code}});
    }

    function tapTarget(attackCode) {
        const action = targets.find((a) => a.payload.targetCardCode === attackCode);
        if (action) {
            play(action);
            selected = null;
        }
    }

    function run(action) {
        play(action);
        selected = null;
    }</script>

<svelte:window bind:innerWidth={viewport}/>

<!--
  ⭐ Экран — шесть полос с ЖЁСТКИМИ границами (макет «сетка стола v3»). Полосы подсказки,
  навеса, руки и кнопок не двигаются никогда, при любом составе: их высоты заданы, а всё
  лишнее забирает себе стол. Раньше каждая зона росла как хотела — при пятерых рейка
  соперников съедала пол-экрана, стол упирался в руку, а подписи ложились на карты.
-->
<div class="table-screen">
    <!--
      ⚠️ Состояние связи видно прямо за столом. Без него мёртвый сокет ничем себя не
      выдавал: экран прежний, карты на местах, а ходы уходят в никуда.
    -->
    {#if connection.status !== 'open'}
        <div class="link-state mono" class:lost={connection.status === 'unauthorized'}>
            {connection.status === 'unauthorized'
                ? 'Сессия потеряна — обнови страницу'
                : 'Связь со столом восстанавливается…'}
        </div>
    {/if}

    <!-- ═══ A · HUD: одна строка, и она обязана оставаться одной ═══ -->
    <div class="hud">
        <div class="hud-info mono">
            <span>Р.{game.dealNo}</span>
            <span class="sep">·</span>
            <span>козырь <span class="suit" class:red={isRedSuit(game.trumpSuit)}>
                {game.trumpSuit ? suitGlyph(game.trumpSuit) : '?'}</span></span>
            {#if game.protectedSuit}
                <span class="sep">·</span>
                <span>защита <span class="suit" class:red={isRedSuit(game.protectedSuit)}>
                    {suitGlyph(game.protectedSuit)}</span></span>
            {/if}
        </div>

        <div class="hud-buttons">
            <button class="icon" type="button" onclick={toggleSound}
                    title={sound.enabled ? 'Выключить звук' : 'Включить звук'}>
                {sound.enabled ? '🔊' : '🔇'}
            </button>
            {#if onMenu}
                <button class="icon" type="button" onclick={onMenu}
                        title="В главное меню — место за столом останется за тобой">☰</button>
            {/if}
            {#if onLeave}
                {#if leaveAsked}
                    <button class="icon danger text" type="button" onclick={onLeave}>отменить</button>
                    <button class="icon text" type="button" onclick={() => (leaveAsked = false)}>играем</button>
                {:else}
                    <button class="icon danger" type="button" onclick={() => (leaveAsked = true)}
                            title="Выйти из-за стола — партия отменится у всех">✕</button>
                {/if}
            {/if}
        </div>
    </div>

    <!--
      ═══ B · рейка соперников: ВСЕГДА один ряд ═══
      ⭐ Четверо помещаются в строку только потому, что место соперника ужато до аватара
      с бейджами. Второй ряд здесь появиться не может по построению — иначе стол поехал бы
      вниз на разную величину в зависимости от состава.
    -->
    <div class="rail">
        {#each opponents as seat (seat.seatNo)}
            <Seat {seat} size={avatarSize}
                  active={seat.seatNo === game.canAttackSeat}
                  defending={seat.seatNo === game.defenderSeat}
                  decision={table.decisions[seat.seatNo] ?? null}
                  taking={seat.seatNo === game.defenderSeat && game.phase === 'TAKING'}
                  hangCta={hangCtaFor(seat)} onHang={takeHangCard}
                  shout={table.shout?.seatNo === seat.seatNo ? table.shout.text : null}/>
        {/each}
    </div>

    <!--
      ═══ C · стол: три жёсткие колонки ═══
      ⚠️ Колода и сброс — колонки фиксированной ширины со своими подписями ПОД стопкой.
      Пока подписи стояли сбоку, «Бито» уезжало под карты, а «Колода 0» сталкивалась
      с «Мой навес». Ставка забирает остаток и никогда не доходит до соседей.
    -->
    <div class="board" use:anchorPoint={'board'}>
        <div class="pile" style="width:{pileWidth}px" use:anchorPoint={'deck'}>
            {#if game.deckLeft > 0}
                <div class="stack" style="height:{Math.round(pileCard * 1.452)}px">
                    <!--
                      ⭐ Козырная карта лежит поперёк под колодой лицом вверх и берётся
                      последней (§1.9). Положение считается геометрией, а не подбором:
                      повёрнутая карта занимает по горизонтали свою ВЫСОТУ, поэтому
                      сдвиг = (высота − ширина) / 2 — тогда её край ровно на границе
                      колонки, а из-под рубашек торчит угол с номиналом.
                    -->
                    {#if game.trumpCard}
                        <span class="trump-under"
                              style="left:{Math.round((pileCard * 1.452 - pileCard) / 2)}px">
                            <Card code={game.trumpCard} width={pileCard}/>
                        </span>
                    {/if}
                    <Card faceDown width={pileCard} style="position:absolute; right:3px; top:3px"/>
                    <Card faceDown width={pileCard}
                          style={'position:absolute; right:0; top:0'
                              + (lastIsHiddenTrump ? '; outline:1px dashed var(--gold); outline-offset:2px' : '')}/>
                </div>
                <!-- ⭐ Последняя карта — потайной козырь: он сменит масть всему столу (§1.9). -->
                <div class="pile-label mono" class:gold={lastIsHiddenTrump}>
                    {lastIsHiddenTrump ? 'потайной' : `Колода ${game.deckLeft}`}
                </div>
            {:else}
                <div class="stack" style="height:{Math.round(pileCard * 1.452)}px">
                    <div class="empty-pile mono" style="width:{pileCard}px">пусто</div>
                </div>
                <div class="pile-label mono">Колода 0</div>
            {/if}
        </div>

        <div class="stake" data-drop="board" class:accepts={dropAccepts('board')}
             style="--slot-w:{slotWidth}px; --slot-h:{slotHeight}px">
            {#if game.table.length === 0}
                <div class="empty-stake mono" style="width:{stakeCard}px; height:{Math.round(stakeCard * 1.452)}px">
                    <span>брось</span><span>карту</span>
                </div>
            {:else}
                {#each game.table as slot (slot.attack)}
                    {@const canBeat = targets.some((a) => a.payload.targetCardCode === slot.attack)}
                    <!--
                      ⭐ Слот разъезжается плавно (animate:flip), а карта въезжает в него
                      из руки (use:flyFrom). Порядок важен: слот уже встал на новое место,
                      и карта летит именно туда, куда ляжет, а не в середину стола.
                    -->
                    <div class="slot" animate:flip={{duration: TIMING.move}}
                         data-drop={`slot:${slot.attack}`}
                         class:accepts={dropAccepts(`slot:${slot.attack}`)}
                         use:anchorPoint={`slot-${slot.attack}`}>
                        <span use:flyFrom={{key: slot.attack}}>
                            <Card code={slot.attack} width={stakeCard} selected={canBeat}
                                  dimmed={slot.attackPinned}
                                  title={slot.attackPinned ? 'Карте место — забрать нельзя'
                                      : slot.attackBy === game.mySeat ? 'Забрать обратно'
                                      : 'Карте место!'}
                                  onclick={() => tapTableCard(slot.attack, slot.attackBy,
                                      slot.attackPinned, canBeat)}/>
                        </span>
                        {#if slot.defend}
                            <!--
                              Отбившая карта ложится поверх атакующей со сдвигом вправо-вниз:
                              видно обе, и видно, что чем побито.
                            -->
                            <span class="defence" style="left:{stakeShiftX}px; top:{stakeShiftY}px">
                                <span use:flyFrom={{key: slot.defend}}>
                                    <Card code={slot.defend} width={stakeCard}
                                          dimmed={slot.defendPinned}
                                          title={slot.defendPinned ? 'Карте место — забрать нельзя'
                                              : slot.defendBy === game.mySeat ? 'Забрать обратно'
                                              : 'Карте место!'}
                                          onclick={() => tapTableCard(slot.defend, slot.defendBy,
                                              slot.defendPinned, false)}/>
                                </span>
                            </span>
                        {/if}
                    </div>
                {/each}
            {/if}
        </div>

        <div class="pile" style="width:{pileWidth}px" use:anchorPoint={'discard'}>
            <div class="stack" style="height:{Math.round(pileCard * 1.452)}px">
                {#if game.discardCount > 0}
                    <Card faceDown width={Math.round(pileCard * 0.86)}
                          style="position:absolute; left:2px; top:6px; transform:rotate(-14deg); filter:brightness(.7)"/>
                    <Card faceDown width={Math.round(pileCard * 0.86)}
                          style="position:absolute; left:11px; top:3px; transform:rotate(7deg); filter:brightness(.85)"/>
                    <Card faceDown width={Math.round(pileCard * 0.86)}
                          style="position:absolute; left:7px; top:0; transform:rotate(-3deg)"/>
                {/if}
            </div>
            <div class="pile-label mono">Бито {game.discardCount}</div>
        </div>
    </div>

    <!--
      ═══ D · подсказка хода ═══
      ⭐ У подсказки своя полоса, и текст физически не может лечь на карты. Раньше она
      висела в зоне стола и при шести картах наезжала на них.
    -->
    <!-- ⭐ Своё «Карте место!» показываем здесь: над собственной рукой облачку места нет,
         а увидеть, что твоё возражение услышано, надо не меньше, чем соседям. -->
    <div class="hint mono" class:urgent={Boolean(prompt) || myShout}>
        {myShout ?? prompt ?? ''}
    </div>

    <!-- ═══ E · мой навес и потайная карта ═══ -->
    <div class="mine">
        <div class="my-hung" use:anchorPoint={`hung-${game.mySeat}`}>
            <div class="hung-stack">
                {#if me?.hung.length}
                    {#each me.hung as code, index (code)}
                        <!-- ⚠️ z-index явный: что навесили позже, лежит СВЕРХУ, как в стопке. -->
                        <Card {code}
                              width={index === me.hung.length - 1
                                  ? Math.round(stakeCard * 0.78) : Math.round(stakeCard * 0.6)}
                              dimmed={index !== me.hung.length - 1}
                              style={'position:relative; z-index:' + (index + 1)
                                  + (index < me.hung.length - 1
                                      ? `; margin-right:-${Math.round(stakeCard * 0.39)}px` : '')}/>
                    {/each}
                {:else}
                    <div class="flying-slot mono" class:gold={me?.nextIsJoker}
                         style="width:{Math.round(stakeCard * 0.7)}px; height:{Math.round(stakeCard * 1.02)}px">
                        {me?.nextIsJoker ? '🃏' : me?.nextNavesRank ?? '6'}
                    </div>
                {/if}
            </div>
            <!--
              ⭐ Считается не сколько навесили, а сколько осталось до джокера: шкала и есть
              счёт в игре (ADR-017), и «навесили 2» ничего не говорит о том, близко ли конец.
            -->
            <div class="hung-text mono">
                Мой навес {me?.hung.length ?? 0}<br>
                <span class="gold">{me?.stepsToJoker ? `до джокера ${me.stepsToJoker}` : 'джокер висит'}</span>
            </div>
        </div>

        <div class="my-hidden">
            {#if game.iHaveHiddenCard}
                <!-- Свою скрытую карту не видит даже владелец (§1.8) — только рубашку. -->
                <Card faceDown width={Math.round(stakeCard * 0.7)}/>
            {:else}
                <div class="flying-slot mono"
                     style="width:{Math.round(stakeCard * 0.7)}px; height:{Math.round(stakeCard * 1.02)}px">взял</div>
            {/if}
            <div class="hidden-label mono">потайная</div>
        </div>
    </div>

    <!--
      ═══ F · моя рука ═══
      ⭐ Веер живёт на внутреннем узле, а перестановка — на внешнем. Так `animate:flip`
      двигает карту по горизонтали, а поворот доезжает своим переходом: при добавлении
      карты соседние расходятся, а не перескакивают в новый угол.
    -->
    <div class="hand" style="--overlap:{overlap}px" use:anchorPoint={'hand'}
         bind:clientWidth={handWidth}>
        {#each sortedHand as code, index (code)}
            {@const middle = (sortedHand.length - 1) / 2}
            {@const offset = index - middle}
            <span class="hand-card" animate:flip={{duration: TIMING.move}}>
                <span class="fan" style="transform: rotate({offset * tiltStep}deg) translateY({Math.abs(offset) * 4}px)">
                    <span use:flyFrom={{key: code, pool: 'hand', delay: index * 40}}
                          use:draggable={{code, enabled: isDraggable(code),
                              onDrop: (target) => onDrop(code, target)}}>
                        <Card {code} width={cardWidth}
                              selected={selected === code || hangPicks.has(code)}
                              playable={hangingNow && hangable.has(code)}
                              onclick={() => tapCard(code)}
                              style={selected === code ? 'transform: translateY(-18px)' : ''}/>
                    </span>
                </span>
            </span>
        {/each}
    </div>

    <!--
      ═══ G · кнопки ═══
      ⭐ Когда ход не мой, здесь стоит ОДНА строка: кто на часах и сколько ему осталось.
      Прежде это писалось поверх стола — единственное место, где текста быть не должно.
    -->
    <div class="actions">
        {#if actions.trumps.length}
            {#each actions.trumps as action (action.payload.suit)}
                <button class="btn trump cta" type="button" onclick={() => run(action)}>
                    <span class="suit" class:red={isRedSuit(action.payload.suit)}>
                        {suitGlyph(action.payload.suit)}
                    </span>
                </button>
            {/each}
        {:else}
            {#if actions.take}
                <!-- Красным «Беру» зовёт только тогда, когда на столе есть что забирать. -->
                <button class="narrow" class:btn={takeMatters} class:btn-red={takeMatters}
                        class:btn-ghost={!takeMatters} type="button" onclick={tapTake}
                        title={takeMatters ? 'Забрать стол' : 'Всё отбито — забирать нечего'}>
                    {takeAsked ? 'Точно беру?' : 'Беру'}
                </button>
            {/if}
            {#if primary}
                <button class="btn wide" class:btn-blue={primary.tone === 'blue'}
                        class:cta={primary.action === actions.reveal && revealUrges} type="button"
                        onclick={() => run(primary.action)}>{primary.label}</button>
            {:else if selectedIsDead}
                <div class="waiting mono">{short(selected)} сейчас не сыграть</div>
            {:else if onTheClock}
                <div class="waiting mono">
                    <span class="dot" class:defend={onTheClock.defending}></span>
                    <span>{onTheClock.name} {onTheClock.role}</span>
                    <TurnClock seconds={game.turnSecondsLeft} active={false}/>
                </div>
            {:else if !myTurn}
                <div class="waiting mono">Ход соперника</div>
            {/if}
            {#if actions.pass}
                <button class="btn-ghost narrow" class:cta={passUrges} type="button"
                        onclick={() => run(actions.pass)}>Пас</button>
            {/if}
            {#if actions.hangSkip}
                <!-- Окно навеса ждёт только этого игрока: либо вешает, либо «мимо». -->
                <button class="btn-ghost narrow cta" type="button"
                        onclick={() => run(actions.hangSkip)}>Мимо</button>
            {/if}
        {/if}
    </div>
</div>

<CardFlights/>

<style>
    /**
     * ⭐ Шесть полос с фиксированными высотами. Гибкая ровно одна — стол: он забирает
     * остаток экрана и на нём же экономит. Подсказка, навес, рука и кнопки прибиты
     * снизу и не сдвигаются НИКОГДА, при любом составе и на любом телефоне.
     *
     * ⚠️ Прокрутки здесь нет и быть не должно: всё, что нужно для хода, видно сразу.
     * Любая новая строка обязана вписаться в свою полосу, а не растянуть её.
     */
    .table-screen {
        flex: 1 1 auto;
        display: flex;
        flex-direction: column;
        align-items: stretch;
        min-height: 0;
        overflow: hidden;
    }

    .link-state {
        flex: none;
        text-align: center;
        padding: 5px 12px;
        font-size: 11px;
        color: var(--gold);
        background: rgba(240, 205, 138, 0.1);
    }

    .link-state.lost {
        color: var(--red);
        background: rgba(232, 132, 140, 0.12);
    }

    /* ═══ A · HUD ═══ */
    .hud {
        flex: none;
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: 8px;
        padding: calc(6px + env(safe-area-inset-top)) 12px 6px;
    }

    /**
     * ⚠️ Строка не переносится и обрезается многоточием. «Раздача 16 · козырь · защита»
     * в две строки толкала вниз весь стол — поэтому и «Р.16», а не «Раздача 16».
     */
    .hud-info {
        display: flex;
        align-items: center;
        gap: 6px;
        min-width: 0;
        overflow: hidden;
        white-space: nowrap;
        text-overflow: ellipsis;
        font-size: 11px;
        color: var(--text-55);
    }

    .sep {
        opacity: 0.35;
    }

    .suit {
        font-size: 13px;
        color: var(--text);
    }

    .suit.red {
        color: #ff8d95;
    }

    .hud-buttons {
        flex: none;
        display: flex;
        align-items: center;
        gap: 6px;
    }

    .icon {
        width: 30px;
        height: 30px;
        border-radius: 9px;
        border: 1px solid var(--line-strong);
        background: transparent;
        color: var(--text-55);
        font-size: 12px;
        display: flex;
        align-items: center;
        justify-content: center;
        cursor: pointer;
        padding: 0;
    }

    /* Подтверждение выхода — единственный случай, когда кнопке нужны слова. */
    .icon.text {
        width: auto;
        padding: 0 10px;
        font-family: var(--mono);
        font-size: 11px;
    }

    .icon.danger {
        border-color: rgba(232, 98, 108, 0.35);
        color: var(--red);
    }

    /* ═══ B · рейка соперников ═══ */
    .rail {
        flex: none;
        display: flex;
        align-items: flex-start;
        justify-content: space-around;
        gap: 4px;
        padding: 8px 12px 0;
        min-height: 92px;
    }

    /* ═══ C · стол ═══ */
    .board {
        flex: 1 1 auto;
        min-height: 0;
        display: flex;
        /* ⭐ По центру, а не по верху: зона стола шире содержимого на разных экранах,
           и прижатые к верху карты оставляли дыру между столом и подсказкой. */
        align-items: center;
        justify-content: space-between;
        gap: 9px;
        padding: 10px 14px 0;
    }

    /**
     * Колода и сброс — колонки постоянной ширины со своими подписями ПОД стопкой.
     * ⚠️ Пока подпись стояла сбоку, она уезжала под карты соседней зоны: «Бито» читалось
     * как «Би…», а «Колода 0» сталкивалась с «Мой навес».
     */
    .pile {
        flex: none;
        display: flex;
        flex-direction: column;
        align-items: center;
        gap: 8px;
    }

    .stack {
        position: relative;
        width: 100%;
    }

    /**
     * ⭐ Козырь под колодой повёрнут вокруг центра: по горизонтали он занимает свою
     * ВЫСОТУ, поэтому сдвиг считается формулой, а не подбирается. Из-под рубашек торчит
     * ровно угол с номиналом — иначе показывать карту вместо масти незачем.
     */
    .trump-under {
        position: absolute;
        top: 0;
        transform: rotate(-90deg);
        transform-origin: center center;
        z-index: 0;
    }

    .pile-label {
        font-size: 10px;
        color: var(--text-45);
        white-space: nowrap;
    }

    .empty-pile {
        aspect-ratio: 1 / 1.452;
        border-radius: 5px;
        border: 1px dashed var(--line-strong);
        display: flex;
        align-items: center;
        justify-content: center;
        font-size: 9px;
        color: var(--text-30);
        margin: 0 auto;
    }

    /**
     * ⭐ Ставка: карты фиксированного размера, до трёх рядов по две пары. Отбиваемых
     * больше шести не бывает по правилам (§1.5) — значит и переполниться зона не может,
     * и масштаб карт менять не приходится.
     */
    .stake {
        flex: 1 1 auto;
        min-width: 0;
        align-self: stretch;
        display: flex;
        flex-wrap: wrap;
        align-content: center;
        justify-content: center;
        gap: 6px 14px;
    }

    /* Подсветка цели во время перетаскивания: сюда эту карту положить можно. */
    .accepts {
        outline: 2px dashed var(--gold);
        outline-offset: 3px;
        border-radius: 8px;
    }

    .slot {
        position: relative;
        flex: none;
        width: var(--slot-w);
        height: var(--slot-h);
    }

    .defence {
        position: absolute;
    }

    .defence :global(.playing-card) {
        box-shadow: -3px 10px 20px rgba(0, 0, 0, 0.55);
    }

    .empty-stake {
        border-radius: 8px;
        border: 1px dashed var(--gold-soft);
        display: flex;
        flex-direction: column;
        align-items: center;
        justify-content: center;
        font-size: 10px;
        color: var(--gold-soft);
    }

    /* ═══ D · подсказка хода ═══ */
    .hint {
        flex: none;
        height: 32px;
        display: flex;
        align-items: center;
        justify-content: center;
        font-size: 11px;
        letter-spacing: 0.08em;
        text-transform: uppercase;
        color: var(--text-45);
        white-space: nowrap;
        overflow: hidden;
    }

    .hint.urgent {
        color: var(--gold);
    }

    /* ═══ E · мой навес и потайная ═══ */
    .mine {
        flex: none;
        height: 72px;
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: 12px;
        padding: 0 16px;
    }

    .my-hung {
        display: flex;
        align-items: center;
        gap: 10px;
        min-width: 0;
    }

    .hung-stack {
        display: flex;
        align-items: flex-end;
        flex: none;
    }

    .flying-slot {
        border-radius: 4px;
        border: 1px dashed var(--line-strong);
        display: flex;
        align-items: center;
        justify-content: center;
        font-size: 10px;
        color: var(--text-45);
    }

    .flying-slot.gold {
        border-color: var(--gold-soft);
        color: var(--gold);
    }

    .hung-text {
        font-size: 10px;
        line-height: 1.65;
        color: var(--text-45);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }

    .my-hidden {
        flex: none;
        display: flex;
        flex-direction: column;
        align-items: center;
        gap: 4px;
    }

    .my-hidden :global(.playing-card) {
        outline: 1px dashed var(--gold-soft);
        outline-offset: 2px;
    }

    .hidden-label {
        font-size: 9px;
        color: var(--text-45);
    }

    /* ═══ F · моя рука ═══ */
    .hand {
        flex: none;
        height: 152px;
        display: flex;
        align-items: flex-end;
        justify-content: center;
        width: 100%;
        box-sizing: border-box;
        padding: 0 8px 6px;
    }

    /**
     * ⚠️ `touch-action: none` именно на карте: иначе первое же движение пальцем браузер
     * считает прокруткой и события указателя до нас не доходят — карта «не тащится».
     * Глобально этого делать нельзя, прокрутка нужна на других экранах.
     */
    .hand-card {
        touch-action: none;
        /* ⭐ Без этого карты сжимаются под ширину экрана — и веер получается из карт
           разного размера, будто часть колоды другая. */
        flex: none;
        display: block;
    }

    .hand-card + .hand-card {
        margin-left: calc(-1 * var(--overlap, 26px));
    }

    .fan {
        display: block;
        transition: transform 0.34s cubic-bezier(0.22, 0.61, 0.25, 1);
    }

    /* ═══ G · кнопки ═══ */
    .actions {
        flex: none;
        width: 100%;
        box-sizing: border-box;
        padding: 12px 14px calc(20px + env(safe-area-inset-bottom));
        display: flex;
        gap: 9px;
        background: linear-gradient(to top, rgba(6, 9, 8, 0.94) 55%, rgba(6, 9, 8, 0));
    }

    .narrow {
        flex: 1;
        height: 58px;
        font-size: 15px;
    }

    .wide {
        flex: 2;
    }

    .trump {
        flex: 1;
        height: 58px;
    }

    /**
     * Кто на часах — здесь, а не на столе. Точка красная у отбивающегося, золотая
     * у ходящего: цвет роли, как и на аватарах.
     */
    .waiting {
        flex: 2;
        height: 58px;
        display: flex;
        align-items: center;
        justify-content: center;
        gap: 9px;
        border-radius: var(--r-btn);
        border: 1px dashed var(--line-strong);
        font-size: 13px;
        letter-spacing: 0.06em;
        color: var(--text-55);
    }

    .dot {
        width: 7px;
        height: 7px;
        border-radius: 50%;
        background: var(--gold);
        flex: none;
    }

    .dot.defend {
        background: var(--red);
    }

    /**
     * Зов к действию: игра ждёт ровно этой кнопки. Пульс мягкий — подсказка,
     * а не тревога; «Беру» этого класса не получает никогда.
     */
    @keyframes cta-pulse {
        0%, 100% {
            box-shadow: 0 0 0 0 rgba(233, 196, 106, 0);
            border-color: rgba(233, 196, 106, 0.35);
        }
        50% {
            box-shadow: 0 0 0 7px rgba(233, 196, 106, 0.22);
            border-color: rgba(233, 196, 106, 0.9);
        }
    }

    .cta {
        animation: cta-pulse 1.5s ease-in-out infinite;
        border: 1px solid rgba(233, 196, 106, 0.35);
    }

    @media (prefers-reduced-motion: reduce) {
        .cta {
            animation: none;
            border-color: rgba(233, 196, 106, 0.9);
        }
    }
</style>
