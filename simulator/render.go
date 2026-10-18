package simulator

import (
	"image"
	"image/color"
	"math"
	"math/rand/v2"
	"runtime"
	"sync"
)

// Трассировка лучей по полю. Мировые координаты в мм: x — на восток (по столбцам),
// y — на юг (по строкам), z — вверх. Начало — центр перекрёстка (0,0) в северо-западном углу,
// центр перекрёстка (col,row) = (280·col, 280·row). Размеры — из п.15 регламента.

const (
	gridPitch     = 280.0
	fieldSize     = 2440.0
	fieldMargin   = (fieldSize - (N-1)*gridPitch) / 2 // поле симметрично вокруг сетки 8×8
	lineHalf      = 10.0                              // линии 20 мм
	borderWidth   = 80.0                              // красная граница вокруг баннера
	tierHeight    = 100.0                             // секция и пандус — 100 мм
	pieceHalf     = 150.0                             // секция и пандус 300×300
	tubeRadius    = 25.0
	tubeHalfLen   = 125.0
	circleRadius  = 40.0
	circleOffset  = 75.0
	satelliteZ    = 2800.0
	satelliteFOV  = 95.0
	rampSlope     = tierHeight / (2 * pieceHalf)
	fieldCenter   = (N - 1) * gridPitch / 2
	fieldLow      = -fieldMargin
	fieldHigh     = (N-1)*gridPitch + fieldMargin
	epsilon       = 1e-4
	defaultStandH = 60.0
)

// CameraConfig — камера робота. Смещение — от центра клетки по осям робота
// (вперёд, вправо, вверх от поверхности, на которой стоит робот), углы в градусах.
type CameraConfig struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Z      float64 `json:"z"`
	Pitch  float64 `json:"pitch"` // угол оси камеры к плоскости поля, отрицательный — вниз
	Yaw    float64 `json:"yaw"`   // поворот вокруг вертикали относительно курса, + — вправо
	Roll   float64 `json:"roll"`  // поворот вокруг оси камеры
	FOV    float64 `json:"fov"`   // горизонтальный угол обзора
	Width  int     `json:"width"`
	Height int     `json:"height"`
	AA     int     `json:"aa"`    // сглаживание: aa×aa лучей на пиксель
	Noise  float64 `json:"noise"` // СКО гауссова шума (0..1 от полной яркости)
	K1     float64 `json:"k1"`    // радиальная дисторсия: >0 — подушка, <0 — бочка
}

// Light — точечный источник света в мировых координатах (мм)
type Light struct {
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	Z         float64 `json:"z"`
	Intensity float64 `json:"intensity"`
}

type LightingConfig struct {
	Ambient  float64 `json:"ambient"`
	Lights   []Light `json:"lights"`
	Shadows  bool    `json:"shadows"`
	Specular float64 `json:"specular"` // множитель бликов (0 — матовые поверхности)
}

// SceneConfig — размеры, которых нет в регламенте явно
type SceneConfig struct {
	StandHeight  float64 `json:"stand_height"`  // от поверхности до низа трубы на белой подставке
	HolderHeight float64 `json:"holder_height"` // то же для зелёной подставки
}

func DefaultCamera() CameraConfig {
	return CameraConfig{X: 80, Z: 200, Pitch: -30, FOV: 70, Width: 320, Height: 240, AA: 2}
}

func DefaultLighting() LightingConfig {
	return LightingConfig{
		Ambient:  0.35,
		Lights:   []Light{{X: fieldCenter, Y: fieldCenter, Z: 2500, Intensity: 0.75}},
		Shadows:  true,
		Specular: 1,
	}
}

func DefaultScene() SceneConfig {
	return SceneConfig{StandHeight: defaultStandH, HolderHeight: defaultStandH}
}

// --- векторы ---

type vec3 struct{ x, y, z float64 }

func (a vec3) add(b vec3) vec3    { return vec3{a.x + b.x, a.y + b.y, a.z + b.z} }
func (a vec3) sub(b vec3) vec3    { return vec3{a.x - b.x, a.y - b.y, a.z - b.z} }
func (a vec3) mul(k float64) vec3 { return vec3{a.x * k, a.y * k, a.z * k} }
func (a vec3) dot(b vec3) float64 { return a.x*b.x + a.y*b.y + a.z*b.z }
func (a vec3) cross(b vec3) vec3 {
	return vec3{a.y*b.z - a.z*b.y, a.z*b.x - a.x*b.z, a.x*b.y - a.y*b.x}
}
func (a vec3) norm() vec3 { return a.mul(1 / math.Sqrt(a.dot(a))) }

// --- материалы ---

type surface int

const (
	surfWood surface = iota
	surfSectionTop
	surfRampTop
	surfTubeRed
	surfTubeBlue
	surfWhite
	surfGreen
)

var (
	colWood   = vec3{0.86, 0.74, 0.55}
	colFilm   = vec3{0.05, 0.05, 0.05}
	colTape   = vec3{0.88, 0.88, 0.88}
	colRed    = vec3{0.80, 0.12, 0.10} // RAL 3028
	colBlue   = vec3{0.12, 0.42, 0.72} // RAL 5015
	colWhite  = vec3{0.92, 0.92, 0.92}
	colGreen  = vec3{0.15, 0.78, 0.15}
	colBanner = vec3{0.82, 0.82, 0.84}
	colLine   = vec3{0.04, 0.04, 0.04}
	colBorder = vec3{0.85, 0.15, 0.08}
	colGround = vec3{0.28, 0.28, 0.30}
	colSky    = vec3{0.58, 0.60, 0.64}
)

type hit struct {
	t     float64
	n     vec3
	obj   *object
	floor bool
}

type plane struct {
	n vec3
	d float64 // точка внутри, если n·p <= d
}

type object struct {
	planes []plane // выпуклый многогранник; пусто — цилиндр
	surf   surface
	top    surface // материал верхней грани (для секций и пандусов)
	cx, cy float64 // центр клетки
	ramp   vec3    // для пандуса — направление подъёма в плоскости поля
	// цилиндр (труба): ось вдоль x (axisX) или y
	cyl   bool
	axisX bool
	c     vec3
	minB  vec3 // AABB для быстрого отсева
	maxB  vec3
}

func box(x0, y0, z0, x1, y1, z1 float64, s surface) *object {
	return &object{
		planes: []plane{
			{vec3{1, 0, 0}, x1}, {vec3{-1, 0, 0}, -x0},
			{vec3{0, 1, 0}, y1}, {vec3{0, -1, 0}, -y0},
			{vec3{0, 0, 1}, z1}, {vec3{0, 0, -1}, -z0},
		},
		surf: s, top: s,
		minB: vec3{x0, y0, z0}, maxB: vec3{x1, y1, z1},
	}
}

// wedge — пандус в клетке (cx,cy), поднимается в направлении a
func wedge(cx, cy float64, a vec3) *object {
	o := box(cx-pieceHalf, cy-pieceHalf, 0, cx+pieceHalf, cy+pieceHalf, tierHeight, surfWood)
	// z <= k·(a·(p-c) + 150)
	n := vec3{-rampSlope * a.x, -rampSlope * a.y, 1}
	d := rampSlope * (pieceHalf - (a.x*cx + a.y*cy))
	l := math.Sqrt(n.dot(n))
	o.planes[4] = plane{n.mul(1 / l), d / l}
	o.top, o.cx, o.cy, o.ramp = surfRampTop, cx, cy, a
	return o
}

func tube(c vec3, axisX bool, s surface) *object {
	o := &object{cyl: true, axisX: axisX, c: c, surf: s, top: s}
	ext := vec3{tubeRadius, tubeHalfLen, tubeRadius}
	if axisX {
		ext = vec3{tubeHalfLen, tubeRadius, tubeRadius}
	}
	o.minB, o.maxB = c.sub(ext), c.add(ext)
	return o
}

// stand — две опоры подставки (п.15.4, 15.5) вдоль оси трубы, верх на высоте base+h
func stand(cx, cy, base, h float64, axisX bool, s surface) []*object {
	var res []*object
	for _, side := range []float64{-45, 45} {
		px, py := cx, cy+side
		if axisX {
			px, py = cx+side, cy
		}
		res = append(res,
			box(px-35, py-35, base, px+35, py+35, base+4, s),     // основание 70×70
			box(px-7, py-7, base+4, px+7, py+7, base+h-6, s),     // стойка
			box(px-25, py-25, base+h-6, px+25, py+25, base+h, s), // ложемент под трубу
		)
	}
	return res
}

const holderBaseHalf = 35.0 // основание зелёной подставки 70×70 (п.15.4)

// holderCenter — центр зелёной подставки: на чёрной линии перекрёстка, вплотную к границе поля
func holderCenter(id uint8, cx, cy float64) (float64, float64) {
	switch id {
	case 17: // северная граница
		return cx, fieldLow + holderBaseHalf
	case 18: // южная
		return cx, fieldHigh - holderBaseHalf
	case 19: // восточная
		return fieldHigh - holderBaseHalf, cy
	}
	return fieldLow + holderBaseHalf, cy // западная
}

// --- сцена ---

// Ускоряющая сетка: объекты раскладываются по клеткам 280×280 (с запасом по клетке
// с каждой стороны поля), луч проходит только по клеткам на своём пути (DDA)
const (
	accelN    = N + 2
	accelLow  = -gridPitch/2 - gridPitch
	accelHigh = accelLow + accelN*gridPitch
)

type scene struct {
	objects []*object
	cells   [accelN * accelN][]*object
	zTop    float64 // выше этой высоты объектов нет
	light   LightingConfig
}

func (s *scene) add(objs ...*object) {
	for _, o := range objs {
		s.objects = append(s.objects, o)
		s.zTop = math.Max(s.zTop, o.maxB.z)
		x0 := int(math.Floor((o.minB.x - accelLow) / gridPitch))
		x1 := int(math.Floor((o.maxB.x - accelLow) / gridPitch))
		y0 := int(math.Floor((o.minB.y - accelLow) / gridPitch))
		y1 := int(math.Floor((o.maxB.y - accelLow) / gridPitch))
		for y := max(y0, 0); y <= min(y1, accelN-1); y++ {
			for x := max(x0, 0); x <= min(x1, accelN-1); x++ {
				s.cells[y*accelN+x] = append(s.cells[y*accelN+x], o)
			}
		}
	}
}

func cellCenter(c int) (float64, float64) {
	return float64(c%N) * gridPitch, float64(c/N) * gridPitch
}

var rampAscent = map[uint8]vec3{3: {0, -1, 0}, 4: {0, 1, 0}, 5: {1, 0, 0}, 6: {-1, 0, 0}}

func (w *World) buildScene(sc SceneConfig, light LightingConfig) *scene {
	s := &scene{light: light}
	for c := 0; c < N*N; c++ {
		id := w.base[c]
		cx, cy := cellCenter(c)
		var surfaceZ float64
		upper := id == 2 || id == 22 || id == 25 || id == 26 || id == 29 || id == 30
		if upper {
			o := box(cx-pieceHalf, cy-pieceHalf, 0, cx+pieceHalf, cy+pieceHalf, tierHeight, surfWood)
			o.top, o.cx, o.cy = surfSectionTop, cx, cy
			s.add(o)
			surfaceZ = tierHeight
		}
		if a, ok := rampAscent[id]; ok {
			s.add(wedge(cx, cy, a))
		}
		switch {
		case isTube(id) || id == 21 || id == 22:
			// 23/25/27/29 — подъезд с севера/юга, значит труба лежит вдоль x
			axisX := w.standAxisX[c]
			if isTube(id) {
				axisX = (id-23)%2 == 0
			}
			s.add(stand(cx, cy, surfaceZ, sc.StandHeight, axisX, surfWhite)...)
			if isTube(id) {
				col := surfTubeBlue
				if colorOf(id) == "red" {
					col = surfTubeRed
				}
				s.add(tube(vec3{cx, cy, surfaceZ + sc.StandHeight + tubeRadius}, axisX, col))
			}
		case isHolder(id):
			// Подставка стоит поперёк чёрной линии и примыкает к красной границе (п.2.3):
			// основание 70 мм касается края баннера, труба лежит вдоль границы
			axisX := id == 17 || id == 18
			hx, hy := holderCenter(id, cx, cy)
			s.add(stand(hx, hy, 0, sc.HolderHeight, axisX, surfGreen)...)
			if col := w.holderTube[c]; col != "" {
				surf := surfTubeBlue
				if col == "red" {
					surf = surfTubeRed
				}
				s.add(tube(vec3{hx, hy, sc.HolderHeight + tubeRadius}, axisX, surf))
			}
		}
	}
	return s
}

// --- пересечения ---

func (o *object) aabbHit(org, inv vec3, tmax float64) bool {
	t0, t1 := 0.0, tmax
	for i := 0; i < 3; i++ {
		var oo, iv, lo, hi float64
		switch i {
		case 0:
			oo, iv, lo, hi = org.x, inv.x, o.minB.x, o.maxB.x
		case 1:
			oo, iv, lo, hi = org.y, inv.y, o.minB.y, o.maxB.y
		default:
			oo, iv, lo, hi = org.z, inv.z, o.minB.z, o.maxB.z
		}
		a, b := (lo-oo)*iv, (hi-oo)*iv
		if a > b {
			a, b = b, a
		}
		t0, t1 = math.Max(t0, a), math.Min(t1, b)
		if t0 > t1 {
			return false
		}
	}
	return true
}

func (o *object) intersect(org, dir vec3, tmin float64) (float64, vec3, bool) {
	if o.cyl {
		return o.intersectTube(org, dir, tmin)
	}
	tNear, tFar := math.Inf(-1), math.Inf(1)
	var nNear vec3
	for _, p := range o.planes {
		denom := p.n.dot(dir)
		dist := p.d - p.n.dot(org)
		if math.Abs(denom) < 1e-12 {
			if dist < 0 {
				return 0, vec3{}, false
			}
			continue
		}
		t := dist / denom
		if denom < 0 {
			if t > tNear {
				tNear, nNear = t, p.n
			}
		} else if t < tFar {
			tFar = t
		}
		if tNear > tFar {
			return 0, vec3{}, false
		}
	}
	if tNear < tmin {
		return 0, vec3{}, false
	}
	return tNear, nNear, true
}

func (o *object) intersectTube(org, dir vec3, tmin float64) (float64, vec3, bool) {
	// переходим в систему, где ось трубы — u
	ou, ov, ow := org.y-o.c.y, org.x-o.c.x, org.z-o.c.z
	du, dv, dw := dir.y, dir.x, dir.z
	if o.axisX {
		ou, ov = org.x-o.c.x, org.y-o.c.y
		du, dv = dir.x, dir.y
	}
	best := math.Inf(1)
	var bn vec3
	a := dv*dv + dw*dw
	if a > 1e-12 {
		b := 2 * (ov*dv + ow*dw)
		c := ov*ov + ow*ow - tubeRadius*tubeRadius
		disc := b*b - 4*a*c
		if disc >= 0 {
			sq := math.Sqrt(disc)
			for _, t := range []float64{(-b - sq) / (2 * a), (-b + sq) / (2 * a)} {
				if t > tmin && t < best && math.Abs(ou+t*du) <= tubeHalfLen {
					best = t
					v, w := ov+t*dv, ow+t*dw
					if o.axisX {
						bn = vec3{0, v, w}.mul(1 / tubeRadius)
					} else {
						bn = vec3{v, 0, w}.mul(1 / tubeRadius)
					}
					break
				}
			}
		}
	}
	if math.Abs(du) > 1e-12 { // торцы
		for _, cap := range []float64{-tubeHalfLen, tubeHalfLen} {
			t := (cap - ou) / du
			v, w := ov+t*dv, ow+t*dw
			if t > tmin && t < best && v*v+w*w <= tubeRadius*tubeRadius {
				best = t
				if o.axisX {
					bn = vec3{math.Copysign(1, cap), 0, 0}
				} else {
					bn = vec3{0, math.Copysign(1, cap), 0}
				}
			}
		}
	}
	if math.IsInf(best, 1) {
		return 0, vec3{}, false
	}
	return best, bn, true
}

// slab сужает [t0,t1] до участка луча, где координата между lo и hi
func slab(o, d, lo, hi, t0, t1 float64) (float64, float64) {
	if math.Abs(d) < 1e-12 {
		if o < lo || o > hi {
			return 1, 0
		}
		return t0, t1
	}
	a, b := (lo-o)/d, (hi-o)/d
	if a > b {
		a, b = b, a
	}
	return math.Max(t0, a), math.Min(t1, b)
}

func (s *scene) trace(org, dir vec3, tmax float64, anyHit bool) (hit, bool) {
	best := hit{t: tmax}
	found := false
	// пол заслоняет всё, что за ним
	if dir.z < 0 {
		if t := -org.z / dir.z; t > epsilon && t < best.t {
			best = hit{t: t, n: vec3{0, 0, 1}, floor: true}
			found = !anyHit
		}
	}
	t0, t1 := slab(org.z, dir.z, 0, s.zTop, epsilon, best.t)
	t0, t1 = slab(org.x, dir.x, accelLow, accelHigh-1e-6, t0, t1)
	t0, t1 = slab(org.y, dir.y, accelLow, accelHigh-1e-6, t0, t1)
	if t0 > t1 {
		return best, found
	}

	inv := vec3{1 / dir.x, 1 / dir.y, 1 / dir.z}
	p := org.add(dir.mul(t0))
	ix := min(max(int((p.x-accelLow)/gridPitch), 0), accelN-1)
	iy := min(max(int((p.y-accelLow)/gridPitch), 0), accelN-1)
	stepX, stepY := 1, 1
	nextX, nextY := math.Inf(1), math.Inf(1)
	dtX, dtY := math.Abs(gridPitch*inv.x), math.Abs(gridPitch*inv.y)
	if dir.x > 0 {
		nextX = (accelLow + float64(ix+1)*gridPitch - org.x) * inv.x
	} else if dir.x < 0 {
		stepX = -1
		nextX = (accelLow + float64(ix)*gridPitch - org.x) * inv.x
	}
	if dir.y > 0 {
		nextY = (accelLow + float64(iy+1)*gridPitch - org.y) * inv.y
	} else if dir.y < 0 {
		stepY = -1
		nextY = (accelLow + float64(iy)*gridPitch - org.y) * inv.y
	}
	for {
		cellExit := math.Min(math.Min(nextX, nextY), t1)
		for _, o := range s.cells[iy*accelN+ix] {
			if !o.aabbHit(org, inv, best.t) {
				continue
			}
			if t, n, ok := o.intersect(org, dir, epsilon); ok && t < best.t {
				best, found = hit{t: t, n: n, obj: o}, true
				if anyHit {
					return best, true
				}
			}
		}
		if found && !best.floor && best.t <= cellExit {
			return best, true
		}
		if cellExit >= t1 {
			return best, found
		}
		if nextX < nextY {
			ix += stepX
			nextX += dtX
		} else {
			iy += stepY
			nextY += dtY
		}
		if ix < 0 || ix >= accelN || iy < 0 || iy >= accelN {
			return best, found
		}
	}
}

// --- текстуры ---

func nearLine(v float64) bool {
	k := math.Round(v / gridPitch)
	return math.Abs(v-k*gridPitch) < lineHalf
}

func floorColor(p vec3) (vec3, float64, float64) {
	in := func(lo, hi float64) bool { return p.x >= lo && p.x <= hi && p.y >= lo && p.y <= hi }
	switch {
	case in(fieldLow, fieldHigh):
		if nearLine(p.x) || nearLine(p.y) {
			return colLine, 0.1, 10
		}
		return colBanner, 0.1, 10
	case in(fieldLow-borderWidth, fieldHigh+borderWidth):
		return colBorder, 0.1, 10
	}
	return colGround, 0, 1
}

// albedo возвращает цвет, силу блика и блеск в точке p поверхности объекта
func (o *object) albedo(p, n vec3) (vec3, float64, float64) {
	isTop := n.z > 0.5
	switch {
	case o.surf == surfTubeRed:
		return colRed, 0.4, 30
	case o.surf == surfTubeBlue:
		return colBlue, 0.4, 30
	case o.surf == surfWhite:
		return colWhite, 0.2, 20
	case o.surf == surfGreen:
		return colGreen, 0.2, 20
	case isTop && o.top == surfSectionTop:
		if math.Abs(p.x-o.cx) < lineHalf || math.Abs(p.y-o.cy) < lineHalf {
			return colTape, 0.25, 60
		}
		return colFilm, 0.25, 60
	case isTop && o.top == surfRampTop:
		a := o.ramp
		right := vec3{-a.y, a.x, 0} // правая сторона, если смотреть на подъём
		d := p.sub(vec3{o.cx, o.cy, 0})
		along := (d.x*a.x + d.y*a.y) * math.Sqrt(1+rampSlope*rampSlope)
		across := d.x*right.x + d.y*right.y
		if along*along+(across+circleOffset)*(across+circleOffset) < circleRadius*circleRadius {
			return colRed, 0.3, 40 // красный круг слева (как в assets/3.png)
		}
		if along*along+(across-circleOffset)*(across-circleOffset) < circleRadius*circleRadius {
			return colBlue, 0.3, 40
		}
		if math.Abs(across) < lineHalf {
			return colTape, 0.25, 60
		}
		return colFilm, 0.25, 60
	}
	return colWood, 0.05, 8
}

func (s *scene) shade(org, dir vec3) vec3 {
	h, ok := s.trace(org, dir, math.Inf(1), false)
	if !ok {
		// стены зала: чуть светлее выше горизонта
		return colSky.mul(0.8 + 0.2*math.Max(0, dir.z))
	}
	p := org.add(dir.mul(h.t))
	n := h.n
	var alb vec3
	var ks, shin float64
	if h.floor {
		alb, ks, shin = floorColor(p)
	} else {
		alb, ks, shin = h.obj.albedo(p, n)
	}
	col := alb.mul(s.light.Ambient)
	view := dir.mul(-1)
	for _, l := range s.light.Lights {
		lp := vec3{l.X, l.Y, l.Z}
		toL := lp.sub(p)
		dist := math.Sqrt(toL.dot(toL))
		ld := toL.mul(1 / dist)
		ndl := n.dot(ld)
		if ndl <= 0 {
			continue
		}
		if s.light.Shadows {
			if _, blocked := s.trace(p.add(n.mul(0.05)), ld, dist, true); blocked {
				continue
			}
		}
		col = col.add(alb.mul(l.Intensity * ndl))
		if ks > 0 && s.light.Specular > 0 {
			hv := ld.add(view).norm()
			sp := math.Pow(math.Max(0, n.dot(hv)), shin) * ks * s.light.Specular * l.Intensity
			col = col.add(vec3{sp, sp, sp})
		}
	}
	return col
}

// --- камера ---

type camera struct {
	org, fwd, right, up vec3
	fov                 float64
}

// robotFrame — центр клетки на поверхности и оси робота (вперёд, вправо, вверх).
// На пандусе робот стоит посередине и наклонён вместе с ним.
func (w *World) robotFrame() (vec3, vec3, vec3, vec3) {
	cx, cy := cellCenter(w.pos)
	id := w.base[w.pos]
	h := vec3{float64(dirDX[w.heading]), float64(dirDY[w.heading]), 0}
	up := vec3{0, 0, 1}
	z := 0.0
	if id == 2 {
		z = tierHeight
	}
	if a, ok := rampAscent[id]; ok {
		z = tierHeight / 2
		up = vec3{-rampSlope * a.x, -rampSlope * a.y, 1}.norm()
	}
	fwd := h.sub(up.mul(h.dot(up))).norm()
	right := up.cross(fwd) // x — восток, y — юг, z — вверх
	return vec3{cx, cy, z}, fwd, right, up
}

func (w *World) robotCamera(c CameraConfig) camera {
	base, fwd, right, up := w.robotFrame()
	org := base.add(fwd.mul(c.X)).add(right.mul(c.Y)).add(up.mul(c.Z))
	rad := math.Pi / 180
	yaw, pitch, roll := c.Yaw*rad, c.Pitch*rad, c.Roll*rad
	f1 := fwd.mul(math.Cos(yaw)).add(right.mul(math.Sin(yaw)))
	r1 := right.mul(math.Cos(yaw)).sub(fwd.mul(math.Sin(yaw)))
	dir := f1.mul(math.Cos(pitch)).add(up.mul(math.Sin(pitch)))
	camUp := up.mul(math.Cos(pitch)).sub(f1.mul(math.Sin(pitch)))
	camRight := r1.mul(math.Cos(roll)).sub(camUp.mul(math.Sin(roll)))
	camUp = camUp.mul(math.Cos(roll)).add(r1.mul(math.Sin(roll)))
	return camera{org: org, fwd: dir, right: camRight, up: camUp, fov: c.FOV}
}

func satelliteCamera() camera {
	return camera{
		org: vec3{fieldCenter, fieldCenter, satelliteZ},
		fwd: vec3{0, 0, -1}, right: vec3{1, 0, 0}, up: vec3{0, -1, 0}, // север сверху
		fov: satelliteFOV,
	}
}

func (s *scene) render(cam camera, cfg CameraConfig, seed uint64) *image.RGBA {
	W, H := cfg.Width, cfg.Height
	img := image.NewRGBA(image.Rect(0, 0, W, H))
	tanX := math.Tan(cam.fov * math.Pi / 360)
	tanY := tanX * float64(H) / float64(W)
	aa := max(cfg.AA, 1)

	rows := make(chan int, H)
	for y := 0; y < H; y++ {
		rows <- y
	}
	close(rows)
	var wg sync.WaitGroup
	for wk := 0; wk < runtime.NumCPU(); wk++ {
		wg.Add(1)
		go func(wk int) {
			defer wg.Done()
			rng := rand.New(rand.NewPCG(seed, uint64(wk)))
			for py := range rows {
				for px := 0; px < W; px++ {
					var sum vec3
					for sy := 0; sy < aa; sy++ {
						for sx := 0; sx < aa; sx++ {
							u := 2*(float64(px)+(float64(sx)+0.5)/float64(aa))/float64(W) - 1
							v := 2*(float64(py)+(float64(sy)+0.5)/float64(aa))/float64(H) - 1
							if cfg.K1 != 0 {
								f := 1 + cfg.K1*(u*u+v*v)
								u, v = u*f, v*f
							}
							dir := cam.fwd.add(cam.right.mul(u * tanX)).sub(cam.up.mul(v * tanY)).norm()
							sum = sum.add(s.shade(cam.org, dir))
						}
					}
					c := sum.mul(1 / float64(aa*aa))
					if cfg.Noise > 0 {
						c = c.add(vec3{rng.NormFloat64(), rng.NormFloat64(), rng.NormFloat64()}.mul(cfg.Noise))
					}
					img.SetRGBA(px, py, color.RGBA{to8(c.x), to8(c.y), to8(c.z), 255})
				}
			}
		}(wk)
	}
	wg.Wait()
	return img
}

func to8(v float64) uint8 {
	return uint8(math.Round(math.Min(1, math.Max(0, v)) * 255))
}
