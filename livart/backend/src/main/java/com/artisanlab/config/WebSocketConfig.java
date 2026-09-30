package com.artisanlab.config;

import com.artisanlab.ai.ImageJobWebSocketHandler;
import org.springframework.context.annotation.Configuration;
import org.springframework.beans.factory.annotation.Value;
import java.util.Arrays;
import org.springframework.web.socket.config.annotation.EnableWebSocket;
import org.springframework.web.socket.config.annotation.WebSocketConfigurer;
import org.springframework.web.socket.config.annotation.WebSocketHandlerRegistry;

@Configuration
@EnableWebSocket
public class WebSocketConfig implements WebSocketConfigurer {
    private final ImageJobWebSocketHandler imageJobWebSocketHandler;
    private final String[] allowedOrigins;

    public WebSocketConfig(ImageJobWebSocketHandler imageJobWebSocketHandler,
            @Value("${CORS_ALLOWED_ORIGINS:http://localhost:5173,http://localhost:8080}") String allowedOrigins) {
        this.imageJobWebSocketHandler = imageJobWebSocketHandler;
        this.allowedOrigins = Arrays.stream(allowedOrigins.split(",")).map(String::trim).filter(s -> !s.isEmpty()).toArray(String[]::new);
    }

    @Override
    public void registerWebSocketHandlers(WebSocketHandlerRegistry registry) {
        registry.addHandler(imageJobWebSocketHandler, "/ws/image-jobs")
                .setAllowedOrigins(allowedOrigins);
    }
}
