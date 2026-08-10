package kz.bardak.game.rules;

import java.util.ArrayList;
import java.util.List;
import java.util.Objects;
import java.util.Optional;

/**
 * Сдача раздачи (§1.2): перемешивание от под-seed, руки, скрытые карты, козырь.
 *
 * <p>⭐ Колода собирается заново из {@link DeckFactory} каждый раз — это и есть «карты из
 * слотов, включая джокеры, возвращаются в колоду» (§2.3). Возвращать их поштучно не нужно
 * и было бы источником ошибок: слот живёт одну раздачу, а колода строится по составу стола.
 *
 * <p>Уровни навесов, наоборот, приходят снаружи: они живут весь матч (ADR-018).
 */
public final class Dealer {

    private final RulesConfig config;
    private final DeckFactory deckFactory = new DeckFactory();
    private final DiceResolver dice;

    public Dealer(final RulesConfig config, final DiceResolver dice) {
        this.config = Objects.requireNonNull(config, "config");
        this.dice = Objects.requireNonNull(dice, "dice");
    }

    /**
     * Сколько раз подряд можно пересдать, прежде чем сдаться. Практический предохранитель:
     * вероятность десяти пересдач подряд исчезающе мала, а бесконечный цикл в движке — нет.
     */
    private static final int MAX_RESHUFFLES = 10;

    /**
     * Новая раздача.
     *
     * <p>⭐ Если козырной масти не оказалось ни у кого, раздача <b>пересдаётся</b> (OQ-22):
     * первый ход определяется младшим козырем, и без козырей на руках определять его
     * не из чего. Пересдача идёт от производного seed и остаётся воспроизводимой.
     *
     * @param navesLevels уровни игроков по местам — переносятся между раздачами
     * @param dealSeed    под-seed раздачи, производный от seed матча (§6)
     */
    public DealState startDeal(final List<Integer> navesLevels, final long dealSeed) {
        Objects.requireNonNull(navesLevels, "navesLevels");
        long seed = dealSeed;
        for (int attempt = 0; attempt < MAX_RESHUFFLES; attempt++) {
            final DealState deal = dealOnce(navesLevels, seed);
            if (!deal.hasTrump() || hasAnyTrumpInHands(deal)) {
                return deal;
            }
            seed = reshuffleSeed(seed, attempt);
        }
        return dealOnce(navesLevels, seed);
    }

    /** Козырь есть хоть у кого-то — иначе первый ход определять не из чего. */
    public boolean hasAnyTrumpInHands(final DealState deal) {
        return deal.players().stream()
                .anyMatch(player -> lowestTrumpRank(player, deal.trump()).isPresent());
    }

    /** Seed пересдачи: другой расклад, но по-прежнему производный от seed матча (§6). */
    public long reshuffleSeed(final long seed, final int attempt) {
        return seed * 31L + attempt + 1;
    }

    private DealState dealOnce(final List<Integer> navesLevels, final long dealSeed) {
        final int playerCount = navesLevels.size();
        final List<Card> deck = new ArrayList<>(deckFactory.buildShuffled(playerCount, dealSeed));
        final List<PlayerState> players = dealHands(deck, navesLevels);
        final Card trumpCard = deck.get(deck.size() - 1);

        final DealState.Builder builder = emptyDeal(deck, players, dealSeed);
        if (trumpCard instanceof PipCard pip) {
            final Trump trump = Trump.of(pip.suit());
            final int starter = firstMoveSeat(players, trump);
            return builder
                    .trump(trump)
                    .phase(DealPhase.ATTACK)
                    .roundStarterSeat(starter)
                    .attackRightSeat(starter)
                    .defenderSeat((starter + 1) % playerCount)
                    .build();
        }
        return builder
                .phase(DealPhase.DICE)
                .attackRightSeat(dice.winnerAmong(seats(playerCount), dealSeed, 0))
                .diceRolls(1)
                .build();
    }

    /**
     * Кому ходить первым: обладателю <b>младшего козыря</b> (§1.2). Правило одно и то же
     * в каждой раздаче матча — проигравший прошлую преимуществ не получает.
     *
     * <p>Козырей нет ни у кого — до этого метода дело не доходит: такая раздача
     * пересдаётся (OQ-22).
     */
    private int firstMoveSeat(final List<PlayerState> players, final Trump trump) {
        Rank lowest = null;
        int starter = 0;
        for (final PlayerState player : players) {
            final Optional<Rank> candidate = lowestTrumpRank(player, trump);
            if (candidate.isPresent() && (lowest == null || lowest.isHigherThan(candidate.get()))) {
                lowest = candidate.get();
                starter = player.seatNo();
            }
        }
        return starter;
    }

    private Optional<Rank> lowestTrumpRank(final PlayerState player, final Trump trump) {
        return player.hand().stream()
                .filter(PipCard.class::isInstance)
                .map(PipCard.class::cast)
                .filter(card -> card.suit() == trump.suit())
                .map(PipCard::rank)
                .min(java.util.Comparator.naturalOrder());
    }

    /**
     * Раздача карт: по {@code dealSize} каждому, плюс одна скрытая карта сверх руки (§1.8).
     * Карты снимаются с верха колоды, поэтому порядок сдачи детерминирован seed'ом.
     */
    private List<PlayerState> dealHands(final List<Card> deck, final List<Integer> navesLevels) {
        final int playerCount = navesLevels.size();
        final List<List<Card>> hands = new ArrayList<>();
        for (int seat = 0; seat < playerCount; seat++) {
            hands.add(new ArrayList<>());
        }
        for (int card = 0; card < config.dealSize(); card++) {
            for (int seat = 0; seat < playerCount; seat++) {
                hands.get(seat).add(deck.remove(0));
            }
        }
        final List<PlayerState> players = new ArrayList<>();
        for (int seat = 0; seat < playerCount; seat++) {
            final Card faceDown = deck.remove(0);
            players.add(new PlayerState(seat, List.copyOf(hands.get(seat)), faceDown, true,
                    navesLevels.get(seat), List.of(), PlayerState.NOBODY));
        }
        return players;
    }

    private DealState.Builder emptyDeal(final List<Card> deck, final List<PlayerState> players,
                                        final long dealSeed) {
        return new DealState(DealPhase.DEALING, null, List.copyOf(deck), players, List.of(),
                0, 0, players.size() > 1 ? 1 : 0, List.of(), List.of(), false, false,
                null, List.of(), dealSeed, 0).toBuilder();
    }

    private List<Integer> seats(final int playerCount) {
        final List<Integer> seats = new ArrayList<>();
        for (int seat = 0; seat < playerCount; seat++) {
            seats.add(seat);
        }
        return List.copyOf(seats);
    }
}
