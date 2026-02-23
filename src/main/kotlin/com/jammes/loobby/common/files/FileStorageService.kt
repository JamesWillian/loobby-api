package com.jammes.loobby.common.files

import org.springframework.stereotype.Service
import org.springframework.web.multipart.MultipartFile
import java.nio.file.Files
import java.nio.file.Path
import java.nio.file.Paths
import java.time.Instant
import java.util.UUID
import kotlin.io.path.exists

@Service
class FileStorageService(
    private val props: FileStorageProperties
) {

    private val rootLocation: Path = Paths.get(props.location)

    init {
        if (!rootLocation.exists()) {
            Files.createDirectories(rootLocation)
        }
    }

    /**
     * Salva um arquivo e retorna a URL pública.
     *
     * @param subfolder ex: "avatars", "groups"
     * @param ownerId   ex: userId ou groupId (pra organizar)
     */
    fun store(
        file: MultipartFile,
        subfolder: String,
        ownerId: UUID,
        allowedContentTypes: List<String> = listOf("image/jpeg", "image/png", "image/webp")
    ): String {
        if (file.isEmpty) {
            throw IllegalArgumentException("Empty file")
        }

        if (!allowedContentTypes.contains(file.contentType)) {
            throw IllegalArgumentException("Invalid file type: ${file.contentType}")
        }

        val extension = when (file.contentType) {
            "image/jpeg" -> "jpg"
            "image/png" -> "png"
            "image/webp" -> "webp"
            else -> "bin"
        }

        // nome do arquivo: <timestamp>.<ext>
        val filename = "${Instant.now().toEpochMilli()}.$extension"

        val dir = rootLocation.resolve(subfolder).resolve(ownerId.toString())
        if (!dir.exists()) {
            Files.createDirectories(dir)
        }

        val destination = dir.resolve(filename)

        file.inputStream.use { input ->
            Files.copy(input, destination)
        }

        // URL pública: <publicBaseUrl>/<subfolder>/<ownerId>/<filename>
        return "${props.publicBaseUrl}/$subfolder/$ownerId/$filename"
    }
}