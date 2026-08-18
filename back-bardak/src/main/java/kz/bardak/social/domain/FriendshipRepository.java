package kz.bardak.social.domain;

import java.util.List;
import java.util.Optional;
import java.util.UUID;
import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.data.jpa.repository.Query;
import org.springframework.data.repository.query.Param;

public interface FriendshipRepository extends JpaRepository<Friendship, Friendship.Key> {

    /** Все пары, где участвует игрок, — и дружбы, и висящие заявки. */
    @Query("select f from Friendship f where f.lowUserId = :userId or f.highUserId = :userId")
    List<Friendship> findAllInvolving(@Param("userId") UUID userId);

    /** ⚠️ Порядок пары считается тем же способом, что и при записи — см. Friendship. */
    default Optional<Friendship> findPair(final UUID one, final UUID two) {
        final boolean oneIsLower = Friendship.comparePairOrder(one, two) < 0;
        return findById(new Friendship.Key(oneIsLower ? one : two, oneIsLower ? two : one));
    }
}
