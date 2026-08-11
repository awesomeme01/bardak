package kz.bardak.history.domain;

import java.util.List;
import java.util.UUID;
import org.springframework.data.jpa.repository.JpaRepository;

public interface DealRepository extends JpaRepository<DealRecord, UUID> {

    List<DealRecord> findByMatchIdOrderByDealNo(UUID matchId);

    boolean existsByMatchIdAndDealNo(UUID matchId, short dealNo);
}
