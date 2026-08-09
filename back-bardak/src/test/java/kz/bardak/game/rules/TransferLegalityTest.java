package kz.bardak.game.rules;

import static kz.bardak.game.rules.DealStateFixture.aDeal;
import static org.assertj.core.api.Assertions.assertThat;

import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;

/**
 * Легальность перевода — §2.2, ADR-031. Перевод жив, только пока не отбита ни одна карта,
 * и потому вся переводимая атака всегда одноранговая.
 */
class TransferLegalityTest {

    private static final Card SEVEN_DIAMONDS = PipCard.of(Rank.SEVEN, Suit.DIAMONDS);
    private static final Card SEVEN_CLUBS = PipCard.of(Rank.SEVEN, Suit.CLUBS);
    private static final Card SEVEN_SPADES = PipCard.of(Rank.SEVEN, Suit.SPADES);

    private final MoveRules moveRules = new MoveRules(RulesConfig.defaults());

    @DisplayName("Should allow a transfer of the same rank When no card has been beaten yet")
    @Test
    void shouldAllowATransferOfTheSameRankWhenNoCardHasBeenBeatenYet() {
        final DealState state = aDeal()
                .withAttackCards(SEVEN_DIAMONDS)
                .withHand(1, SEVEN_CLUBS)
                .withHand(2, PipCard.of(Rank.ACE, Suit.CLUBS), PipCard.of(Rank.KING, Suit.CLUBS))
                .build();

        assertThat(moveRules.canTransfer(state, 1, SEVEN_CLUBS)).isEqualTo(MoveVerdict.allowed());
    }

    @DisplayName("Should reject a transfer When a card has already been beaten in this round")
    @Test
    void shouldRejectATransferWhenACardHasAlreadyBeenBeatenInThisRound() {
        final DealState state = aDeal()
                .withBeatenPair(PipCard.of(Rank.SIX, Suit.DIAMONDS), PipCard.of(Rank.NINE, Suit.DIAMONDS))
                .withAttackCards(SEVEN_DIAMONDS)
                .withHand(1, SEVEN_CLUBS)
                .withHand(2, PipCard.of(Rank.ACE, Suit.CLUBS), PipCard.of(Rank.KING, Suit.CLUBS))
                .build();

        assertThat(moveRules.canTransfer(state, 1, SEVEN_CLUBS))
                .isEqualTo(MoveVerdict.rejected(RejectionReason.TRANSFER_AFTER_FIRST_BEAT));
    }

    @DisplayName("Should reject a transfer with a different rank When the card does not match the attack")
    @Test
    void shouldRejectATransferWithADifferentRankWhenTheCardDoesNotMatchTheAttack() {
        final Card king = PipCard.of(Rank.KING, Suit.CLUBS);
        final DealState state = aDeal()
                .withAttackCards(SEVEN_DIAMONDS)
                .withHand(1, king)
                .withHand(2, PipCard.of(Rank.ACE, Suit.CLUBS), PipCard.of(Rank.QUEEN, Suit.CLUBS))
                .build();

        assertThat(moveRules.canTransfer(state, 1, king))
                .isEqualTo(MoveVerdict.rejected(RejectionReason.TRANSFER_RANK_MISMATCH));
    }

    @DisplayName("Should allow transferring a spade with any suit of the same rank When the attack is protected suit")
    @Test
    void shouldAllowTransferringASpadeWithAnySuitOfTheSameRankWhenTheAttackIsProtectedSuit() {
        final DealState state = aDeal()
                .withAttackCards(SEVEN_SPADES)
                .withHand(1, SEVEN_CLUBS)
                .withHand(2, PipCard.of(Rank.ACE, Suit.CLUBS), PipCard.of(Rank.KING, Suit.CLUBS))
                .build();

        assertThat(moveRules.canTransfer(state, 1, SEVEN_CLUBS)).isEqualTo(MoveVerdict.allowed());
    }

    @DisplayName("Should allow a chained transfer When the attack already grew to two cards")
    @Test
    void shouldAllowAChainedTransferWhenTheAttackAlreadyGrewToTwoCards() {
        final DealState state = aDeal()
                .withAttackCards(SEVEN_DIAMONDS, SEVEN_CLUBS)
                .withDefenderAt(2)
                .withHand(2, SEVEN_SPADES)
                .withHand(0, PipCard.of(Rank.ACE, Suit.CLUBS), PipCard.of(Rank.KING, Suit.CLUBS),
                        PipCard.of(Rank.QUEEN, Suit.CLUBS))
                .build();

        assertThat(moveRules.canTransfer(state, 2, SEVEN_SPADES)).isEqualTo(MoveVerdict.allowed());
    }

    @DisplayName("Should reject a transfer When the receiver cannot beat the grown attack")
    @Test
    void shouldRejectATransferWhenTheReceiverCannotBeatTheGrownAttack() {
        final DealState state = aDeal()
                .withAttackCards(SEVEN_DIAMONDS)
                .withHand(1, SEVEN_CLUBS)
                .withHand(2, PipCard.of(Rank.ACE, Suit.CLUBS))
                .build();

        assertThat(moveRules.canTransfer(state, 1, SEVEN_CLUBS))
                .isEqualTo(MoveVerdict.rejected(RejectionReason.NEXT_PLAYER_HAS_TOO_FEW_CARDS));
    }

    @DisplayName("Should skip players out of the deal When the receiver is chosen")
    @Test
    void shouldSkipPlayersOutOfTheDealWhenTheReceiverIsChosen() {
        final DealState state = aDeal()
                .withAttackCards(SEVEN_DIAMONDS)
                .withHand(1, SEVEN_CLUBS)
                .withPlayerOutOfDeal(2)
                .withHand(0, PipCard.of(Rank.ACE, Suit.CLUBS), PipCard.of(Rank.KING, Suit.CLUBS))
                .build();

        assertThat(state.nextActiveSeatAfter(1)).isZero();
        assertThat(moveRules.canTransfer(state, 1, SEVEN_CLUBS)).isEqualTo(MoveVerdict.allowed());
    }

    @DisplayName("Should reject a transfer from a seat that is not defending When another player tries")
    @Test
    void shouldRejectATransferFromASeatThatIsNotDefendingWhenAnotherPlayerTries() {
        final DealState state = aDeal()
                .withAttackCards(SEVEN_DIAMONDS)
                .withHand(2, SEVEN_CLUBS)
                .build();

        assertThat(moveRules.canTransfer(state, 2, SEVEN_CLUBS))
                .isEqualTo(MoveVerdict.rejected(RejectionReason.NOT_YOUR_TURN));
    }

    @DisplayName("Should reject a transfer When transfers are switched off in the table config")
    @Test
    void shouldRejectATransferWhenTransfersAreSwitchedOffInTheTableConfig() {
        final RulesConfig noTransfers = new RulesConfig(6, 5, 6, false, true);
        final DealState state = aDeal()
                .withAttackCards(SEVEN_DIAMONDS)
                .withHand(1, SEVEN_CLUBS)
                .withHand(2, PipCard.of(Rank.ACE, Suit.CLUBS), PipCard.of(Rank.KING, Suit.CLUBS))
                .build();

        assertThat(new MoveRules(noTransfers).canTransfer(state, 1, SEVEN_CLUBS))
                .isEqualTo(MoveVerdict.rejected(RejectionReason.TRANSFERS_DISABLED));
    }

    @DisplayName("Should allow transferring a joker with another joker When the attack is a joker")
    @Test
    void shouldAllowTransferringAJokerWithAnotherJokerWhenTheAttackIsAJoker() {
        final Card secondJoker = new JokerCard(2);
        final DealState state = aDeal()
                .withAttackCards(new JokerCard(1))
                .withHand(1, secondJoker)
                .withHand(2, PipCard.of(Rank.ACE, Suit.CLUBS), PipCard.of(Rank.KING, Suit.CLUBS))
                .build();

        assertThat(moveRules.canTransfer(state, 1, secondJoker)).isEqualTo(MoveVerdict.allowed());
    }

    @DisplayName("Should reject transferring a joker with a plain card of any rank When the attack is a joker")
    @Test
    void shouldRejectTransferringAJokerWithAPlainCardOfAnyRankWhenTheAttackIsAJoker() {
        final Card ace = PipCard.of(Rank.ACE, Suit.SPADES);
        final DealState state = aDeal()
                .withAttackCards(new JokerCard(1))
                .withHand(1, ace)
                .withHand(2, PipCard.of(Rank.ACE, Suit.CLUBS), PipCard.of(Rank.KING, Suit.CLUBS))
                .build();

        assertThat(moveRules.canTransfer(state, 1, ace))
                .isEqualTo(MoveVerdict.rejected(RejectionReason.TRANSFER_RANK_MISMATCH));
    }
}
