// Команда import-offline — перенос прошлых партий компании в историю приложения.
//
// ⭐ Написана на Go и зовёт ТОТ ЖЕ application.Recalculate, что и живая игра. Это главное
// свойство этой команды: перенесённые партии обязаны считаться ровно теми же правилами,
// иначе рейтинг после импорта означал бы одно, а после первого онлайн-матча — другое,
// и сравнивать их было бы нельзя.
//
// ⚠️ Команда ИДЕМПОТЕНТНА: идентификатор матча выводится из номера партии, поэтому
// повторный запуск не удваивает историю, а пропускает уже перенесённое. Без этого
// прерванный на середине импорт чинился бы только руками.
//
// Запуск:
//
//	go run ./cmd/import-offline -dsn postgres://... -file база.md -map игроки.json [-dry]
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/awesomeme01/bardak/back-go/internal/application"
	"github.com/awesomeme01/bardak/back-go/internal/domain/game"
	"github.com/awesomeme01/bardak/back-go/internal/repository"
)

// importNamespace — пространство имён для выведенных идентификаторов матчей.
//
// ⚠️ Константа. Смена этого значения означает, что вся прошлая история переедет
// на новые идентификаторы и продублируется.
var importNamespace = uuid.MustParse("bd8f0a2c-6f21-4a3a-9c1f-0f0f5a2e7c10")

// oldOutcome — исход старой системы в терминах новой.
//
// ⭐ Шкалы совпадают почти один в один: «6»…«туз» — это ступени навесов, «джокер» —
// джокер без степени, «проебал»…«кор.отсос» — степени проигрыша.
//
// ⚠️ Одно расхождение осознанное: у старой системы ТРИ уровня отсоса, у новой два.
// «суперотсосал» и «супермегаотсосал» сводятся оба в SUPER_MEGA_SUCK. Это 24 записи
// из 1511, и разница между ними — один балл из шестнадцати; заводить ради неё шестую
// степень значило бы менять правила игры под импорт.
type oldOutcome struct {
	level  int
	degree game.LossDegree
}

var oldOutcomes = map[string]oldOutcome{
	"6":                {level: 0, degree: game.NoLossDegree},
	"7":                {level: 1, degree: game.NoLossDegree},
	"8":                {level: 2, degree: game.NoLossDegree},
	"9":                {level: 3, degree: game.NoLossDegree},
	"10":               {level: 4, degree: game.NoLossDegree},
	"валет":            {level: 5, degree: game.NoLossDegree},
	"дама":             {level: 6, degree: game.NoLossDegree},
	"король":           {level: 7, degree: game.NoLossDegree},
	"туз":              {level: 8, degree: game.NoLossDegree},
	"джокер":           {level: 9, degree: game.NoLossDegree},
	"проебал":          {level: 9, degree: game.LossFail},
	"суперпроебал":     {level: 9, degree: game.LossSuperFail},
	"супермегапроебал": {level: 9, degree: game.LossSuperMegaFail},
	"суперотсосал":     {level: 9, degree: game.LossSuperMegaSuck},
	"супермегаотсосал": {level: 9, degree: game.LossSuperMegaSuck},
	"кор.отсос":        {level: 9, degree: game.LossRoyal},
}

// outcomeNames — исходы, отсортированные от длинного к короткому.
//
// ⚠️ Порядок обязателен: имя игрока и исход разделены пробелом, а имена бывают
// составные («Димаш длинный», «Слава М»). Разбирать надо С КОНЦА и самым длинным
// совпадением, иначе «супермегапроебал» распознается как «проебал», а остаток
// прилипнет к имени.
var outcomeNames = sortedOutcomeNames()

func sortedOutcomeNames() []string {
	names := make([]string, 0, len(oldOutcomes))
	for name := range oldOutcomes {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool { return len(names[i]) > len(names[j]) })
	return names
}

var months = map[string]time.Month{
	"января": time.January, "февраля": time.February, "марта": time.March,
	"апреля": time.April, "мая": time.May, "июня": time.June,
	"июля": time.July, "августа": time.August, "сентября": time.September,
	"октября": time.October, "ноября": time.November, "декабря": time.December,
}

var (
	dateLine   = regexp.MustCompile(`^### (\d+) (\S+) (\d{4})$`)
	headerLine = regexp.MustCompile(`^\*\*№(\d+)\*\*`)
	playerPart = regexp.MustCompile(`^(.*?)\s*\((\d+)→[\d.]+\)$`)
)

// offlinePlayer — один игрок записанной партии.
type offlinePlayer struct {
	name    string
	outcome oldOutcome
}

// offlineGame — одна партия старой базы.
type offlineGame struct {
	number   int
	playedAt time.Time
	players  []offlinePlayer
}

func main() {
	dsn := flag.String("dsn", os.Getenv("BARDAK_DSN"), "строка подключения к Postgres")
	file := flag.String("file", "", "выгрузка старой системы (markdown)")
	mapping := flag.String("map", "", "JSON: имя в старой базе -> логин в приложении")
	seasonName := flag.String("season", "Оффлайн-архив", "имя сезона для перенесённых партий")
	dry := flag.Bool("dry", false, "посчитать и показать итог, но ничего не писать")
	flag.Parse()

	if *dsn == "" || *file == "" || *mapping == "" {
		log.Fatal("нужны -dsn, -file и -map")
	}

	games, err := parseGames(*file)
	if err != nil {
		log.Fatalf("разбор выгрузки: %v", err)
	}
	log.Printf("разобрано партий: %d", len(games))

	names, err := loadMapping(*mapping)
	if err != nil {
		log.Fatalf("разбор соответствия имён: %v", err)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *dsn)
	if err != nil {
		log.Fatalf("подключение к базе: %v", err)
	}
	defer pool.Close()

	users, err := resolveUsers(ctx, pool, names)
	if err != nil {
		log.Fatalf("поиск игроков: %v", err)
	}
	for name, login := range names {
		log.Printf("  %s -> @%s (%s)", name, login, users[name])
	}

	if err := run(ctx, pool, games, users, *seasonName, *dry); err != nil {
		log.Fatalf("импорт: %v", err)
	}
}

// parseGames разбирает раздел «Полная история партий».
func parseGames(path string) ([]offlineGame, error) {
	source, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer source.Close()

	var (
		games   []offlineGame
		current time.Time
		started bool
		pending bool
		number  int
	)
	scanner := bufio.NewScanner(source)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "## 13.") {
			started = true
			continue
		}
		if !started {
			continue
		}

		if match := dateLine.FindStringSubmatch(line); match != nil {
			day, _ := strconv.Atoi(match[1])
			year, _ := strconv.Atoi(match[3])
			month, ok := months[match[2]]
			if !ok {
				return nil, fmt.Errorf("неизвестный месяц %q", match[2])
			}
			current = time.Date(year, month, day, 20, 0, 0, 0, time.UTC)
			continue
		}
		if match := headerLine.FindStringSubmatch(line); match != nil {
			number, _ = strconv.Atoi(match[1])
			pending = true
			continue
		}
		if !pending || !strings.Contains(line, "→") {
			continue
		}

		players, err := parsePlayers(line)
		if err != nil {
			return nil, fmt.Errorf("партия №%d: %w", number, err)
		}
		// ⚠️ Время партии сдвигается на её номер: в один день играли по нескольку
		// партий, а история рейтинга сортируется по времени. Без сдвига порядок внутри
		// дня выбирала бы база, и график прыгал бы туда-сюда.
		games = append(games, offlineGame{
			number:   number,
			playedAt: current.Add(time.Duration(number) * time.Second),
			players:  players,
		})
		pending = false
	}
	return games, scanner.Err()
}

func parsePlayers(line string) ([]offlinePlayer, error) {
	players := make([]offlinePlayer, 0, 6)
	for _, part := range strings.Split(line, "·") {
		match := playerPart.FindStringSubmatch(strings.TrimSpace(part))
		if match == nil {
			return nil, fmt.Errorf("не разобрано: %q", part)
		}
		body := strings.TrimSpace(match[1])

		found := false
		for _, name := range outcomeNames {
			if body != name && !strings.HasSuffix(body, " "+name) {
				continue
			}
			players = append(players, offlinePlayer{
				name:    strings.TrimSpace(strings.TrimSuffix(body, name)),
				outcome: oldOutcomes[name],
			})
			found = true
			break
		}
		if !found {
			return nil, fmt.Errorf("исход не опознан: %q", body)
		}
	}
	return players, nil
}

func loadMapping(path string) (map[string]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var names map[string]string
	if err := json.Unmarshal(raw, &names); err != nil {
		return nil, err
	}
	return names, nil
}

// resolveUsers переводит логины в идентификаторы.
//
// ⚠️ Отсутствие игрока — это отказ, а не пропуск: молча выкинутый из состава игрок
// исказил бы рейтинг всех остальных в его партиях, и заметили бы это очень нескоро.
func resolveUsers(ctx context.Context, pool *pgxpool.Pool,
	names map[string]string) (map[string]string, error) {
	users := make(map[string]string, len(names))
	for name, login := range names {
		var id string
		err := pool.QueryRow(ctx,
			`select id from users where lower(username) = lower($1)`, login).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("игрока @%s (%s) нет в базе", login, name)
		}
		if err != nil {
			return nil, err
		}
		users[name] = id
	}
	return users, nil
}

func run(ctx context.Context, pool *pgxpool.Pool, games []offlineGame,
	users map[string]string, seasonName string, dry bool) error {
	scale := game.FullNavesScale()
	results := repository.NewMatchResults(pool)
	ratings := repository.NewRatings(pool)

	seasonID, err := ensureSeason(ctx, pool, games, seasonName, dry)
	if err != nil {
		return err
	}

	imported, skipped, dropped := 0, 0, 0
	for _, offline := range games {
		players := make([]offlinePlayer, 0, len(offline.players))
		for _, player := range offline.players {
			if _, ok := users[player.name]; ok {
				players = append(players, player)
			}
		}
		// ⭐ Партия, где своих меньше двоих, рейтингу ничего не говорит: сравнивать не с кем.
		if len(players) < 2 {
			dropped++
			continue
		}

		matchID := uuid.NewSHA1(importNamespace,
			[]byte(strconv.Itoa(offline.number))).String()
		exists, err := matchExists(ctx, pool, matchID)
		if err != nil {
			return err
		}
		if exists {
			skipped++
			continue
		}

		outcomes, err := countMatch(ctx, ratings, scale, players, users)
		if err != nil {
			return fmt.Errorf("партия №%d: %w", offline.number, err)
		}
		if dry {
			imported++
			continue
		}

		match := repository.OfflineMatch{
			MatchID:   matchID,
			CreatedBy: users[players[0].name],
			PlayedAt:  offline.playedAt,
			SeasonID:  seasonID,
			Outcomes:  outcomes,
		}
		if loser, found := heaviestLoser(players, users); found {
			match.LoserUserID = &loser
		}
		if err := results.CreateOffline(ctx, match); err != nil {
			return fmt.Errorf("партия №%d: %w", offline.number, err)
		}
		imported++
	}

	log.Printf("перенесено %d, пропущено как уже перенесённые %d, "+
		"отброшено (меньше двух своих) %d", imported, skipped, dropped)
	return report(ctx, pool, users, dry)
}

// countMatch считает рейтинг партии тем же кодом, что и живая игра.
func countMatch(ctx context.Context, ratings repository.Ratings, scale game.NavesScale,
	players []offlinePlayer, users map[string]string) ([]repository.SeatOutcome, error) {
	participants := make([]application.EloParticipant, 0, len(players))
	before := make([]string, 0, len(players))
	for _, player := range players {
		rating, err := currentRating(ctx, ratings, users[player.name])
		if err != nil {
			return nil, err
		}
		value, err := strconv.ParseFloat(rating.Rating, 64)
		if err != nil {
			return nil, err
		}
		before = append(before, rating.Rating)
		participants = append(participants, application.EloParticipant{
			Rating:        value,
			MatchesPlayed: rating.MatchesPlayed,
			Damage: application.DamageOf(scale, player.outcome.level,
				player.outcome.degree),
		})
	}

	updated, err := application.Recalculate(participants)
	if err != nil {
		return nil, err
	}

	places := placesOf(participants)
	outcomes := make([]repository.SeatOutcome, 0, len(players))
	for index, player := range players {
		level, degree := encode(scale, player.outcome)
		outcomes = append(outcomes, repository.SeatOutcome{
			UserID: users[player.name], SeatNo: index, Place: places[index],
			NavesLevel: level, LossType: degree,
			RatingBefore: before[index],
			RatingAfter:  strconv.FormatFloat(updated[index], 'f', 2, 64),
		})
	}
	return outcomes, nil
}

func placesOf(participants []application.EloParticipant) []int {
	order := make([]int, len(participants))
	for index := range order {
		order[index] = index
	}
	sort.SliceStable(order, func(left, right int) bool {
		return participants[order[left]].Damage < participants[order[right]].Damage
	})
	places := make([]int, len(participants))
	for position, index := range order {
		places[index] = position + 1
	}
	return places
}

func encode(scale game.NavesScale, outcome oldOutcome) (*string, *string) {
	var level, degree *string
	if scale.IsFinished(outcome.level) {
		joker := application.JokerNavesLevel
		level = &joker
	} else {
		code := scale.Ranks[outcome.level].Code()
		level = &code
	}
	if outcome.degree != game.NoLossDegree {
		name := outcome.degree.String()
		degree = &name
	}
	return level, degree
}

func heaviestLoser(players []offlinePlayer, users map[string]string) (string, bool) {
	worst, found := 0, false
	for index, player := range players {
		if player.outcome.degree == game.NoLossDegree {
			continue
		}
		if !found || player.outcome.degree.IsHeavierThan(players[worst].outcome.degree) {
			worst, found = index, true
		}
	}
	if !found {
		return "", false
	}
	return users[players[worst].name], true
}

func currentRating(ctx context.Context, ratings repository.Ratings,
	userID string) (repository.UserRating, error) {
	rating, err := ratings.FindRating(ctx, userID)
	if errors.Is(err, repository.ErrNotFound) {
		return repository.UserRating{UserID: userID, Rating: "1000.00"}, nil
	}
	return rating, err
}

func matchExists(ctx context.Context, pool *pgxpool.Pool, matchID string) (bool, error) {
	var exists bool
	err := pool.QueryRow(ctx,
		`select exists (select 1 from matches where id = $1)`, matchID).Scan(&exists)
	return exists, err
}

// ensureSeason заводит сезон под перенесённые партии.
//
// ⚠️ Сезон создаётся СРАЗУ ЗАКРЫТЫМ: открытый сезон в базе ровно один (частичный
// уникальный индекс), и им остаётся текущий, в котором идут онлайн-матчи.
func ensureSeason(ctx context.Context, pool *pgxpool.Pool, games []offlineGame,
	name string, dry bool) (*string, error) {
	if len(games) == 0 {
		return nil, nil
	}

	var id string
	err := pool.QueryRow(ctx, `select id from seasons where name = $1`, name).Scan(&id)
	if err == nil {
		return &id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if dry {
		return nil, nil
	}

	id = uuid.NewString()
	_, err = pool.Exec(ctx,
		`insert into seasons (id, name, started_at, closed_at) values ($1, $2, $3, $4)`,
		id, name, games[0].playedAt, games[len(games)-1].playedAt)
	if err != nil {
		return nil, err
	}
	log.Printf("заведён сезон %q (%s — %s)", name,
		games[0].playedAt.Format("02.01.2006"),
		games[len(games)-1].playedAt.Format("02.01.2006"))
	return &id, nil
}

func report(ctx context.Context, pool *pgxpool.Pool, users map[string]string, dry bool) error {
	if dry {
		log.Print("сухой прогон: в базу ничего не записано")
		return nil
	}

	rows, err := pool.Query(ctx,
		`select u.display_name, r.rating::text, r.matches_played
		 from user_rating r join users u on u.id = r.user_id
		 order by r.rating desc`)
	if err != nil {
		return err
	}
	defer rows.Close()

	log.Print("рейтинг после импорта:")
	for rows.Next() {
		var name, rating string
		var played int
		if err := rows.Scan(&name, &rating, &played); err != nil {
			return err
		}
		log.Printf("  %-28s %8s  (%d матчей)", name, rating, played)
	}
	return rows.Err()
}
