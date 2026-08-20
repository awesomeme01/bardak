package application

import (
	"context"

	"github.com/awesomeme01/bardak/back-go/internal/repository"
)

// TableInviteLookup — переходник от репозитория столов к тому, что нужно друзьям.
//
// ⚠️ Друзья не знают про лобби и не должны: им нужны три поля стола, а не его состояние.
// Отдельный тип вместо метода на репозитории — чтобы репозиторий не обрастал формами
// под каждого потребителя.
type TableInviteLookup struct {
	Tables repository.Tables
}

// InviteTableByID отдаёт стол для приглашения.
func (l TableInviteLookup) InviteTableByID(ctx context.Context, tableID string) (InviteTable, error) {
	table, err := l.Tables.FindByID(ctx, tableID)
	if err != nil {
		// ⚠️ Ошибку не подменяем: сценарий отличает «стола нет» от «база упала»
		// и превращает первое в 404, а не в 500.
		return InviteTable{}, err
	}
	return InviteTable{ID: table.ID, Name: table.Name, Code: table.Code}, nil
}
