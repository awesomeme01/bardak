package kz.bardak.game.rules;

import java.util.Objects;

/**
 * Приговор по ходу: разрешён либо отклонён с причиной. Отдельный тип вместо {@code boolean}
 * ровно затем, чтобы причину нельзя было потерять по дороге.
 */
public sealed interface MoveVerdict permits MoveVerdict.Allowed, MoveVerdict.Rejected {

    Allowed ALLOWED = new Allowed();

    static MoveVerdict allowed() {
        return ALLOWED;
    }

    static MoveVerdict rejected(final RejectionReason reason) {
        return new Rejected(reason);
    }

    default boolean isAllowed() {
        return this instanceof Allowed;
    }

    record Allowed() implements MoveVerdict {
    }

    record Rejected(RejectionReason reason) implements MoveVerdict {

        public Rejected {
            Objects.requireNonNull(reason, "reason");
        }
    }
}
