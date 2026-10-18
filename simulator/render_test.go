package simulator

import (
	"image/png"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"
)

func TestRobotFrameIsRightHandedOnField(t *testing.T) {
	w := newTestWorld(t, 3)
	for d, want := range map[int]vec3{north: {1, 0, 0}, east: {0, 1, 0}, south: {-1, 0, 0}, west: {0, -1, 0}} {
		w.heading = d
		if w.base[w.pos] >= 3 && w.base[w.pos] <= 6 {
			t.Skip("робот на пандусе")
		}
		_, _, right, _ := w.robotFrame()
		if right.sub(want).dot(right.sub(want)) > 1e-9 {
			t.Fatalf("курс %s: вправо %v, ожидалось %v", dirNames[d], right, want)
		}
	}
}

// Камера, смотрящая строго вниз над клеткой пола, видит перекрестие линий в центре кадра
func TestCameraSeesGridLineUnderRobot(t *testing.T) {
	w := newTestWorld(t, 3)
	for c := 0; c < N*N; c++ {
		if w.base[c] == 1 {
			w.pos = c
			break
		}
	}
	cfg := DefaultConfig()
	cfg.Camera = CameraConfig{Z: 300, Pitch: -90, FOV: 40, Width: 41, Height: 41, AA: 1}
	cfg.Lighting.Shadows = false
	img := w.buildScene(cfg.Scene, cfg.Lighting).render(w.robotCamera(cfg.Camera), cfg.Camera, 1)
	center := img.RGBAAt(20, 20)
	corner := img.RGBAAt(5, 5)
	if center.R > 60 || corner.R < 120 {
		t.Fatalf("в центре ожидалась чёрная линия (%v), в углу — светлый баннер (%v)", center, corner)
	}
}

func TestRenderWritesPreview(t *testing.T) {
	out := os.Getenv("RENDER_OUT")
	if out == "" {
		t.Skip("RENDER_OUT не задан")
	}
	w := newTestWorld(t, 11)
	cfg := DefaultConfig()
	sc := w.buildScene(cfg.Scene, cfg.Lighting)
	save := func(name string, cam camera, cc CameraConfig) {
		f, _ := os.Create(filepath.Join(out, name))
		png.Encode(f, sc.render(cam, cc, 1))
		f.Close()
	}
	sat := cfg.Camera
	sat.Width, sat.Height = 480, 480
	save("satellite.png", satelliteCamera(), sat)
	for d := 0; d < 4; d++ {
		w.heading = d
		save("robot_"+dirNames[d]+".png", w.robotCamera(cfg.Camera), cfg.Camera)
	}
}

func BenchmarkRenderRobotCamera(b *testing.B) {
	t := &testing.T{}
	w := newTestWorld(t, 11)
	cfg := DefaultConfig()
	for b.Loop() {
		w.buildScene(cfg.Scene, cfg.Lighting).render(w.robotCamera(cfg.Camera), cfg.Camera, 1)
	}
}

// Ускоряющая сетка не должна менять результат по сравнению с перебором всех объектов
func TestAccelMatchesBruteForce(t *testing.T) {
	w := newTestWorld(t, 21)
	sc := w.buildScene(DefaultScene(), DefaultLighting())
	brute := func(org, dir vec3) (float64, bool) {
		best, ok := math.Inf(1), false
		for _, o := range sc.objects {
			if tt, _, hit := o.intersect(org, dir, epsilon); hit && tt < best {
				best, ok = tt, true
			}
		}
		if dir.z < 0 {
			if tt := -org.z / dir.z; tt > epsilon && tt < best {
				best, ok = tt, true
			}
		}
		return best, ok
	}
	rng := newTestRand()
	for i := 0; i < 20000; i++ {
		org := vec3{rng.Float64()*3000 - 500, rng.Float64()*3000 - 500, rng.Float64() * 600}
		dir := vec3{rng.NormFloat64(), rng.NormFloat64(), rng.NormFloat64()}.norm()
		want, wok := brute(org, dir)
		got, gok := sc.trace(org, dir, math.Inf(1), false)
		if wok != gok || wok && math.Abs(want-got.t) > 1e-6 {
			t.Fatalf("луч %v %v: сетка %v/%v, перебор %v/%v", org, dir, got.t, gok, want, wok)
		}
	}
}

func newTestRand() *rand.Rand { return rand.New(rand.NewPCG(1, 2)) }

// Основание каждой зелёной подставки касается края баннера (красной границы)
func TestHoldersTouchBorder(t *testing.T) {
	for seed := uint64(0); seed < 50; seed++ {
		w := newTestWorld(t, seed)
		sc := w.buildScene(DefaultScene(), DefaultLighting())
		touching := 0
		for _, o := range sc.objects {
			if o.surf != surfGreen || o.maxB.z-o.minB.z > 5 { // только основания
				continue
			}
			if math.Abs(o.minB.x-fieldLow) < 1e-9 || math.Abs(o.maxB.x-fieldHigh) < 1e-9 ||
				math.Abs(o.minB.y-fieldLow) < 1e-9 || math.Abs(o.maxB.y-fieldHigh) < 1e-9 {
				touching++
			}
		}
		if touching != 6 { // 3 подставки × 2 опоры
			t.Fatalf("seed %d: у границы %d оснований из 6", seed, touching)
		}
	}
}
