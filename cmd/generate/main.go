package main

import (
	"SlamSim/fastgen"
	"SlamSim/file_operator"
	"fmt"
	"math/rand/v2"
	"runtime"
)

const batchSize = 1000
const perPatternsAlert = 10000

func main() {
	file_operator.Init()
	fmt.Print("Успешно инициализирован\nНе завершайте программу до конца генерации, это может привести к повреждению файла с паттернами!\n")
	patN, _ := file_operator.GetPatsNumber()
	fmt.Printf("На данный момент в файле сохранено %v паттернов\n", patN)
	var n int
	fmt.Print("Введите количество паттернов для генерации\n")

	_, err := fmt.Scan(&n)
	if err != nil {
		fmt.Println("Ошибка: Вы ввели не число!")
		return
	}

	fmt.Print("Начало генерации...\n")

	// Каждый воркер со своим генератором, запись в файл — из одной горутины пачками
	results := make(chan fastgen.Result, batchSize)
	jobs := make(chan struct{}, n)
	for i := 0; i < n; i++ {
		jobs <- struct{}{}
	}
	close(jobs)

	for w := 0; w < runtime.NumCPU(); w++ {
		g := fastgen.New(rand.Uint64(), rand.Uint64(), fastgen.DefaultConfig())
		go func() {
			for range jobs {
				results <- g.Next()
			}
		}()
	}

	matrices := make([][][]uint8, 0, batchSize)
	routes := make([][][]int, 0, batchSize)
	flush := func() {
		if err := file_operator.InsertBatch(matrices, routes); err != nil {
			fmt.Printf("Ошибка записи: %v\n", err)
		}
		matrices, routes = matrices[:0], routes[:0]
	}

	for totalPatGened := 1; totalPatGened <= n; totalPatGened++ {
		r := <-results
		matrices = append(matrices, r.Pattern)
		routes = append(routes, r.Route)
		if len(matrices) == batchSize {
			flush()
		}
		if totalPatGened%perPatternsAlert == 0 {
			fmt.Printf("Сгенерировано %v паттернов, %v%% от общего числа\n", totalPatGened, (totalPatGened*100)/n)
		}
	}
	flush()
	fmt.Print("Генерация закончена\n")
}
