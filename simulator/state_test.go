package simulator

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

// У края поля, когда все клетки датчиков за полем, состояние для окна не должно
// содержать null вместо списков: иначе окно перестаёт рисовать робота
func TestStateHasNoNullListsAtFieldEdge(t *testing.T) {
	srv, err := NewServer(fstest.MapFS{})
	if err != nil {
		t.Fatal(err)
	}
	srv.cfg.SensorCells = [][2]int{{1, -1}, {1, 1}}
	srv.world.pos, srv.world.heading = N*N-N, south // левый нижний угол, лицом за поле
	if got := srv.world.SensorAbs(srv.cfg); len(got) != 0 {
		t.Fatalf("ожидались датчики за полем, получено %v", got)
	}

	for _, path := range []string{"/api/state", "/api/scan"} {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"sensor_cells", "carrying", "telemetry", "logs", "scan", "cells"} {
			if v, ok := body[key]; ok && v == nil {
				t.Errorf("%s: поле %q пришло как null", path, key)
			}
		}
	}
}
