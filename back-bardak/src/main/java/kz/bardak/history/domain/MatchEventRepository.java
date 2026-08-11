package kz.bardak.history.domain;

import java.util.List;
import java.util.UUID;
import org.springframework.data.jpa.repository.JpaRepository;

public interface MatchEventRepository extends JpaRepository<MatchEventRecord, Long> {

    List<MatchEventRecord> findByMatchIdOrderBySeqAsc(UUID matchId);

    List<MatchEventRecord> findByMatchIdAndSeqGreaterThanOrderBySeqAsc(UUID matchId, int seq);
}
