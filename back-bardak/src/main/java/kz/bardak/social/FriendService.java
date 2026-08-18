package kz.bardak.social;

import java.time.Clock;
import java.util.ArrayList;
import java.util.Comparator;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.Optional;
import java.util.UUID;
import java.util.stream.Collectors;
import kz.bardak.auth.domain.User;
import kz.bardak.auth.domain.UserRepository;
import kz.bardak.common.web.ApiException;
import kz.bardak.social.api.FriendDtos;
import kz.bardak.social.domain.Friendship;
import kz.bardak.social.domain.FriendshipRepository;
import org.springframework.http.HttpStatus;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

/**
 * Друзья: заявки, список и приглашение за стол.
 *
 * <p>⭐ Дружба взаимна и потому требует согласия. Односторонний «подписался» здесь не
 * подходит: друг видит, что ты в сети, и зовёт за стол — это доступ к присутствию, а его
 * не выдают без спроса.
 */
@Service
public class FriendService {

    private final FriendshipRepository friendships;
    private final UserRepository users;
    private final Presence presence;
    private final TableInvites invites;
    private final Clock clock;

    public FriendService(final FriendshipRepository friendships, final UserRepository users,
                         final Presence presence, final TableInvites invites, final Clock clock) {
        this.friendships = Objects.requireNonNull(friendships, "friendships");
        this.users = Objects.requireNonNull(users, "users");
        this.presence = Objects.requireNonNull(presence, "presence");
        this.invites = Objects.requireNonNull(invites, "invites");
        this.clock = Objects.requireNonNull(clock, "clock");
    }

    /**
     * Позвать в друзья по логину.
     *
     * <p>⚠️ Повторная заявка тому, кто уже позвал тебя сам, — это согласие, а не вторая
     * заявка. Иначе двое, нажавшие «добавить» одновременно, остались бы с двумя висящими
     * приглашениями и без дружбы.
     */
    @Transactional
    public FriendDtos.Friend request(final UUID userId, final String username) {
        final User target = users.findByUsernameIgnoreCase(username.trim())
                .orElseThrow(() -> new ApiException(HttpStatus.NOT_FOUND, "USER_NOT_FOUND",
                        "Игрока с таким логином нет"));
        if (target.id().equals(userId)) {
            throw new ApiException(HttpStatus.CONFLICT, "CANNOT_FRIEND_SELF",
                    "С самим собой дружить не получится");
        }

        final Friendship existing = friendships.findPair(userId, target.id()).orElse(null);
        if (existing != null) {
            if (existing.isAccepted()) {
                throw new ApiException(HttpStatus.CONFLICT, "ALREADY_FRIENDS",
                        "Вы уже друзья");
            }
            if (existing.canBeAcceptedBy(userId)) {
                existing.accept(clock.instant());
                return toDto(friendships.save(existing), userId, target);
            }
            throw new ApiException(HttpStatus.CONFLICT, "REQUEST_ALREADY_SENT",
                    "Заявка уже отправлена — ждём ответа");
        }
        return toDto(friendships.save(Friendship.requested(userId, target.id())), userId, target);
    }

    /** Принять заявку. Принимает только тот, кому её прислали. */
    @Transactional
    public FriendDtos.Friend accept(final UUID userId, final UUID friendId) {
        final Friendship pair = pairOrFail(userId, friendId);
        if (pair.isAccepted()) {
            return toDto(pair, userId, userOrFail(friendId));
        }
        if (!pair.canBeAcceptedBy(userId)) {
            throw new ApiException(HttpStatus.CONFLICT, "NOT_YOUR_REQUEST",
                    "Эту заявку принимаешь не ты");
        }
        pair.accept(clock.instant());
        return toDto(friendships.save(pair), userId, userOrFail(friendId));
    }

    /**
     * Убрать из друзей или отклонить заявку — одно и то же действие.
     *
     * <p>Отдельного «отклонить» нет намеренно: отказ и разрыв означают одно — пары больше
     * нет. Хранить отказы значило бы помнить, кому уже отказали, а это память не про игру.
     */
    @Transactional
    public void remove(final UUID userId, final UUID friendId) {
        friendships.delete(pairOrFail(userId, friendId));
    }

    /**
     * Список друзей и заявок, разложенный по смыслу.
     *
     * <p>⭐ Онлайн считается одним запросом к присутствию, а не по одному на друга: список
     * читается целиком и часто.
     */
    @Transactional(readOnly = true)
    public FriendDtos.FriendList list(final UUID userId) {
        final List<Friendship> pairs = friendships.findAllInvolving(userId);
        final Map<UUID, User> others = users.findAllById(pairs.stream()
                        .map(pair -> pair.otherThan(userId))
                        .toList()).stream()
                .collect(Collectors.toMap(User::id, user -> user));

        final List<FriendDtos.Friend> friends = new ArrayList<>();
        final List<FriendDtos.Friend> incoming = new ArrayList<>();
        final List<FriendDtos.Friend> outgoing = new ArrayList<>();
        for (final Friendship pair : pairs) {
            final User other = others.get(pair.otherThan(userId));
            if (other == null) {
                continue;
            }
            final FriendDtos.Friend dto = toDto(pair, userId, other);
            if (pair.isAccepted()) {
                friends.add(dto);
            } else if (pair.requestedBy().equals(userId)) {
                outgoing.add(dto);
            } else {
                incoming.add(dto);
            }
        }
        // Онлайн — наверх: за стол зовут тех, кто сейчас здесь.
        friends.sort(Comparator.comparing(FriendDtos.Friend::online).reversed()
                .thenComparing(FriendDtos.Friend::displayName, String.CASE_INSENSITIVE_ORDER));
        return new FriendDtos.FriendList(friends, incoming, outgoing);
    }

    /** Друзья, которым можно слать приглашение: только принятые. */
    @Transactional(readOnly = true)
    public boolean isFriend(final UUID userId, final UUID otherId) {
        return friendships.findPair(userId, otherId).filter(Friendship::isAccepted).isPresent();
    }

    /**
     * Позвать друга за стол.
     *
     * <p>⚠️ Звать можно только друга — иначе приглашение становится способом писать
     * незнакомым людям. Проверка здесь, а не на экране: экран показывает лишь друзей,
     * но запрос приходит из сети, а не с экрана.
     *
     * @return дошло ли приглашение прямо сейчас; нет — друг не в сети
     */
    @Transactional(readOnly = true)
    public boolean invite(final UUID userId, final UUID friendId, final UUID tableId,
                          final String tableName, final String tableCode) {
        if (!isFriend(userId, friendId)) {
            throw new ApiException(HttpStatus.FORBIDDEN, "NOT_FRIENDS",
                    "Звать за стол можно только друзей");
        }
        final User me = userOrFail(userId);
        return invites.send(friendId, me.displayName(), tableId, tableName, tableCode);
    }

    private Friendship pairOrFail(final UUID userId, final UUID friendId) {
        return friendships.findPair(userId, friendId)
                .filter(pair -> pair.involves(userId))
                .orElseThrow(() -> new ApiException(HttpStatus.NOT_FOUND, "NOT_FRIENDS",
                        "Такой пары нет"));
    }

    private User userOrFail(final UUID userId) {
        return users.findById(userId)
                .orElseThrow(() -> new ApiException(HttpStatus.NOT_FOUND, "USER_NOT_FOUND",
                        "Игрок не найден"));
    }

    private FriendDtos.Friend toDto(final Friendship pair, final UUID viewer, final User other) {
        return new FriendDtos.Friend(other.id().toString(), other.username(), other.displayName(),
                other.avatar(), presence.isOnline(other.id()), pair.status().name(),
                pair.requestedBy().equals(viewer));
    }

    /** Пользователь по логину — нужен экрану поиска до отправки заявки. */
    @Transactional(readOnly = true)
    public Optional<User> byUsername(final String username) {
        return users.findByUsernameIgnoreCase(username.trim());
    }
}
