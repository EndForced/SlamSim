"""Клиент симулятора «Ремонт труб» (РРО 2026).

Пример:
    from rrosim import Sim, IllegalCommand

    sim = Sim()                                   # http://127.0.0.1:8080
    sim.configure(sensor_cells=[(1, 0), (1, -1), (1, 1)])

    @sim.on_iteration                             # кнопка «Новая итерация» в окне
    def step(sim):
        obs = sim.scan()
        sim.telemetry(front=obs.front.kind)
        try:
            sim.forward()
        except IllegalCommand as e:
            sim.log(f"не проехал: {e}")
            sim.right()

    sim.run()
"""

from __future__ import annotations

import json
import time
import traceback
import urllib.error
import urllib.request
from dataclasses import dataclass, field
from typing import Any, Callable, Optional

__all__ = ["Sim", "Cell", "Observation", "SimError", "IllegalCommand"]


class SimError(Exception):
    """Ошибка API (неверные настройки, сервер недоступен и т.п.)."""

    def __init__(self, code: str, message: str):
        super().__init__(f"{code}: {message}")
        self.code = code
        self.message = message


class IllegalCommand(SimError):
    """Нелегальная команда робота. Состояние симулятора не изменилось.

    code: out_of_field, blocked, no_passage, no_tube, wrong_approach,
          capacity, no_holder, holder_busy, empty, no_such_color, unknown_command
    """


@dataclass
class Cell:
    """Клетка в системе координат робота: f — вперёд, r — вправо."""

    f: int
    r: int
    kind: str  # out, floor, section, ramp, tube, stand, holder
    level: Optional[int] = None  # 1 — нижний ярус, 2 — верхний
    color: Optional[str] = None  # red/blue: труба или труба на зелёной подставке
    ramp_up: Optional[str] = None  # forward/backward/left/right — куда пандус поднимается
    tube_axis: Optional[str] = None  # across — к трубе можно подъехать с текущим курсом
    id: Optional[int] = None  # сырой ID клетки, только в режиме спутника

    @classmethod
    def from_json(cls, d: dict) -> "Cell":
        return cls(**{k: d.get(k) for k in cls.__dataclass_fields__})


@dataclass
class Observation:
    scan: list[Cell]
    carrying: list[str]
    sections: int
    score: dict
    pose: Optional[dict] = None  # только в режиме спутника
    raw: dict = field(default_factory=dict, repr=False)

    def at(self, f: int, r: int) -> Optional[Cell]:
        """Клетка по смещению (вперёд, вправо), если она есть в настройках датчиков."""
        for c in self.scan:
            if c.f == f and c.r == r:
                return c
        return None

    @property
    def front(self) -> Optional[Cell]:
        return self.at(1, 0)

    @classmethod
    def from_json(cls, d: dict) -> "Observation":
        return cls(
            scan=[Cell.from_json(c) for c in d.get("scan", [])],
            carrying=d.get("carrying", []),
            sections=d.get("sections", 0),
            score=d.get("score", {}),
            pose=d.get("pose"),
            raw=d,
        )


class Sim:
    def __init__(self, url: str = "http://127.0.0.1:8080", timeout: float = 70):
        self.url = url.rstrip("/")
        self.timeout = timeout
        self._iteration: Optional[Callable[["Sim"], Any]] = None

    # --- транспорт ---

    def _request(self, method: str, path: str, body: Any = None, command: bool = False, raw: bool = False) -> Any:
        data = None if body is None else json.dumps(body).encode()
        req = urllib.request.Request(self.url + path, data=data, method=method,
                                     headers={"Content-Type": "application/json"})
        try:
            with urllib.request.urlopen(req, timeout=self.timeout) as resp:
                if raw:
                    return resp.read(), resp.headers
                return json.loads(resp.read())
        except urllib.error.HTTPError as e:
            try:
                err = json.loads(e.read())
            except ValueError:
                raise SimError("http_%d" % e.code, e.reason) from None
            cls = IllegalCommand if command else SimError
            raise cls(err.get("error", "error"), err.get("message", "")) from None
        except urllib.error.URLError as e:
            raise SimError("unavailable", f"симулятор недоступен по {self.url}: {e.reason}") from None

    def _cmd(self, cmd: str, **extra) -> Observation:
        return Observation.from_json(self._request("POST", "/api/cmd", {"cmd": cmd, **extra}, command=True))

    # --- настройки и поле ---

    def configure(self, sensor_cells=None, satellite=None, capacity=None, step_delay_ms=None,
                  camera: Optional[dict] = None, lighting: Optional[dict] = None,
                  scene: Optional[dict] = None) -> dict:
        """Меняет только переданные настройки. sensor_cells — список (вперёд, вправо).

        camera:   x, y, z (мм от центра клетки: вперёд, вправо, вверх), pitch (к плоскости поля,
                  <0 — вниз), yaw (+ — вправо), roll, fov (горизонтальный, градусы),
                  width, height, aa (сглаживание 1..4), noise (0..1), k1 (дисторсия)
        lighting: ambient, shadows, specular, lights=[{"x", "y", "z", "intensity"}] — мм в координатах
                  поля: x на восток, y на юг, z вверх, (0,0) — центр северо-западного перекрёстка,
                  перекрёсток (col,row) = (280·col, 280·row)
        scene:    stand_height, holder_height — высота низа трубы над поверхностью, мм
        """
        body: dict = {}
        if camera is not None:
            body["camera"] = camera
        if lighting is not None:
            body["lighting"] = lighting
        if scene is not None:
            body["scene"] = scene
        if sensor_cells is not None:
            body["sensor_cells"] = [list(c) for c in sensor_cells]
        if satellite is not None:
            body["satellite"] = satellite
        if capacity is not None:
            body["capacity"] = capacity
        if step_delay_ms is not None:
            body["step_delay_ms"] = step_delay_ms
        return self._request("POST", "/api/config", body)

    def config(self) -> dict:
        return self._request("GET", "/api/config")

    def reset(self, index: Optional[int] = None, same: bool = False) -> Observation:
        """Новое случайное поле из data/pats.bin; index — конкретное поле, same — перезапуск текущего."""
        body: dict = {"same": same}
        if index is not None:
            body["index"] = index
        return Observation.from_json(self._request("POST", "/api/reset", body))

    # --- робот ---

    def scan(self) -> Observation:
        return Observation.from_json(self._request("GET", "/api/scan"))

    def forward(self) -> Observation:
        return self._cmd("forward")

    def backward(self) -> Observation:
        return self._cmd("backward")

    def left(self) -> Observation:
        return self._cmd("left")

    def right(self) -> Observation:
        return self._cmd("right")

    def pick(self) -> Observation:
        """Снять трубу с белой подставки прямо перед роботом."""
        return self._cmd("pick")

    def place(self, color: Optional[str] = None) -> Observation:
        """Положить трубу на зелёную подставку перед роботом (по умолчанию — последнюю взятую)."""
        return self._cmd("place", **({"color": color} if color else {}))

    def camera(self, view: str = "robot", size: Optional[int] = None) -> bytes:
        """PNG-кадр камеры робота (view="robot") или «спутника» (view="satellite", нужен satellite=True)."""
        path = f"/api/camera?view={view}" + (f"&size={size}" if size else "")
        return self._request("GET", path, raw=True)[0]

    def camera_array(self, view: str = "robot", size: Optional[int] = None):
        """Кадр как numpy-массив H×W×3 (uint8, RGB). Требует numpy."""
        import numpy as np

        path = f"/api/camera?view={view}&format=raw" + (f"&size={size}" if size else "")
        data, headers = self._request("GET", path, raw=True)
        h, w = int(headers["X-Height"]), int(headers["X-Width"])
        return np.frombuffer(data, dtype=np.uint8).reshape(h, w, 3)

    def save_camera(self, path: str, view: str = "robot", size: Optional[int] = None) -> None:
        with open(path, "wb") as f:
            f.write(self.camera(view, size))

    def map(self) -> dict:
        """Карта целиком — только в режиме спутника (configure(satellite=True))."""
        return self._request("GET", "/api/map")

    def reference(self) -> dict:
        """Эталонный маршрут поля — для отладки, только в режиме спутника."""
        return self._request("GET", "/api/debug/reference")

    # --- телеметрия ---

    def telemetry(self, _clear: bool = False, **values) -> None:
        """Показать значения в окне симулятора: sim.telemetry(state="search", target=(3, 4))."""
        self._request("POST", "/api/telemetry", {"data": values, "clear": _clear})

    def log(self, text: str) -> None:
        self._request("POST", "/api/telemetry", {"log": str(text)})

    # --- кнопка «Новая итерация» ---

    def on_iteration(self, fn: Callable[["Sim"], Any]) -> Callable[["Sim"], Any]:
        """Декоратор: fn(sim) вызывается при каждом нажатии кнопки в окне."""
        self._iteration = fn
        return fn

    def run(self) -> None:
        """Ждёт нажатий кнопки и вызывает обработчик. Ошибки обработчика пишутся в журнал окна."""
        if self._iteration is None:
            raise RuntimeError("сначала зарегистрируйте обработчик через @sim.on_iteration")
        print(f"Жду кнопку «Новая итерация» в окне {self.url} (Ctrl+C — выход)")
        while True:
            try:
                events = self._request("GET", "/api/events?timeout=25").get("events", [])
            except SimError as e:
                print(e)
                time.sleep(1)
                continue
            for ev in events:
                if ev != "iteration":
                    continue
                try:
                    self._iteration(self)
                except Exception as e:  # noqa: BLE001 — показываем любую ошибку в окне и ждём дальше
                    traceback.print_exc()
                    try:
                        self.log(f"Ошибка в обработчике итерации: {e!r}")
                    except SimError:
                        pass
