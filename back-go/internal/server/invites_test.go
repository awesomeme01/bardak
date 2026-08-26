package server

import (
	"context"
	"testing"

	"github.com/awesomeme01/bardak/back-go/internal/application"
)

// Оклик за стол: кому он уходит сокетом, а кому уведомлением.
//
// ⭐ Проверяется граница между двумя способами позвать. Позвать ушедшего уведомлением
// и оставить того, кто в сети, с одним лишь окном на экране — весь смысл этой проводки:
// звонок поверх уже показанного приглашения раздражает, а молчание в адрес ушедшего
// означает, что его не позвали вовсе.

type fakeDirect struct {
	delivered bool
	calls     int
}

func (f *fakeDirect) SendTableInvite(context.Context, string, string,
	application.InviteTable) bool {
	f.calls++
	return f.delivered
}

type fakeInvitePush struct {
	invites []application.InviteTable
	toWhom  []string
	from    []string
}

func (f *fakeInvitePush) NotifyInvite(userID, fromName, tableName, tableID string) {
	f.toWhom = append(f.toWhom, userID)
	f.from = append(f.from, fromName)
	f.invites = append(f.invites, application.InviteTable{ID: tableID, Name: tableName})
}

func invitesFixture(delivered bool) (inviteWithPush, *fakeDirect, *fakeInvitePush) {
	direct := &fakeDirect{delivered: delivered}
	pushes := &fakeInvitePush{}
	return inviteWithPush{direct: direct, push: pushes}, direct, pushes
}

var eveningTable = application.InviteTable{ID: "table-1", Name: "Вечерний", Code: "ABC123"}

func TestInviteGoesByPushWhenTheFriendIsAway(t *testing.T) {
	invites, _, pushes := invitesFixture(false)

	delivered := invites.SendTableInvite(context.Background(), "friend-1", "Аскар", eveningTable)

	if delivered {
		t.Fatalf("оклик к ушедшему объявлен доставленным по сокету")
	}
	if len(pushes.invites) != 1 {
		t.Fatalf("ушедшего не позвали уведомлением: %+v", pushes.invites)
	}
	if pushes.toWhom[0] != "friend-1" || pushes.from[0] != "Аскар" {
		t.Fatalf("позвали не того или не от того: %v / %v", pushes.toWhom, pushes.from)
	}
	// ⭐ Стол в уведомлении нужен не для красоты: по нему клик открывает нужную партию.
	if pushes.invites[0].ID != "table-1" || pushes.invites[0].Name != "Вечерний" {
		t.Fatalf("в уведомлении не тот стол: %+v", pushes.invites[0])
	}
}

func TestInviteStaysOnTheSocketWhenTheFriendIsOnline(t *testing.T) {
	invites, direct, pushes := invitesFixture(true)

	delivered := invites.SendTableInvite(context.Background(), "friend-1", "Аскар", eveningTable)

	if !delivered {
		t.Fatalf("доставленный по сокету оклик объявлен недоставленным")
	}
	if direct.calls != 1 {
		t.Fatalf("сокет спросили %d раз", direct.calls)
	}
	// Приглашение уже на экране: звонок поверх него — верный способ попасть
	// в «отключить уведомления».
	if len(pushes.invites) != 0 {
		t.Fatalf("позвонили тому, кто и так увидел приглашение: %+v", pushes.invites)
	}
}
