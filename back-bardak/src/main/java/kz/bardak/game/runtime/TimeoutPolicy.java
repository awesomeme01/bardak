package kz.bardak.game.runtime;

import java.util.Comparator;
import java.util.Map;
import java.util.Objects;
import java.util.Optional;
import java.util.function.Function;
import java.util.stream.Collectors;
import kz.bardak.game.rules.Card;
import kz.bardak.game.rules.DealCommand;
import kz.bardak.game.rules.DealPhase;
import kz.bardak.game.rules.DealState;
import kz.bardak.game.rules.PipCard;
import kz.bardak.game.rules.Suit;

/**
 * Что делает сервер за игрока, который не успел (§5.1).
 *
 * <p>⭐ Всегда выбирается <b>самое безобидное</b> действие: движок никогда не решает
 * за человека, какой картой ходить. Пропустил ход — пас или взял, но не «сервер сыграл
 * твоим козырем».
 *
 * <p>Единственное исключение — бросок кости: там выбора нет по сути, и пассивность
 * не должна лишать права (ADR-030).
 */
public final class TimeoutPolicy {

    private TimeoutPolicy() {
    }

    /** Кто сейчас на часах. Пусто — ждать некого, таймер не нужен. */
    public static Optional<Integer> seatOnTheClock(final DealState deal) {
        Objects.requireNonNull(deal, "deal");
        return switch (deal.phase()) {
            case ATTACK, TAKING -> Optional.of(deal.attackRightSeat());
            case DEFEND -> Optional.of(deal.defenderSeat());
            case DICE -> Optional.of(deal.hiddenTrumpAwaitingSuit()
                    .map(pending -> pending.chooserSeat())
                    .orElse(deal.attackRightSeat()));
            case HANGING -> deal.hanging()
                    .flatMap(window -> window.currentStep().stream()
                            .filter(seat -> !window.decided().contains(seat))
                            .findFirst());
            default -> Optional.empty();
        };
    }

    /** Команда, которую сервер выполнит за молчащего игрока. */
    public static Optional<DealCommand> autoActionFor(final DealState deal) {
        return seatOnTheClock(deal).map(seat -> switch (deal.phase()) {
            case ATTACK, TAKING -> new DealCommand.Pass(seat);
            case DEFEND -> defenderAction(deal, seat);
            case HANGING -> new DealCommand.HangSkip(seat);
            case DICE -> new DealCommand.ChooseTrump(seat, richestSuit(deal, seat));
            default -> null;
        });
    }

    /**
     * Защищающийся по таймауту берёт. Но если брать нечего — стол пуст, — то и «взял»
     * невозможно (правило пустого стола), и остаётся пас.
     */
    private static DealCommand defenderAction(final DealState deal, final int seat) {
        return deal.table().isEmpty() ? new DealCommand.Pass(seat) : new DealCommand.Take(seat);
    }

    /**
     * Масть, которой у игрока больше всего.
     *
     * <p>Спецификация не описывает, что делать, если победитель кости молчит: сама кость
     * бросается за него (ADR-030), а выбор масти — уже решение. Берём самую многочисленную
     * масть на руках — то, что человек и выбрал бы, глядя в свои карты (§1.2).
     */
    private static Suit richestSuit(final DealState deal, final int seat) {
        final Map<Suit, Long> bySuit = deal.playerAt(seat).hand().stream()
                .filter(PipCard.class::isInstance)
                .map(PipCard.class::cast)
                .collect(Collectors.groupingBy(PipCard::suit, Collectors.counting()));
        return bySuit.entrySet().stream()
                .max(Comparator.<Map.Entry<Suit, Long>>comparingLong(Map.Entry::getValue)
                        .thenComparing(entry -> entry.getKey().ordinal(), Comparator.reverseOrder()))
                .map(Map.Entry::getKey)
                .orElse(Suit.values()[0]);
    }
}
