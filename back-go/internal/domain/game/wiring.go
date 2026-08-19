package game

// Сборка движка из частей, перенесённых по отдельности.
//
// ⚠️ Утверждения ниже — проверка ВРЕМЕНЕМ КОМПИЛЯЦИИ, что части подходят друг другу.
// Модули переносились параллельно, и разъехавшаяся сигнатура иначе всплыла бы только
// при первом живом матче.
var (
	_ dicer             = SeededDice{}
	_ hangingProvider   = HangingRules{}
	_ AttackOrderPolicy = BardakStrictNeighbours{}
)

// NewDefaultDealEngine — движок с боевыми правилами и стратегиями по умолчанию.
func NewDefaultDealEngine() DealEngine {
	return NewDealEngineFor(DefaultRulesConfig())
}

// NewDealEngineFor — движок для конкретных правил стола.
func NewDealEngineFor(config RulesConfig) DealEngine {
	return NewDealEngine(config, BardakStrictNeighbours{}, SeededDice{}, NewHangingRules(config))
}
