plugins {
	kotlin("jvm") version "2.3.10"
	kotlin("plugin.spring") version "2.3.10"
    kotlin("plugin.jpa") version "2.1.20"
	id("org.springframework.boot") version "4.1.0-SNAPSHOT"
	id("io.spring.dependency-management") version "1.1.7"
}

group = "com.jammes"
version = "0.0.1-SNAPSHOT"
description = "Demo project for Spring Boot"

java {
	toolchain {
		languageVersion = JavaLanguageVersion.of(25)
	}
}

repositories {
	mavenCentral()
	maven { url = uri("https://repo.spring.io/snapshot") }
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

    implementation("org.flywaydb:flyway-core")

    implementation("com.fasterxml.jackson.module:jackson-module-kotlin")
	implementation("org.jetbrains.kotlin:kotlin-reflect")

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
