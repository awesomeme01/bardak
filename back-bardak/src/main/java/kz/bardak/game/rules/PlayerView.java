package kz.bardak.game.rules;

import java.util.List;
import java.util.Objects;
import java.util.Optional;

/**
 * Персональная проекция состояния — то, что видит один игрок (fog of war).
 *
 * <p>⭐ Проекция — обязательный слой, а не фильтр в сериализаторе. Внутреннее состояние
 * содержит все карты всех игроков и наружу не сериализуется никогда.
 *
 * <p>Скрытая карта не попадает сюда вообще ни к кому — <b>включая владельца</b> (§1.8,
 * ADR-026). Владелец знает только {@link #iHaveHiddenCard()}.
 *
 * @param mySeat            место смотрящего
 * @param phase             фаза раздачи
 * @param trumpSuit         козырная масть; пусто, пока её разыгрывают костью (§1.2)
 * @param protectedSuit     защищённая масть — считается сервером, фронт её не выводит
 * @param deckLeft          сколько карт осталось в колоде; сами карты не отдаются
 * @param myHand            своя рука целиком
 * @param iHaveHiddenCard   есть ли у меня не вскрытая скрытая карта
 * @param table             стол виден всем
 * @param seats             остальные места, включая своё
 * @param roundStarterSeat  кто начал раунд
 * @param canAttackSeat     у кого сейчас право положить карту
 * @param defenderSeat      кто отбивается
 * @param hangingVictimSeat кому сейчас навешивают; пусто, если окна нет
 * @param availableActions  что именно я могу сделать прямо сейчас — считает сервер,
 *                          чтобы фронт не воспроизводил правила
 */
public record PlayerView(
        int mySeat,
        DealPhase phase,
        Suit trumpSuit,
        Suit protectedSuit,
        int deckLeft,
        List<Card> myHand,
        boolean iHaveHiddenCard,
        List<TableSlot> table,
        List<SeatView> seats,
        int roundStarterSeat,
        int canAttackSeat,
        int defenderSeat,
        Integer hangingVictimSeat,
        List<DealCommand> availableActions) {

    public PlayerView {
        Objects.requireNonNull(phase, "phase");
        myHand = List.copyOf(Objects.requireNonNull(myHand, "myHand"));
        table = List.copyOf(Objects.requireNonNull(table, "table"));
        seats = List.copyOf(Objects.requireNonNull(seats, "seats"));
        availableActions = List.copyOf(Objects.requireNonNull(availableActions, "availableActions"));
    }

    public Optional<Suit> trump() {
        return Optional.ofNullable(trumpSuit);
    }

    public Optional<Integer> hangingVictim() {
        return Optional.ofNullable(hangingVictimSeat);
    }

    public SeatView seat(final int seatNo) {
        return seats.stream()
                .filter(view -> view.seatNo() == seatNo)
                .findFirst()
                .orElseThrow(() -> new IllegalArgumentException("В проекции нет места " + seatNo));
    }
}
