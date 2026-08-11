package kz.bardak.rating.domain;

import java.util.List;
import java.util.UUID;
import org.springframework.data.jpa.repository.JpaRepository;

public interface RatingHistoryRepository extends JpaRepository<RatingHistoryEntry, Long> {

    List<RatingHistoryEntry> findByUserIdOrderByCreatedAtDesc(UUID userId);

    boolean existsByMatchId(UUID matchId);
}
