package kz.bardak.rating.domain;

import java.util.List;
import java.util.Optional;
import java.util.UUID;
import org.springframework.data.jpa.repository.JpaRepository;

public interface SeasonRepository extends JpaRepository<Season, UUID> {

    Optional<Season> findFirstByClosedAtIsNull();

    List<Season> findAllByOrderByStartedAtDesc();
}
