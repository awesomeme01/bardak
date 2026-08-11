package kz.bardak.history.domain;

import java.util.List;
import java.util.UUID;
import org.springframework.data.jpa.repository.JpaRepository;

public interface DealResultRepository extends JpaRepository<DealResultRecord, DealResultRecord.Key> {

    List<DealResultRecord> findByDealIdOrderBySeatNo(UUID dealId);
}
