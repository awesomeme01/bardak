/**
 * Точка входа. Разметку строит Svelte (ADR-036): состояние приходит с сервера
 * целыми снимками, и раскладывать его в DOM руками — прямой путь к рассогласованию
 * картинки и правды сервера.
 */

import {mount} from 'svelte';
import App from './App.svelte';
import {initPwa} from './stores/pwa.svelte.js';
import {readInviteFromUrl} from './stores/invite-link.svelte.js';

initPwa();

// ⭐ Код из ссылки читается ДО построения разметки: приглашение адресовано и вошедшему,
// и тому, у кого учётки ещё нет, а адрес надо забрать до первой же перерисовки.
readInviteFromUrl();

export default mount(App, {target: document.getElementById('app')});
