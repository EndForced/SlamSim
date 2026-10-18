"""Пример: одна итерация = скан и один шаг. Запуск: python python/examples/explore.py
(из корня репозитория, при запущенном симуляторе)."""

import os
import sys

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from rrosim import IllegalCommand, Sim  # noqa: E402

sim = Sim()
sim.configure(sensor_cells=[(1, 0), (2, 0), (1, -1), (1, 1), (0, -1), (0, 1)], satellite=False)
sim.telemetry(_clear=True, mode="explore")


@sim.on_iteration
def step(sim: Sim):
    obs = sim.scan()
    front = obs.front
    sim.telemetry(front=front.kind if front else "?", carrying=obs.carrying, sections=obs.sections)

    if front and front.kind == "tube" and front.tube_axis == "across" and not obs.carrying:
        try:
            sim.pick()
            sim.log(f"взял {front.color} трубу")
            return
        except IllegalCommand as e:
            sim.log(f"не взял трубу: {e.message}")
    if front and front.kind == "holder" and obs.carrying and not front.color:
        try:
            sim.place()
            sim.log("положил трубу")
            return
        except IllegalCommand as e:
            sim.log(f"не положил: {e.message}")
    try:
        sim.forward()
    except IllegalCommand as e:
        sim.log(f"вперёд нельзя ({e.code}), поворачиваю")
        sim.right()


sim.run()
