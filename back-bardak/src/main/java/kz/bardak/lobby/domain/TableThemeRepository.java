package kz.bardak.lobby.domain;

import java.util.List;
import java.util.Optional;
import java.util.UUID;
import org.springframework.data.jpa.repository.JpaRepository;

public interface TableThemeRepository extends JpaRepository<TableTheme, UUID> {

    List<TableTheme> findAllByOrderByNameAsc();

    Optional<TableTheme> findByIsDefaultTrue();
}
