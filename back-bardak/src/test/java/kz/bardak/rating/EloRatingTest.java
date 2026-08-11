package kz.bardak.rating;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import java.util.List;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;

/**
 * Elo с попарным разложением (`07-rating-system.md`).
 */
class EloRatingTest {

    @DisplayName("Should move the rating towards the winner When two equals play")
    @Test
    void shouldMoveTheRatingTowardsTheWinnerWhenTwoEqualsPlay() {
        final List<Double> updated = EloRating.recalculate(List.of(
                new EloRating.Participant(1000, 50, 1),
                new EloRating.Participant(1000, 50, 2)));

        // Равные соперники, K=20: победитель получает половину K, проигравший столько же теряет.
        assertThat(updated.get(0)).isEqualTo(1010.0);
        assertThat(updated.get(1)).isEqualTo(990.0);
    }

    @DisplayName("Should give the newcomer a bigger step When they have few matches")
    @Test
    void shouldGiveTheNewcomerABiggerStepWhenTheyHaveFewMatches() {
        final double newcomerGain = EloRating.recalculate(List.of(
                new EloRating.Participant(1000, 0, 1),
                new EloRating.Participant(1000, 50, 2))).get(0) - 1000;
        final double regularGain = EloRating.recalculate(List.of(
                new EloRating.Participant(1000, 50, 1),
                new EloRating.Participant(1000, 50, 2))).get(0) - 1000;

        assertThat(newcomerGain).isGreaterThan(regularGain);
    }

    @DisplayName("Should barely move the rating When it is already very high")
    @Test
    void shouldBarelyMoveTheRatingWhenItIsAlreadyVeryHigh() {
        final List<Double> updated = EloRating.recalculate(List.of(
                new EloRating.Participant(2300, 100, 1),
                new EloRating.Participant(2300, 100, 2)));

        assertThat(updated.get(0) - 2300).isEqualTo(5.0);
    }

    @DisplayName("Should reward beating a stronger player more When the ratings differ")
    @Test
    void shouldRewardBeatingAStrongerPlayerMoreWhenTheRatingsDiffer() {
        final double underdogGain = EloRating.recalculate(List.of(
                new EloRating.Participant(800, 50, 1),
                new EloRating.Participant(1400, 50, 2))).get(0) - 800;
        final double favouriteGain = EloRating.recalculate(List.of(
                new EloRating.Participant(1400, 50, 1),
                new EloRating.Participant(800, 50, 2))).get(0) - 1400;

        assertThat(underdogGain).isGreaterThan(favouriteGain);
    }

    @DisplayName("Should keep the swing the same size When five play instead of two")
    @Test
    void shouldKeepTheSwingTheSameSizeWhenFivePlayInsteadOfTwo() {
        final double duelWinner = EloRating.recalculate(List.of(
                new EloRating.Participant(1000, 50, 1),
                new EloRating.Participant(1000, 50, 2))).get(0) - 1000;

        final double fiveWinner = EloRating.recalculate(List.of(
                new EloRating.Participant(1000, 50, 1),
                new EloRating.Participant(1000, 50, 2),
                new EloRating.Participant(1000, 50, 3),
                new EloRating.Participant(1000, 50, 4),
                new EloRating.Participant(1000, 50, 5))).get(0) - 1000;

        // ⭐ Без нормировки на (N−1) матч на пятерых двигал бы рейтинг вчетверо сильнее.
        assertThat(fiveWinner).isEqualTo(duelWinner);
    }

    @DisplayName("Should keep the sum of changes at zero When everyone has the same rating")
    @Test
    void shouldKeepTheSumOfChangesAtZeroWhenEveryoneHasTheSameRating() {
        final List<EloRating.Participant> table = List.of(
                new EloRating.Participant(1000, 50, 1),
                new EloRating.Participant(1000, 50, 2),
                new EloRating.Participant(1000, 50, 3),
                new EloRating.Participant(1000, 50, 4));

        final double sum = EloRating.recalculate(table).stream().mapToDouble(Double::doubleValue).sum();

        assertThat(sum).isEqualTo(4000.0, org.assertj.core.data.Offset.offset(0.0001));
    }

    @DisplayName("Should never fall below the floor When a weak player keeps losing")
    @Test
    void shouldNeverFallBelowTheFloorWhenAWeakPlayerKeepsLosing() {
        final List<Double> updated = EloRating.recalculate(List.of(
                new EloRating.Participant(2000, 50, 1),
                new EloRating.Participant(100, 5, 2)));

        assertThat(updated.get(1)).isGreaterThanOrEqualTo(100.0);
    }

    @DisplayName("Should refuse to count a solo match When there is nobody to compare with")
    @Test
    void shouldRefuseToCountASoloMatchWhenThereIsNobodyToCompareWith() {
        assertThatThrownBy(() -> EloRating.recalculate(List.of(
                new EloRating.Participant(1000, 1, 1))))
                .isInstanceOf(IllegalArgumentException.class);
    }
}
