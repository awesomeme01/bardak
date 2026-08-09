package kz.bardak.game.rules;

import java.util.ArrayList;
import java.util.List;
import java.util.Objects;

/**
 * Открытое окно навеса на одного получателя — того, кто поднял карты (§2.3).
 *
 * <p>Окно устроено как последовательность <b>ступеней права</b>. В обычном случае ступеней
 * три: атаковавший, поддержавший и все остальные наравне; при джокере и при уникальном
 * отстающем ступень одна — право сразу у всех.
 *
 * <p>Ступень из одного игрока решается им самим. Ступень из нескольких собирает заявки
 * и разрешается костью (ADR-029) — «кто первый успел» не используется нигде, это состязание
 * пинга, а не игры.
 *
 * @param victimSeat        кому навешивают: одно окно — один получатель
 * @param steps             ступени права по порядку
 * @param stepIndex         текущая ступень
 * @param claims            заявки на текущей ступени
 * @param decided           кто на текущей ступени уже решил — заявился или пропустил
 * @param everyClaimantHangs правило отстающего: навешивают все заявившиеся, а уровень
 *                          поднимается ровно на одну ступень (§2.3)
 */
public record HangingWindow(
        int victimSeat,
        List<List<Integer>> steps,
        int stepIndex,
        List<HangClaim> claims,
        List<Integer> decided,
        boolean everyClaimantHangs) {

    public HangingWindow {
        steps = List.copyOf(Objects.requireNonNull(steps, "steps")).stream().map(List::copyOf).toList();
        claims = List.copyOf(Objects.requireNonNull(claims, "claims"));
        decided = List.copyOf(Objects.requireNonNull(decided, "decided"));
    }

    public static HangingWindow open(final int victimSeat, final List<List<Integer>> steps,
                                     final boolean everyClaimantHangs) {
        return new HangingWindow(victimSeat, steps, 0, List.of(), List.of(), everyClaimantHangs);
    }

    public List<Integer> currentStep() {
        return steps.get(stepIndex);
    }

    public boolean isSeatOnCurrentStep(final int seatNo) {
        return currentStep().contains(seatNo) && !decided.contains(seatNo);
    }

    /** Ступень исчерпана: все, кто на ней стоял, заявились или пропустили. */
    public boolean isStepComplete() {
        return decided.size() >= currentStep().size();
    }

    public boolean hasNextStep() {
        return stepIndex + 1 < steps.size();
    }

    public HangingWindow withClaim(final HangClaim claim) {
        final List<HangClaim> updated = new ArrayList<>(claims);
        updated.add(Objects.requireNonNull(claim, "claim"));
        return new HangingWindow(victimSeat, steps, stepIndex, List.copyOf(updated),
                withDecided(claim.seatNo()), everyClaimantHangs);
    }

    public HangingWindow withDecline(final int seatNo) {
        return new HangingWindow(victimSeat, steps, stepIndex, claims,
                withDecided(seatNo), everyClaimantHangs);
    }

    /** Следующая ступень: заявки и решения предыдущей не переносятся. */
    public HangingWindow nextStep() {
        return new HangingWindow(victimSeat, steps, stepIndex + 1, List.of(), List.of(), everyClaimantHangs);
    }

    private List<Integer> withDecided(final int seatNo) {
        final List<Integer> updated = new ArrayList<>(decided);
        updated.add(seatNo);
        return List.copyOf(updated);
    }
}
