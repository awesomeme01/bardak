/**
 * REST-клиент. M1: только /api/health.
 * M2 сюда добавится хранение JWT, авто-refresh и получение ws-тикета.
 */

const BASE = '/api';

export async function apiGet(path) {
    const response = await fetch(`${BASE}${path}`, {
        headers: {'Accept': 'application/json'},
    });

    const body = await response.json().catch(() => null);

    if (!response.ok) {
        // Единый формат ошибок описан в planning/05-api-contracts.md.
        throw new ApiError(body?.code ?? 'HTTP_' + response.status,
            body?.message ?? response.statusText, body);
    }
    return body;
}

export class ApiError extends Error {
    constructor(code, message, body) {
        super(message);
        this.name = 'ApiError';
        this.code = code;
        this.body = body;
    }
}
