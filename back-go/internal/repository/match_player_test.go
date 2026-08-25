package repository

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// Места матча и их итоги на настоящем Postgres.
//
// ⚠️ Смысл теста не в SQL, а в двух свойствах, которые чинятся потом только руками:
// места записаны ДО итога, и повторная запись итога не удваивает ничего.

func matchPlayersFixture(t *testing.T) (MatchPlayers, MatchHistory, string, []string, context.Context) {
	t.Helper()
	pool := testDB(t)
	ctx := context.Background()
	users := NewUsers(pool)

	players := make([]string, 0, 2)
	for _, name := range []string{"Первый", "Второй"} {
		id := uuid.NewString()
		if _, err := users.Insert(ctx, User{ID: id, Username: "mp-" + id[:8],
			DisplayName: name, PasswordHash: "hash"}); err != nil {
			t.Fatal(err)
		}
		players = append(players, id)
	}

	tableID := uuid.NewString()
	if _, err := pool.Exec(ctx, `insert into game_tables
		(id, code, name, host_user_id, max_players, status, card_set_id, theme_id, rules_config, is_private)
		values ($1, $2, 'Итоги', $3, 2, 'IN_MATCH',
		        (select id from card_sets where is_default limit 1),
		        (select id from table_themes where is_default limit 1), '{}'::jsonb, false)`,
		tableID, "M"+players[0][:7], players[0]); err != nil {
		t.Fatal(err)
	}

	match, err := NewMatchLog(pool).StartMatch(ctx, uuid.NewString(), tableID, 2, 7, "{}")
	if err != nil {
		t.Fatal(err)
	}
	return NewMatchPlayers(pool), NewMatchHistory(pool), match.ID, players, ctx
}

func TestMatchSeatsAreWrittenAtStartWithoutResult(t *testing.T) {
	players, history, matchID, users, ctx := matchPlayersFixture(t)

	if err := players.Seat(ctx, matchID, users); err != nil {
		t.Fatalf("места не записались: %v", err)
	}

	seated, err := history.ParticipantsOf(ctx, matchID)
	if err != nil {
		t.Fatal(err)
	}
	if len(seated) != 2 || seated[0].UserID != users[0] || seated[0].SeatNo != 0 {
		t.Fatalf("места записаны не в том порядке: %+v", seated)
	}

	// ⭐ Место в матче есть, итога нет: именно пустой place потом служит фильтром
	// в статистике — сыгранным матч считается только с итогом.
	rows, err := history.PlayersOf(ctx, []string{matchID})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows[matchID] {
		if row.Place != nil {
			t.Fatalf("у только что посаженного игрока уже есть место: %+v", row)
		}
	}

	// Повтор безвреден: матч уже посажен, а не посажен второй раз.
	if err := players.Seat(ctx, matchID, users); err != nil {
		t.Fatalf("повторная посадка сломалась: %v", err)
	}
	again, err := history.ParticipantsOf(ctx, matchID)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 2 {
		t.Fatalf("повторная посадка удвоила места: %+v", again)
	}
}
