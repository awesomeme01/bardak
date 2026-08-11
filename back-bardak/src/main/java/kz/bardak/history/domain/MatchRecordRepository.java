package kz.bardak.history.domain;

import java.util.List;
import java.util.UUID;
import org.springframework.data.jpa.repository.JpaRepository;

public interface MatchRecordRepository extends JpaRepository<MatchRecord, UUID> {

    List<MatchRecord> findByTableIdOrderByStartedAtDesc(UUID tableId);

    List<MatchRecord> findByIdInOrderByStartedAtDesc(List<UUID> ids);
}
