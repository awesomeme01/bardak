/**
 * История матчей и рейтинг.
 *
 * Данные читаются по REST и не живут в сокете: это прошлое, оно не меняется само.
 */

import {apiGet} from '../net/rest-client.js';

export const history = $state({
    matches: [],
    rating: null,
    details: null,   // раскрытый матч: раздачи и итоги
    replay: null,
    error: null,
    loading: false,
});

export async function loadHistory() {
    history.loading = true;
    history.error = null;
    try {
        const [matches, rating] = await Promise.all([
            apiGet('/matches'),
            apiGet('/rating/me'),
        ]);
        history.matches = matches;
        history.rating = rating;
    } catch (e) {
        history.error = e.message;
    } finally {
        history.loading = false;
    }
}

export async function openMatch(matchId) {
    if (history.details?.match?.id === matchId) {
        history.details = null;   // повторный клик сворачивает
        return;
    }
    history.replay = null;
    try {
        history.details = await apiGet(`/matches/${matchId}`);
    } catch (e) {
        history.error = e.message;
    }
}

export async function loadReplay(matchId) {
    try {
        history.replay = await apiGet(`/matches/${matchId}/replay`);
    } catch (e) {
        history.error = e.message;
    }
}

export function closeReplay() {
    history.replay = null;
}
