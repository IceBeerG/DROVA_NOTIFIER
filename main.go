package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
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
	isRunning                                              bool
)

const (
	appName    = "ese.exe"             // Имя запускаемого файла
	newTitle   = "Drova Notifier"      // Имя окна программы
	timeFormat = "02.01.2006 15:04:05" // Формат времени для записи в CSV файл
	remoutPort = "7990"                // порт для поиска IP подключившегося
	localPort  = "139"                 // порт для поиска IP станции
)

type IPInfoResponse struct {
	IP     string `json:"ip"`
	City   string `json:"city"`
	Region string `json:"region"`
	ISP    string `json:"org"`
}

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

	// Получаем CHAT_ID, BotToken из файла telebot.txt
	CHAT_IDstr, _ := setFromFile("id", botlist)
	CHAT_ID, _ := strconv.ParseInt(CHAT_IDstr, 10, 64)
	BotToken, _ := setFromFile("token", botlist)

	go listenPort(remoutPort)
	go listenPort(localPort)

	// Получаем имя ПК
	hostname, err := os.Hostname()
	if err != nil {
		log.Println("Ошибка при получении имени компьютера: ", err, getLine())
		return
	}
	fmt.Printf("Hostname - %s\n\n", hostname)

	for {
		//ждем запуска приложения ese.exe
		for i := 0; i != 2; {
			isRunning = checkIfProcessRunning(appName) // запущено ли приложение
			if isRunning {
				startTime = time.Now() // переменная для вычисления продолжительности сессии
				startTimeApp = time.Now().Format(timeFormat)
				fmt.Println("Время запуска - ", startTimeApp)
				i = 2 //т.к. приложение запущено, выходим из цикла
			}
			time.Sleep(5 * time.Second)
		}

		// получаем список процессов
		for i := 0; i < 24; {
			time.Sleep(10 * time.Second)
			processes, err := ps.Processes()
			if err != nil {
				log.Println("Ошибка при получении списка процессов: ", err, getLine())
				fmt.Println("Ошибка при получении списка процессов: ", err)
				return
			}
			// сверяем запущенные процессы со списком игры в файле games.txt
			for _, process := range processes {
				proc := strings.Replace(process.Executable(), ".exe", "", -1) // убираем .exe из имен процесса

				// ищем значение proc среди ключенй в файле, и если находим, передаем значение в переменную
				gameN, err := setFromFile(proc, gamelist)
				if err != nil {
					log.Println("Ошибка получения данных из файла ", gamelist, "- ", err, getLine())
					fmt.Println(err)
				}
				// если нашли совпадение записываем название игры и выходим из цикла
				if gameN != "" {
					game = gameN
					i = 24
				}
			}
			i++
		}
		// если не нашли игру в списке, ищем процесс, максимально нагружающий процессор
		if game == "" {
			game = TopLoad()
		}
		gamerIP := remoteAddr
		serverIP := localAddr

		city, region, isp := ipInfo(remoteAddr)

		// Создаем сообщение
		chatMessage := hostname + " - " + gamerIP + "\nHaчaлo сессии - " + startTimeApp + "\nИгpa - " + game
		chatMessage += "\nserverIP = " + serverIP + "\nГopoд: " + city + "\nOблacть: " + region + "\nПpoвaйдep: " + isp
		fmt.Println(chatMessage)
		fmt.Println()

		// Отправляем сообщение через бота о начале сессии
		err := SendMessage(BotToken, CHAT_ID, chatMessage)
		if err != nil {
			log.Fatal("Ошибка отправки сообщения: ", err, getLine())
		}

		// ждем закрытия процесса ese.exe
		for i := 0; i != 3; {
			isRunning = checkIfProcessRunning(appName)
			if !isRunning {
				stopTime = time.Now()
				i = 3
			}

			time.Sleep(5 * time.Second)
		}

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
		chatMessage = hostname + " - " + gamerIP + "\nCeccия завершена\nBpeмя ceccии - " + sessionDur + "\nИгpa - " + game
		fmt.Println(chatMessage, "\nKoнeц ceccии: ", stopTimeApp)

		// Отправляем сообщение об окончании сессии
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

// ищет процесс максимально нагруущающий процессор
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

// слушаем порты
func listenPort(port string) {
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Println("Ошибка при прослушивании порта: ", err, getLine())
		fmt.Println("Ошибка при прослушивании порта: ", err)
		return
	}
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("Ошибка при принятии соединения: ", err, getLine())
			fmt.Println("Ошибка при принятии соединения: ", err)
			return
		}

		// Обработка соединения в отдельной горутине
		go findIP(conn)
	}
}

// поиск IP сервера и игрока
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

// получаем данные из файла в виде ключ = значение
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

	return nil
}

// для смены заголовока программы
func setConsoleTitle(title string) {
	ptrTitle, _ := syscall.UTF16PtrFromString(title)
	_, _, _ = procSetConsoleTitleW.Call(uintptr(unsafe.Pointer(ptrTitle)))
}

// получение строки кода где возникла ошибка
func getLine() int {
	_, _, line, _ := runtime.Caller(1)
	return line
}

// инфо по IP
func ipInfo(ip string) (city, region, isp string) {
	apiURL := fmt.Sprintf("https://ipinfo.io/%s/json", ip)

	resp, err := http.Get(apiURL)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	var ipInfo IPInfoResponse
	err = json.NewDecoder(resp.Body).Decode(&ipInfo)
	if err != nil {
		log.Fatal(err)
	}

	city = ipInfo.City
	region = ipInfo.Region
	isp = ipInfo.ISP
	return
}
