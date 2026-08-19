package game

// RejectionReason — почему ход отклонён.
//
// ⭐ Причина уходит клиенту как есть: фронт правил не знает и объясняет игроку отказ
// именно этим кодом. Значит имена — часть контракта, а не внутренняя деталь.
type RejectionReason string

const (
	NotYourTurn              RejectionReason = "NOT_YOUR_TURN"
	CardNotInHand            RejectionReason = "CARD_NOT_IN_HAND"
	FaceDownCardNotPlayable  RejectionReason = "FACE_DOWN_CARD_NOT_PLAYABLE"
	AttackLimitReached       RejectionReason = "ATTACK_LIMIT_REACHED"
	DefenderHasTooFewCards   RejectionReason = "DEFENDER_HAS_TOO_FEW_CARDS"
	RankNotOnTable           RejectionReason = "RANK_NOT_ON_TABLE"
	TargetNotOnTable         RejectionReason = "TARGET_NOT_ON_TABLE"
	TargetAlreadyBeaten      RejectionReason = "TARGET_ALREADY_BEATEN"
	CardDoesNotBeat          RejectionReason = "CARD_DOES_NOT_BEAT"
	DefenderAlreadyTook      RejectionReason = "DEFENDER_ALREADY_TOOK"
	TransfersDisabled        RejectionReason = "TRANSFERS_DISABLED"
	TransferAfterFirstBeat   RejectionReason = "TRANSFER_AFTER_FIRST_BEAT"
	TransferRankMismatch     RejectionReason = "TRANSFER_RANK_MISMATCH"
	NextPlayerHasTooFewCards RejectionReason = "NEXT_PLAYER_HAS_TOO_FEW_CARDS"
	NavesDisabled            RejectionReason = "NAVES_DISABLED"
	CannotHangOnSelf         RejectionReason = "CANNOT_HANG_ON_SELF"
	CardNotOnNavesScale      RejectionReason = "CARD_NOT_ON_NAVES_SCALE"
	NotInHangingWindow       RejectionReason = "NOT_IN_HANGING_WINDOW"
	TrumpNotInDispute        RejectionReason = "TRUMP_NOT_IN_DISPUTE"
	TrumpNotChosenYet        RejectionReason = "TRUMP_NOT_CHOSEN_YET"
	NothingToTake            RejectionReason = "NOTHING_TO_TAKE"
	MustRevealFaceDown       RejectionReason = "MUST_REVEAL_FACE_DOWN"
)

// MoveVerdict — приговор по ходу: разрешён либо отклонён с причиной.
//
// ⭐ Отдельный тип вместо bool ровно затем, чтобы причину нельзя было потерять по дороге.
// Отказ без причины игрок читает как «непонятно что», и это худшее, что можно ему показать.
type MoveVerdict struct {
	allowed bool
	reason  RejectionReason
}

// Allowed — ход разрешён.
func Allowed() MoveVerdict { return MoveVerdict{allowed: true} }

// Rejected — ход отклонён с причиной.
func Rejected(reason RejectionReason) MoveVerdict {
	return MoveVerdict{allowed: false, reason: reason}
}

// IsAllowed — разрешён ли ход.
func (v MoveVerdict) IsAllowed() bool { return v.allowed }

// Reason — причина отказа; пусто, если ход разрешён.
func (v MoveVerdict) Reason() RejectionReason {
	if v.allowed {
		return ""
	}
	return v.reason
}
