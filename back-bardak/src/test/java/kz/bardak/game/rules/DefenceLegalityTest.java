package kz.bardak.game.rules;

import static kz.bardak.game.rules.DealStateFixture.aDeal;
import static org.assertj.core.api.Assertions.assertThat;

import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;

/**
 * Легальность защиты — §1.1.1, §1.8, §2.1 правил. Цель обязательна: при нескольких картах
 * на столе иначе не зафиксировать, что чем отбито.
 */
class DefenceLegalityTest {

    private final MoveRules moveRules = new MoveRules(RulesConfig.defaults());

    @DisplayName("Should allow beating the named target When the defence card is higher in suit")
    @Test
    void shouldAllowBeatingTheNamedTargetWhenTheDefenceCardIsHigherInSuit() {
        final Card attack = PipCard.of(Rank.SEVEN, Suit.DIAMONDS);
        final Card defence = PipCard.of(Rank.NINE, Suit.DIAMONDS);
        final DealState state = aDeal().withAttackCards(attack).withHand(1, defence).build();

        assertThat(moveRules.canDefend(state, 1, defence, attack)).isEqualTo(MoveVerdict.allowed());
    }

    @DisplayName("Should reject a defence from a seat that is not defending When another player tries")
    @Test
    void shouldRejectADefenceFromASeatThatIsNotDefendingWhenAnotherPlayerTries() {
        final Card attack = PipCard.of(Rank.SEVEN, Suit.DIAMONDS);
        final Card defence = PipCard.of(Rank.NINE, Suit.DIAMONDS);
        final DealState state = aDeal().withAttackCards(attack).withHand(2, defence).build();

        assertThat(moveRules.canDefend(state, 2, defence, attack))
                .isEqualTo(MoveVerdict.rejected(RejectionReason.NOT_YOUR_TURN));
    }

    @DisplayName("Should reject a defence naming a card that is not on the table When the target is absent")
    @Test
    void shouldRejectADefenceNamingACardThatIsNotOnTheTableWhenTheTargetIsAbsent() {
        final Card defence = PipCard.of(Rank.NINE, Suit.DIAMONDS);
        final DealState state = aDeal()
                .withAttackCards(PipCard.of(Rank.SEVEN, Suit.DIAMONDS))
                .withHand(1, defence)
                .build();

        assertThat(moveRules.canDefend(state, 1, defence, PipCard.of(Rank.SEVEN, Suit.CLUBS)))
                .isEqualTo(MoveVerdict.rejected(RejectionReason.TARGET_NOT_ON_TABLE));
    }

    @DisplayName("Should reject a defence against an already beaten card When the target is covered")
    @Test
    void shouldRejectADefenceAgainstAnAlreadyBeatenCardWhenTheTargetIsCovered() {
        final Card attack = PipCard.of(Rank.SEVEN, Suit.DIAMONDS);
        final Card second = PipCard.of(Rank.TEN, Suit.DIAMONDS);
        final DealState state = aDeal()
                .withBeatenPair(attack, PipCard.of(Rank.NINE, Suit.DIAMONDS))
                .withHand(1, second)
                .build();

        assertThat(moveRules.canDefend(state, 1, second, attack))
                .isEqualTo(MoveVerdict.rejected(RejectionReason.TARGET_ALREADY_BEATEN));
    }

    @DisplayName("Should reject a defence card that does not beat the target When the rank is lower")
    @Test
    void shouldRejectADefenceCardThatDoesNotBeatTheTargetWhenTheRankIsLower() {
        final Card attack = PipCard.of(Rank.NINE, Suit.DIAMONDS);
        final Card defence = PipCard.of(Rank.SEVEN, Suit.DIAMONDS);
        final DealState state = aDeal().withAttackCards(attack).withHand(1, defence).build();

        assertThat(moveRules.canDefend(state, 1, defence, attack))
                .isEqualTo(MoveVerdict.rejected(RejectionReason.CARD_DOES_NOT_BEAT));
    }

    @DisplayName("Should reject a trump against the protected suit When spades are attacked")
    @Test
    void shouldRejectATrumpAgainstTheProtectedSuitWhenSpadesAreAttacked() {
        final Card attack = PipCard.of(Rank.SIX, Suit.SPADES);
        final Card trumpAce = PipCard.of(Rank.ACE, Suit.HEARTS);
        final DealState state = aDeal().withAttackCards(attack).withHand(1, trumpAce).build();

        assertThat(moveRules.canDefend(state, 1, trumpAce, attack))
                .isEqualTo(MoveVerdict.rejected(RejectionReason.CARD_DOES_NOT_BEAT));
    }

    @DisplayName("Should allow a joker against the protected suit When the defence has no spades")
    @Test
    void shouldAllowAJokerAgainstTheProtectedSuitWhenTheDefenceHasNoSpades() {
        final Card attack = PipCard.of(Rank.ACE, Suit.SPADES);
        final Card joker = new JokerCard(1);
        final DealState state = aDeal().withAttackCards(attack).withHand(1, joker).build();

        assertThat(moveRules.canDefend(state, 1, joker, attack)).isEqualTo(MoveVerdict.allowed());
    }

    @DisplayName("Should allow a joker against another joker When the attack is a joker")
    @Test
    void shouldAllowAJokerAgainstAnotherJokerWhenTheAttackIsAJoker() {
        final Card attack = new JokerCard(1);
        final Card defence = new JokerCard(2);
        final DealState state = aDeal().withAttackCards(attack).withHand(1, defence).build();

        assertThat(moveRules.canDefend(state, 1, defence, attack)).isEqualTo(MoveVerdict.allowed());
    }

    @DisplayName("Should reject a trump ace against a joker When the attack cannot be beaten")
    @Test
    void shouldRejectATrumpAceAgainstAJokerWhenTheAttackCannotBeBeaten() {
        final Card attack = new JokerCard(1);
        final Card trumpAce = PipCard.of(Rank.ACE, Suit.HEARTS);
        final DealState state = aDeal().withAttackCards(attack).withHand(1, trumpAce).build();

        assertThat(moveRules.canDefend(state, 1, trumpAce, attack))
                .isEqualTo(MoveVerdict.rejected(RejectionReason.CARD_DOES_NOT_BEAT));
    }

    @DisplayName("Should allow defending with the face-down card When the deck is empty and the hand is spent")
    @Test
    void shouldAllowDefendingWithTheFaceDownCardWhenTheDeckIsEmptyAndTheHandIsSpent() {
        final Card attack = PipCard.of(Rank.SEVEN, Suit.DIAMONDS);
        final Card faceDown = PipCard.of(Rank.NINE, Suit.DIAMONDS);
        final DealState state = aDeal()
                .withEmptyDeck()
                .withAttackCards(attack)
                .withFaceDownCard(1, faceDown)
                .build();

        assertThat(moveRules.canDefend(state, 1, faceDown, attack)).isEqualTo(MoveVerdict.allowed());
    }

    @DisplayName("Should reject defending with the face-down card When the deck is not empty")
    @Test
    void shouldRejectDefendingWithTheFaceDownCardWhenTheDeckIsNotEmpty() {
        final Card attack = PipCard.of(Rank.SEVEN, Suit.DIAMONDS);
        final Card faceDown = PipCard.of(Rank.NINE, Suit.DIAMONDS);
        final DealState state = aDeal()
                .withDeckOf(3)
                .withAttackCards(attack)
                .withFaceDownCard(1, faceDown)
                .build();

        assertThat(moveRules.canDefend(state, 1, faceDown, attack))
                .isEqualTo(MoveVerdict.rejected(RejectionReason.FACE_DOWN_CARD_NOT_PLAYABLE));
    }

    @DisplayName("Should allow beating a spade with a higher spade When spades are protected")
    @Test
    void shouldAllowBeatingASpadeWithAHigherSpadeWhenSpadesAreProtected() {
        final Card attack = PipCard.of(Rank.TEN, Suit.SPADES);
        final Card defence = PipCard.of(Rank.JACK, Suit.SPADES);
        final DealState state = aDeal().withAttackCards(attack).withHand(1, defence).build();

        assertThat(moveRules.canDefend(state, 1, defence, attack)).isEqualTo(MoveVerdict.allowed());
    }
}
