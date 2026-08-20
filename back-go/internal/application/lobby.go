package application

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/awesomeme01/bardak/back-go/internal/repository"
)

// Отказы лобби. Транспорт превращает их в коды и статусы; сам сценарий про HTTP не знает.
//
// ⚠️ Текст отказа у MATCH_IN_PROGRESS и ALREADY_AT_TABLE зависит от МЕСТА, а не от кода
// (в Java это три разные строки на один код), поэтому оба — типы с полем-сообщением,
// а не голые sentinel-значения.
var (
	ErrTableNotFound    = errors.New("стол не найден")
	ErrTableNotOpen     = errors.New("за этот стол уже нельзя сесть")
	ErrTableFull        = errors.New("за столом нет свободных мест")
	ErrNotTableHost     = errors.New("стол закрывает только хозяин")
	ErrMatchInProgress  = errors.New("идёт матч")
	ErrAlreadyAtTable   = errors.New("игрок уже за другим столом")
	ErrNoDefaultCardSet = errors.New("не настроен набор карт по умолчанию")
	ErrNoDefaultTheme   = errors.New("не настроена тема стола по умолчанию")
)

// MatchInProgressError — отказ «посреди матча» с текстом того места, где он случился.
type MatchInProgressError struct{ Message string }

func (e MatchInProgressError) Error() string { return e.Message }

// Is делает ошибку узнаваемой через errors.Is(err, ErrMatchInProgress).
func (e MatchInProgressError) Is(target error) bool { return target == ErrMatchInProgress }

// AlreadyAtTableError — игрок уже сидит за другим столом; в сообщении его название.
//
// ⭐ Название обязано быть в ответе: без него игрок видит «ты уже за столом» и не
// понимает, за каким именно, — а встать можно только зная стол.
type AlreadyAtTableError struct{ TableName string }

func (e AlreadyAtTableError) Error() string {
	return fmt.Sprintf("Ты уже за столом «%s» — сначала встань из-за него", e.TableName)
}

// Is делает ошибку узнаваемой через errors.Is(err, ErrAlreadyAtTable).
func (e AlreadyAtTableError) Is(target error) bool { return target == ErrAlreadyAtTable }

// TableStore — что нужно лобби от базы.
//
// ⭐ Интерфейс объявлен здесь, на стороне потребителя: сценарии посадки — самое ценное
// в этом файле (гонка за последнее место, один стол на игрока), и проверять их надо
// без базы, за миллисекунды. repository.Tables ему удовлетворяет как есть.
type TableStore interface {
	FindOpen(ctx context.Context) ([]repository.GameTable, error)
	FindByID(ctx context.Context, id string) (repository.GameTable, error)
	FindByCode(ctx context.Context, code string) (repository.GameTable, error)
	ExistsByCode(ctx context.Context, code string) (bool, error)
	Insert(ctx context.Context, table repository.GameTable) (repository.GameTable, error)
	Close(ctx context.Context, id string, at time.Time) error
	Seats(ctx context.Context, tableID string) ([]repository.TablePlayer, error)
	SeatAt(ctx context.Context, tableID, userID string) (repository.TablePlayer, error)
	SeatOf(ctx context.Context, userID string) (repository.TablePlayer, error)
	InsertSeat(ctx context.Context, seat repository.TablePlayer) (repository.TablePlayer, error)
	DeleteSeat(ctx context.Context, tableID, userID string) (bool, error)
	DefaultCardSetID(ctx context.Context) (string, error)
	DefaultThemeID(ctx context.Context) (string, error)
	DisplayNamesOf(ctx context.Context, userIDs []string) (map[string]string, error)
}

// Буквы кода приглашения: без похожих друг на друга — код диктуют голосом.
const (
	codeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	codeLength   = 6
	// ⭐ Столько же попыток, сколько в Java: и на подбор свободного кода, и на посадку.
	// Шестая попытка не спасла бы там, где не помогли пять, — она бы только растянула
	// ответ клиенту, который уже ждёт.
	seatAttempts = 5
	// Прочерк вместо имени того, кого не нашли в users. Em-dash, как в Java.
	unknownDisplayName = "—"
)

// SeatSnapshot — место за столом глазами лобби: строка базы плюс имя игрока.
type SeatSnapshot struct {
	SeatNo      int
	UserID      string
	DisplayName string
	Ready       bool
}

// TableSnapshot — стол вместе с занятыми местами.
type TableSnapshot struct {
	Table repository.GameTable
	Seats []SeatSnapshot
}

// CurrentSnapshot — где игрок сидит сейчас. Table пуст — он нигде не сидит.
type CurrentSnapshot struct {
	Table    *TableSnapshot
	InMatch  bool
	MySeatNo *int
}

// InviteSnapshot — стол глазами того, кто пришёл по ссылке и ещё не вошёл в игру.
//
// ⚠️ Здесь намеренно НЕТ ни имён игроков, ни их идентификаторов — только сам стол и
// сколько мест занято. Код короткий и живёт в переписке, поэтому всё, что попадёт сюда,
// станет доступно любому, кто код увидел.
type InviteSnapshot struct {
	Code       string
	Name       string
	MaxPlayers int
	SeatsTaken int
	IsPrivate  bool
	Joinable   bool
}

// CreateTableCommand — что нужно, чтобы завести стол.
//
// CardSetID и ThemeID пусты — берутся значения по умолчанию.
type CreateTableCommand struct {
	HostUserID  string
	Name        string
	MaxPlayers  int
	CardSetID   string
	ThemeID     string
	RulesConfig string
	IsPrivate   bool
}

// LobbyService — столы: список, создание, просмотр, посадка и закрытие.
//
// ⭐ Главное здесь — посадка за последнее место. Двое могут нажать «сесть» одновременно,
// и проверка «есть ли свободное место» пройдёт у обоих. Поэтому защиты две: уникальный
// индекс (table_id, seat_no) в базе и повтор попытки на нарушении — второй игрок либо
// займёт другое место, либо получит «стол полон».
type LobbyService struct {
	tables TableStore
	now    func() time.Time
	log    *slog.Logger
}

// NewLobbyService собирает сценарии лобби.
func NewLobbyService(tables TableStore, now func() time.Time, log *slog.Logger) LobbyService {
	if now == nil {
		now = time.Now
	}
	return LobbyService{tables: tables, now: now, log: log}
}

// OpenTables — список для лобби: только ожидающие и только не приватные.
func (s LobbyService) OpenTables(ctx context.Context) ([]TableSnapshot, error) {
	tables, err := s.tables.FindOpen(ctx)
	if err != nil {
		return nil, err
	}
	snapshots := make([]TableSnapshot, 0, len(tables))
	for _, table := range tables {
		snapshot, err := s.snapshot(ctx, table)
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, nil
}

// ByID — стол по идентификатору.
func (s LobbyService) ByID(ctx context.Context, tableID string) (TableSnapshot, error) {
	table, err := s.table(ctx, tableID)
	if err != nil {
		return TableSnapshot{}, err
	}
	return s.snapshot(ctx, table)
}

// ByCode — стол по коду приглашения; регистр не важен.
func (s LobbyService) ByCode(ctx context.Context, code string) (TableSnapshot, error) {
	table, err := s.byCode(ctx, code)
	if err != nil {
		return TableSnapshot{}, err
	}
	return s.snapshot(ctx, table)
}

// Invite — что видно по ссылке-приглашению ДО входа в игру.
func (s LobbyService) Invite(ctx context.Context, code string) (InviteSnapshot, error) {
	table, err := s.byCode(ctx, code)
	if err != nil {
		return InviteSnapshot{}, err
	}
	seats, err := s.tables.Seats(ctx, table.ID)
	if err != nil {
		return InviteSnapshot{}, err
	}
	taken := len(seats)
	return InviteSnapshot{
		Code:       table.Code,
		Name:       table.Name,
		MaxPlayers: table.MaxPlayers,
		SeatsTaken: taken,
		IsPrivate:  table.IsPrivate,
		Joinable:   table.Status == repository.TableWaiting && taken < table.MaxPlayers,
	}, nil
}

// Current — стол, за которым игрок сидит сейчас.
//
// ⭐ Нужен, чтобы вернуться после случайного выхода: вкладку закрыли, телефон уснул,
// браузер перезагрузился. Место осталось за игроком, и найти его он должен не глазами
// в общем списке, а по этому ответу.
//
// ⚠️ «Не найдено» здесь не бывает: игрок нигде не сидит — это тоже ответ.
func (s LobbyService) Current(ctx context.Context, userID string) (CurrentSnapshot, error) {
	seat, table, seated, err := s.currentSeat(ctx, userID)
	if err != nil || !seated {
		return CurrentSnapshot{}, err
	}
	snapshot, err := s.snapshot(ctx, table)
	if err != nil {
		return CurrentSnapshot{}, err
	}
	seatNo := seat.SeatNo
	return CurrentSnapshot{
		Table:    &snapshot,
		InMatch:  table.Status == repository.TableInMatch,
		MySeatNo: &seatNo,
	}, nil
}

// Create заводит стол и сажает за него хозяина.
//
// ⚠️ Порядок шагов повторяет Java и важен: значения по умолчанию разбираются ПЕРВЫМИ.
// Иначе ненастроенный набор карт сначала поднимал бы игрока из-за прошлого стола и
// закрывал его, а потом отвечал 500 — игрок остался бы без стола и без нового.
func (s LobbyService) Create(ctx context.Context, cmd CreateTableCommand) (TableSnapshot, error) {
	cardSetID, themeID, err := s.resolveDefaults(ctx, cmd.CardSetID, cmd.ThemeID)
	if err != nil {
		return TableSnapshot{}, err
	}

	// ⚠️ Игрок сидит за одним столом за раз. Без этого каждое нажатие «Создать стол»
	// заводило новый: нетерпеливый двойной клик оставлял в лобби десяток одинаковых
	// столов с одним хозяином, и разгребать их было некому.
	if err := s.releaseSeatBeforeNewTable(ctx, cmd.HostUserID); err != nil {
		return TableSnapshot{}, err
	}

	code, err := s.newCode(ctx)
	if err != nil {
		return TableSnapshot{}, err
	}

	table, err := s.tables.Insert(ctx, repository.GameTable{
		ID:          uuid.NewString(),
		Code:        code,
		Name:        cmd.Name,
		HostUserID:  cmd.HostUserID,
		MaxPlayers:  cmd.MaxPlayers,
		CardSetID:   cardSetID,
		ThemeID:     themeID,
		RulesConfig: cmd.RulesConfig,
		IsPrivate:   cmd.IsPrivate,
	})
	if err != nil {
		return TableSnapshot{}, err
	}

	if _, err := s.Join(ctx, table.ID, cmd.HostUserID); err != nil {
		// ⚠️ Хозяина не удалось посадить — значит стол родился мёртвым: пустой, никому
		// не нужный и видный всем в лобби. Такой убирается сразу, иначе проигравший
		// гонку запрос оставляет мусор ровно там, где его больше всего видно.
		s.closeIfDeserted(ctx, table.ID)
		return TableSnapshot{}, err
	}
	return s.snapshot(ctx, table)
}

// Join сажает игрока за стол. Повторный вызов возвращает то же место.
//
// ⚠️ Транзакции здесь нет намеренно: каждая попытка занять место — отдельная вставка,
// потому что после нарушения уникального индекса Postgres отвечает «current transaction
// is aborted» на любую следующую команду в той же транзакции.
func (s LobbyService) Join(ctx context.Context, tableID, userID string) (repository.TablePlayer, error) {
	table, err := s.table(ctx, tableID)
	if err != nil {
		return repository.TablePlayer{}, err
	}
	if !table.IsOpenForJoin() {
		return repository.TablePlayer{}, ErrTableNotOpen
	}

	seat, err := s.tables.SeatAt(ctx, tableID, userID)
	if err == nil {
		return seat, nil
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return repository.TablePlayer{}, err
	}

	// ⚠️ Проверка нужна не вместо индекса ux_table_players_user, а ради внятного ответа:
	// без неё вставка падала бы на уникальности, цикл исчерпывал попытки и врал
	// «нет свободных мест» про пустой только что созданный стол.
	if err := s.refuseIfSeatedElsewhere(ctx, userID, tableID); err != nil {
		return repository.TablePlayer{}, err
	}

	for attempt := 0; attempt < seatAttempts; attempt++ {
		seats, err := s.tables.Seats(ctx, tableID)
		if err != nil {
			return repository.TablePlayer{}, err
		}
		seatNo := firstFreeSeatFor(seats, table.MaxPlayers)
		if seatNo < 0 {
			return repository.TablePlayer{}, ErrTableFull
		}

		seat, err := s.tables.InsertSeat(ctx, repository.TablePlayer{
			TableID: tableID, UserID: userID, SeatNo: seatNo, State: repository.SeatJoined,
		})
		switch {
		case err == nil:
			return seat, nil
		case errors.Is(err, repository.ErrSeatTaken):
			// Место увели между выбором и вставкой — считаем расклад заново.
			continue
		case errors.Is(err, repository.ErrAlreadySeated):
			// Тот же игрок сел параллельным запросом. Посадка идемпотентна: отдаём место.
			return s.tables.SeatAt(ctx, tableID, userID)
		case errors.Is(err, repository.ErrSeatedElsewhere):
			// ⚠️ Увели не место, а самого игрока за другой стол. Пересчитывать нечего:
			// сколько ни пробуй, второе место ему не положено.
			if refusal := s.refuseIfSeatedElsewhere(ctx, userID, tableID); refusal != nil {
				return repository.TablePlayer{}, refusal
			}
			return repository.TablePlayer{}, AlreadyAtTableError{}
		default:
			return repository.TablePlayer{}, err
		}
	}
	return repository.TablePlayer{}, ErrTableFull
}

// Leave освобождает место.
//
// ⚠️ Посреди матча уйти нельзя: место тут же занял бы посторонний, а движок продолжал бы
// ждать ушедшего. Пропавший игрок ставит матч на паузу — это единственный способ его
// покинуть.
func (s LobbyService) Leave(ctx context.Context, tableID, userID string) error {
	table, err := s.table(ctx, tableID)
	if err != nil {
		return err
	}
	if _, err := s.tables.SeatAt(ctx, tableID, userID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			// Его за этим столом и нет — цель достигнута.
			return nil
		}
		return err
	}
	if table.Status == repository.TableInMatch {
		return MatchInProgressError{Message: "Посреди матча из-за стола не встают"}
	}
	_, err = s.tables.DeleteSeat(ctx, tableID, userID)
	return err
}

// Close закрывает стол. Места при этом остаются — стол просто перестаёт быть текущим.
func (s LobbyService) Close(ctx context.Context, tableID, userID string) error {
	table, err := s.table(ctx, tableID)
	if err != nil {
		return err
	}
	if !table.IsHost(userID) {
		return ErrNotTableHost
	}
	if table.Status == repository.TableInMatch {
		return MatchInProgressError{Message: "Нельзя закрыть стол посреди матча"}
	}
	return s.tables.Close(ctx, tableID, s.now())
}

// resolveDefaults подставляет набор карт и тему, если их не выбрали.
func (s LobbyService) resolveDefaults(ctx context.Context, cardSetID, themeID string) (string, string, error) {
	if cardSetID == "" {
		id, err := s.tables.DefaultCardSetID(ctx)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return "", "", ErrNoDefaultCardSet
			}
			return "", "", err
		}
		cardSetID = id
	}
	if themeID == "" {
		id, err := s.tables.DefaultThemeID(ctx)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return "", "", ErrNoDefaultTheme
			}
			return "", "", err
		}
		themeID = id
	}
	return cardSetID, themeID, nil
}

// releaseSeatBeforeNewTable поднимает игрока из-за прошлого стола.
//
// Стол, за которым не осталось никого, закрывается: пустой стол в списке — это
// приглашение, за которым никого нет.
//
// ⚠️ Посреди матча новый стол не создаётся вовсе — по той же причине, по которой из-за
// стола не встают: движок продолжал бы ждать ушедшего.
func (s LobbyService) releaseSeatBeforeNewTable(ctx context.Context, userID string) error {
	_, previous, seated, err := s.currentSeat(ctx, userID)
	if err != nil || !seated {
		return err
	}
	if previous.Status == repository.TableInMatch {
		return MatchInProgressError{Message: "Сначала доиграй за текущим столом"}
	}
	// ⚠️ Освобождение места — работа «по возможности», а не гарантия: строку мог удалить
	// параллельный запрос того же игрока, и тогда цель достигнута чужими руками. Настоящую
	// гарантию «один стол на игрока» держит уникальный индекс, а не этот метод.
	if err := s.Leave(ctx, previous.ID, userID); err != nil {
		if errors.Is(err, ErrTableNotFound) {
			return nil
		}
		return err
	}
	s.closeIfDeserted(ctx, previous.ID)
	return nil
}

// closeIfDeserted закрывает стол без единого игрока — он больше никому не нужен.
//
// Ошибка здесь не отменяет основное действие: игрок уже встал, а недозакрытый пустой стол
// — беспорядок в списке, но не поломка.
func (s LobbyService) closeIfDeserted(ctx context.Context, tableID string) {
	seats, err := s.tables.Seats(ctx, tableID)
	if err != nil || len(seats) > 0 {
		return
	}
	if err := s.tables.Close(ctx, tableID, s.now()); err != nil && s.log != nil {
		s.log.Warn("пустой стол не закрылся", "table", tableID, "error", err)
	}
}

// currentSeat — место игрока и стол, за которым он сидит.
//
// ⚠️ Расхождение с Java, осознанное. Java берёт строку table_players и отбрасывает её,
// если стол закрыт, — но саму строку не удаляет. А DELETE /api/tables/{id} места не
// чистит, поэтому после закрытия стола за игроком навсегда остаётся строка, которую он
// не видит и не может убрать: любая следующая посадка падает на ux_table_players_user,
// и в Java это выглядит как «за столом нет свободных мест» про пустой новый стол.
// Здесь протухшая строка удаляется на месте — иначе игрок запирается насовсем.
func (s LobbyService) currentSeat(ctx context.Context, userID string) (
	repository.TablePlayer, repository.GameTable, bool, error) {

	seat, err := s.tables.SeatOf(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return repository.TablePlayer{}, repository.GameTable{}, false, nil
		}
		return repository.TablePlayer{}, repository.GameTable{}, false, err
	}

	table, err := s.tables.FindByID(ctx, seat.TableID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			// Стола нет вовсе — строка тем более протухла.
			_, _ = s.tables.DeleteSeat(ctx, seat.TableID, userID)
			return repository.TablePlayer{}, repository.GameTable{}, false, nil
		}
		return repository.TablePlayer{}, repository.GameTable{}, false, err
	}
	if table.Status == repository.TableClosed {
		if _, err := s.tables.DeleteSeat(ctx, seat.TableID, userID); err != nil {
			return repository.TablePlayer{}, repository.GameTable{}, false, err
		}
		return repository.TablePlayer{}, repository.GameTable{}, false, nil
	}
	return seat, table, true, nil
}

// refuseIfSeatedElsewhere отказывает, если игрок сидит за другим столом.
func (s LobbyService) refuseIfSeatedElsewhere(ctx context.Context, userID, tableID string) error {
	_, seated, ok, err := s.currentSeat(ctx, userID)
	if err != nil || !ok || seated.ID == tableID {
		return err
	}
	return AlreadyAtTableError{TableName: seated.Name}
}

func (s LobbyService) table(ctx context.Context, tableID string) (repository.GameTable, error) {
	table, err := s.tables.FindByID(ctx, tableID)
	if errors.Is(err, repository.ErrNotFound) {
		return repository.GameTable{}, ErrTableNotFound
	}
	return table, err
}

func (s LobbyService) byCode(ctx context.Context, code string) (repository.GameTable, error) {
	table, err := s.tables.FindByCode(ctx, code)
	if errors.Is(err, repository.ErrNotFound) {
		return repository.GameTable{}, ErrTableNotFound
	}
	return table, err
}

// snapshot собирает стол с местами и именами игроков.
func (s LobbyService) snapshot(ctx context.Context, table repository.GameTable) (TableSnapshot, error) {
	seats, err := s.tables.Seats(ctx, table.ID)
	if err != nil {
		return TableSnapshot{}, err
	}
	userIDs := make([]string, 0, len(seats))
	for _, seat := range seats {
		userIDs = append(userIDs, seat.UserID)
	}
	names, err := s.tables.DisplayNamesOf(ctx, userIDs)
	if err != nil {
		return TableSnapshot{}, err
	}

	view := make([]SeatSnapshot, 0, len(seats))
	for _, seat := range seats {
		name, found := names[seat.UserID]
		if !found {
			// ⚠️ Прочерк, а не отказ: пропавшее имя не повод не показать стол.
			name = unknownDisplayName
		}
		view = append(view, SeatSnapshot{
			SeatNo:      seat.SeatNo,
			UserID:      seat.UserID,
			DisplayName: name,
			Ready:       seat.IsReady(),
		})
	}
	return TableSnapshot{Table: table, Seats: view}, nil
}

// newCode подбирает свободный код приглашения.
func (s LobbyService) newCode(ctx context.Context) (string, error) {
	for attempt := 0; attempt < seatAttempts; attempt++ {
		code, err := randomCode()
		if err != nil {
			return "", err
		}
		taken, err := s.tables.ExistsByCode(ctx, code)
		if err != nil {
			return "", err
		}
		if !taken {
			return code, nil
		}
	}
	return "", errors.New("не удалось подобрать свободный код стола")
}

// randomCode — шесть букв из алфавита без похожих друг на друга символов.
//
// ⭐ Источник — crypto/rand, как SecureRandom в Java: код приглашения угадывать не
// должны, а предсказуемый генератор превращает приватный стол в общедоступный.
// Длина алфавита 32 — степень двойки, поэтому маска даёт равномерность без отбраковки.
func randomCode() (string, error) {
	buf := make([]byte, codeLength)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("код стола: %w", err)
	}
	code := make([]byte, codeLength)
	for i, b := range buf {
		code[i] = codeAlphabet[b&31]
	}
	return string(code), nil
}

// firstFreeSeatFor — минимальный свободный номер места или -1.
//
// Нумерация с нуля и порядок по часовой стрелке фиксируются на весь матч, поэтому
// «первое свободное» — это именно минимальное, а не любое.
func firstFreeSeatFor(seats []repository.TablePlayer, maxPlayers int) int {
	for seatNo := 0; seatNo < maxPlayers; seatNo++ {
		free := true
		for _, seat := range seats {
			if seat.SeatNo == seatNo {
				free = false
				break
			}
		}
		if free {
			return seatNo
		}
	}
	return -1
}
