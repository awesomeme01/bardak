package kz.bardak.lobby.domain;

import java.util.List;
import java.util.Optional;
import java.util.UUID;
import org.springframework.data.jpa.repository.JpaRepository;

public interface CardSetRepository extends JpaRepository<CardSet, UUID> {

    List<CardSet> findAllByOrderByNameAsc();

    Optional<CardSet> findByIsDefaultTrue();
}
