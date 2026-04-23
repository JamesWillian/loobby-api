package app.loobby.notifications.config

import org.springframework.context.annotation.Bean
import org.springframework.context.annotation.Configuration
import org.springframework.scheduling.TaskScheduler
import org.springframework.scheduling.annotation.EnableScheduling
import org.springframework.scheduling.concurrent.ThreadPoolTaskScheduler

/**
 * Habilita @Scheduled em toda a aplicação e provê um ThreadPoolTaskScheduler
 * nomeado `notificationsTaskScheduler`, usado tanto pelos jobs agendados
 * (@Scheduled) quanto pelo [app.loobby.notifications.service.PeerRsvpDebouncer].
 */
@Configuration
@EnableScheduling
class NotificationSchedulerConfig {

    @Bean
    fun notificationsTaskScheduler(): TaskScheduler {
        val scheduler = ThreadPoolTaskScheduler()
        scheduler.poolSize = 4
        scheduler.setThreadNamePrefix("notif-sched-")
        scheduler.setWaitForTasksToCompleteOnShutdown(true)
        scheduler.setAwaitTerminationSeconds(30)
        scheduler.initialize()
        return scheduler
    }
}
