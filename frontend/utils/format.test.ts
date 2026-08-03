import { describe, expect, it } from "vitest";
import { formatCompactCount } from "./format";

describe("formatCompactCount", () => {
  it("renders sub-1000 values exactly", () => {
    expect(formatCompactCount(0)).toBe("0");
    expect(formatCompactCount(1)).toBe("1");
    expect(formatCompactCount(999)).toBe("999");
  });

  it("abbreviates thousands with one decimal, staying at 4 chars", () => {
    expect(formatCompactCount(1000)).toBe("1.0K");
    expect(formatCompactCount(1074)).toBe("1.1K");
    expect(formatCompactCount(9949)).toBe("9.9K");
  });

  it("drops the decimal once the K value hits double digits", () => {
    expect(formatCompactCount(12_345)).toBe("12K");
    expect(formatCompactCount(999_000)).toBe("999K");
  });

  it("escalates to the next unit when rounding would overflow 3 digits", () => {
    // 999.5K rounds to "1000K" pre-escalation; must bump to 1.0M instead.
    expect(formatCompactCount(999_500)).toBe("1.0M");
  });

  it("abbreviates millions and billions the same way", () => {
    expect(formatCompactCount(1_500_000)).toBe("1.5M");
    expect(formatCompactCount(25_000_000)).toBe("25M");
    expect(formatCompactCount(2_000_000_000)).toBe("2.0B");
  });

  it("every output is at most 4 characters", () => {
    for (const n of [1, 999, 1000, 1074, 9999, 12_345, 999_500, 999_999, 1_000_000, 25_000_000, 2_000_000_000]) {
      expect(formatCompactCount(n).length).toBeLessThanOrEqual(4);
    }
  });
});
