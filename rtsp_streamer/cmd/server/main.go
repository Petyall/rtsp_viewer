package main

import (
	"log"
	"rtsp_streamer/config"
	"rtsp_streamer/database"
	"rtsp_streamer/routes"

	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
	// Загружаем конфигурацию
	err := config.LoadEnv()
	if err != nil {
		log.Fatalf("Ошибка загрузки файла .env: %v", err)
	}

	// Устанавливаем соединение с базой данных
	db, err := database.Connect()
	if err != nil {
		log.Fatalf("Ошибка подключения к базе данных: %v", err)
	}
	defer db.Close()

	// Проверяем соединение
	err = db.Ping()
	if err != nil {
		log.Fatalf("Не удалось подключиться к базе данных: %v", err)
	}
	log.Println("Успешное подключение к базе данных")

	// Создаём новый роутер Gin
	r := gin.Default()

	// Настраиваем CORS
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:3000", "http://localhost:8000"},
		AllowMethods:     []string{"GET", "POST", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour, // Кешируем CORS-настройки на 12 часов
	}))

	// Настраиваем маршруты
	routes.RegisterRoutes(r, db)

	// Запускаем сервер
	log.Fatal(r.Run("localhost:8080"))
}
