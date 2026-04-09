package app.loobby.common.files

import org.springframework.boot.context.properties.ConfigurationProperties
import org.springframework.context.annotation.Configuration

@Configuration
@ConfigurationProperties(prefix = "file.storage")
class FileStorageProperties {
    lateinit var location: String
    lateinit var publicBaseUrl: String
}