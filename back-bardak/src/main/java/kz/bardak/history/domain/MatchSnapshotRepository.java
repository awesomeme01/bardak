package kz.bardak.history.domain;

import java.util.Optional;
import java.util.UUID;
import org.springframework.data.jpa.repository.JpaRepository;

public interface MatchSnapshotRepository
        extends JpaRepository<MatchSnapshotRecord, MatchSnapshotRecord.Key> {

    Optional<MatchSnapshotRecord> findFirstByMatchIdOrderBySeqDesc(UUID matchId);
}
