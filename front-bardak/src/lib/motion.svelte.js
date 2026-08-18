/**
 * Движение карт по столу.
 *
 * ⭐ Карта не «перерисовывается в новом месте» — она туда летит. Сервер для этого уже всё
 * прислал: события идут перед снимком именно затем, чтобы клиент успел показать причину
 * до того, как покажет следствие (`GameCommandHandler#broadcast`).
 *
 * ⭐ Полёт живёт в отдельном слое поверх стола, а не внутри вёрстки. Причина в том, что
 * концы перелёта принадлежат разным контейнерам — колода, чужое место, слот навеса, — и
 * анимировать переход между ними средствами обычного потока нельзя вообще никак.
 *
 * ⭐ Координаты снимаются в момент события, пока на экране ещё старое состояние. Снимок
 * прилетает следующим сообщением и перерисовывает стол; если снять координаты после него,
 * карта полетит из того места, где она уже и так лежит.
 */

/** Именованные точки стола: откуда и куда летают карты. */
const anchors = new Map();

/**
 * Отметить узел точкой стола: {@code use:anchorPoint={'deck'}}.
 *
 * <p>Узлы приходят и уходят вместе с состоянием, поэтому карта чистится при удалении —
 * иначе полёт однажды нацелится на элемент, которого уже нет в документе.
 */
export function anchorPoint(node, name) {
    anchors.set(name, node);
    return {
        update(next) {
            if (anchors.get(name) === node) {
                anchors.delete(name);
            }
            name = next;
            anchors.set(name, node);
        },
        destroy() {
            if (anchors.get(name) === node) {
                anchors.delete(name);
            }
        },
    };
}

/**
 * Центр именованной точки в координатах окна. Пусто — точки сейчас нет на экране.
 *
 * <p>⚠️ Отсюда возвращается только центр. Размер якоря к размеру карты отношения не имеет:
 * якорь — это контейнер (рука во всю ширину экрана, колода, слот навеса), и карта,
 * взявшая его ширину, накрывает собой пол-стола.
 */
export function anchorCentre(name) {
    const node = anchors.get(name);
    if (!node?.isConnected) {
        return null;
    }
    const box = node.getBoundingClientRect();
    if (box.width === 0 && box.height === 0) {
        return null;
    }
    return {x: box.left + box.width / 2, y: box.top + box.height / 2};
}

/**
 * Точки вылета для карт, которые вот-вот появятся на своих местах.
 *
 * ⭐ Прилетающая карта — это не призрак поверх стола, а сама карта. Событие только
 * запоминает, откуда она стартует; летит потом настоящий узел из своего конечного места
 * назад к точке вылета и обратно (приём FLIP).
 *
 * Почему не призрак: призрак не знает, куда именно ляжет карта — слоты на столе
 * разъезжаются, когда добавляется новый. Он летел в середину стола, а карта проявлялась
 * в своём слоте, и в конце было видно обе сразу. У настоящего узла конечная точка —
 * его собственное место, промахнуться невозможно.
 */
const origins = new Map();

/** Добор: карт в руке ещё нет, и по коду их не запомнить — приходит только счёт. */
let handDraw = {point: null, left: 0};

/** Запомнить, откуда прилетит карта. Вызывается на событии, пока на экране старое. */
export function rememberOrigin(code, from, spin = 0) {
    const point = anchorCentre(from);
    if (point) {
        origins.set(code, {...point, width: widthAt(from), spin});
    }
}

/** То же для добора: следующие {@code count} новых карт руки прилетят из колоды. */
export function rememberDraw(count, from) {
    const point = anchorCentre(from);
    if (point) {
        handDraw = {point: {...point, width: widthAt(from), spin: -6}, left: count};
    }
}

function takeOrigin(key, pool) {
    if (origins.has(key)) {
        const origin = origins.get(key);
        origins.delete(key);
        return origin;
    }
    if (pool === 'hand' && handDraw.left > 0) {
        handDraw.left--;
        return handDraw.point;
    }
    return null;
}

/**
 * Действие: карта въезжает на своё место из запомненной точки.
 *
 * <p>Порядок именно такой: узел уже стоит там, где должен, мы измеряем его настоящее
 * место, отбрасываем к точке вылета без перехода и только следующим кадром отпускаем.
 * Поэтому «куда лететь» не вычисляется — оно известно точно.
 */
export function flyFrom(node, params) {
    const {key, pool = null, delay = 0} = params ?? {};
    const origin = takeOrigin(key, pool);
    if (!origin) {
        return {};
    }
    const box = node.getBoundingClientRect();
    if (!box.width) {
        return {};
    }
    const dx = origin.x - (box.left + box.width / 2);
    const dy = origin.y - (box.top + box.height / 2);
    const scale = origin.width ? Math.max(0.25, origin.width / box.width) : 1;

    node.style.transformOrigin = 'center center';
    node.style.transition = 'none';
    node.style.transform = `translate(${dx}px, ${dy}px) scale(${scale}) rotate(${origin.spin}deg)`;
    node.style.zIndex = '30';
    node.style.position = 'relative';

    requestAnimationFrame(() => requestAnimationFrame(() => {
        node.style.transition = `transform ${TIMING.move}ms cubic-bezier(0.22, 0.61, 0.25, 1) ${delay}ms`;
        node.style.transform = '';
    }));
    setTimeout(() => {
        node.style.transition = '';
        node.style.transform = '';
        node.style.zIndex = '';
        node.style.transformOrigin = '';
        node.style.position = '';
    }, TIMING.move + delay + 80);
    return {};
}

/** Карты в полёте — только те, что со стола УХОДЯТ: у них конечного узла нет. */
export const flights = $state([]);

/** Карта, которую сейчас показывают крупно посреди стола (потайной козырь). */
export const spotlight = $state({card: null});

let flightCounter = 0;

export const TIMING = {
    /** Обычный перелёт карты. Короче — рвано, длиннее — игра начинает тормозить. */
    move: 420,
    /** Добор: карты идут очередью, а не пачкой, иначе перелёт читается как одна клякса. */
    dealStagger: 90,
    /** Сколько потайной козырь висит по центру. Успеть прочитать — и не заскучать. */
    spotlight: 1400,
};

/**
 * Размер карты в каждой точке стола.
 *
 * ⭐ Карта не летает одного размера. Уходя из руки, она крупная, на столе становится
 * меньше, в отбое — совсем мелкой. Один размер на весь перелёт выдаёт себя сразу:
 * карта поверх оказывается меньше той, что под ней, и это читается как ошибка вёрстки,
 * а не как лежащая сверху карта.
 */
const ANCHOR_WIDTH = {deck: 52, board: 62, discard: 40};

function widthAt(name) {
    if (name === 'hand') {
        // Рука на десктопе крупнее — размер задан в вёрстке стола, здесь он повторён.
        return matchMedia('(min-width: 900px)').matches ? 96 : 70;
    }
    if (name.startsWith('seat-')) {
        return 34;
    }
    if (name.startsWith('hung-')) {
        return 48;
    }
    return ANCHOR_WIDTH[name] ?? 58;
}

/**
 * Запустить перелёт карты.
 *
 * <p>Если хотя бы одного конца сейчас нет на экране — молча ничего не делаем. Анимация
 * необязательна: пропущенный полёт означает, что игрок увидит результат без красивого
 * перехода, а брошенное исключение сломало бы обработку снимка.
 *
 * <p>⚠️ Всё дальнейшее делается <b>через прокси</b>, а не над исходным объектом.
 * {@code flights} — это `$state`, то есть глубокий прокси: запись в сырой объект проходит
 * мимо него и не вызывает перерисовку, а {@code indexOf} по сырому объекту не находит
 * ничего, потому что в массиве лежит уже прокси. Ровно на этом полёты и стояли на месте,
 * не улетая и не исчезая.
 */
export function flyCard({from, to, code = null, faceDown = false, delay = 0, spin = 0}) {
    const start = anchorCentre(from);
    const end = anchorCentre(to);
    if (!start || !end) {
        return;
    }
    const id = ++flightCounter;
    flights.push({
        id,
        code,
        faceDown,
        fromWidth: widthAt(from),
        toWidth: widthAt(to),
        from: start,
        to: end,
        spin,
        delay,
        arrived: false,
    });
    const flight = flights[flights.length - 1];

    // Кадр между постановкой и целью обязателен: без него браузер объединит оба
    // положения в одну перерисовку и никакого перехода не случится.
    requestAnimationFrame(() => requestAnimationFrame(() => (flight.arrived = true)));
    setTimeout(() => {
        const index = flights.findIndex((candidate) => candidate.id === id);
        if (index >= 0) {
            flights.splice(index, 1);
        }
    }, TIMING.move + delay + 120);
}

/** Показать карту крупно посреди стола — так вскрывается потайной козырь (§1.9). */
export function showSpotlight(code) {
    spotlight.card = code;
    setTimeout(() => {
        if (spotlight.card === code) {
            spotlight.card = null;
        }
    }, TIMING.spotlight);
}

export function clearFlights() {
    flights.length = 0;
    spotlight.card = null;
    origins.clear();
    handDraw = {point: null, left: 0};
}
