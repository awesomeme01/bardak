package kz.bardak.lobby.domain;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.Id;
import jakarta.persistence.Table;
import java.util.UUID;

/**
 * Набор дизайна карт. Движок о наборах не знает вообще (ADR-009): он оперирует кодами
 * карт, а картинку по коду находит клиент через манифест.
 */
@Entity
@Table(name = "card_sets")
public class CardSet {

    @Id
    private UUID id;

    @Column(nullable = false, unique = true)
    private String code;

    @Column(nullable = false)
    private String name;

    private String description;

    @Column(nullable = false)
    private String version;

    @Column(name = "preview_url")
    private String previewUrl;

    @Column(name = "is_default", nullable = false)
    private boolean isDefault;

    protected CardSet() {
        // для JPA
    }

    public UUID id() {
        return id;
    }

    public String code() {
        return code;
    }

    public String name() {
        return name;
    }

    public String description() {
        return description;
    }

    public String version() {
        return version;
    }

    public String previewUrl() {
        return previewUrl;
    }

    public boolean isDefault() {
        return isDefault;
    }
}
