package kz.bardak.game.protocol;

import kz.bardak.game.rules.NavesScale;
import kz.bardak.game.rules.Rank;

/**
 * Уровень навесов ↔ короткий код для базы и клиента.
 *
 * <p>Движок кодирует уровень индексом ступени, и это правильно внутри: сравнивать индексы
 * дешевле, чем строки, а длина шкалы — параметр стола. Но в истории уровень должен
 * читаться глазами и переживать смену шкалы: {@code 4} осмысленно только вместе с той
 * шкалой, при которой записано, а {@code "10"} — само по себе.
 */
public final class NavesLevelCodec {

    /** Джокер навешен — игрок проиграл. Двух символов хватает: колонка varchar(2). */
    public static final String JOKER = "Jk";

    private NavesLevelCodec() {
    }

    /** Код уровня или {@code null}, если навесов ещё не было. */
    public static String encode(final NavesScale scale, final int level) {
        if (level == NavesScale.NO_NAVES) {
            return null;
        }
        if (scale.isFinished(level)) {
            return JOKER;
        }
        return scale.ranks().get(level).code();
    }

    /** Обратное преобразование: {@code null} — навесов не было. */
    public static int decode(final NavesScale scale, final String code) {
        if (code == null) {
            return NavesScale.NO_NAVES;
        }
        if (JOKER.equals(code)) {
            return scale.jokerLevel();
        }
        for (int level = 0; level < scale.ranks().size(); level++) {
            final Rank rank = scale.ranks().get(level);
            if (rank.code().equals(code)) {
                return level;
            }
        }
        throw new IllegalArgumentException("Шкала не содержит ступени " + code);
    }
}
