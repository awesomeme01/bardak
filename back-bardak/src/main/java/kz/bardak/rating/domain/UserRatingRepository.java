package kz.bardak.rating.domain;

import java.util.List;
import java.util.UUID;
import org.springframework.data.jpa.repository.JpaRepository;

public interface UserRatingRepository extends JpaRepository<UserRating, UUID> {

    List<UserRating> findAllByOrderByRatingDesc();
}
