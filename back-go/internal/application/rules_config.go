package application

import (
	"encoding/json"
	"log/slog"

	"github.com/awesomeme01/bardak/back-go/internal/domain/game"
)

// ParseRulesConfig разбирает `rules_config` стола в конфиг движка (ADR-016).
//
// ⭐ Всё, чего в конфиге нет, берётся из умолчаний: стол, созданный до появления нового
// параметра, обязан продолжать играть.
//
// ⚠️ Неразобранный конфиг — не отказ, а игра по умолчанию с записью в журнал: уронить
// старт матча из-за одного кривого поля значит запереть за столом живых людей.
func ParseRulesConfig(raw string, log *slog.Logger) game.RulesConfig {
	defaults := game.DefaultRulesConfig()
	if raw == "" {
		return defaults
	}

	var node map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &node); err != nil {
		if log != nil {
			log.Warn("не разобрал rules_config стола, играем по умолчанию", "err", err)
		}
		return defaults
	}

	// ⚠️ Поля читаются ПООТДЕЛЬНОСТИ и терпимо, как это делает Jackson (`asInt(default)`):
	// одно поле чужого типа не должно отменять остальные. Разбор «всё или ничего» тихо
	// вернул бы стол к умолчаниям целиком — и правила игры за ним поменялись бы посреди
	// вечера, без единой записи в журнале.
	config := defaults
	readInt(node, "dealSize", &config.DealSize)
	readInt(node, "maxAttackFirstRound", &config.MaxAttackFirstRound)
	readInt(node, "maxAttackPerRound", &config.MaxAttackPerRound)
	readBool(node, "transfersEnabled", &config.TransfersEnabled)
	readBool(node, "jokersEnabled", &config.JokersEnabled)

	var naves map[string]json.RawMessage
	if encoded, ok := node["naves"]; ok {
		_ = json.Unmarshal(encoded, &naves)
	}
	readBool(naves, "enabled", &config.NavesEnabled)
	config.NavesScale = scaleOf(naves["scale"], defaults.NavesScale, log)

	// ⚠️ Проверка после разбора, а не вместо него: конфиг с ерундой в числах (нулевая
	// раздача, лимит меньше нуля) прошёл бы дальше молча и сломался бы в движке посреди
	// матча, где чинить его уже некому.
	if err := config.Validate(); err != nil {
		if log != nil {
			log.Warn("rules_config стола не проходит проверку, играем по умолчанию", "err", err)
		}
		return defaults
	}
	return config
}

// readInt читает целое поле; отсутствующее или неразборчивое оставляет как есть.
func readInt(node map[string]json.RawMessage, name string, target *int) {
	encoded, ok := node[name]
	if !ok {
		return
	}
	var value int
	if err := json.Unmarshal(encoded, &value); err == nil {
		*target = value
	}
}

// readBool — то же для флага.
func readBool(node map[string]json.RawMessage, name string, target *bool) {
	encoded, ok := node[name]
	if !ok {
		return
	}
	var value bool
	if err := json.Unmarshal(encoded, &value); err == nil {
		*target = value
	}
}

// scaleOf — шкала навесов из кодов рангов. Пустая или неполная — умолчание целиком:
// шкала из половины ступеней меняет длину матча, а не одну настройку.
func scaleOf(encoded json.RawMessage, defaults game.NavesScale, log *slog.Logger) game.NavesScale {
	var codes []string
	if len(encoded) == 0 || json.Unmarshal(encoded, &codes) != nil || len(codes) == 0 {
		return defaults
	}
	ranks := make([]game.Rank, 0, len(codes))
	for _, code := range codes {
		rank, ok := rankByCode(code)
		if !ok {
			if log != nil {
				log.Warn("шкала навесов содержит неизвестную ступень, беру умолчание", "code", code)
			}
			return defaults
		}
		ranks = append(ranks, rank)
	}
	scale, err := game.NewNavesScale(ranks)
	if err != nil {
		return defaults
	}
	return scale
}

// rankByCode — ступень шкалы по короткому коду («6», «10», «A»).
//
// Перебор по списку рангов, а не карта соответствий: рангов девять, а лишняя таблица
// разошлась бы с Code() при первой же правке.
func rankByCode(code string) (game.Rank, bool) {
	for _, rank := range game.Ranks() {
		if rank.Code() == code {
			return rank, true
		}
	}
	return 0, false
}
