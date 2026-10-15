import { describe, expect, it } from "vitest";
import { deltaBucket, resultState } from "./analytics";
import type { Game, TodayResultOk } from "~/types/api";

const player = { id: "1", name: "", club_name: "", avatar_url: "", club_logo_url: "", rgx: 1000 };
const ok = (extra: Partial<TodayResultOk> = {}): TodayResultOk => ({
  status: "ok",
  player,
  date: "2026-10-17",
  games: [{} as Game],
  ...extra,
});

describe("deltaBucket", () => {
  it("reports exactly zero on its own", () => {
    expect(deltaBucket(0)).toBe("0");
  });

  it("buckets positive deltas with an inclusive lower bound", () => {
    expect(deltaBucket(4.5)).toBe("0..+10");
    expect(deltaBucket(10)).toBe("+10..+20");
    expect(deltaBucket(19.5)).toBe("+10..+20");
  });

  it("buckets negative deltas with an inclusive lower bound", () => {
    expect(deltaBucket(-0.5)).toBe("-10..0");
    expect(deltaBucket(-10)).toBe("-10..0");
    expect(deltaBucket(-10.5)).toBe("-20..-10");
  });
});

describe("resultState", () => {
  it("maps a missing result to error", () => {
    expect(resultState(null)).toBe("error");
  });

  it("maps every result shape to its view", () => {
    expect(resultState({ status: "no_tournament", player })).toBe("no_tournament");
    expect(resultState(ok({ stale: true }))).toBe("stale");
    expect(resultState(ok({ games: undefined }))).toBe("no_games");
    expect(resultState(ok())).toBe("ok");
  });
});
