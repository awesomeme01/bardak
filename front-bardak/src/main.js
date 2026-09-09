/**
 * Точка входа. Разметку строит Svelte (ADR-036): состояние приходит с сервера
 * целыми снимками, и раскладывать его в DOM руками — прямой путь к рассогласованию
 * картинки и правды сервера.
 */

import {mount} from 'svelte';
import App from './App.svelte';
import {initPwa} from './stores/pwa.svelte.js';
import {readInviteFromUrl} from './stores/invite-link.svelte.js';

/**
 * Щипок двумя пальцами не масштабирует игру.
 *
 * ⚠️ Слоя viewport здесь мало: iOS Safari во вкладке игнорирует `user-scalable=no`.
 * Отменяются как раз те события, которыми WebKit ведёт масштабирование, — своих
 * `gesture*` нет больше ни у кого, и на других браузерах эта подписка просто молчит.
 *
 * ⭐ Зум убран осознанно: стол верстается ровно под экран и не прокручивается, поэтому
 * случайно «приближённая» игра — это не помощь, а потерянный край стола, который
 * обратно не вернуть без второго щипка.
 */
for (const name of ['gesturestart', 'gesturechange', 'gestureend']) {
    document.addEventListener(name, (event) => event.preventDefault(), {passive: false});
}

/**
 * Пока приложение не на экране — не анимируется ничего.
 *
 * ⭐ Партия живёт в фоне подолгу: телефон убрали в карман, сунули в держатель, переключились
 * на карту — а кольца очереди и зов кнопки продолжают крутиться. WebKit тормозит анимации
 * в скрытых вкладках сам, но в установленном приложении и при частично перекрытом окне
 * на это полагаться нельзя. Правило, которое их останавливает, — в `styles.css`.
 *
 * ⚠️ Останавливаются именно АНИМАЦИИ, а не игра: сокет, часы хода и звук работают дальше.
 * Иначе вернувшийся из фона увидел бы стол, отставший на минуту.
 */
const markVisibility = () =>
    document.documentElement.classList.toggle('page-hidden', document.hidden);

document.addEventListener('visibilitychange', markVisibility);
markVisibility();

initPwa();

// ⭐ Код из ссылки читается ДО построения разметки: приглашение адресовано и вошедшему,
// и тому, у кого учётки ещё нет, а адрес надо забрать до первой же перерисовки.
readInviteFromUrl();

export default mount(App, {target: document.getElementById('app')});
