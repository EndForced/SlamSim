package simulator

import "testing"

func TestMergeConfigCameras(t *testing.T) {
	cur := DefaultConfig()
	cfg, err := mergeConfig(cur, []byte(`{"cameras": {"rear": {"yaw": 180}, "down": {"pitch": -90, "fov": 50}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.validate(); err != nil {
		t.Fatal(err)
	}
	rear := cfg.Cameras["rear"]
	if rear.Yaw != 180 || rear.FOV != DefaultCamera().FOV || rear.Width != DefaultCamera().Width {
		t.Fatalf("новая камера должна начинаться с настроек по умолчанию: %+v", rear)
	}
	if len(cur.Cameras) != 0 {
		t.Fatal("слияние изменило исходный конфиг")
	}

	// частичное изменение одной камеры не трогает её остальные поля и другие камеры
	cfg2, err := mergeConfig(cfg, []byte(`{"cameras": {"rear": {"z": 300}}, "capacity": 2}`))
	if err != nil {
		t.Fatal(err)
	}
	if r := cfg2.Cameras["rear"]; r.Yaw != 180 || r.Z != 300 {
		t.Fatalf("частичное изменение камеры: %+v", r)
	}
	if d := cfg2.Cameras["down"]; d.Pitch != -90 || d.FOV != 50 {
		t.Fatalf("другая камера изменилась: %+v", d)
	}
	if cfg2.Capacity != 2 || cfg.Cameras["rear"].Z == 300 {
		t.Fatal("слияние должно возвращать новый конфиг, не меняя предыдущий")
	}

	// null удаляет камеру, основная камера меняется через "camera"
	cfg3, err := mergeConfig(cfg2, []byte(`{"cameras": {"down": null}, "camera": {"pitch": -10}}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg3.Cameras["down"]; ok {
		t.Fatal("камера не удалена")
	}
	if cfg3.Camera.Pitch != -10 || cfg3.Camera.FOV != DefaultCamera().FOV {
		t.Fatalf("основная камера: %+v", cfg3.Camera)
	}

	// неверное имя и неизвестное поле — ошибки
	bad, _ := mergeConfig(cur, []byte(`{"cameras": {"main": {}}}`))
	if bad.validate() == nil {
		t.Fatal("имя main для дополнительной камеры должно быть запрещено")
	}
	if _, err := mergeConfig(cur, []byte(`{"cameras": {"x": {"foo": 1}}}`)); err == nil {
		t.Fatal("неизвестное поле камеры должно быть ошибкой")
	}
}

// Камеры с разными настройками дают разные кадры одного и того же поля
func TestDifferentCamerasRenderDifferently(t *testing.T) {
	w := newTestWorld(t, 4)
	cfg := DefaultConfig()
	cfg.Camera.Width, cfg.Camera.Height, cfg.Camera.AA = 40, 30, 1
	rear := cfg.Camera
	rear.Yaw = 180
	sc := w.buildScene(cfg.Scene, cfg.Lighting)
	a := sc.render(w.robotCamera(cfg.Camera), cfg.Camera, 1)
	b := sc.render(w.robotCamera(rear), rear, 1)
	diff := 0
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			diff++
		}
	}
	if diff == 0 {
		t.Fatal("камеры вперёд и назад дали одинаковый кадр")
	}
}
