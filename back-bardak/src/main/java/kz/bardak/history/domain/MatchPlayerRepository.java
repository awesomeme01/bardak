package kz.bardak.history.domain;

import java.util.List;
import java.util.UUID;
import org.springframework.data.jpa.repository.JpaRepository;

public interface MatchPlayerRepository
        extends JpaRepository<MatchPlayerRecord, MatchPlayerRecord.Key> {

    List<MatchPlayerRecord> findByMatchIdOrderBySeatNo(UUID matchId);

    List<MatchPlayerRecord> findByUserId(UUID userId);
}
