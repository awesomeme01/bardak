package kz.bardak.game.rules;

import java.util.List;
import java.util.Objects;
import java.util.Optional;

/**
 * Что видно про соседа по столу.
 *
 * <p>⭐ Руки здесь нет физически — только {@code cardsCount}. Не «есть, но скрыта»:
 * если DTO может содержать чужую карту, он спроектирован неправильно.
 *
 * <p>Слот навесов, наоборот, открыт всем легально (§2.3): «кому осталось два навеса
 * до джокера» — ключевая информация за столом, заменяющая счёт.
 *
 * @param seatNo        место
 * @param cardsCount    сколько карт в руке; скрытая в счёт не входит (§1.8)
 * @param hasHiddenCard есть ли у него ещё не вскрытая скрытая карта — только факт
 * @param hungCards     навешенное в этой раздаче; очищается перераздачей
 * @param navesLevel    достигнутый уровень шкалы; переносится между раздачами
 * @param nextNavesRank что можно навесить следующим; пусто, если следующий шаг — джокер
 * @param nextIsJoker   следующая ступень — джокер
 * @param passed        спасовал в этом раунде
 * @param inDeal        ещё в раздаче
 * @param exitPlace     каким по счёту вышел из раздачи, начиная с первого; пусто — ещё играет.
 *                      Порядок выхода не украшение: первый вышедший получает −1 по шкале (§0.1)
 * @param stepsToJoker  сколько навесов осталось до джокера. ⭐ Считает сервер: это и есть счёт
 *                      в игре (ADR-017), и выводить его на клиенте значило бы держать копию
 *                      шкалы в двух местах
 */
public record SeatView(
        int seatNo,
        int cardsCount,
        boolean hasHiddenCard,
        List<Card> hungCards,
        int navesLevel,
        Rank nextNavesRank,
        boolean nextIsJoker,
        boolean passed,
        boolean inDeal,
        Integer exitPlace,
        int stepsToJoker) {

    public SeatView {
        hungCards = List.copyOf(Objects.requireNonNull(hungCards, "hungCards"));
    }

    public Optional<Rank> nextRank() {
        return Optional.ofNullable(nextNavesRank);
    }
}
