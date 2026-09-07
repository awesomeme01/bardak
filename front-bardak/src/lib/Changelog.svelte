<script>
    /**
     * Что менялось в игре.
     *
     * ⭐ Список ведётся руками в `public/changelog.json`, а не собирается из истории
     * коммитов: коммит объясняет ПОЧЕМУ правка сделана и адресован тому, кто читает код.
     * Игроку нужно другое — что изменилось за столом. Автосборка дала бы шум вместо ответа.
     */
    import {onMount} from 'svelte';

    

    let releases = $state([]);
    let error = $state(null);

    onMount(async () => {
        try {
            const response = await fetch('/changelog.json', {cache: 'no-cache'});
            if (!response.ok) {
                throw new Error('Список изменений не загрузился');
            }
            releases = (await response.json()).releases ?? [];
        } catch (e) {
            error = e.message;
        }
    });

    /** «2026-09-02» → «2 сентября». Год не пишем: он и так виден по порядку. */
    const MONTHS = ['января', 'февраля', 'марта', 'апреля', 'мая', 'июня',
        'июля', 'августа', 'сентября', 'октября', 'ноября', 'декабря'];

    function humanDate(iso) {
        const [year, month, day] = (iso ?? '').split('-').map(Number);
        if (!year || !month || !day) {
            return iso ?? '';
        }
        return `${day} ${MONTHS[month - 1]} ${year}`;
    }
</script>

<div class="screen">

    <h2 class="title">Что менялось</h2>

    {#if error}
        <p class="notice notice-fail">{error}</p>
    {:else if !releases.length}
        <p class="muted">Загружаю…</p>
    {:else}
        <div class="releases">
            {#each releases as release (release.date)}
                <section class="release">
                    <div class="head">
                        <span class="date mono">{humanDate(release.date)}</span>
                        <span class="name">{release.title}</span>
                    </div>
                    <ul>
                        {#each release.items as item, index (index)}
                            <li>{item}</li>
                        {/each}
                    </ul>
                </section>
            {/each}
        </div>
    {/if}
</div>

<style>
    .screen {
        padding: 12px 20px 40px;
        display: flex;
        flex-direction: column;
        gap: 14px;
    }

    .title {
        font-family: var(--display);
        font-weight: 600;
        font-size: 26px;
        margin: 0;
    }

    .releases {
        display: flex;
        flex-direction: column;
        gap: 22px;
    }

    .head {
        display: flex;
        align-items: baseline;
        gap: 10px;
        flex-wrap: wrap;
    }

    .date {
        font-size: 11px;
        color: var(--text-45);
    }

    .name {
        font-weight: 700;
        color: var(--gold);
    }

    ul {
        margin: 8px 0 0;
        padding-left: 18px;
        display: flex;
        flex-direction: column;
        gap: 6px;
    }

    li {
        color: var(--text-70);
        line-height: 1.5;
    }
</style>
