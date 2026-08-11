package kz.bardak.game.runtime;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import java.util.ArrayList;
import java.util.List;
import java.util.Objects;
import kz.bardak.game.rules.NavesScale;
import kz.bardak.game.rules.Rank;
import kz.bardak.game.rules.RulesConfig;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

/**
 * Разбор {@code rules_config} стола в конфиг движка (ADR-016).
 *
 * <p>Всё, чего в конфиге нет, берётся из умолчаний: стол, созданный до появления нового
 * параметра, обязан продолжать играть.
 */
public final class RulesConfigCodec {

    private static final Logger log = LoggerFactory.getLogger(RulesConfigCodec.class);

    private final ObjectMapper objectMapper;

    public RulesConfigCodec(final ObjectMapper objectMapper) {
        this.objectMapper = Objects.requireNonNull(objectMapper, "objectMapper");
    }

    public RulesConfig parse(final String json) {
        final RulesConfig defaults = RulesConfig.defaults();
        if (json == null || json.isBlank()) {
            return defaults;
        }
        try {
            final JsonNode node = objectMapper.readTree(json);
            final JsonNode naves = node.path("naves");
            return new RulesConfig(
                    node.path("dealSize").asInt(defaults.dealSize()),
                    node.path("maxAttackFirstRound").asInt(defaults.maxAttackFirstRound()),
                    node.path("maxAttackPerRound").asInt(defaults.maxAttackPerRound()),
                    node.path("transfersEnabled").asBoolean(defaults.transfersEnabled()),
                    node.path("jokersEnabled").asBoolean(defaults.jokersEnabled()),
                    naves.path("enabled").asBoolean(defaults.navesEnabled()),
                    scaleOf(naves.path("scale"), defaults.navesScale()));
        } catch (final RuntimeException | com.fasterxml.jackson.core.JsonProcessingException e) {
            log.warn("Не разобрал rules_config стола, играем по умолчанию: {}", e.toString());
            return defaults;
        }
    }

    private NavesScale scaleOf(final JsonNode scale, final NavesScale defaults) {
        if (!scale.isArray() || scale.isEmpty()) {
            return defaults;
        }
        final List<Rank> ranks = new ArrayList<>();
        for (final JsonNode element : scale) {
            ranks.add(rankOf(element.asText()));
        }
        return new NavesScale(ranks);
    }

    private Rank rankOf(final String code) {
        for (final Rank rank : Rank.values()) {
            if (rank.code().equals(code)) {
                return rank;
            }
        }
        throw new IllegalArgumentException("Неизвестный ранг в шкале навесов: " + code);
    }
}
