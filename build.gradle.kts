plugins {
	kotlin("jvm") version "2.3.10"
	kotlin("plugin.spring") version "2.3.10"
    kotlin("plugin.jpa") version "2.3.10"
	id("org.springframework.boot") version "3.5.6"
	id("io.spring.dependency-management") version "1.1.7"
}

group = "app.loobby"
version = "0.0.1-SNAPSHOT"
description = "Demo project for Spring Boot"

repositories {
	mavenCentral()
}

dependencies {
	implementation("org.springframework.boot:spring-boot-starter")
    implementation("org.springframework.boot:spring-boot-starter-web")
    implementation("org.springframework.boot:spring-boot-starter-validation")
    implementation("org.springframework.boot:spring-boot-starter-actuator")

    implementation("org.springframework.boot:spring-boot-starter-data-jpa")
    runtimeOnly("org.postgresql:postgresql")

    implementation("org.springframework.boot:spring-boot-starter-security")
    implementation("org.springframework.boot:spring-boot-starter-oauth2-resource-server") // JWT bearer parsing/validation
    implementation("org.springframework.security:spring-security-oauth2-jose") // Nimbus

    implementation("org.hibernate.orm:hibernate-core")
    implementation("org.flywaydb:flyway-core")

    implementation("com.fasterxml.jackson.module:jackson-module-kotlin")
	implementation("org.jetbrains.kotlin:kotlin-reflect")

	implementation("com.google.api-client:google-api-client:2.7.0")

	// Push notifications — Firebase Admin SDK (FCM)
	implementation("com.google.firebase:firebase-admin:9.3.0")

	// Normalização e validação de telefones (E.164) — usado no fluxo de RSVP via link público
	implementation("com.googlecode.libphonenumber:libphonenumber:8.13.50")

	// Minificação de JavaScript ES6+ servido pelo backend (PublicWebController).
	// Roda uma vez no boot (lazy), resultado cacheado em memória.
	implementation("com.google.javascript:closure-compiler:v20240317")

	testImplementation("org.springframework.boot:spring-boot-starter-test")
	testImplementation("org.jetbrains.kotlin:kotlin-test-junit5")
	testRuntimeOnly("org.junit.platform:junit-platform-launcher")
}

kotlin {
	compilerOptions {
		freeCompilerArgs.addAll("-Xjsr305=strict", "-Xannotation-default-target=param-property")
	}
}

tasks.withType<Test> {
	useJUnitPlatform()
}
