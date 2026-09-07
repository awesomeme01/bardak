package repository

import (
	"context"
	"fmt"
)

// Общая статистика: все игроки в одной таблице.
//
// ⭐ Личная статистика отвечает на вопрос «как я играю», а этот экран — на вопрос
// «как мы играем». Это разные вопросы: своё среднее место ничего не значит, пока
// не видно чужого, а «кто чаще всех доводит до королевского» вообще нельзя узнать,
// глядя в один профиль.
//
// ⚠️ Считается ОДНИМ запросом на все строки, а не личной статистикой в цикле по
// игрокам: на десятке игроков разница незаметна, но цикл — это N+1, который однажды
// станет заметен весь сразу.

// OverviewPlayer — строка сводной таблицы.
type OverviewPlayer struct {
	UserID      string
	DisplayName string
	// Rating — текущий рейтинг; пусто у того, кто ещё не доиграл ни одного матча.
	Rating        *string
	MatchesPlayed int
	Wins          int
	Losses        int
	Royals        int
	// PlacesSum — сумма мест; среднее считает слой сценариев, чтобы округление
	// было тем же, что и в личной статистике.
	PlacesSum int
	// Hung — сколько карт игрок навесил сам; только по онлайн-матчам.
	Hung int
}

// OverviewTotals — общие итоги компании.
type OverviewTotals struct {
	Players int
	Matches int
	Offline int
	Deals   int
}

// Overview — сводка по всем игрокам, лучшие сверху.
//
// ⚠️ Берутся только засчитанные участия (`place is not null`): у отменённого матча
// итог пуст, и в сводку он не идёт — ровно как в личной статистике.
func (r Ratings) Overview(ctx context.Context) ([]OverviewPlayer, error) {
	const query = `select u.id, u.display_name, ur.rating::text,
	                      count(*) filter (where p.place is not null),
	                      count(*) filter (where p.place = 1),
	                      count(*) filter (where p.loss_type is not null),
	                      count(*) filter (where p.loss_type = 'ROYAL'),
	                      coalesce(sum(p.place) filter (where p.place is not null), 0),
	                      coalesce(hung.total, 0)
	               from users u
	               join match_players p on p.user_id = u.id and p.place is not null
	               left join user_rating ur on ur.user_id = u.id
	               left join lateral (
	                   select count(*) as total
	                   from match_events e
	                   join match_players mp
	                     on mp.match_id = e.match_id and mp.seat_no = e.actor_seat
	                   where e.type = 'CARD_HUNG' and mp.user_id = u.id
	               ) hung on true
	               group by u.id, u.display_name, ur.rating, hung.total
	               order by ur.rating desc nulls last, count(*) desc`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("сводная статистика: %w", err)
	}
	defer rows.Close()

	players := make([]OverviewPlayer, 0)
	for rows.Next() {
		var row OverviewPlayer
		err := rows.Scan(&row.UserID, &row.DisplayName, &row.Rating, &row.MatchesPlayed,
			&row.Wins, &row.Losses, &row.Royals, &row.PlacesSum, &row.Hung)
		if err != nil {
			return nil, fmt.Errorf("разбор строки сводки: %w", err)
		}
		players = append(players, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("сводная статистика: %w", err)
	}
	return players, nil
}

// OverviewTotals — сколько всего сыграно.
func (r Ratings) OverviewTotals(ctx context.Context) (OverviewTotals, error) {
	const query = `select
	    (select count(*) from user_rating),
	    (select count(*) from matches where status = 'FINISHED'),
	    (select count(*) from matches where is_offline),
	    (select coalesce(sum(deals_played), 0) from matches where status = 'FINISHED')`

	var totals OverviewTotals
	err := r.pool.QueryRow(ctx, query).Scan(&totals.Players, &totals.Matches,
		&totals.Offline, &totals.Deals)
	if err != nil {
		return OverviewTotals{}, fmt.Errorf("итоги: %w", err)
	}
	return totals, nil
}
