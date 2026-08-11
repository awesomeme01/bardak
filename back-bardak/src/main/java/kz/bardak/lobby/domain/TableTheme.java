package kz.bardak.lobby.domain;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.Id;
import jakarta.persistence.Table;
import java.util.UUID;

/** Тема стола: фон, цвет сукна, рубашка по умолчанию. */
@Entity
@Table(name = "table_themes")
public class TableTheme {

    @Id
    private UUID id;

    @Column(nullable = false, unique = true)
    private String code;

    @Column(nullable = false)
    private String name;

    @Column(name = "background_url")
    private String backgroundUrl;

    @Column(name = "felt_color")
    private String feltColor;

    @Column(name = "default_back_code")
    private String defaultBackCode;

    @Column(name = "is_default", nullable = false)
    private boolean isDefault;

    protected TableTheme() {
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

    public String feltColor() {
        return feltColor;
    }

    public String defaultBackCode() {
        return defaultBackCode;
    }

    public boolean isDefault() {
        return isDefault;
    }
}
