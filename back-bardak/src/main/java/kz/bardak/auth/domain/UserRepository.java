package kz.bardak.auth.domain;

import java.util.Optional;
import java.util.UUID;
import org.springframework.data.jpa.repository.JpaRepository;

/**
 * ⚠️ Логин ищется <b>без учёта регистра</b>.
 *
 * <p>Регистрозависимый вход выглядел как «пароль не подходит»: телефонная клавиатура сама
 * ставит заглавную первую букву, {@code shabdan} превращается в {@code Shabdan}, и запрос
 * не находит никого. Человек при этом уверен, что ввёл всё верно — и он прав.
 *
 * <p>Записан логин по-прежнему так, как его набрали: показывать «Shabdan» вместо «shabdan»
 * значило бы поправить человека там, где его никто не просил.
 */
public interface UserRepository extends JpaRepository<User, UUID> {

    Optional<User> findByUsernameIgnoreCase(String username);

    boolean existsByUsernameIgnoreCase(String username);
}
