/**
 * Точка входа. Разметку строит Svelte (ADR-036): состояние приходит с сервера
 * целыми снимками, и раскладывать его в DOM руками — прямой путь к рассогласованию
 * картинки и правды сервера.
 */

import {mount} from 'svelte';
import App from './App.svelte';
import {initPwa} from './stores/pwa.svelte.js';

initPwa();

export default mount(App, {target: document.getElementById('app')});
