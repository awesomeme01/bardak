package kz.bardak.game.rules;

import java.util.List;

/**
 * Мост к тестовой фикстуре раздачи для тестов вне пакета правил.
 *
 * <p>Фикстура пакетно-приватная намеренно: снимок раздачи собирается только тестами правил.
 * Чтобы не размывать эту границу, наружу отдаются готовые состояния, а не конструктор.
 */
public final class DealStateFixtureAccess {

    private DealStateFixtureAccess() {
    }

    /** Ход атакующего: он на часах. */
    public static DealState attacking() {
        return DealStateFixture.aDeal()
                .withHand(0, PipCard.of(Rank.SIX, Suit.CLUBS))
                .withHand(1, PipCard.of(Rank.ACE, Suit.CLUBS))
                .build();
    }

    /** Защищающийся не отбился: на часах он. */
    public static DealState defending(final Card attack, final Card defence) {
        return DealStateFixture.aDeal()
                .withPhase(DealPhase.DEFEND)
                .withAttackCards(attack)
                .withHand(1, defence)
                .build();
    }

    /** Защищающийся на часах, но стол пуст — брать нечего. */
    public static DealState defendingEmptyTable() {
        return DealStateFixture.aDeal()
                .withPhase(DealPhase.DEFEND)
                .withHand(1, PipCard.of(Rank.ACE, Suit.CLUBS))
                .build();
    }

    /** Открыто окно навеса. */
    public static DealState hanging() {
        return DealStateFixture.aDeal()
                .withHand(0, PipCard.of(Rank.SIX, Suit.CLUBS))
                .withHangingWindow(HangingWindow.open(1, List.of(List.of(0)), false))
                .build();
    }

    /** Козырь разыгрывается костью, победитель молчит; больше всего у него треф. */
    public static DealState choosingTrump() {
        return DealStateFixture.aDeal()
                .withPhase(DealPhase.DICE)
                .withAttackRightAt(0)
                .withHand(0, PipCard.of(Rank.SIX, Suit.CLUBS), PipCard.of(Rank.SEVEN, Suit.CLUBS),
                        PipCard.of(Rank.ACE, Suit.HEARTS))
                .build();
    }

    public static DealState finished() {
        return DealStateFixture.aDeal().withPhase(DealPhase.DEAL_OVER).build();
    }
}
