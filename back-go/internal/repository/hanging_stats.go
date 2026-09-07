package repository

import (
	"context"
	"fmt"
)

// Статистика навесов: что игрок навешивал сам.
//
// ⭐ Считается по ЛОГУ СОБЫТИЙ, а не по отдельным счётчикам. `deal_results.hung_cards`
// хранит то, что навесили игроку, и по нему не узнать, кто навесил: автор есть только
// у события `CARD_HUNG` (`actor_seat`). Лог append-only, поэтому пересчёт по нему всегда
// даёт то же число — в отличие от счётчика, который однажды разъедется с историей.
//
// ⚠️ Оффлайн-партии сюда не попадают вовсе, и это не упущение: за настоящим столом
// никто не протоколирует, кто кому что навесил. Пустая статистика у такого матча честнее
// выдуманной.

// HungRank — сколько раз игрок навесил карты этой ступени.
type HungRank struct {
	// Rank — код ступени: «6»…«A» или «Jk» у джокера.
	Rank  string
	Count int
}

// InflictedDegree — до какой степени проигрыша игрок доводил соперников и сколько раз.
type InflictedDegree struct {
	Degree string
	Count  int
}

// HungRanksOf — что игрок навешивал, по ступеням.
//
// ⚠️ Джокер сводится к «Jk» прямо в запросе: в логе он записан как «Joker-1» и «Joker-2»,
// то есть двумя разными кодами, а для статистики это одна и та же ступень.
func (r MatchHistory) HungRanksOf(ctx context.Context, userID string) ([]HungRank, error) {
	const query = `select case
	                          when e.payload->>'cardCode' like 'Joker-%' then 'Jk'
	                          else split_part(e.payload->>'cardCode', '-', 1)
	                      end as rank,
	                      count(*)
	               from match_events e
	               join match_players p
	                 on p.match_id = e.match_id and p.seat_no = e.actor_seat
	               where e.type = 'CARD_HUNG' and p.user_id = $1
	               group by 1`

	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("статистика навесов: %w", err)
	}
	defer rows.Close()

	hung := make([]HungRank, 0)
	for rows.Next() {
		var row HungRank
		if err := rows.Scan(&row.Rank, &row.Count); err != nil {
			return nil, fmt.Errorf("разбор навеса: %w", err)
		}
		hung = append(hung, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("статистика навесов: %w", err)
	}
	return hung, nil
}

// InflictedDegreesOf — кого и до чего игрок довёл джокером.
//
// ⭐ Засчитывается тот, кто навесил ИМЕННО ДЖОКЕР проигравшему: степень проигрыша
// определяется последней ступенью, и «накинул королевский» — это про того, чья карта
// довела соперника до конца шкалы. Навесивший до этого шестёрку к степени непричастен.
//
// ⚠️ Себе навесить нельзя, но защита от этого стоит и здесь: матч, восстановленный
// из снимка с битым actor_seat, иначе записал бы игроку победу над самим собой.
func (r MatchHistory) InflictedDegreesOf(ctx context.Context,
	userID string) ([]InflictedDegree, error) {
	const query = `select victim.loss_type, count(*)
	               from match_events e
	               join match_players hanger
	                 on hanger.match_id = e.match_id and hanger.seat_no = e.actor_seat
	               join match_players victim
	                 on victim.match_id = e.match_id
	                and victim.seat_no = (e.payload->>'victimSeat')::int
	               where e.type = 'CARD_HUNG'
	                 and e.payload->>'cardCode' like 'Joker-%'
	                 and hanger.user_id = $1
	                 and victim.user_id <> $1
	                 and victim.loss_type is not null
	               group by 1`

	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("статистика доведённых до степени: %w", err)
	}
	defer rows.Close()

	inflicted := make([]InflictedDegree, 0)
	for rows.Next() {
		var row InflictedDegree
		if err := rows.Scan(&row.Degree, &row.Count); err != nil {
			return nil, fmt.Errorf("разбор степени: %w", err)
		}
		inflicted = append(inflicted, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("статистика доведённых до степени: %w", err)
	}
	return inflicted, nil
}
