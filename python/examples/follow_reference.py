"""Проверка всей цепочки: робот проезжает эталонный маршрут поля в режиме спутника.
Должно получиться 55 баллов и ровно эталонное число секций.
Запуск: python python/examples/follow_reference.py [--wait] (--wait — по кнопке в окне)."""

import os
import sys

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from rrosim import Sim  # noqa: E402

N = 8
HEADINGS = ["north", "east", "south", "west"]  # по часовой стрелке


def direction(a: int, b: int) -> str:
    return {-N: "north", N: "south", 1: "east", -1: "west"}[b - a]


def drive(sim: Sim) -> dict:
    sim.reset(same=True)
    ref = sim.reference()
    pose = sim.scan().pose
    pos = pose["y"] * N + pose["x"]
    heading = pose["heading"]
    cells = sim.map()["cells"]
    terminal = {y * N + x for y in range(N) for x in range(N) if cells[y][x]["kind"] in ("tube", "holder")}

    for seg in ref["route"]:
        start = 0 if seg[0] == pos else 1
        for a, b in zip(seg[start:], seg[start + 1:]):
            want = direction(a, b)
            while heading != want:
                sim.right()
                heading = HEADINGS[(HEADINGS.index(heading) + 1) % 4]
            if b in terminal:
                obs = sim.pick() if cells[b // N][b % N]["kind"] == "tube" else sim.place()
            else:
                obs = sim.forward()
                pos = b
            sim.telemetry(sections=obs.sections, reference=ref["sections"], score=obs.score["total"])
    sim.log(f"эталон пройден: {obs.score['total']} баллов, {obs.sections} секций из {ref['sections']}")
    return {"score": obs.score["total"], "sections": obs.sections, "reference": ref["sections"]}


if __name__ == "__main__":
    sim = Sim()
    sim.configure(satellite=True)
    if "--wait" in sys.argv:
        sim.on_iteration(drive)
        sim.run()
    else:
        sim.configure(step_delay_ms=0)
        print(drive(sim))
