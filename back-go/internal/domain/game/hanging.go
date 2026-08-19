package game

// HangClaim — заявка на навес: кто и какой картой.
type HangClaim struct {
	SeatNo int
	Card   Card
}

// HangingWindow — открытое окно навеса на одного получателя, того, кто поднял карты.
//
// ⭐ Окно устроено как последовательность СТУПЕНЕЙ ПРАВА. В обычном случае ступеней три:
// атаковавший, поддержавший и все остальные наравне; при джокере и при уникальном
// отстающем ступень одна — право сразу у всех.
//
// ⚠️ Ступень из нескольких игроков решается КОСТЬЮ, а не «кто первый успел»: последнее —
// состязание пинга, а не игры.
type HangingWindow struct {
	// VictimSeat — кому навешивают: одно окно — один получатель.
	VictimSeat int
	// Steps — ступени права по порядку.
	Steps [][]int
	// StepIndex — текущая ступень.
	StepIndex int
	// Claims — заявки на текущей ступени.
	Claims []HangClaim
	// Decided — кто на текущей ступени уже решил: заявился или пропустил.
	Decided []int
	// EveryClaimantHangs — правило отстающего: навешивают все заявившиеся,
	// а уровень поднимается ровно на одну ступень.
	EveryClaimantHangs bool
}

// OpenHangingWindow открывает окно на первой ступени.
func OpenHangingWindow(victimSeat int, steps [][]int, everyClaimantHangs bool) HangingWindow {
	copied := make([][]int, len(steps))
	for i, step := range steps {
		copied[i] = append([]int(nil), step...)
	}
	return HangingWindow{
		VictimSeat:         victimSeat,
		Steps:              copied,
		StepIndex:          0,
		Claims:             []HangClaim{},
		Decided:            []int{},
		EveryClaimantHangs: everyClaimantHangs,
	}
}

// CurrentStep — места, у которых сейчас право.
func (w HangingWindow) CurrentStep() []int {
	if w.StepIndex < 0 || w.StepIndex >= len(w.Steps) {
		return nil
	}
	return w.Steps[w.StepIndex]
}

// IsSeatOnCurrentStep — место стоит на текущей ступени и ещё не решило.
func (w HangingWindow) IsSeatOnCurrentStep(seatNo int) bool {
	return containsInt(w.CurrentStep(), seatNo) && !containsInt(w.Decided, seatNo)
}

// IsStepComplete — ступень исчерпана: все, кто на ней стоял, заявились или пропустили.
func (w HangingWindow) IsStepComplete() bool {
	return len(w.Decided) >= len(w.CurrentStep())
}

// HasNextStep — есть ли следующая ступень права.
func (w HangingWindow) HasNextStep() bool { return w.StepIndex+1 < len(w.Steps) }

// WithClaim — заявка принята; заявившийся считается решившим.
func (w HangingWindow) WithClaim(claim HangClaim) HangingWindow {
	next := w
	next.Claims = append(append([]HangClaim(nil), w.Claims...), claim)
	next.Decided = append(append([]int(nil), w.Decided...), claim.SeatNo)
	return next
}

// WithDecline — место пропустило ход.
func (w HangingWindow) WithDecline(seatNo int) HangingWindow {
	next := w
	next.Decided = append(append([]int(nil), w.Decided...), seatNo)
	return next
}

// NextStep — следующая ступень: заявки и решения предыдущей НЕ переносятся.
func (w HangingWindow) NextStep() HangingWindow {
	next := w
	next.StepIndex = w.StepIndex + 1
	next.Claims = []HangClaim{}
	next.Decided = []int{}
	return next
}

// PendingHiddenTrump — потайной козырь оказался джокером и ждёт броска кости.
//
// ⭐ Порядок существенный: СНАЧАЛА кость и выбор масти, ПОТОМ карта в руку. Поэтому она
// и лежит отдельно — не в колоде, которая уже пуста, и не в руке добирающего, который
// иначе выбирал бы козырь, уже держа её.
type PendingHiddenTrump struct {
	// Card — сам джокер.
	Card Card
	// RecipientSeat — кому он достанется после выбора масти.
	RecipientSeat int
	// ChooserSeat — победитель кости: он называет масть.
	ChooserSeat int
}

func containsInt(values []int, value int) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
