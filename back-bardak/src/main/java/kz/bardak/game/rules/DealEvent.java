package kz.bardak.game.rules;

import java.util.List;
import java.util.Objects;
import java.util.Optional;

/**
 * Что произошло в раздаче. Партия хранится последовательностью событий (ADR-004), поэтому
 * событие описывает свершившийся факт, а не намерение, и его достаточно, чтобы восстановить
 * состояние.
 */
public sealed interface DealEvent {

    int seatNo();

    /**
     * ⭐ Кому событие видно, если не всем. Пусто — событие публичное.
     *
     * <p>Единственный приватный случай — вскрытие скрытой карты: она уходит в руку
     * владельца и дальше играется как обычная, а чужую руку не видит никто (§1.8).
     * Остальные узнают только то, что видно в проекции: скрытой карты у него больше нет.
     */
    default Optional<Integer> privateToSeat() {
        return Optional.empty();
    }

    record CardAttacked(int seatNo, Card card) implements DealEvent {

        public CardAttacked {
            Objects.requireNonNull(card, "card");
        }
    }

    record CardDefended(int seatNo, Card card, Card target) implements DealEvent {

        public CardDefended {
            Objects.requireNonNull(card, "card");
            Objects.requireNonNull(target, "target");
        }
    }

    record AttackTransferred(int seatNo, int toSeatNo, Card card) implements DealEvent {

        public AttackTransferred {
            Objects.requireNonNull(card, "card");
        }
    }

    /**
     * ⭐ Скрытая карта вскрыта — <b>событие только для владельца</b>. Карта переходит
     * в его руку, и дальше он играет ею как обычной; соперники её не видят (§1.8).
     * Открытие необратимо и от исхода хода не зависит: событие есть даже тогда, когда
     * ход этой картой не прошёл.
     */
    record FaceDownRevealed(int seatNo, Card card) implements DealEvent {

        public FaceDownRevealed {
            Objects.requireNonNull(card, "card");
        }

        @Override
        public Optional<Integer> privateToSeat() {
            return Optional.of(seatNo);
        }
    }

    record Passed(int seatNo) implements DealEvent {
    }

    /** Право подкидывать ушло следующему — второму соседу (§2.1). */
    record AttackRightMoved(int seatNo) implements DealEvent {
    }

    /** «Бито»: все атаки отбиты, карты уходят в отбой. */
    record RoundBeaten(int seatNo, List<Card> discarded) implements DealEvent {

        public RoundBeaten {
            discarded = List.copyOf(Objects.requireNonNull(discarded, "discarded"));
        }
    }

    /**
     * «Беру» объявлено. Отдельное событие от {@link CardsTaken}: между ними подкидывающие
     * ещё докидывают карты (ADR-038), и на клиенте это разные моменты.
     */
    record TakeAnnounced(int seatNo) implements DealEvent {
    }

    /** «Взял»: защищающийся забрал стол в руку. */
    record CardsTaken(int seatNo, List<Card> cards) implements DealEvent {

        public CardsTaken {
            cards = List.copyOf(Objects.requireNonNull(cards, "cards"));
        }
    }

    record CardsDrawn(int seatNo, List<Card> cards) implements DealEvent {

        public CardsDrawn {
            cards = List.copyOf(Objects.requireNonNull(cards, "cards"));
        }
    }

    /** Игрок избавился от карт и вышел из раздачи. Порядок выхода важен для шкалы (§0.1). */
    record PlayerLeftDeal(int seatNo) implements DealEvent {
    }

    /**
     * ⭐ Потайной козырь вскрыт (§1.9). Событие <b>публичное, с картой</b>: видны и масть,
     * и номинал — он меняет козырь всему столу, в отличие от скрытой карты игрока (§1.8),
     * которую при вскрытии видит только владелец.
     */
    record HiddenTrumpRevealed(int seatNo, Card card) implements DealEvent {

        public HiddenTrumpRevealed {
            Objects.requireNonNull(card, "card");
        }
    }

    /** Козырь сменился — со следующего раунда (ADR-035). */
    record TrumpChanged(int seatNo, Suit suit) implements DealEvent {

        public TrumpChanged {
            Objects.requireNonNull(suit, "suit");
        }
    }

    /** Победитель кости назвал козырную масть (§1.2). */
    record TrumpChosen(int seatNo, Suit suit) implements DealEvent {

        public TrumpChosen {
            Objects.requireNonNull(suit, "suit");
        }
    }

    /** Окно навеса открылось на взявшего карты (§2.3). */
    record HangingWindowOpened(int seatNo) implements DealEvent {
    }

    /**
     * ⭐ Карта ушла из руки навесившего в чужой слот и выбыла из игры до конца раздачи.
     * {@code seatNo} — навесивший, {@code victimSeat} — жертва: для награды за добивание
     * джокером (§0.4) важно именно кто навесил.
     */
    record CardHung(int seatNo, int victimSeat, Card card) implements DealEvent {

        public CardHung {
            Objects.requireNonNull(card, "card");
        }
    }

    /** Уровень по шкале поднялся — ровно на одну ступень за окно (§2.3). */
    record NavesLevelChanged(int seatNo, int level) implements DealEvent {
    }

    /** Спор за право навесить разрешён костью (ADR-029). */
    record DiceRolled(int seatNo, List<Integer> participants) implements DealEvent {

        public DiceRolled {
            participants = List.copyOf(Objects.requireNonNull(participants, "participants"));
        }
    }

    /** Окно закрылось: либо навес случился, либо нужной карты ни у кого не нашлось. */
    record HangingWindowClosed(int seatNo) implements DealEvent {
    }

    /** Карты остались у одного — он и «дурак» раздачи (§1.7). */
    record DealFinished(int seatNo) implements DealEvent {
    }
}
