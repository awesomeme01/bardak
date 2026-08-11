package kz.bardak.lobby.domain;

import java.util.List;
import java.util.Optional;
import java.util.UUID;
import org.springframework.data.jpa.repository.JpaRepository;

public interface TablePlayerRepository extends JpaRepository<TablePlayer, TablePlayer.Key> {

    List<TablePlayer> findByTableIdOrderBySeatNo(UUID tableId);

    Optional<TablePlayer> findByTableIdAndUserId(UUID tableId, UUID userId);
}
