package kz.bardak.lobby.domain;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.Id;
import jakarta.persistence.Table;
import java.util.UUID;

/** Одна картинка набора: код карты → URL. Из этих строк собирается манифест. */
@Entity
@Table(name = "card_assets")
public class CardAsset {

    @Id
    private UUID id;

    @Column(name = "card_set_id", nullable = false)
    private UUID cardSetId;

    @Column(name = "card_code", nullable = false)
    private String cardCode;

    @Column(name = "asset_url", nullable = false)
    private String assetUrl;

    @Column(nullable = false)
    private String mime;

    @Column(nullable = false)
    private short ordinal;

    protected CardAsset() {
        // для JPA
    }

    public UUID cardSetId() {
        return cardSetId;
    }

    public String cardCode() {
        return cardCode;
    }

    public String assetUrl() {
        return assetUrl;
    }

    public String mime() {
        return mime;
    }
}
