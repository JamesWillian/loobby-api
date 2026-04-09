package app.loobby.common.email

import org.springframework.beans.factory.annotation.Value
import org.springframework.stereotype.Service
import java.net.URI
import java.net.http.HttpClient
import java.net.http.HttpRequest
import java.net.http.HttpResponse

@Service
class EmailService(
    @Value("\${resend.api-key}") private val apiKey: String,
    @Value("\${resend.from-email}") private val fromEmail: String
) {
    private val client = HttpClient.newHttpClient()

    fun send(to: String, subject: String, html: String) {
        // Escape aspas duplas no HTML para não quebrar o JSON
        val escapedHtml = html
            .replace("\\", "\\\\")
            .replace("\"", "\\\"")
            .replace("\n", "\\n")
            .replace("\r", "\\r")

        val body = """
            {
                "from": "$fromEmail",
                "to": ["$to"],
                "subject": "$subject",
                "html": "$escapedHtml"
            }
        """.trimIndent()

        val request = HttpRequest.newBuilder()
            .uri(URI.create("https://api.resend.com/emails"))
            .header("Authorization", "Bearer $apiKey")
            .header("Content-Type", "application/json")
            .POST(HttpRequest.BodyPublishers.ofString(body))
            .build()

        val response = client.send(request, HttpResponse.BodyHandlers.ofString())
        if (response.statusCode() !in 200..299) {
            throw RuntimeException("Falha ao enviar email para $to: ${response.body()}")
        }
    }
}