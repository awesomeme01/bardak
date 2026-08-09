package kz.bardak.game.rules;

import java.util.List;
import java.util.Objects;

/**
 * Что произошло в раздаче. Партия хранится последовательностью событий (ADR-004), поэтому
 * событие описывает свершившийся факт, а не намерение, и его достаточно, чтобы восстановить
 * состояние.
 */
public sealed interface DealEvent {

    int seatNo();

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
     * ⭐ Скрытая карта вскрыта. Открытие необратимо и от исхода хода не зависит (§1.8):
     * событие есть даже тогда, когда сам ход потом окажется отклонён.
     */
    record FaceDownRevealed(int seatNo, Card card) implements DealEvent {

        public FaceDownRevealed {
            Objects.requireNonNull(card, "card");
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

    /** Карты остались у одного — он и «дурак» раздачи (§1.7). */
    record DealFinished(int seatNo) implements DealEvent {
    }
}
