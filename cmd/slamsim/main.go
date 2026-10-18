// Симулятор: go run ./cmd/slamsim (или готовый бинарник из релиза), окно — http://127.0.0.1:8080
package main

import (
	"SlamSim/assets"
	"SlamSim/file_operator"
	"SlamSim/simulator"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// version подставляется при сборке релиза: -ldflags "-X main.version=v0.1.0"
var version = "dev"

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "адрес HTTP API и окна симулятора")
	root := flag.String("root", ".", "папка с data/pats.bin (создаётся, если её нет)")
	open := flag.Bool("open", true, "открыть окно симулятора в браузере при запуске")
	showVersion := flag.Bool("version", false, "показать версию и выйти")
	flag.Parse()

	if *showVersion {
		fmt.Println("slamsim", version)
		return
	}
	if err := os.Chdir(*root); err != nil {
		log.Fatal(err)
	}
	if err := file_operator.Init(); err != nil {
		log.Fatal(err)
	}
	srv, err := simulator.NewServer(assets.FS)
	if err != nil {
		log.Fatal(err)
	}

	// Слушаем до открытия браузера, чтобы страница не открылась раньше сервера
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("не удалось занять %s: %v (порт занят? запустите с -addr 127.0.0.1:9000)", *addr, err)
	}
	url := "http://" + browserHost(ln.Addr().String())
	log.Printf("SlamSim %s запущен: %s (Ctrl+C — остановить)", version, url)
	if *open {
		openBrowser(url)
	}
	log.Fatal(http.Serve(ln, srv.Handler()))
}

// browserHost заменяет 0.0.0.0/[::] на localhost, чтобы адрес открывался в браузере
func browserHost(a string) string {
	host, port, err := net.SplitHostPort(a)
	if err != nil {
		return a
	}
	if ip := net.ParseIP(host); ip == nil || ip.IsUnspecified() {
		host = "127.0.0.1"
	}
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return host + ":" + port
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		log.Printf("не удалось открыть браузер (%v), откройте %s вручную", err, url)
	}
}
