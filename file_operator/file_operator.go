package file_operator

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
)

const SaveFile = "data/pats.bin"

// Матрица занимает 48 байт. Маршрут (128 чисел по 6 бит) занимает 96 байт.
// Итого: 48 + 96 = 144 байта на одну запись.
const PatSizeBytes = 48 + 96
const RouteMaxElements = 128
const MaxMetadataSlots = 16 // Первые 16 слотов (6-битных) отданы под длины под-слайсов

func get_file_path() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("не удалось получить рабочий каталог: %w", err)
	}
	return filepath.Join(wd, SaveFile), nil
}

// Init безопасно проверяет и создает файл, не загружая его данные в ОЗУ
func Init() error {
	fp, err := get_file_path()
	if err != nil {
		return err
	}

	_, err = os.Stat(fp)
	if os.IsNotExist(err) {
		log.Println("No pat file found. Trying to create a new one")

		errDir := os.MkdirAll(filepath.Dir(fp), 0755)
		if errDir != nil {
			return fmt.Errorf("не удалось создать папку для файла: %w", errDir)
		}

		file, err := os.OpenFile(fp, os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("не удалось создать файл для расстановок: %w", err)
		}
		file.Close()
	} else if err != nil {
		return fmt.Errorf("ошибка при проверке файла: %w", err)
	}

	log.Println("File operator successfully initialized")
	return nil
}

// Insert упаковывает матрицу и маршрут, после чего дописывает 144 байта в конец файла
func Insert(matrix [][]uint8, route [][]int) error {
	return InsertBatch([][][]uint8{matrix}, [][][]int{route})
}

// InsertBatch дописывает сразу несколько записей, открывая файл один раз
func InsertBatch(matrices [][][]uint8, routes [][][]int) error {
	if len(matrices) != len(routes) {
		return fmt.Errorf("количество матриц (%d) и маршрутов (%d) не совпадает", len(matrices), len(routes))
	}
	serialized := make([]byte, 0, PatSizeBytes*len(matrices))
	for i := range matrices {
		serialized = appendRecord(serialized, matrices[i], routes[i])
	}

	fp, err := get_file_path()
	if err != nil {
		return err
	}

	file, err := os.OpenFile(fp, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file for append: %w", err)
	}
	defer file.Close()

	_, err = file.Write(serialized)
	if err != nil {
		return fmt.Errorf("failed to write data to file: %w", err)
	}

	return nil
}

func appendRecord(serialized []byte, matrix [][]uint8, route [][]int) []byte {
	// 1. Сериализация матрицы (48 байт)
	for i := 0; i < 8; i++ {
		for j := 0; j < 8; j += 4 {
			a := uint8(matrix[i][j])
			b := uint8(matrix[i][j+1])
			c := uint8(matrix[i][j+2])
			d := uint8(matrix[i][j+3])

			n1 := (a << 2) | (b >> 4)
			n2 := (b << 4) | (c >> 2)
			n3 := (c << 6) | d

			serialized = append(serialized, n1, n2, n3)
		}
	}

	// 2. Сериализация маршрута (96 байт) через карту длин под-слайсов
	routeBytes := serializeRoute(route)
	return append(serialized, routeBytes[:]...)
}

// GetPatsNumber возвращает количество записей в файле на основе его размера
func GetPatsNumber() (int, error) {
	fp, err := get_file_path()
	if err != nil {
		return 0, err
	}

	file, err := os.Open(fp)
	if err != nil {
		return 0, fmt.Errorf("failed to open pattern file: %w", err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return 0, fmt.Errorf("failed to get pattern file size: %w", err)
	}

	if stat.Size()%PatSizeBytes != 0 {
		return 0, fmt.Errorf("pattern file is corrupted (size %d is not a multiple of %d)", stat.Size(), PatSizeBytes)
	}

	return int(stat.Size()) / PatSizeBytes, nil
}

// GetNsPat мгновенно считывает ровно 144 байта по индексу записи (произвольный доступ)
func GetNsPat(n int) ([8][8]int, [][]int, error) {
	var matrix [8][8]int
	offset := int64(PatSizeBytes * n)

	fp, err := get_file_path()
	if err != nil {
		return matrix, nil, err
	}

	file, err := os.Open(fp)
	if err != nil {
		return matrix, nil, fmt.Errorf("failed to open pattern file: %w", err)
	}
	defer file.Close()

	_, err = file.Seek(offset, io.SeekStart)
	if err != nil {
		return matrix, nil, fmt.Errorf("failed to seek to pattern index %d: %w", n, err)
	}

	buffer := make([]byte, PatSizeBytes)
	_, err = io.ReadFull(file, buffer)
	if err != nil {
		return matrix, nil, fmt.Errorf("failed to read pattern data: %w", err)
	}

	// Разделяем считанные байты на матрицу (первые 48) и путь (оставшиеся 96)
	var matrixData [48]byte
	copy(matrixData[:], buffer[:48])
	matrix = deserializeMatrix(matrixData)

	var routeData [96]byte
	copy(routeData[:], buffer[48:])
	route := deserializeRoute(routeData)

	return matrix, route, nil
}

func deserializeMatrix(data [48]byte) [8][8]int {
	var matrix [8][8]int
	byteIdx := 0

	for i := 0; i < 8; i++ {
		for j := 0; j < 8; j += 4 {
			n1 := data[byteIdx]
			n2 := data[byteIdx+1]
			n3 := data[byteIdx+2]
			byteIdx += 3

			a := n1 >> 2
			b := ((n1 & 0b11) << 4) | (n2 >> 4)
			c := ((n2 & 0b1111) << 2) | (n3 >> 6)
			d := n3 & 0b111111

			matrix[i][j] = int(a)
			matrix[i][j+1] = int(b)
			matrix[i][j+2] = int(c)
			matrix[i][j+3] = int(d)
		}
	}
	return matrix
}

func serializeRoute(route [][]int) [96]byte {
	var flatRoute [RouteMaxElements]int

	// 1. Записываем длины под-слайсов в первые 16 слотов метаданных
	for i := 0; i < len(route) && i < MaxMetadataSlots; i++ {
		flatRoute[i] = len(route[i])
	}

	// 2. Записываем сами координаты в оставшиеся 112 слотов
	dataIdx := MaxMetadataSlots
	for _, slice := range route {
		for _, val := range slice {
			if dataIdx < RouteMaxElements {
				flatRoute[dataIdx] = val
				dataIdx++
			}
		}
	}

	// 3. Упаковка плоского массива (128 чисел по 6 бит) в 96 байт
	var packed [96]byte
	byteIdx := 0
	for i := 0; i < RouteMaxElements; i += 4 {
		a := uint8(flatRoute[i])
		b := uint8(flatRoute[i+1])
		c := uint8(flatRoute[i+2])
		d := uint8(flatRoute[i+3])

		packed[byteIdx] = (a << 2) | (b >> 4)
		packed[byteIdx+1] = (b << 4) | (c >> 2)
		packed[byteIdx+2] = (c << 6) | d
		byteIdx += 3
	}

	return packed
}

func deserializeRoute(data [96]byte) [][]int {
	// 1. Распаковка 96 байт обратно в плоский массив из 128 чисел
	flatRoute := make([]int, 0, RouteMaxElements)
	byteIdx := 0

	for i := 0; i < RouteMaxElements; i += 4 {
		n1 := data[byteIdx]
		n2 := data[byteIdx+1]
		n3 := data[byteIdx+2]
		byteIdx += 3

		a := n1 >> 2
		b := ((n1 & 0b11) << 4) | (n2 >> 4)
		c := ((n2 & 0b1111) << 2) | (n3 >> 6)
		d := n3 & 0b111111

		flatRoute = append(flatRoute, int(a), int(b), int(c), int(d))
	}

	// 2. Восстановление исходной структуры [][]int по карте длин
	var route [][]int
	dataIdx := MaxMetadataSlots

	for i := 0; i < MaxMetadataSlots; i++ {
		sliceLen := flatRoute[i]
		if sliceLen == 0 {
			break // Длины под-слайсов закончились
		}

		endIdx := dataIdx + sliceLen
		if endIdx > RouteMaxElements {
			endIdx = RouteMaxElements
		}

		// Копируем данные под-слайса
		sliceData := make([]int, endIdx-dataIdx)
		copy(sliceData, flatRoute[dataIdx:endIdx])
		route = append(route, sliceData)

		dataIdx = endIdx
		if dataIdx >= RouteMaxElements {
			break
		}
	}

	return route
}

// RouteToMatrices преобразует маршрут ([][]int) в две матрицы 8x8:
// 1. Матрицу шагов (глобальный индекс посещения клетки, где пара чисел - это X и Y)
// 2. Матрицу ID под-маршрутов (индекс исходного под-слайса)
func RouteToMatrices(route [][]int) ([8][8]int, [8][8]int) {
	var stepMatrix [8][8]int
	var idMatrix [8][8]int

	for i := 0; i < 8; i++ {
		for j := 0; j < 8; j++ {
			stepMatrix[i][j] = -1
			idMatrix[i][j] = -1
		}
	}

	globalStep := 0
	_ = 1
	for sliceIdx, slice := range route {
		for i := 0; i < len(slice); i += 2 {
			if i+1 >= len(slice) {
				break
			}
			x := slice[i]
			y := slice[i+1]

			if x >= 0 && x < 8 && y >= 0 && y < 8 {
				stepMatrix[x][y] = globalStep
				idMatrix[x][y] = sliceIdx
				globalStep++
			}
		}
	}

	return stepMatrix, idMatrix
}
