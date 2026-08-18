package kz.bardak.social.domain;

/** Состояние пары: заявка ждёт ответа или дружба состоялась. Отказ строку удаляет. */
public enum FriendshipStatus {

    PENDING,
    ACCEPTED
}
