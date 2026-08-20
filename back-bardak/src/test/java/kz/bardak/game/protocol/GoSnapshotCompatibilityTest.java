package kz.bardak.game.protocol;

import static org.assertj.core.api.Assertions.assertThat;
import static org.junit.jupiter.api.Assumptions.assumeTrue;

import com.fasterxml.jackson.databind.ObjectMapper;
import java.nio.file.Files;
import java.nio.file.Path;
import kz.bardak.game.rules.MatchState;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;

/**
 * Снимок, записанный Go-версией, читается Java.
 *
 * <p>⭐ Обратное направление совместимости данных. Прямое (Java пишет — Go читает)
 * проверяется на стороне Go; без этого теста окно отката было бы односторонним:
 * переключились на Go, а вернуться уже нельзя, потому что её снимки Java не понимает.
 *
 * <p>⚠️ Файл подаётся переменной {@code BARDAK_GO_SNAPSHOT} и в репозитории не лежит:
 * это снимок конкретного матча, а не фикстура. Без переменной проверка пропускается,
 * а не притворяется пройденной.
 */
class GoSnapshotCompatibilityTest {

    private final MatchStateCodec codec = new MatchStateCodec(new ObjectMapper());

    @DisplayName("Should read the snapshot When it was written by the Go backend")
    @Test
    void shouldReadTheSnapshotWhenItWasWrittenByTheGoBackend() throws Exception {
        final String path = System.getenv("BARDAK_GO_SNAPSHOT");
        assumeTrue(path != null && !path.isBlank(), "BARDAK_GO_SNAPSHOT не задан");

        final String json = Files.readString(Path.of(path));

        final MatchState state = codec.decode(json);

        assertThat(state.deal().players())
                .as("в снимке Go нет игроков — разошёлся формат")
                .isNotEmpty();
        assertThat(state.navesLevels())
                .as("уровни навесов — это счёт матча, потерять их нельзя")
                .isNotEmpty();

        // ⭐ И круговой прогон: Java перезаписывает снимок Go и читает его снова.
        // Если формат совместим, состояние переживает оба кодека.
        final MatchState again = codec.decode(codec.encode(state));
        assertThat(again.dealNo()).isEqualTo(state.dealNo());
        assertThat(again.deal().players()).hasSameSizeAs(state.deal().players());
        assertThat(again.results()).hasSameSizeAs(state.results());
    }
}
