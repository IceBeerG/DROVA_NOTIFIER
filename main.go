package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/mitchellh/go-ps"
	"github.com/shirou/gopsutil/process"
)

var (
	startTimeApp, stopTimeApp, remoteAddr, localAddr, game string
	kernel32                                               = syscall.NewLazyDLL("kernel32.dll")
	procSetConsoleTitleW                                   = kernel32.NewProc("SetConsoleTitleW")
	startTime, stopTime                                    time.Time
)

const (
	appName    = "ese.exe"             // Имя запускаемого файла
	newTitle   = "Drova Notifier"      // Имя окна программы
	timeFormat = "02.01.2006 15:04:05" // Формат времени для записи в CSV файл
	remoutPort = "7990"                // порт для поиска IP подключившегося
	localPort  = "7989"                // порт для поиска IP станции
)

func main() {

	logFilePath := "errors.log" // Имя файла для логирования ошибок
	logFilePath = filepath.Join(filepath.Dir(os.Args[0]), logFilePath)

	// Открываем файл для записи логов
	logFile, err := os.OpenFile(logFilePath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		log.Fatal("Ошибка открытия файла", err, getLine())
	}
	defer logFile.Close()

	// Устанавливаем файл в качестве вывода для логгера
	log.SetOutput(logFile)

	// Получаем текущую директорию программы
	dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		log.Fatal("Ошибка получения текущей деректории: ", err, getLine())
	}

	setConsoleTitle(newTitle) // Устанавливаем новое имя окна

	// Указываем относительный путь к файлу
	gamelist := filepath.Join(dir, "games.txt")  // Файл со списком игр
	botlist := filepath.Join(dir, "telebot.txt") // Файл с BotToken и CHAT_ID

	// Получаем CHAT_ID и BotToken из файла telebot.txt
	CHAT_IDstr, _ := setFromFile("id", botlist)
	CHAT_ID, _ := strconv.ParseInt(CHAT_IDstr, 10, 64)
	BotToken, _ := setFromFile("token", botlist)

	isRunning := checkIfProcessRunning(appName)
	go listenPort(remoutPort)
	go listenPort(localPort)

	// Получаем имя ПК
	hostname, err := os.Hostname()
	if err != nil {
		log.Println("Ошибка при получении имени компьютера: ", err, getLine())
		fmt.Println("Ошибка при получении имени компьютера: ", err)
		return
	}
	fmt.Printf("Hostname - %s\n\n", hostname)

	for {
		//ждем запуска приложения ese.exe
		for i := 0; i != 2; {
			// Ожидаем 5 секунд перед проверкой
			time.Sleep(5 * time.Second)
			if isRunning {
				startTime = time.Now() // переменная для вычисления продолжительности сессии
				startTimeApp = time.Now().Format(timeFormat)
				fmt.Println("Время запуска - ", startTimeApp)
				i = 2 //т.к. приложение запущено, выходим из цикла
			}
			// Проверяем, запущено ли приложение в следующей итерации
			isRunning = checkIfProcessRunning(appName)

		}
		for i := 0; i < 12; {
			time.Sleep(10 * time.Second)
			processes, err := ps.Processes()
			if err != nil {
				log.Println("Ошибка при получении списка процессов: ", err, getLine())
				fmt.Println("Ошибка при получении списка процессов: ", err)
				return
			}

			for _, process := range processes {
				val := strings.Replace(process.Executable(), ".exe", "", -1)
				gameN, err := setFromFile(val, gamelist)
				if err != nil {
					log.Println("Ошибка получения данных из файла ", gamelist, "- ", err, getLine())
					fmt.Println(err)
				}
				if gameN != "" {
					i = 11
					game = gameN
				}
			}
			i++
		}
		if game == "" {
			game = TopLoad()
		}
		gamerIP := remoteAddr
		serverIP := localAddr

		// Создаем сообщение
		chatMessage := "Имя ПК = " + hostname + "\nНачало сессии - " + startTimeApp + "\nИгра - " + game + "\nserverIP = " + serverIP + "\nGamerIP - " + gamerIP

		fmt.Println(chatMessage)
		fmt.Println()

		// Отправляем сообщение через бота о начале сессии
		err := SendMessage(BotToken, CHAT_ID, chatMessage)
		if err != nil {
			log.Fatal("Ошибка отправки сообщения: ", err, getLine())
		}

		// time.Sleep(100 * time.Second)

		for i := 0; i != 3; {
			isRunning = checkIfProcessRunning(appName)
			if !isRunning {
				stopTime = time.Now()
				i = 3
			}

			time.Sleep(5 * time.Second)
		}

		// startTimeApp = startTime.Format(timeFormat)
		stopTimeApp = stopTime.Format(timeFormat)
		duration := stopTime.Sub(startTime).Round(time.Second)
		hours := int(duration.Hours())
		minutes := int(duration.Minutes()) % 60
		seconds := int(duration.Seconds()) % 60
		hou := strconv.Itoa(hours)
		sessionDur := ""
		if hours < 2 {
			sessionDur = sessionDur + "0" + hou + ":"
		} else {
			sessionDur = sessionDur + hou + ":"
		}
		min := strconv.Itoa(minutes)
		if minutes < 10 {
			sessionDur = sessionDur + "0" + min + ":"
		} else {
			sessionDur = sessionDur + min + ":"
		}
		sec := strconv.Itoa(seconds)
		if seconds < 10 {
			sessionDur = sessionDur + "0" + sec
		} else {
			sessionDur = sessionDur + sec
		}
		fmt.Println("sessionDur - ", sessionDur)
		// sessionDur := hou + ":" + min + ":" + sec
		fmt.Printf("Сессия завершена:\n%s;\n%s;\n%s;\n%s;\n%s;\n%s\n", startTimeApp, stopTimeApp, sessionDur, hostname, serverIP, gamerIP)
		chatMessage = "\nСессия на " + hostname + " завершена\nВремя сессии - " + sessionDur + "\nИгра - " + game + "\nGamerIP - " + gamerIP
		// Отправляем сообщение через бота об окончании сессии
		err = SendMessage(BotToken, CHAT_ID, chatMessage)
		if err != nil {
			log.Fatal("Ошибка отправки сообщения: ", err, getLine())
		}
		fmt.Println()
		fmt.Println()
	}
}

// Проверяет, запущен ли указанный процесс
func checkIfProcessRunning(processName string) bool {
	cmd := exec.Command("tasklist")
	output, err := cmd.Output()
	if err != nil {
		log.Fatal("Ошибка получения списка процессов:", err, getLine())
	}

	return strings.Contains(string(output), processName)
}

func TopLoad() string {
	processes, _ := process.Processes()

	maxCPUUsage := float64(0)
	maxCPUProcess := ""

	for _, proc := range processes {
		cpuPercent, _ := proc.CPUPercent()
		if cpuPercent > maxCPUUsage {
			maxCPUUsage = cpuPercent
			maxCPUProcess, _ = proc.Name()
		}
	}

	fmt.Printf("Process with highest CPU usage: %s, CPU usage: %f%%\n", maxCPUProcess, maxCPUUsage)
	return maxCPUProcess
}

func listenPort(port string) {
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Println("Ошибка при прослушивании порта: ", err, getLine())
		fmt.Println("Ошибка при прослушивании порта: ", err)
		return
	}
	defer listener.Close()

	// fmt.Println("Слушаем порт", port)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("Ошибка при принятии соединения: ", err, getLine())
			fmt.Println("Ошибка при принятии соединения: ", err)
			return
		}

		// Обработка соединения в отдельной горутине
		go findIP(conn)
		time.Sleep(5 * time.Second)
	}
}

func findIP(conn net.Conn) {
	remoteIP := conn.RemoteAddr().String()
	ip, _, err := net.SplitHostPort(remoteIP)
	if err != nil {
		log.Println("Ошибка при разделении адреса и порта: ", err, getLine())
		fmt.Println("Ошибка при разделении адреса и порта: ", err)
		return
	}

	remoteAddr = ip

	localIP := conn.LocalAddr().String()
	locip, _, err := net.SplitHostPort(localIP)
	if err != nil {
		log.Println("Ошибка при разделении адреса и порта: ", err, getLine())
		fmt.Println("Ошибка при разделении адреса и порта: ", err)
		return
	}

	localAddr = locip

	conn.Close()
}

func setFromFile(keys, filename string) (string, error) {
	var gname string
	file, err := os.Open(filename)
	if err != nil {
		log.Println("Ошибка при открытии файла ", filename, ": ", err, getLine())
		fmt.Println("Ошибка при открытии файла ", filename, ": ", err)
		return "Ошибка при открытии файла: ", err
	}
	defer file.Close()

	// Создать сканер для чтения содержимого файла построчно
	scanner := bufio.NewScanner(file)

	// Создать словарь для хранения пары "ключ-значение"
	data := make(map[string]string)

	// Перебирать строки из файла
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, " = ")
		if len(parts) == 2 {
			key := parts[0]
			value := parts[1]
			data[key] = value
		}
	}

	// Проверить наличие SS1
	if value, ok := data[keys]; ok {
		gname = value
	}
	return gname, err
}

func SendMessage(botToken string, chatID int64, text string) error {
	bot, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		log.Println("Ошибка подключения бота: ", err, getLine())
		return err
	}

	message := tgbotapi.NewMessage(chatID, text)

	_, err = bot.Send(message)
	if err != nil {
		log.Println("Ошибка отправки сообщения: ", err, getLine())
		return err
	}

	// Опциональные настройки сообщения
	message.ParseMode = "Markdown" // Может быть "Markdown" или "HTML"
	message.DisableNotification = false
	message.DisableWebPagePreview = true

	return nil
}

func setConsoleTitle(title string) { // для смены заголовока программы
	ptrTitle, _ := syscall.UTF16PtrFromString(title)
	_, _, _ = procSetConsoleTitleW.Call(uintptr(unsafe.Pointer(ptrTitle)))
}

func getLine() int {
	_, _, line, _ := runtime.Caller(1)
	return line
}
