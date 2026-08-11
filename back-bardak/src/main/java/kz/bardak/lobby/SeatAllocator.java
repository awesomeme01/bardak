package kz.bardak.lobby;

import java.util.List;
import java.util.Objects;
import java.util.UUID;
import kz.bardak.lobby.domain.TablePlayer;
import kz.bardak.lobby.domain.TablePlayerRepository;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Propagation;
import org.springframework.transaction.annotation.Transactional;

/**
 * Одна попытка занять место.
 *
 * <p>⭐ Вынесено в отдельный бин ради {@link Propagation#REQUIRES_NEW}: при гонке вставка
 * нарушает уникальный индекс {@code (table_id, seat_no)}, и транзакция после этого
 * непригодна — в Postgres любая следующая команда в ней получает
 * «current transaction is aborted». Повторять попытку можно только в новой транзакции,
 * поэтому цикл живёт снаружи, а каждая попытка — здесь.
 */
@Service
public class SeatAllocator {

    private final TablePlayerRepository players;

    public SeatAllocator(final TablePlayerRepository players) {
        this.players = Objects.requireNonNull(players, "players");
    }

    /**
     * Занять минимальное свободное место.
     *
     * @return занятое место или пусто, если свободных нет
     * @throws org.springframework.dao.DataIntegrityViolationException место увели между
     *                                                                выбором и вставкой
     */
    @Transactional(propagation = Propagation.REQUIRES_NEW)
    public TablePlayer allocate(final UUID tableId, final UUID userId, final int maxPlayers) {
        final List<TablePlayer> seated = players.findByTableIdOrderBySeatNo(tableId);
        final int seatNo = firstFreeSeat(seated, maxPlayers);
        if (seatNo < 0) {
            return null;
        }
        return players.saveAndFlush(new TablePlayer(tableId, userId, seatNo));
    }

    private int firstFreeSeat(final List<TablePlayer> seated, final int maxPlayers) {
        for (int seatNo = 0; seatNo < maxPlayers; seatNo++) {
            final int candidate = seatNo;
            if (seated.stream().noneMatch(seat -> seat.seatNo() == candidate)) {
                return seatNo;
            }
        }
        return -1;
    }
}
