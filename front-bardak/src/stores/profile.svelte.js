/**
 * Кто я и какой у меня рейтинг.
 *
 * Отдельно от сессии: сессия отвечает на вопрос «пустить ли», а это — то, что показывают
 * человеку. Рейтинг меняется после матча, поэтому его перечитывают, а не запоминают навсегда.
 */

import {apiGet} from '../net/rest-client.js';

export const profile = $state({
    user: null,      // {id, username, displayName}
    rating: null,
    matches: 0,
});

export async function loadProfile() {
    profile.user = await apiGet('/profile');
    await loadRating();
    return profile.user;
}

export async function loadRating() {
    try {
        const view = await apiGet('/rating/me');
        profile.rating = Math.round(Number(view.rating));
        profile.matches = view.matchesPlayed;
    } catch {
        // Рейтинг — украшение шапки: без него играть можно, а ронять экран из-за него нельзя.
    }
}

export function forgetProfile() {
    profile.user = null;
    profile.rating = null;
    profile.matches = 0;
}
