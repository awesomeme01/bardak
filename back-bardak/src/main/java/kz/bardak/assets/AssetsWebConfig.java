package kz.bardak.assets;

import org.springframework.beans.factory.annotation.Value;
import org.springframework.context.annotation.Configuration;
import org.springframework.web.servlet.config.annotation.ResourceHandlerRegistry;
import org.springframework.web.servlet.config.annotation.WebMvcConfigurer;

/**
 * Раздача ассетов наборов карт и тем стола: {@code /assets/card-sets/<набор>/<карта>.png}.
 *
 * <p>Путь к папке — параметр, а сам домен про ассеты ничего не знает о файловой системе
 * (см. planning/06-card-design-system.md): позже здесь появится {@code AssetStorage}
 * с реализацией поверх S3/MinIO, и URL-ы в БД менять не придётся.
 */
@Configuration
public class AssetsWebConfig implements WebMvcConfigurer {

    private final String assetsPath;
    private final long cacheSeconds;

    public AssetsWebConfig(@Value("${bardak.assets.path}") String assetsPath,
                           @Value("${bardak.assets.cache-seconds}") long cacheSeconds) {
        this.assetsPath = assetsPath;
        this.cacheSeconds = cacheSeconds;
    }

    @Override
    public void addResourceHandlers(ResourceHandlerRegistry registry) {
        registry.addResourceHandler("/assets/**")
                .addResourceLocations("file:" + assetsPath)
                .setCachePeriod((int) cacheSeconds);
    }
}
