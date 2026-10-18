package simulator

import (
	"SlamSim/fastgen"
	"SlamSim/file_operator"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"io/fs"
	"math/rand/v2"
	"net/http"
	"strconv"
	"sync"
	"time"
)

//go:embed ui
var uiFiles embed.FS

type logLine struct {
	Time  string `json:"time"`
	Level string `json:"level"` // info, error, client
	Text  string `json:"text"`
}

type telemetryItem struct {
	Key   string `json:"key"`
	Value any    `json:"value"`
}

type Server struct {
	mu        sync.Mutex
	world     *World
	pattern   [][]uint8 // исходное поле текущего заезда (для перезапуска)
	cfg       Config
	telemetry []telemetryItem
	logs      []logLine
	lastError *CommandError
	lastPoll  time.Time
	events    chan string
	assets    fs.FS
}

// NewServer создаёт сервер; assets — картинки клеток для окна (обычно assets.FS)
func NewServer(assets fs.FS) (*Server, error) {
	s := &Server{cfg: DefaultConfig(), events: make(chan string, 16), assets: assets}
	if err := s.reset(nil, false); err != nil {
		return nil, err
	}
	return s, nil
}

// reset загружает случайное поле из data/pats.bin (или поле с номером index).
// Если файл пуст — генерирует поле через fastgen. same — перезапустить текущее поле.
func (s *Server) reset(index *int, same bool) error {
	var pattern [][]uint8
	var route [][]int
	idx := -1

	n, err := file_operator.GetPatsNumber()
	if same && s.world != nil {
		pattern, route, idx = s.pattern, s.world.route, s.world.PatternIndex
	} else if index != nil && n == 0 {
		return fmt.Errorf("в data/pats.bin нет полей, номер %d не выбрать", *index)
	} else if err == nil && n > 0 {
		idx = rand.IntN(n)
		if index != nil {
			if *index >= n {
				return fmt.Errorf("в файле %d полей, номера %d нет", n, *index)
			}
			idx = *index
		}
		m, r, err := file_operator.GetNsPat(idx)
		if err != nil {
			return err
		}
		pattern = make([][]uint8, N)
		for y := range pattern {
			pattern[y] = make([]uint8, N)
			for x := range pattern[y] {
				pattern[y][x] = uint8(m[y][x])
			}
		}
		route = r
	} else {
		res := fastgen.New(rand.Uint64(), rand.Uint64(), fastgen.DefaultConfig()).Next()
		pattern, route = res.Pattern, res.Route
	}

	w, err := NewWorld(pattern, route, idx)
	if err != nil {
		return err
	}
	s.world = w
	s.pattern = pattern
	s.lastError = nil
	if idx >= 0 {
		s.logf("info", "Загружено поле №%d из data/pats.bin, эталон %d секций", idx, w.reference)
	} else {
		s.logf("info", "Файл с полями пуст — сгенерировано новое поле, эталон %d секций", w.reference)
	}
	return nil
}

func (s *Server) logf(level, format string, args ...any) {
	s.logs = append(s.logs, logLine{Time: time.Now().Format("15:04:05"), Level: level, Text: fmt.Sprintf(format, args...)})
	if len(s.logs) > 300 {
		s.logs = s.logs[len(s.logs)-300:]
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	ui, _ := fs.Sub(uiFiles, "ui")
	mux.Handle("GET /", http.FileServerFS(ui))
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServerFS(s.assets)))

	mux.HandleFunc("POST /api/reset", s.handleReset)
	mux.HandleFunc("GET /api/config", s.handleGetConfig)
	mux.HandleFunc("POST /api/config", s.handleSetConfig)
	mux.HandleFunc("POST /api/cmd", s.handleCmd)
	mux.HandleFunc("GET /api/scan", s.handleScan)
	mux.HandleFunc("GET /api/map", s.handleMap)
	mux.HandleFunc("GET /api/debug/reference", s.handleReference)
	mux.HandleFunc("POST /api/telemetry", s.handleTelemetry)
	mux.HandleFunc("GET /api/events", s.handleEvents)
	mux.HandleFunc("POST /api/iterate", s.handleIterate)
	mux.HandleFunc("GET /api/state", s.handleState)
	mux.HandleFunc("GET /api/camera", s.handleCamera)
	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func badRequest(w http.ResponseWriter, code, msg string) {
	writeJSON(w, http.StatusBadRequest, CommandError{Code: code, Message: msg})
}

func readJSON(r *http.Request, v any) error {
	if r.ContentLength == 0 {
		return nil
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// observation — то, что робот получает после каждой команды
func (s *Server) observation() map[string]any {
	w := s.world
	obs := map[string]any{
		"ok":       true,
		"scan":     w.Scan(s.cfg),
		"carrying": append([]string{}, w.carrying...),
		"sections": w.sections,
		"score":    w.Score(),
	}
	if s.cfg.Satellite {
		obs["pose"] = w.Pose()
	}
	return obs
}

func (s *Server) handleReset(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Index *int `json:"index"`
		Same  bool `json:"same"`
	}
	if err := readJSON(r, &req); err != nil {
		badRequest(w, "bad_json", err.Error())
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.reset(req.Index, req.Same); err != nil {
		badRequest(w, "reset_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.observation())
}

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	writeJSON(w, http.StatusOK, s.cfg)
}

func (s *Server) handleSetConfig(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg := s.cfg // присланные поля перезаписывают текущие, остальные остаются
	if err := readJSON(r, &cfg); err != nil {
		badRequest(w, "bad_json", err.Error())
		return
	}
	if err := cfg.validate(); err != nil {
		badRequest(w, "bad_config", err.Error())
		return
	}
	s.cfg = cfg
	writeJSON(w, http.StatusOK, s.cfg)
}

func (s *Server) handleCmd(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Cmd    string `json:"cmd"`
		Color  string `json:"color"`
		Source string `json:"source"`
	}
	if err := readJSON(r, &req); err != nil {
		badRequest(w, "bad_json", err.Error())
		return
	}

	s.mu.Lock()
	var err error
	wd := s.world
	switch req.Cmd {
	case "forward":
		err = wd.Forward()
	case "backward":
		err = wd.Backward()
	case "left":
		err = wd.TurnLeft()
	case "right":
		err = wd.TurnRight()
	case "pick":
		err = wd.Pick(s.cfg.Capacity)
	case "place":
		err = wd.Place(req.Color)
	default:
		err = illegal("unknown_command", "неизвестная команда %q (forward, backward, left, right, pick, place)", req.Cmd)
	}

	var cmdErr *CommandError
	if errors.As(err, &cmdErr) {
		s.lastError = cmdErr
		s.logf("error", "%s: %s", req.Cmd, cmdErr.Message)
		s.mu.Unlock()
		writeJSON(w, http.StatusBadRequest, cmdErr)
		return
	}
	s.lastError = nil
	if req.Cmd == "pick" || req.Cmd == "place" {
		s.logf("info", "%s → везёт %v, баллы %d", req.Cmd, wd.carrying, wd.Score().Total)
	}
	obs := s.observation()
	delay := time.Duration(s.cfg.StepDelayMs) * time.Millisecond
	s.mu.Unlock()

	if req.Source != "ui" {
		time.Sleep(delay)
	}
	writeJSON(w, http.StatusOK, obs)
}

func (s *Server) handleScan(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	writeJSON(w, http.StatusOK, s.observation())
}

func (s *Server) handleMap(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.cfg.Satellite {
		badRequest(w, "satellite_disabled", "карта доступна только при satellite=true (режим «со спутником»)")
		return
	}
	wd := s.world
	cells := make([][]Cell, N)
	for y := 0; y < N; y++ {
		cells[y] = make([]Cell, N)
		for x := 0; x < N; x++ {
			c := wd.describe(y*N+x, 0, 0, true)
			c.RampUp, c.TubeAxis = "", "" // в абсолютной карте относительные поля не имеют смысла
			cells[y][x] = c
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"cells": cells, "pose": wd.Pose()})
}

func (s *Server) handleReference(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.cfg.Satellite {
		badRequest(w, "satellite_disabled", "эталонный маршрут доступен только при satellite=true")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"route": s.world.route, "sections": s.world.reference})
}

func (s *Server) handleTelemetry(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Data  map[string]any `json:"data"`
		Log   string         `json:"log"`
		Clear bool           `json:"clear"`
	}
	if err := readJSON(r, &req); err != nil {
		badRequest(w, "bad_json", err.Error())
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if req.Clear {
		s.telemetry = nil
	}
	for k, v := range req.Data {
		found := false
		for i := range s.telemetry {
			if s.telemetry[i].Key == k {
				s.telemetry[i].Value = v
				found = true
			}
		}
		if !found {
			s.telemetry = append(s.telemetry, telemetryItem{Key: k, Value: v})
		}
	}
	if req.Log != "" {
		s.logf("client", "%s", req.Log)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleEvents — long-poll: клиент ждёт нажатия кнопки «Новая итерация» в окне симулятора
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	timeout := 25 * time.Second
	if t, err := strconv.Atoi(r.URL.Query().Get("timeout")); err == nil && t >= 0 && t <= 60 {
		timeout = time.Duration(t) * time.Second
	}
	s.mu.Lock()
	s.lastPoll = time.Now().Add(timeout)
	s.mu.Unlock()

	var events []string
	select {
	case e := <-s.events:
		events = append(events, e)
	case <-time.After(timeout):
	case <-r.Context().Done():
		return
	}
	// забираем всё, что успело накопиться
	for len(events) < 16 {
		select {
		case e := <-s.events:
			events = append(events, e)
			continue
		default:
		}
		break
	}
	s.mu.Lock()
	s.lastPoll = time.Now()
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

func (s *Server) handleIterate(w http.ResponseWriter, r *http.Request) {
	select {
	case s.events <- "iteration":
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		badRequest(w, "queue_full", "клиент не успевает обрабатывать итерации")
	}
}

// handleState — полное состояние для окна симулятора (включая то, чего робот не видит)
func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	wd := s.world
	cells := make([]map[string]any, N*N)
	for c := range cells {
		cells[c] = map[string]any{"id": wd.base[c], "holder_color": wd.holderTube[c]}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"cells":            cells,
		"pose":             wd.Pose(),
		"sensor_cells":     wd.SensorAbs(s.cfg),
		"carrying":         append([]string{}, wd.carrying...),
		"sections":         wd.sections,
		"reference":        wd.reference,
		"route":            wd.route,
		"moves":            wd.moves,
		"score":            wd.Score(),
		"pattern_index":    wd.PatternIndex,
		"elapsed_s":        int(time.Since(wd.Started).Seconds()),
		"config":           s.cfg,
		"telemetry":        append([]telemetryItem{}, s.telemetry...),
		"logs":             append([]logLine{}, s.logs...),
		"last_error":       s.lastError,
		"client_connected": time.Since(s.lastPoll) < 2*time.Second || time.Now().Before(s.lastPoll),
	})
}

// handleCamera рендерит кадр камеры робота (view=robot) или «спутника» (view=satellite).
// format=png (по умолчанию) или raw — RGB-байты построчно, размеры в X-Width/X-Height.
func (s *Server) handleCamera(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	view := q.Get("view")
	s.mu.Lock()
	cfg := s.cfg
	if view == "satellite" && !cfg.Satellite && q.Get("source") != "ui" {
		s.mu.Unlock()
		badRequest(w, "satellite_disabled", "камера «спутника» доступна только при satellite=true")
		return
	}
	sc := s.world.buildScene(cfg.Scene, cfg.Lighting)
	cam := s.world.robotCamera(cfg.Camera)
	s.mu.Unlock()

	camCfg := cfg.Camera
	switch view {
	case "", "robot":
	case "satellite":
		cam = satelliteCamera()
		side := 480
		if v, err := strconv.Atoi(q.Get("size")); err == nil && v >= 64 && v <= 1920 {
			side = v
		}
		camCfg.Width, camCfg.Height, camCfg.K1 = side, side, 0
	default:
		badRequest(w, "bad_view", "view: robot или satellite")
		return
	}
	img := sc.render(cam, camCfg, rand.Uint64())

	w.Header().Set("X-Width", strconv.Itoa(camCfg.Width))
	w.Header().Set("X-Height", strconv.Itoa(camCfg.Height))
	w.Header().Set("Cache-Control", "no-store")
	if q.Get("format") == "raw" {
		w.Header().Set("Content-Type", "application/octet-stream")
		buf := make([]byte, 0, camCfg.Width*camCfg.Height*3)
		for i := 0; i < len(img.Pix); i += 4 {
			buf = append(buf, img.Pix[i], img.Pix[i+1], img.Pix[i+2])
		}
		w.Write(buf)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	enc.Encode(w, img)
}
