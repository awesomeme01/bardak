package server

import (
	"context"

	"github.com/awesomeme01/bardak/back-go/internal/application"
)

// inviteNotifications — что нужно окликам от отправителя уведомлений.
type inviteNotifications interface {
	NotifyInvite(userID, fromName, tableName, tableID string)
}

// inviteWithPush — оклик за стол по сокету, а ушедшему — уведомлением.
//
// ⭐ Того, кто в сети, зовут сокетом: приглашение появляется прямо на экране, и звонок
// поверх него — лишний. Уведомлением догоняют только ушедшего — это ровно тот случай,
// ради которого push и заведён.
//
// ⚠️ Возвращается по-прежнему «дошло ли ПО СОКЕТУ», а не «отправлено хоть как-нибудь»:
// экран показывает «позвал» или «его нет — отправил уведомление», и склеить эти два
// ответа в один значило бы обещать доставку, которой не было.
type inviteWithPush struct {
	direct application.InviteDelivery
	push   inviteNotifications
}

func (i inviteWithPush) SendTableInvite(ctx context.Context, friendID, fromName string,
	table application.InviteTable) bool {
	if i.direct.SendTableInvite(ctx, friendID, fromName, table) {
		return true
	}
	i.push.NotifyInvite(friendID, fromName, table.Name, table.ID)
	return false
}
