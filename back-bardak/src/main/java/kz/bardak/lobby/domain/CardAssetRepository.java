package kz.bardak.lobby.domain;

import java.util.List;
import java.util.UUID;
import org.springframework.data.jpa.repository.JpaRepository;

public interface CardAssetRepository extends JpaRepository<CardAsset, UUID> {

    List<CardAsset> findByCardSetIdOrderByOrdinalAsc(UUID cardSetId);
}
