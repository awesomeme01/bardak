package game

// DealCommand — намерение игрока.
//
// ⭐ Клиент присылает намерение, решает сервер. Поэтому команда не несёт ничего, кроме
// места и карт: всё остальное движок берёт из состояния. Доверять клиенту роль или фазу
// значило бы разрешить ему объявить себя атакующим.
//
// У игрока одно основное действие — положить карту на стол; смысл действия задаёт роль,
// поэтому здесь оно разложено на три РАЗНЫЕ команды, а не на одну с флагом.
type DealCommand interface {
	SeatNo() int
	sealedCommand()
}

// AttackCommand — положить карту в атаку: первой картой раунда или подкидом.
type AttackCommand struct {
	Seat int
	Card Card
}

func (c AttackCommand) SeatNo() int  { return c.Seat }
func (AttackCommand) sealedCommand() {}

// DefendCommand — отбить конкретную атакующую карту.
//
// ⚠️ Цель обязательна: при нескольких картах на столе иначе не зафиксировать,
// что чем отбито.
type DefendCommand struct {
	Seat   int
	Card   Card
	Target Card
}

func (c DefendCommand) SeatNo() int  { return c.Seat }
func (DefendCommand) sealedCommand() {}

// TransferCommand — перевести атаку дальше по кругу.
type TransferCommand struct {
	Seat int
	Card Card
}

func (c TransferCommand) SeatNo() int  { return c.Seat }
func (TransferCommand) sealedCommand() {}

// PassCommand — «пас»: явная фиксация того, что игрок больше не подкидывает.
//
// ⭐ Раунд не завершается сам по себе, пока обладатель права не спасовал.
type PassCommand struct{ Seat int }

func (c PassCommand) SeatNo() int  { return c.Seat }
func (PassCommand) sealedCommand() {}

// TakeCommand — «взял»: защищающийся забирает стол в руку.
type TakeCommand struct{ Seat int }

func (c TakeCommand) SeatNo() int  { return c.Seat }
func (TakeCommand) sealedCommand() {}

// HangCardCommand — заявка в открытом окне навеса.
//
// ⚠️ Карта называется СРАЗУ: иначе после броска кости победитель мог бы передумать,
// а заявка должна быть обязательством.
type HangCardCommand struct {
	Seat int
	Card Card
}

func (c HangCardCommand) SeatNo() int  { return c.Seat }
func (HangCardCommand) sealedCommand() {}

// HangSkipCommand — «пропустить»: навес всегда выбор, даже когда карта есть.
type HangSkipCommand struct{ Seat int }

func (c HangSkipCommand) SeatNo() int  { return c.Seat }
func (HangSkipCommand) sealedCommand() {}

// RevealFaceDownCommand — вскрыть скрытую карту и пойти ею.
//
// ⭐ Карта НЕ называется: игрок сам её не видит. Вскрытие необратимо и происходит даже
// тогда, когда ход ею не проходит по рангу, — тогда карта просто остаётся в руке.
type RevealFaceDownCommand struct{ Seat int }

func (c RevealFaceDownCommand) SeatNo() int  { return c.Seat }
func (RevealFaceDownCommand) sealedCommand() {}

// RevealFaceDownToDefendCommand — то же для защиты: вскрыть и попробовать побить цель.
type RevealFaceDownToDefendCommand struct {
	Seat   int
	Target Card
}

func (c RevealFaceDownToDefendCommand) SeatNo() int  { return c.Seat }
func (RevealFaceDownToDefendCommand) sealedCommand() {}

// ChooseTrumpCommand — победитель кости называет козырную масть, когда нижней картой
// колоды оказался джокер.
//
// ⭐ Это команда, а не вычисление: победитель именно ВЫБИРАЕТ, глядя в свои карты,
// и может назвать масть, которой у него нет.
type ChooseTrumpCommand struct {
	Seat int
	Suit Suit
}

func (c ChooseTrumpCommand) SeatNo() int  { return c.Seat }
func (ChooseTrumpCommand) sealedCommand() {}
