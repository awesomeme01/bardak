/**
 * Перетаскивание карты из руки на стол.
 *
 * ⚠️ Своими руками на событиях указателя, а не через HTML5 drag-and-drop: тот на телефоне
 * не работает вовсе — `dragstart` там просто не приходит. А телефон здесь главный.
 *
 * ⭐ Тап продолжает работать как раньше: пока палец не уехал дальше порога, это обычное
 * нажатие, и карта просто выбирается. Перетаскивание — ускорение для тех, кому так удобнее,
 * а не замена: одной рукой в транспорте тапать проще, чем тащить.
 */

/** Насколько надо увести палец, чтобы это перестало быть тапом. */
const DRAG_THRESHOLD = 8;

/** Куда перетащили: `board` — на стол, `slot:<код карты>` — на конкретную карту. */
export const dropTargets = $state({active: null, hovered: null});

/**
 * @param {HTMLElement} node
 * @param {{code: string, enabled: boolean, onDrop: (target: string) => void}} params
 */
export function draggable(node, params) {
    let current = params;
    let ghost = null;
    let dragging = false;
    let startX = 0;
    let startY = 0;

    /**
     * ⚠️ Клон, а не сама карта: карта живёт в веере со своим поворотом и нахлёстом,
     * и вырывать её оттуда — значит на каждом кадре пересчитывать веер целиком.
     */
    function makeGhost(x, y) {
        const source = node.querySelector('img') ?? node;
        const box = source.getBoundingClientRect();
        ghost = /** @type {HTMLElement} */ (source.cloneNode(true));
        ghost.style.cssText = `position:fixed; left:0; top:0; width:${box.width}px;
            pointer-events:none; z-index:120; opacity:0.92;
            box-shadow:0 18px 36px rgba(0,0,0,0.6); border-radius:8px;
            transform: translate(${x - box.width / 2}px, ${y - box.height / 2}px) rotate(2deg);`;
        ghost.dataset.ghostWidth = String(box.width);
        ghost.dataset.ghostHeight = String(box.height);
        document.body.appendChild(ghost);
    }

    function moveGhost(x, y) {
        if (!ghost) {
            return;
        }
        const w = Number(ghost.dataset.ghostWidth);
        const h = Number(ghost.dataset.ghostHeight);
        ghost.style.transform = `translate(${x - w / 2}px, ${y - h / 2}px) rotate(2deg)`;
    }

    function dropTargetAt(x, y) {
        // ⚠️ Призрак висит под пальцем и перехватил бы попадание, поэтому он
        // `pointer-events:none` — иначе целью всегда оказывался бы он сам.
        const element = document.elementFromPoint(x, y);
        return element?.closest('[data-drop]')?.getAttribute('data-drop') ?? null;
    }

    function cleanup() {
        ghost?.remove();
        ghost = null;
        dragging = false;
        dropTargets.active = null;
        dropTargets.hovered = null;
    }

    function onPointerDown(event) {
        if (!current.enabled || event.button > 0) {
            return;
        }
        startX = event.clientX;
        startY = event.clientY;
        node.setPointerCapture(event.pointerId);
        node.addEventListener('pointermove', onPointerMove);
        node.addEventListener('pointerup', onPointerUp);
        node.addEventListener('pointercancel', onPointerCancel);
    }

    function onPointerMove(event) {
        if (!dragging) {
            const moved = Math.hypot(event.clientX - startX, event.clientY - startY);
            if (moved < DRAG_THRESHOLD) {
                return;
            }
            dragging = true;
            dropTargets.active = current.code;
            makeGhost(event.clientX, event.clientY);
        }
        moveGhost(event.clientX, event.clientY);
        dropTargets.hovered = dropTargetAt(event.clientX, event.clientY);
    }

    function onPointerUp(event) {
        detach(event);
        if (!dragging) {
            return;   // это был тап — его отработает обычный обработчик нажатия
        }
        const target = dropTargetAt(event.clientX, event.clientY);
        cleanup();
        if (target) {
            current.onDrop(target);
        }
    }

    function onPointerCancel(event) {
        detach(event);
        cleanup();
    }

    function detach(event) {
        node.releasePointerCapture?.(event.pointerId);
        node.removeEventListener('pointermove', onPointerMove);
        node.removeEventListener('pointerup', onPointerUp);
        node.removeEventListener('pointercancel', onPointerCancel);
    }

    node.addEventListener('pointerdown', onPointerDown);

    return {
        update(next) {
            current = next;
        },
        destroy() {
            node.removeEventListener('pointerdown', onPointerDown);
            cleanup();
        },
    };
}

/** Перетаскивание только что закончилось — следующий клик не считается тапом. */
export function justDropped() {
    return dropTargets.active !== null;
}
