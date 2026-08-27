-- Наполнение каталогов: второй набор карт и новые темы стола.
--
-- Инфраструктура выбора существует с M3, а выбирать было не из чего: набор один,
-- тема одна — и формы выбора прятались (список из одного пункта не выбор, а шум).
-- Наполнение живёт миграцией, как и записано в README наборов и в M9.
--
-- ⚠️ Первая миграция ПОСЛЕ эры Flyway: Java о ней не знает, и знать не должна —
-- она читает те же таблицы и просто увидит больше строк.

-- classic-svg: тот же дизайн, что classic (тот же public-domain источник), но вектор —
-- карты остаются чёткими на любой плотности экрана. Файлы лежат в assets/ с M9.
insert into card_sets (id, code, name, description, version, preview_url, is_default)
values ('33333333-3333-3333-3333-333333333333', 'classic-svg', 'Классический, вектор',
        'Тот же классический дизайн вектором: чёткие карты на любом экране', '1.0.0',
        '/assets/card-sets/classic-svg/A-spades.svg', false);

-- Карты генерируются перебором, как и в classic: ручной список — 54 возможности опечататься.
insert into card_assets (id, card_set_id, card_code, asset_url, mime, ordinal)
select gen_random_uuid(),
       '33333333-3333-3333-3333-333333333333',
       rank.code || '-' || suit.code,
       '/assets/card-sets/classic-svg/' || rank.code || '-' || suit.code || '.svg',
       'image/svg+xml',
       rank.ordinal * 10 + suit.ordinal
from (values ('2', 1), ('3', 2), ('4', 3), ('5', 4), ('6', 5), ('7', 6), ('8', 7),
             ('9', 8), ('10', 9), ('J', 10), ('Q', 11), ('K', 12), ('A', 13)) as rank(code, ordinal)
cross join (values ('diamonds', 1), ('hearts', 2), ('spades', 3), ('clubs', 4)) as suit(code, ordinal);

insert into card_assets (id, card_set_id, card_code, asset_url, mime, ordinal)
values (gen_random_uuid(), '33333333-3333-3333-3333-333333333333', 'Joker',
        '/assets/card-sets/classic-svg/Joker.svg', 'image/svg+xml', 200),
       (gen_random_uuid(), '33333333-3333-3333-3333-333333333333', 'back',
        '/assets/card-sets/classic-svg/back.svg', 'image/svg+xml', 201);

-- Темы стола. Дефолтное зелёное сукно своим цветом не красится (у него рукодельный
-- градиент в стилях — см. App.svelte); у новых тем цвет из базы — единственный источник
-- вида, и клиент разводит его в такой же градиент сам.
insert into table_themes (id, code, name, felt_color, default_back_code, is_default)
values ('44444444-4444-4444-4444-444444444401', 'blue-felt', 'Синее сукно',
        '#1f5276', 'back', false),
       ('44444444-4444-4444-4444-444444444402', 'crimson-felt', 'Бордовое сукно',
        '#6e2436', 'back', false),
       ('44444444-4444-4444-4444-444444444403', 'graphite-felt', 'Графит',
        '#3a3f44', 'back', false);
