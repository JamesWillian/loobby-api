package app.loobby

import org.springframework.boot.autoconfigure.SpringBootApplication
import org.springframework.boot.runApplication

@SpringBootApplication
class LoobbyApplication

fun main(args: Array<String>) {
	runApplication<app.loobby.LoobbyApplication>(*args)
}
