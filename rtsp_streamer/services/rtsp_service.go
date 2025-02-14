package services

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

type streamInfo struct {
	process *exec.Cmd
	viewers map[string]bool // Уникальные ID зрителей
	mux     sync.Mutex      // Защита данных о процессе и зрителях
}

var (
	streams           = make(map[string]*streamInfo)
	mux               sync.RWMutex
	userStreamHistory = make(map[string]map[string]time.Time) // История потоков
	historyMux        sync.Mutex
)

func StartFFMPEG(cameraID int, streamURL, viewerID string) error {
	// Получаем текущую историю пользователя
	userHistory := GetUserStreamHistory(viewerID)
	if len(userHistory) >= 4 {
		// Если пользователь уже смотрит 4 камеры, удаляем самую старую
		var oldestCameraID string
		var oldestTime time.Time

		for camID, startTime := range userHistory {
			if oldestCameraID == "" || startTime.Before(oldestTime) {
				oldestCameraID = camID
				oldestTime = startTime
			}
		}

		if oldestCameraID != "" {
			oldestCamIDInt, err := strconv.Atoi(oldestCameraID)
			if err != nil {
				log.Printf("StartFFMPEG - Ошибка преобразования ID камеры %s в int: %v", oldestCameraID, err)
				return fmt.Errorf("ошибка преобразования ID камеры %s в int: %v", oldestCameraID, err)
			}

			log.Printf("StartFFMPEG - Удаление самого старого потока камеры %s для пользователя %s\n", oldestCameraID, viewerID)
			if err := StopFFMPEG(oldestCamIDInt, viewerID); err != nil {
				log.Printf("StartFFMPEG - Ошибка остановки самого старого потока камеры %s: %v", oldestCameraID, err)
				return fmt.Errorf("ошибка остановки потока камеры %s: %v", oldestCameraID, err)
			}
		}
	}

	// Запускаем новый поток
	mux.Lock()

	info, exists := streams[strconv.Itoa(cameraID)]
	if exists {
		info.mux.Lock()
		defer info.mux.Unlock()

		// Проверяем, запущен ли процесс
		if info.process != nil && info.process.ProcessState == nil {
			if _, alreadyViewing := info.viewers[viewerID]; !alreadyViewing {
				info.viewers[viewerID] = true
				log.Printf("StartFFMPEG - Пользователь %s присоединился к просмотру RTSP потока %d\n", viewerID, cameraID)

				// Обновляем историю просмотров
				updateStreamHistory(viewerID, strconv.Itoa(cameraID))
			} else {
				log.Printf("StartFFMPEG - Пользователь %s уже подключен к RTSP потоку %d\n", viewerID, cameraID)
			}
			mux.Unlock()
			return nil
		}

		// Процесс завершился или неактивен
		log.Printf("StartFFMPEG - Перезапуск трансляции RTSP потока камеры %d\n", cameraID)
		info.viewers = make(map[string]bool) // Очистка списка зрителей
	}

	log.Printf("StartFFMPEG - Начало трансляции RTSP потока камеры %d\n", cameraID)

	dirPath := filepath.Join("streams", "camera_"+strconv.Itoa(cameraID))
	if err := os.MkdirAll(dirPath, os.ModePerm); err != nil {
		log.Printf("StartFFMPEG - Ошибка при работе с директориями %v", err)
		mux.Unlock()
		return err
	}

	cmd := exec.Command("ffmpeg", "-i", streamURL, "-c:v", "libx264", "-preset",
		"ultrafast", "-b:v", "500k", "-s", "1280x720", "-f", "hls", "-hls_time",
		"2", "-hls_list_size", "10", "-hls_flags", "delete_segments",
		"./streams/camera_"+strconv.Itoa(cameraID)+"/index.m3u8")

	if err := cmd.Start(); err != nil {
		log.Printf("StartFFMPEG - Ошибка при запуске потока %v", err)
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

	// Обновляем историю просмотров
	updateStreamHistory(viewerID, strconv.Itoa(cameraID))

	mux.Unlock()

	filePath := "./streams/camera_" + strconv.Itoa(cameraID) + "/index.m3u8"
	log.Printf("StartFFMPEG - Начало ожидания создания файла ./streams/camera_%s/index.m3u8", strconv.Itoa(cameraID))
	for i := 0; i < 30; i++ {
		log.Printf("StartFFMPEG - Для файла ./streams/camera_%s/index.m3u8 прошло %d секунд", strconv.Itoa(cameraID), i)
		if _, err := os.Stat(filePath); err == nil {
			break
		}
		time.Sleep(time.Second)
	}

	if _, err := os.Stat(filePath); err != nil {
		log.Printf("StartFFMPEG - Для файла ./streams/camera_%s/index.m3u8 превышено время ожидания", strconv.Itoa(cameraID))
		if err := StopFFMPEG(cameraID, viewerID); err != nil {
			log.Printf("StartFFMPEG - Не удалось отключить пользователя %s от камеры %d: %v", viewerID, cameraID, err)
			return fmt.Errorf("ошибка отключения зрителя %s от камеры %d: %v", viewerID, cameraID, err)
		}
		return fmt.Errorf("время ожидания для файла %s превышено", filePath)
	}

	return nil
}

func StopFFMPEG(cameraID int, viewerID string) error {
	mux.Lock()
	defer mux.Unlock()

	info, exists := streams[strconv.Itoa(cameraID)]
	if !exists {
		log.Printf("StopFFMPEG - Камера %d не активна", cameraID)
		return fmt.Errorf("камера %d не активна", cameraID)
	}

	info.mux.Lock()
	defer info.mux.Unlock()

	// Удаляем зрителя
	if _, viewerExists := info.viewers[viewerID]; viewerExists {
		delete(info.viewers, viewerID)

		// Удаляем запись из истории
		removeStreamHistory(viewerID, strconv.Itoa(cameraID))

		log.Printf("StopFFMPEG - Пользователь %s отключился от камеры %d\n", viewerID, cameraID)
	} else {
		log.Printf("StopFFMPEG - Пользователь %s не найден в списке зрителей для камеры %d", viewerID, cameraID)
		return fmt.Errorf("пользователь %s не найден в списке для камеры %d", viewerID, cameraID)
	}

	// Проверяем количество оставшихся зрителей
	if len(info.viewers) == 0 {
		log.Printf("StopFFMPEG - Остановка трансляции RTSP потока для камеры %d (нет зрителей)\n", cameraID)
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
		log.Printf("StopFFMPEG - Количество зрителей камеры %d уменьшено до %d\n", cameraID, len(info.viewers))
	}

	return nil
}

// Функция для обновления истории
func updateStreamHistory(userID, cameraID string) {
	historyMux.Lock()
	defer historyMux.Unlock()

	if _, exists := userStreamHistory[userID]; !exists {
		userStreamHistory[userID] = make(map[string]time.Time)
	}
	userStreamHistory[userID][cameraID] = time.Now()
}

// Функция для удаления истории
func removeStreamHistory(userID, cameraID string) {
	historyMux.Lock()
	defer historyMux.Unlock()

	if userCameras, exists := userStreamHistory[userID]; exists {
		delete(userCameras, cameraID)
		if len(userCameras) == 0 {
			delete(userStreamHistory, userID)
		}
	}
}

// Функция для получения истории пользователя
func GetUserStreamHistory(userID string) map[string]time.Time {
	historyMux.Lock()
	defer historyMux.Unlock()

	if history, exists := userStreamHistory[userID]; exists {
		return history
	}
	return nil
}

// func printLogs() {
// 	ticker := time.NewTicker(time.Second)
// 	go func() {
// 		for range ticker.C {
// 			mux.Lock()
// 			log.Println("???????????????", GetUserStreamHistory("8ef9f2af-a1c1-4009-9306-118174b3e96f"))
// 			log.Println("<<<<<<<<<<<<<<<", streams)
// 			log.Println(">>>>>>>>>>>>>>>", userStreamHistory)
// 			mux.Unlock()
// 		}
// 	}()
// }

// func init() {
// 	printLogs()
// }
