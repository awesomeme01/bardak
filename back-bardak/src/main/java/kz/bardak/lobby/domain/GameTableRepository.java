package kz.bardak.lobby.domain;

import java.util.List;
import java.util.Optional;
import java.util.UUID;
import org.springframework.data.jpa.repository.JpaRepository;

public interface GameTableRepository extends JpaRepository<GameTable, UUID> {

    Optional<GameTable> findByCode(String code);

    /** Открытые столы для лобби. Приватные в общий список не попадают — только по коду. */
    List<GameTable> findByStatusAndIsPrivateFalseOrderByCreatedAtDesc(TableStatus status);

    boolean existsByCode(String code);
}
