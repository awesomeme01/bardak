package kz.bardak.game.rules;

import static kz.bardak.game.rules.DealStateFixture.aDeal;
import static org.assertj.core.api.Assertions.assertThat;

import java.util.ArrayList;
import java.util.List;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;

/**
 * Легальность атаки и подкидывания — §1.5, §1.8, §2.1 правил.
 */
class AttackLegalityTest {

    private static final RulesConfig CONFIG = RulesConfig.defaults();

    private final MoveRules moveRules = new MoveRules(CONFIG);

    @DisplayName("Should allow any card as the first attack When the table is empty")
    @Test
    void shouldAllowAnyCardAsTheFirstAttackWhenTheTableIsEmpty() {
        final Card six = PipCard.of(Rank.SIX, Suit.SPADES);
        final DealState state = aDeal().withHand(0, six).withHand(1, PipCard.of(Rank.ACE, Suit.CLUBS)).build();

        assertThat(moveRules.canAttack(state, 0, six)).isEqualTo(MoveVerdict.allowed());
    }

    @DisplayName("Should reject an attack from a player without the attack right When another seat holds it")
    @Test
    void shouldRejectAnAttackFromAPlayerWithoutTheAttackRightWhenAnotherSeatHoldsIt() {
        final Card six = PipCard.of(Rank.SIX, Suit.SPADES);
        final DealState state = aDeal().withHand(2, six).withHand(1, PipCard.of(Rank.ACE, Suit.CLUBS)).build();

        assertThat(moveRules.canAttack(state, 2, six))
                .isEqualTo(MoveVerdict.rejected(RejectionReason.NOT_YOUR_TURN));
    }

    @DisplayName("Should reject an attack with a card the player does not hold When the hand lacks it")
    @Test
    void shouldRejectAnAttackWithACardThePlayerDoesNotHoldWhenTheHandLacksIt() {
        final DealState state = aDeal().withHand(0, PipCard.of(Rank.SIX, Suit.SPADES))
                .withHand(1, PipCard.of(Rank.ACE, Suit.CLUBS)).build();

        assertThat(moveRules.canAttack(state, 0, PipCard.of(Rank.KING, Suit.HEARTS)))
                .isEqualTo(MoveVerdict.rejected(RejectionReason.CARD_NOT_IN_HAND));
    }

    @DisplayName("Should allow a follow-up card of a rank already on the table When the rank matches an attack card")
    @Test
    void shouldAllowAFollowUpCardOfARankAlreadyOnTheTableWhenTheRankMatchesAnAttackCard() {
        final Card sevenClubs = PipCard.of(Rank.SEVEN, Suit.CLUBS);
        final DealState state = aDeal()
                .withAttackCards(PipCard.of(Rank.SEVEN, Suit.DIAMONDS))
                .withHand(0, sevenClubs)
                .withHand(1, PipCard.of(Rank.ACE, Suit.CLUBS), PipCard.of(Rank.KING, Suit.CLUBS))
                .build();

        assertThat(moveRules.canAttack(state, 0, sevenClubs)).isEqualTo(MoveVerdict.allowed());
    }

    @DisplayName("Should allow a follow-up card matching a defending card When the rank is only on the defence")
    @Test
    void shouldAllowAFollowUpCardMatchingADefendingCardWhenTheRankIsOnlyOnTheDefence() {
        final Card nineClubs = PipCard.of(Rank.NINE, Suit.CLUBS);
        final DealState state = aDeal()
                .withBeatenPair(PipCard.of(Rank.SEVEN, Suit.DIAMONDS), PipCard.of(Rank.NINE, Suit.DIAMONDS))
                .withHand(0, nineClubs)
                .withHand(1, PipCard.of(Rank.ACE, Suit.CLUBS), PipCard.of(Rank.KING, Suit.CLUBS))
                .build();

        assertThat(moveRules.canAttack(state, 0, nineClubs)).isEqualTo(MoveVerdict.allowed());
    }

    @DisplayName("Should reject a follow-up card of a rank absent from the table When the table is not empty")
    @Test
    void shouldRejectAFollowUpCardOfARankAbsentFromTheTableWhenTheTableIsNotEmpty() {
        final Card king = PipCard.of(Rank.KING, Suit.CLUBS);
        final DealState state = aDeal()
                .withAttackCards(PipCard.of(Rank.SEVEN, Suit.DIAMONDS))
                .withHand(0, king)
                .withHand(1, PipCard.of(Rank.ACE, Suit.CLUBS), PipCard.of(Rank.QUEEN, Suit.CLUBS))
                .build();

        assertThat(moveRules.canAttack(state, 0, king))
                .isEqualTo(MoveVerdict.rejected(RejectionReason.RANK_NOT_ON_TABLE));
    }

    @DisplayName("Should stop at the first round limit When nothing has been discarded yet")
    @Test
    void shouldStopAtTheFirstRoundLimitWhenNothingHasBeenDiscardedYet() {
        final Card extraSix = PipCard.of(Rank.SIX, Suit.HEARTS);
        final DealState state = aDeal()
                .withAttackCards(attackCardsOfDistinctRanks(CONFIG.maxAttackFirstRound()))
                .withHand(0, extraSix)
                .withHand(1, handOf(CONFIG.maxAttackPerRound()))
                .build();

        assertThat(state.attackCardCount()).isEqualTo(CONFIG.maxAttackFirstRound());
        assertThat(moveRules.canAttack(state, 0, extraSix))
                .isEqualTo(MoveVerdict.rejected(RejectionReason.ATTACK_LIMIT_REACHED));
    }

    @DisplayName("Should allow one more card after the first discard When the later round limit applies")
    @Test
    void shouldAllowOneMoreCardAfterTheFirstDiscardWhenTheLaterRoundLimitApplies() {
        final Card extraSix = PipCard.of(Rank.SIX, Suit.HEARTS);
        final DealState state = aDeal()
                .withAnyPileDiscarded()
                .withAttackCards(attackCardsOfDistinctRanks(CONFIG.maxAttackFirstRound()))
                .withHand(0, extraSix)
                .withHand(1, handOf(CONFIG.maxAttackPerRound()))
                .build();

        assertThat(moveRules.canAttack(state, 0, extraSix)).isEqualTo(MoveVerdict.allowed());
    }

    @DisplayName("Should cap the attack at the defence ceiling When the defender holds more cards than the limit")
    @Test
    void shouldCapTheAttackAtTheDefenceCeilingWhenTheDefenderHoldsMoreCardsThanTheLimit() {
        final Card extraSix = PipCard.of(Rank.SIX, Suit.HEARTS);
        final DealState state = aDeal()
                .withAnyPileDiscarded()
                .withAttackCards(attackCardsOfDistinctRanks(CONFIG.maxAttackPerRound()))
                .withHand(0, extraSix)
                .withHand(1, handOf(CONFIG.maxAttackPerRound() + 2))
                .build();

        assertThat(moveRules.canAttack(state, 0, extraSix))
                .isEqualTo(MoveVerdict.rejected(RejectionReason.ATTACK_LIMIT_REACHED));
    }

    @DisplayName("Should reject an attack exceeding what the defender can beat When unbeaten cards outnumber the hand")
    @Test
    void shouldRejectAnAttackExceedingWhatTheDefenderCanBeatWhenUnbeatenCardsOutnumberTheHand() {
        final Card sevenClubs = PipCard.of(Rank.SEVEN, Suit.CLUBS);
        final DealState state = aDeal()
                .withAttackCards(PipCard.of(Rank.SEVEN, Suit.DIAMONDS))
                .withHand(0, sevenClubs)
                .withHand(1, PipCard.of(Rank.ACE, Suit.CLUBS))
                .build();

        assertThat(moveRules.canAttack(state, 0, sevenClubs))
                .isEqualTo(MoveVerdict.rejected(RejectionReason.DEFENDER_HAS_TOO_FEW_CARDS));
    }

    @DisplayName("Should count beaten cards out of the defence budget When the defender already beat one")
    @Test
    void shouldCountBeatenCardsOutOfTheDefenceBudgetWhenTheDefenderAlreadyBeatOne() {
        final Card sevenClubs = PipCard.of(Rank.SEVEN, Suit.CLUBS);
        final DealState state = aDeal()
                .withBeatenPair(PipCard.of(Rank.SEVEN, Suit.DIAMONDS), PipCard.of(Rank.NINE, Suit.DIAMONDS))
                .withHand(0, sevenClubs)
                .withHand(1, PipCard.of(Rank.ACE, Suit.CLUBS))
                .build();

        assertThat(moveRules.canAttack(state, 0, sevenClubs)).isEqualTo(MoveVerdict.allowed());
    }

    @DisplayName("Should not count the face-down card into the defence budget When the deck is not empty")
    @Test
    void shouldNotCountTheFaceDownCardIntoTheDefenceBudgetWhenTheDeckIsNotEmpty() {
        final Card sevenClubs = PipCard.of(Rank.SEVEN, Suit.CLUBS);
        final DealState state = aDeal()
                .withDeckOf(1)
                .withAttackCards(PipCard.of(Rank.SEVEN, Suit.DIAMONDS))
                .withHand(0, sevenClubs)
                .withHand(1, PipCard.of(Rank.ACE, Suit.CLUBS))
                .withFaceDownCard(1, PipCard.of(Rank.KING, Suit.SPADES))
                .build();

        assertThat(moveRules.canAttack(state, 0, sevenClubs))
                .isEqualTo(MoveVerdict.rejected(RejectionReason.DEFENDER_HAS_TOO_FEW_CARDS));
    }

    @DisplayName("Should count the face-down card into the defence budget When the deck is empty")
    @Test
    void shouldCountTheFaceDownCardIntoTheDefenceBudgetWhenTheDeckIsEmpty() {
        final Card sevenClubs = PipCard.of(Rank.SEVEN, Suit.CLUBS);
        final DealState state = aDeal()
                .withEmptyDeck()
                .withAttackCards(PipCard.of(Rank.SEVEN, Suit.DIAMONDS))
                .withHand(0, sevenClubs)
                .withHand(1, PipCard.of(Rank.ACE, Suit.CLUBS))
                .withFaceDownCard(1, PipCard.of(Rank.KING, Suit.SPADES))
                .build();

        assertThat(moveRules.canAttack(state, 0, sevenClubs)).isEqualTo(MoveVerdict.allowed());
    }

    @DisplayName("Should allow revealing the face-down card When the deck is empty and the hand is spent")
    @Test
    void shouldAllowRevealingTheFaceDownCardWhenTheDeckIsEmptyAndTheHandIsSpent() {
        final DealState state = aDeal()
                .withEmptyDeck()
                .withFaceDownCard(0, PipCard.of(Rank.KING, Suit.SPADES))
                .withHand(1, PipCard.of(Rank.ACE, Suit.CLUBS))
                .build();

        assertThat(moveRules.canRevealFaceDown(state, 0)).isEqualTo(MoveVerdict.allowed());
    }

    @DisplayName("Should reject revealing the face-down card When ordinary cards are still in hand")
    @Test
    void shouldRejectRevealingTheFaceDownCardWhenOrdinaryCardsAreStillInHand() {
        final DealState state = aDeal()
                .withEmptyDeck()
                .withHand(0, PipCard.of(Rank.SIX, Suit.CLUBS))
                .withFaceDownCard(0, PipCard.of(Rank.KING, Suit.SPADES))
                .withHand(1, PipCard.of(Rank.ACE, Suit.CLUBS))
                .build();

        assertThat(moveRules.canRevealFaceDown(state, 0))
                .isEqualTo(MoveVerdict.rejected(RejectionReason.FACE_DOWN_CARD_NOT_PLAYABLE));
    }

    @DisplayName("Should reject revealing the face-down card When the deck still has cards")
    @Test
    void shouldRejectRevealingTheFaceDownCardWhenTheDeckStillHasCards() {
        final DealState state = aDeal()
                .withDeckOf(2)
                .withFaceDownCard(0, PipCard.of(Rank.KING, Suit.SPADES))
                .withHand(1, PipCard.of(Rank.ACE, Suit.CLUBS))
                .build();

        assertThat(moveRules.canRevealFaceDown(state, 0))
                .isEqualTo(MoveVerdict.rejected(RejectionReason.FACE_DOWN_CARD_NOT_PLAYABLE));
    }

    @DisplayName("Should refuse to name the face-down card When a player tries to play it directly")
    @Test
    void shouldRefuseToNameTheFaceDownCardWhenAPlayerTriesToPlayItDirectly() {
        final Card faceDown = PipCard.of(Rank.KING, Suit.SPADES);
        final DealState state = aDeal()
                .withEmptyDeck()
                .withFaceDownCard(0, faceDown)
                .withHand(1, PipCard.of(Rank.ACE, Suit.CLUBS))
                .build();

        assertThat(moveRules.canAttack(state, 0, faceDown))
                .isEqualTo(MoveVerdict.rejected(RejectionReason.CARD_NOT_IN_HAND));
    }

    @DisplayName("Should allow attacking with a joker When the joker starts the round")
    @Test
    void shouldAllowAttackingWithAJokerWhenTheJokerStartsTheRound() {
        final Card joker = new JokerCard(1);
        final DealState state = aDeal().withHand(0, joker)
                .withHand(1, PipCard.of(Rank.ACE, Suit.CLUBS)).build();

        assertThat(moveRules.canAttack(state, 0, joker)).isEqualTo(MoveVerdict.allowed());
    }

    @DisplayName("Should reject a plain follow-up card When a joker lies on the table")
    @Test
    void shouldRejectAPlainFollowUpCardWhenAJokerLiesOnTheTable() {
        final Card ace = PipCard.of(Rank.ACE, Suit.SPADES);
        final DealState state = aDeal()
                .withAttackCards(new JokerCard(1))
                .withHand(0, ace)
                .withHand(1, PipCard.of(Rank.ACE, Suit.CLUBS), PipCard.of(Rank.KING, Suit.CLUBS))
                .build();

        assertThat(moveRules.canAttack(state, 0, ace))
                .isEqualTo(MoveVerdict.rejected(RejectionReason.RANK_NOT_ON_TABLE));
    }

    @DisplayName("Should allow a second joker as a follow-up When a joker lies on the table")
    @Test
    void shouldAllowASecondJokerAsAFollowUpWhenAJokerLiesOnTheTable() {
        final Card secondJoker = new JokerCard(2);
        final DealState state = aDeal()
                .withAttackCards(new JokerCard(1))
                .withHand(0, secondJoker)
                .withHand(1, PipCard.of(Rank.ACE, Suit.CLUBS), PipCard.of(Rank.KING, Suit.CLUBS))
                .build();

        assertThat(moveRules.canAttack(state, 0, secondJoker)).isEqualTo(MoveVerdict.allowed());
    }

    /**
     * Атакующие карты разных рангов одной масти — чтобы карта из руки в тесте совпадала
     * по рангу с лежащей на столе и при этом не дублировала её.
     */
    private static Card[] attackCardsOfDistinctRanks(final int count) {
        final List<Card> cards = new ArrayList<>();
        for (int index = 0; index < count; index++) {
            cards.add(PipCard.of(Rank.values()[index], Suit.DIAMONDS));
        }
        return cards.toArray(new Card[0]);
    }

    private static Card[] handOf(final int count) {
        final List<Card> cards = new ArrayList<>();
        for (int index = 0; index < count; index++) {
            cards.add(new JokerCard(index + 1));
        }
        return cards.toArray(new Card[0]);
    }
}
