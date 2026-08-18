package kz.bardak.social.api;

import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.Size;
import java.util.List;

/**
 * Формы экрана друзей.
 *
 * @param online       в сети прямо сейчас — считается по живому сокету, а не по «был недавно»
 * @param mine         заявку отправил я: по этому флагу экран решает, звать или отвечать
 */
public final class FriendDtos {

    private FriendDtos() {
    }

    public record Friend(String userId, String username, String displayName, String avatar,
                         boolean online, String status, boolean mine) {
    }

    /**
     * @param friends  принятые
     * @param incoming заявки ко мне — на них отвечают
     * @param outgoing мои заявки — их ждут
     */
    public record FriendList(List<Friend> friends, List<Friend> incoming, List<Friend> outgoing) {
    }

    public record AddFriendRequest(@NotBlank @Size(max = 32) String username) {
    }

    public record InviteRequest(@NotBlank String tableId) {
    }
}
