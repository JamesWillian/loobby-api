package app.loobby.config

import app.loobby.common.files.FileStorageProperties
import org.springframework.context.annotation.Configuration
import org.springframework.web.servlet.config.annotation.ResourceHandlerRegistry
import org.springframework.web.servlet.config.annotation.WebMvcConfigurer
import java.nio.file.Paths

@Configuration
class WebConfig(
    private val fileProps: FileStorageProperties
) : WebMvcConfigurer {

    override fun addResourceHandlers(registry: ResourceHandlerRegistry) {
        val uploadPath = Paths.get(fileProps.location).toUri().toString()

        registry.addResourceHandler("/files/**")
            .addResourceLocations(uploadPath)
            .setCachePeriod(3600)  // cache 1h
    }
}