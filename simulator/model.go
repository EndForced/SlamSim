// Package simulator — симулятор робота на поле «Ремонт труб».
// Модель проездов берётся из matrixfactory (через fastgen.Connects), поле — из data/pats.bin.
package simulator

import (
	"SlamSim/fastgen"
	"SlamSim/matrixfactory"
	"fmt"
	"math"
	"time"
)

const N = fastgen.N

// Направления как в matrixfactory: 0 North, 1 South, 2 East, 3 West
const (
	north = int(matrixfactory.North)
	south = int(matrixfactory.South)
	east  = int(matrixfactory.East)
	west  = int(matrixfactory.West)
)

var dirNames = [4]string{"north", "south", "east", "west"}
var dirDY = [4]int{-1, 1, 0, 0}
var dirDX = [4]int{0, 0, 1, -1}
var rightOf = [4]int{east, west, south, north}
var leftOf = [4]int{west, east, north, south}

func opposite(d int) int { return d ^ 1 }

// Config — настройки, которые клиент может менять через API
type Config struct {
	// Клетки, которые видит робот, в его системе координат: [вперёд, вправо].
	// [1,0] — клетка перед роботом, [0,-1] — слева, [-1,0] — сзади.
	SensorCells [][2]int `json:"sensor_cells"`
	// Режим «со спутником»: в ответах есть абсолютная поза, доступна карта целиком.
	// false — режим хардкор (SLAM), только бортовые датчики.
	Satellite bool `json:"satellite"`
	// Сколько труб робот может везти одновременно
	Capacity int `json:"capacity"`
	// Задержка после каждой команды, чтобы движение было видно в окне симулятора
	StepDelayMs int `json:"step_delay_ms"`

	Camera   CameraConfig   `json:"camera"`
	Lighting LightingConfig `json:"lighting"`
	Scene    SceneConfig    `json:"scene"`
}

func DefaultConfig() Config {
	return Config{
		SensorCells: [][2]int{{1, 0}, {2, 0}, {1, -1}, {1, 1}, {0, -1}, {0, 1}},
		Satellite:   false,
		Capacity:    1,
		StepDelayMs: 150,
		Camera:      DefaultCamera(),
		Lighting:    DefaultLighting(),
		Scene:       DefaultScene(),
	}
}

func (c Config) validate() error {
	if len(c.SensorCells) > 64 {
		return fmt.Errorf("sensor_cells: не больше 64 клеток")
	}
	for _, s := range c.SensorCells {
		if s[0] < -N || s[0] > N || s[1] < -N || s[1] > N {
			return fmt.Errorf("sensor_cells: смещение %v вне диапазона ±%d", s, N)
		}
	}
	if c.Capacity < 1 || c.Capacity > 3 {
		return fmt.Errorf("capacity: допустимо 1..3")
	}
	if c.StepDelayMs < 0 || c.StepDelayMs > 5000 {
		return fmt.Errorf("step_delay_ms: допустимо 0..5000")
	}
	cam := c.Camera
	switch {
	case cam.FOV < 5 || cam.FOV > 170:
		return fmt.Errorf("camera.fov: допустимо 5..170 градусов")
	case cam.Width < 8 || cam.Width > 1920 || cam.Height < 8 || cam.Height > 1920:
		return fmt.Errorf("camera.width/height: допустимо 8..1920")
	case cam.AA < 1 || cam.AA > 4:
		return fmt.Errorf("camera.aa: допустимо 1..4")
	case cam.Pitch < -90 || cam.Pitch > 90:
		return fmt.Errorf("camera.pitch: допустимо -90..90")
	case cam.Noise < 0 || cam.Noise > 1:
		return fmt.Errorf("camera.noise: допустимо 0..1")
	case math.Abs(cam.K1) > 1:
		return fmt.Errorf("camera.k1: допустимо -1..1")
	case math.Abs(cam.X) > 500 || math.Abs(cam.Y) > 500 || cam.Z < 0 || cam.Z > 1000:
		return fmt.Errorf("camera.x/y: ±500 мм, camera.z: 0..1000 мм")
	}
	l := c.Lighting
	if l.Ambient < 0 || l.Ambient > 2 || l.Specular < 0 || l.Specular > 5 || len(l.Lights) > 8 {
		return fmt.Errorf("lighting: ambient 0..2, specular 0..5, не больше 8 источников")
	}
	for _, li := range l.Lights {
		if li.Intensity < 0 || li.Intensity > 5 || li.Z <= 0 {
			return fmt.Errorf("lighting.lights: intensity 0..5, z > 0 (источник над полем)")
		}
	}
	if c.Scene.StandHeight < 10 || c.Scene.StandHeight > 300 || c.Scene.HolderHeight < 10 || c.Scene.HolderHeight > 300 {
		return fmt.Errorf("scene: высоты подставок 10..300 мм")
	}
	return nil
}

// CommandError — нелегальная команда. Состояние симулятора при этом не меняется.
type CommandError struct {
	Code    string `json:"error"`
	Message string `json:"message"`
}

func (e *CommandError) Error() string { return e.Code + ": " + e.Message }

func illegal(code, format string, args ...any) *CommandError {
	return &CommandError{Code: code, Message: fmt.Sprintf(format, args...)}
}

// World — состояние одного заезда
type World struct {
	base       [N * N]uint8  // клетки без робота: трубы 23-30, пустые белые подставки 21/22, зелёные 17-20
	holderTube [N * N]string // цвет трубы, лежащей на зелёной подставке
	standAxisX [N * N]bool   // ориентация белой подставки, с которой сняли трубу (для рендера)
	holders    [3]int        // зелёные подставки по порядку вдоль границы

	pos     int
	heading int

	carrying  []string
	picked    int
	touched   bool // секции считаются после первого касания трубы (п.8.3)
	sections  int
	moves     int
	reference int // эталонная сумма секций для этого поля
	route     [][]int

	PatternIndex int
	Started      time.Time
}

func colorOf(id uint8) string {
	switch {
	case id >= 23 && id <= 26:
		return "blue"
	case id >= 27 && id <= 30:
		return "red"
	}
	return ""
}

func isTube(id uint8) bool   { return id >= 23 && id <= 30 }
func isHolder(id uint8) bool { return id >= 17 && id <= 20 }

// NewWorld строит заезд из матрицы поля и эталонного маршрута fastgen
func NewWorld(pattern [][]uint8, route [][]int, index int) (*World, error) {
	w := &World{pos: -1, PatternIndex: index, Started: time.Now(), route: route}
	var holders []int
	for y := 0; y < N; y++ {
		for x := 0; x < N; x++ {
			c := y*N + x
			id := pattern[y][x]
			switch id {
			case 11, 12:
				w.pos = c
				id -= 10
			}
			if isHolder(id) {
				holders = append(holders, c)
			}
			w.base[c] = id
		}
	}
	if w.pos < 0 {
		return nil, fmt.Errorf("на поле нет робота")
	}
	if len(holders) != 3 {
		return nil, fmt.Errorf("на поле %d зелёных подставок вместо 3", len(holders))
	}
	copy(w.holders[:], holders)

	// Эталон: сумма секций маршрута после первого касания трубы. В маршруте fastgen
	// каждый из 5 отрезков дополнительно «въезжает» в трубу/подставку и обратно (по 1 секции),
	// а робот в симуляторе берёт и кладёт трубы с соседнего перекрёстка — вычитаем их.
	if len(route) == 6 {
		sum := 0
		for _, seg := range route[1:] {
			for _, c := range seg[1:] {
				sum += fastgen.CellWeight(w.base[c])
			}
		}
		w.reference = sum - 2*5
	} else {
		w.reference = -1
	}

	// Начальное направление: первое, в котором можно куда-то поехать
	w.heading = north
	for _, d := range []int{north, east, south, west} {
		if t, ok := w.neighbour(w.pos, d); ok && fastgen.Connects(w.base[w.pos], w.base[t], matrixfactory.DirectionType(d)) && !w.blocked(t) {
			w.heading = d
			break
		}
	}
	return w, nil
}

func (w *World) neighbour(c, d int) (int, bool) {
	y, x := c/N+dirDY[d], c%N+dirDX[d]
	if y < 0 || y >= N || x < 0 || x >= N {
		return -1, false
	}
	return y*N + x, true
}

// blocked — в клетку нельзя въехать: труба, пустая белая подставка или зелёная подставка
func (w *World) blocked(c int) bool {
	id := w.base[c]
	return isTube(id) || isHolder(id) || id == 21 || id == 22
}

// relative переводит смещение в системе робота (вперёд, вправо) в абсолютную клетку
func (w *World) relative(f, r int) (int, bool) {
	h, rd := w.heading, rightOf[w.heading]
	y := w.pos/N + f*dirDY[h] + r*dirDY[rd]
	x := w.pos%N + f*dirDX[h] + r*dirDX[rd]
	if y < 0 || y >= N || x < 0 || x >= N {
		return -1, false
	}
	return y*N + x, true
}

// --- команды ---

func (w *World) Forward() error  { return w.drive(w.heading, "вперёд") }
func (w *World) Backward() error { return w.drive(opposite(w.heading), "назад") }

func (w *World) TurnLeft() error {
	w.heading = leftOf[w.heading]
	w.moves++
	return nil
}

func (w *World) TurnRight() error {
	w.heading = rightOf[w.heading]
	w.moves++
	return nil
}

func (w *World) drive(d int, name string) error {
	t, ok := w.neighbour(w.pos, d)
	if !ok {
		return illegal("out_of_field", "%s: за краем поля", name)
	}
	cur, next := w.base[w.pos], w.base[t]
	if w.blocked(t) {
		return illegal("blocked", "%s: клетка занята (%s)", name, kindOf(w, t))
	}
	if !fastgen.Connects(cur, next, matrixfactory.DirectionType(d)) {
		return illegal("no_passage", "%s: нет проезда из «%s» в «%s»", name, tileName(cur), tileName(next))
	}
	w.pos = t
	w.moves++
	if w.touched {
		w.sections += fastgen.CellWeight(next)
	}
	return nil
}

// Pick снимает трубу с белой подставки перед роботом
func (w *World) Pick(capacity int) error {
	t, ok := w.neighbour(w.pos, w.heading)
	if !ok || !isTube(w.base[t]) {
		return illegal("no_tube", "перед роботом нет трубы")
	}
	if !fastgen.Connects(w.base[w.pos], w.base[t], matrixfactory.DirectionType(w.heading)) {
		return illegal("wrong_approach", "к трубе нужно подъехать поперёк и с того же яруса")
	}
	if len(w.carrying) >= capacity {
		return illegal("capacity", "робот уже везёт %d труб(ы), capacity=%d", len(w.carrying), capacity)
	}
	w.carrying = append(w.carrying, colorOf(w.base[t]))
	w.standAxisX[t] = (w.base[t]-23)%2 == 0
	// Труба снята, подставка осталась: клетка по-прежнему занята (ID 21/22 — пустая подставка)
	if w.base[t] == 25 || w.base[t] == 26 || w.base[t] == 29 || w.base[t] == 30 {
		w.base[t] = 22
	} else {
		w.base[t] = 21
	}
	w.picked++
	w.touched = true
	w.moves++
	return nil
}

// Place кладёт трубу на зелёную подставку перед роботом. color == "" — последнюю взятую.
func (w *World) Place(color string) error {
	t, ok := w.neighbour(w.pos, w.heading)
	if !ok || !isHolder(w.base[t]) {
		return illegal("no_holder", "перед роботом нет зелёной подставки")
	}
	if !fastgen.Connects(w.base[w.pos], w.base[t], matrixfactory.DirectionType(w.heading)) {
		return illegal("wrong_approach", "к зелёной подставке подъезжают только спереди")
	}
	if w.holderTube[t] != "" {
		return illegal("holder_busy", "на подставке уже лежит труба")
	}
	if len(w.carrying) == 0 {
		return illegal("empty", "робот не везёт трубу")
	}
	idx := len(w.carrying) - 1
	if color != "" {
		idx = -1
		for i, c := range w.carrying {
			if c == color {
				idx = i
			}
		}
		if idx < 0 {
			return illegal("no_such_color", "робот не везёт трубу цвета %q", color)
		}
	}
	w.holderTube[t] = w.carrying[idx]
	w.carrying = append(w.carrying[:idx], w.carrying[idx+1:]...)
	w.moves++
	return nil
}

// --- наблюдения ---

// Cell — клетка глазами робота
type Cell struct {
	F      int    `json:"f"` // вперёд
	R      int    `json:"r"` // вправо
	Kind   string `json:"kind"`
	Level  int    `json:"level,omitempty"`   // 1 — нижний ярус, 2 — верхний
	Color  string `json:"color,omitempty"`   // труба или труба на зелёной подставке
	RampUp string `json:"ramp_up,omitempty"` // куда пандус поднимается: forward/backward/left/right
	// Для трубы: across — робот, глядящий вперёд, может к ней подъехать (если она прямо по курсу)
	TubeAxis string `json:"tube_axis,omitempty"`
	ID       *uint8 `json:"id,omitempty"` // сырой ID клетки, только в режиме спутника
}

// Kinds: out, floor, section, ramp, tube, stand (белая подставка без трубы), holder (зелёная)
func kindOf(w *World, c int) string {
	id := w.base[c]
	switch {
	case id == 1:
		return "floor"
	case id == 2:
		return "section"
	case id >= 3 && id <= 6:
		return "ramp"
	case id == 21 || id == 22:
		return "stand"
	case isTube(id):
		return "tube"
	case isHolder(id):
		return "holder"
	}
	return "unknown"
}

func tileName(id uint8) string {
	if t, ok := matrixfactory.Tiles[id]; ok {
		return t.Name
	}
	return fmt.Sprint(id)
}

// rampUpDir — абсолютное направление подъёма пандуса
func rampUpDir(id uint8) int {
	switch id {
	case 3:
		return north
	case 4:
		return south
	case 5:
		return east
	}
	return west
}

func (w *World) toRobotFrame(d int) string {
	switch d {
	case w.heading:
		return "forward"
	case opposite(w.heading):
		return "backward"
	case rightOf[w.heading]:
		return "right"
	}
	return "left"
}

func (w *World) describe(c, f, r int, satellite bool) Cell {
	cell := Cell{F: f, R: r, Kind: kindOf(w, c)}
	id := w.base[c]
	switch cell.Kind {
	case "floor", "holder":
		cell.Level = 1
	case "section":
		cell.Level = 2
	case "ramp":
		cell.RampUp = w.toRobotFrame(rampUpDir(id))
	case "tube", "stand":
		cell.Level = 1
		if id == 22 || id == 25 || id == 26 || id == 29 || id == 30 {
			cell.Level = 2
		}
	}
	if isTube(id) {
		cell.Color = colorOf(id)
		// 23/25/27/29 подъезжают с севера/юга, 24/26/28/30 — с востока/запада
		vertical := (id-23)%2 == 0
		facingVertical := w.heading == north || w.heading == south
		if vertical == facingVertical {
			cell.TubeAxis = "across"
		} else {
			cell.TubeAxis = "along"
		}
	}
	if isHolder(id) {
		cell.Color = w.holderTube[c]
	}
	if satellite {
		v := id
		cell.ID = &v
	}
	return cell
}

func (w *World) Scan(cfg Config) []Cell {
	res := make([]Cell, 0, len(cfg.SensorCells))
	for _, s := range cfg.SensorCells {
		c, ok := w.relative(s[0], s[1])
		if !ok {
			res = append(res, Cell{F: s[0], R: s[1], Kind: "out"})
			continue
		}
		res = append(res, w.describe(c, s[0], s[1], cfg.Satellite))
	}
	return res
}

// SensorAbs — абсолютные клетки датчиков (для отрисовки в окне)
func (w *World) SensorAbs(cfg Config) []int {
	var res []int
	for _, s := range cfg.SensorCells {
		if c, ok := w.relative(s[0], s[1]); ok {
			res = append(res, c)
		}
	}
	return res
}

type Pose struct {
	X       int    `json:"x"`
	Y       int    `json:"y"`
	Heading string `json:"heading"`
}

func (w *World) Pose() Pose {
	return Pose{X: w.pos % N, Y: w.pos / N, Heading: dirNames[w.heading]}
}

// Score — баллы по таблице п.8.3 (без инженерной задачи и SLAM)
type Score struct {
	Picked     int  `json:"picked"`
	Placed     int  `json:"placed"`
	PickPoints int  `json:"pick_points"`
	PlacePts   int  `json:"place_points"`
	Bonus      int  `json:"bonus"`
	Logistics  int  `json:"logistics"`
	Total      int  `json:"total"`
	Finished   bool `json:"finished"`
}

func (w *World) Score() Score {
	s := Score{Picked: w.picked}
	for _, h := range w.holders {
		if w.holderTube[h] != "" {
			s.Placed++
		}
	}
	s.PickPoints = 2 * s.Picked
	s.PlacePts = 6 * s.Placed
	if s.Placed == 3 {
		s.Finished = true
		a, b, c := w.holderTube[w.holders[0]], w.holderTube[w.holders[1]], w.holderTube[w.holders[2]]
		if a == c && b != a {
			s.Bonus = 6
		}
		if w.reference >= 0 && w.sections <= w.reference {
			s.Logistics = 25
		}
	}
	s.Total = s.PickPoints + s.PlacePts + s.Bonus + s.Logistics
	return s
}
