package kz.bardak.lobby.api;

import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.UUID;
import kz.bardak.common.web.ApiException;
import kz.bardak.lobby.domain.CardAsset;
import kz.bardak.lobby.domain.CardAssetRepository;
import kz.bardak.lobby.domain.CardSet;
import kz.bardak.lobby.domain.CardSetRepository;
import kz.bardak.lobby.domain.TableTheme;
import kz.bardak.lobby.domain.TableThemeRepository;
import org.springframework.http.HttpStatus;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

/**
 * Каталог наборов карт и тем стола.
 *
 * <p>Манифест — единственное, что связывает код карты с картинкой (ADR-009). Ни движок,
 * ни протокол игры про URL не знают.
 */
@RestController
@RequestMapping("/api")
public class CardSetController {

    private final CardSetRepository cardSets;
    private final CardAssetRepository assets;
    private final TableThemeRepository themes;

    public CardSetController(final CardSetRepository cardSets, final CardAssetRepository assets,
                             final TableThemeRepository themes) {
        this.cardSets = Objects.requireNonNull(cardSets, "cardSets");
        this.assets = Objects.requireNonNull(assets, "assets");
        this.themes = Objects.requireNonNull(themes, "themes");
    }

    @GetMapping("/card-sets")
    public List<LobbyDtos.CardSetView> cardSets() {
        return cardSets.findAllByOrderByNameAsc().stream().map(CardSetController::toView).toList();
    }

    @GetMapping("/card-sets/{id}/manifest")
    public LobbyDtos.CardSetManifest manifest(@PathVariable final UUID id) {
        final CardSet set = cardSets.findById(id)
                .orElseThrow(() -> new ApiException(HttpStatus.NOT_FOUND, "CARD_SET_NOT_FOUND",
                        "Набор карт не найден"));
        final Map<String, String> cards = new LinkedHashMap<>();
        for (final CardAsset asset : assets.findByCardSetIdOrderByOrdinalAsc(id)) {
            cards.put(asset.cardCode(), asset.assetUrl());
        }
        return new LobbyDtos.CardSetManifest(set.id().toString(), set.code(), set.version(), cards);
    }

    @GetMapping("/table-themes")
    public List<LobbyDtos.TableThemeView> themes() {
        return themes.findAllByOrderByNameAsc().stream().map(CardSetController::toView).toList();
    }

    private static LobbyDtos.CardSetView toView(final CardSet set) {
        return new LobbyDtos.CardSetView(set.id().toString(), set.code(), set.name(),
                set.description(), set.version(), set.previewUrl(), set.isDefault());
    }

    private static LobbyDtos.TableThemeView toView(final TableTheme theme) {
        return new LobbyDtos.TableThemeView(theme.id().toString(), theme.code(), theme.name(),
                theme.feltColor(), theme.defaultBackCode(), theme.isDefault());
    }
}
