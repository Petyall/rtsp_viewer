package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"gorm.io/gorm"

	"database/sql"

	_ "github.com/lib/pq"
)

type streamInfo struct {
	process *exec.Cmd
	viewers map[string]bool // Уникальные ID зрителей
	mux     sync.Mutex      // Защита данных о процессе и зрителях
}

var Database *gorm.DB

var (
	streams       = make(map[string]*streamInfo)
	mux           sync.RWMutex
	activeViewers = make(map[string]int) // viewerID -> cameraID
)

func startFFMPEG(cameraID int, streamURL, viewerID string) error {
	mux.Lock()

	// Проверяем, смотрит ли пользователь другой поток
	if oldCameraID, watching := activeViewers[viewerID]; watching && oldCameraID != cameraID {
		fmt.Printf("Зритель %s уже смотрит поток камеры %d, отключаем...\n", viewerID, oldCameraID)
		mux.Unlock() // Освобождаем мьютекс для вызова stopFFMPEG
		if err := stopFFMPEG(oldCameraID, viewerID); err != nil {
			return fmt.Errorf("не удалось отключить зрителя %s от камеры %d: %v", viewerID, oldCameraID, err)
		}
		mux.Lock() // Возвращаем блокировку для продолжения
	}

	info, exists := streams[strconv.Itoa(cameraID)]
	if exists {
		info.mux.Lock()
		defer info.mux.Unlock()

		// Проверяем, запущен ли процесс
		if info.process != nil && info.process.ProcessState == nil {
			if _, alreadyViewing := info.viewers[viewerID]; !alreadyViewing {
				info.viewers[viewerID] = true
				fmt.Printf("Зритель %s присоединился к просмотру RTSP потока %d\n", viewerID, cameraID)
				activeViewers[viewerID] = cameraID
			} else {
				fmt.Printf("Зритель %s уже подключен к RTSP потоку %d\n", viewerID, cameraID)
			}
			mux.Unlock()
			return nil
		}

		// Процесс завершился или неактивен
		fmt.Printf("Перезапуск трансляции RTSP потока камеры %d\n", cameraID)
		info.viewers = make(map[string]bool) // Очистка списка зрителей
	}

	fmt.Printf("Начало трансляции RTSP потока камеры %d\n", cameraID)

	dirPath := filepath.Join("streams", "camera_"+strconv.Itoa(cameraID))
	if err := os.MkdirAll(dirPath, os.ModePerm); err != nil {
		mux.Unlock()
		return err
	}

	cmd := exec.Command("ffmpeg", "-i", streamURL, "-c:v", "libx264", "-preset",
		"ultrafast", "-b:v", "500k", "-s", "640x360", "-f", "hls", "-hls_time",
		"2", "-hls_list_size", "10", "-hls_flags", "delete_segments",
		"./streams/camera_"+strconv.Itoa(cameraID)+"/index.m3u8")

	if err := cmd.Start(); err != nil {
		mux.Unlock()
		return err
	}

	if !exists {
		streams[strconv.Itoa(cameraID)] = &streamInfo{
			process: cmd,
			viewers: map[string]bool{viewerID: true},
		}
	} else {
		info.process = cmd
		info.viewers = map[string]bool{viewerID: true}
	}

	activeViewers[viewerID] = cameraID
	mux.Unlock()

	filePath := "./streams/camera_" + strconv.Itoa(cameraID) + "/index.m3u8"
	for i := 0; i < 30; i++ {
		if _, err := os.Stat(filePath); err == nil {
			break
		}
		time.Sleep(time.Second)
	}

	if _, err := os.Stat(filePath); err != nil {
		return fmt.Errorf("время ожидания для файла %s превышено", filePath)
	}

	return nil
}

func stopFFMPEG(cameraID int, viewerID string) error {
	mux.Lock()
	defer mux.Unlock()

	info, exists := streams[strconv.Itoa(cameraID)]
	if !exists {
		return fmt.Errorf("камера %d не активна", cameraID)
	}

	info.mux.Lock()
	defer info.mux.Unlock()

	// Удаляем зрителя
	if _, viewerExists := info.viewers[viewerID]; viewerExists {
		delete(info.viewers, viewerID)
		delete(activeViewers, viewerID)
		fmt.Printf("Зритель %s отключился от камеры %d\n", viewerID, cameraID)
	} else {
		return fmt.Errorf("зритель %s не найден в списке для камеры %d", viewerID, cameraID)
	}

	// Проверяем количество оставшихся зрителей
	if len(info.viewers) == 0 {
		fmt.Printf("Остановка трансляции RTSP потока для камеры %d (нет зрителей)\n", cameraID)
		if err := info.process.Process.Kill(); err != nil {
			return err
		}
		delete(streams, strconv.Itoa(cameraID))

		// Удаляем временные файлы трансляции
		err := os.RemoveAll(filepath.Join("streams", "camera_"+strconv.Itoa(cameraID)))
		if err != nil {
			return err
		}
	} else {
		fmt.Printf("Количество зрителей камеры %d уменьшено до %d\n", cameraID, len(info.viewers))
	}

	return nil
}

type Camera struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	StreamURL string `json:"streamurl"`
	Location  string `json:"location"`
}

func get_camera(id int, db *sql.DB) (Camera, error) {
	query := "SELECT id, name, stream_url, location FROM cameras WHERE id = $1"

	row := db.QueryRow(query, id)

	var camera Camera

	err := row.Scan(&camera.ID, &camera.Name, &camera.StreamURL, &camera.Location)
	if err != nil {
		return Camera{}, err
	}

	return camera, nil
}

func check_user_camera(camera_id int, user_id string, db *sql.DB) (bool, error) {
	query := "SELECT camera_id FROM user_cameras WHERE camera_id = $1 AND user_id = $2"

	row := db.QueryRow(query, camera_id, user_id)

	err := row.Scan(&camera_id)
	if err != nil {
		return false, err
	}

	return true, nil
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatalf("Ошибка загрузки файла .env: %v", err)
	}

	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbUser := os.Getenv("DB_USER")
	dbPassword := os.Getenv("DB_PASS")
	dbName := os.Getenv("DB_NAME")
	dbSSLMode := os.Getenv("DB_SSLMODE")

	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		dbHost, dbPort, dbUser, dbPassword, dbName, dbSSLMode)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatalf("Ошибка подключения к базе данных: %v", err)
	}
	defer db.Close()

	err = db.Ping()
	if err != nil {
		log.Fatalf("Не удалось подключиться к базе данных: %v", err)
	}
	fmt.Println("Успешное подключение к базе данных")

	r := gin.Default()

	r.StaticFile("/", "./index.html")
	r.StaticFS("/streams", http.Dir("./streams"))

	r.POST("/start/:cameraID", AuthMiddleware(), func(c *gin.Context) {
		userID, _ := c.Get("user_id")
		fmt.Println(">>>>>>>>>>>>>>>>", userID)

		cameraIDStr := c.Param("cameraID")
		cameraID, err := strconv.Atoi(cameraIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный ID камеры"})
			return
		}
		fmt.Println(">>>>>>>>>>>>>>>>", cameraID)
		user_check, err := check_user_camera(cameraID, userID.(string), db)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Ошибка при проверке пользователя"})
			return
		}
		fmt.Println(">>>>>>>>>>>>>>>>", user_check)
		if !user_check {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Недостаточно прав"})
			return
		}

		camera, err := get_camera(cameraID, db)
		if err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusNotFound, gin.H{"error": "Камера не найдена"})
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при получении камеры"})
			}
			return
		}

		stream_url, err := DecryptStreamURL(camera.StreamURL, EncryptionKey)
		if err != nil {
			log.Fatalf("Ошибка при дешифровании ссылки на rtsp поток %v", err)
		}
		if err := startFFMPEG(camera.ID, stream_url, userID.(string)); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Запущена трансляция RTSP потока камеры " + camera.Location})
	})

	r.POST("/stop/:cameraID", AuthMiddleware(), func(c *gin.Context) {
		userID, _ := c.Get("user_id")
		fmt.Println("?????????", userID)
		cameraIDStr := c.Param("cameraID")
		cameraID, err := strconv.Atoi(cameraIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный ID камеры"})
			return
		}
		if err := stopFFMPEG(cameraID, userID.(string)); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Остановлена трансляция RTSP потока камеры " + strconv.Itoa(cameraID)})
	})

	r.Run("localhost:8080")
}
