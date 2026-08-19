package kz.bardak.push.domain;

import java.util.List;
import java.util.Optional;
import java.util.UUID;
import org.springframework.data.jpa.repository.JpaRepository;

public interface PushSubscriptionRepository extends JpaRepository<PushSubscription, UUID> {

    List<PushSubscription> findByUserId(UUID userId);

    Optional<PushSubscription> findByEndpoint(String endpoint);

    void deleteByEndpoint(String endpoint);

    /**
     * Удалить подписку, принадлежащую именно этому пользователю.
     *
     * <p>⚠️ Раньше отписка шла только по {@code endpoint}, без владельца: любой вошедший,
     * знающий чужой адрес подписки, отписывал чужое устройство от уведомлений.
     */
    int deleteByEndpointAndUserId(String endpoint, UUID userId);
}
