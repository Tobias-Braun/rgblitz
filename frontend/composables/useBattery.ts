import { ref, computed, onScopeDispose } from "vue";

// Web Battery Status API has no official TS lib types; this is the minimal
// shape used here. Not all browsers implement it (see docs/02: "default to
// beautiful mode" when unsupported).
interface BatteryManagerLike extends EventTarget {
  level: number;
  charging: boolean;
}

type NavigatorWithBattery = Navigator & {
  getBattery?: () => Promise<BatteryManagerLike>;
};

// docs/02-ui-requirements.md §Battery UX: low battery = level <= 20%,
// or discharging with level <= 30%.
const LOW_BATTERY_LEVEL = 0.2;
const DISCHARGING_LOW_BATTERY_LEVEL = 0.3;

/**
 * Battery- and motion-preference-aware "fast mode" signal. Components should
 * disable shader/glow/count-up/transition animations when `isFastMode` is
 * true (docs/02-ui-requirements.md §Battery UX).
 */
export function useBattery() {
  const isLowBattery = ref(false);
  const prefersReducedMotion = ref(false);
  const isFastMode = computed(
    () => isLowBattery.value || prefersReducedMotion.value
  );

  const fakeIsBattery = ref(true);

  if (import.meta.client) {
    const nav = navigator as NavigatorWithBattery;

    if (typeof nav.getBattery === "function") {
      nav
        .getBattery()
        .then((battery) => {
          const evaluate = () => {
            isLowBattery.value =
              battery.level <= LOW_BATTERY_LEVEL ||
              (!battery.charging &&
                battery.level <= DISCHARGING_LOW_BATTERY_LEVEL);
          };
          evaluate();
          battery.addEventListener("levelchange", evaluate);
          battery.addEventListener("chargingchange", evaluate);
          onScopeDispose(() => {
            battery.removeEventListener("levelchange", evaluate);
            battery.removeEventListener("chargingchange", evaluate);
          });
        })
        .catch(() => {
          isLowBattery.value = false;
        });
    }

    const motionQuery = window.matchMedia("(prefers-reduced-motion: reduce)");
    prefersReducedMotion.value = motionQuery.matches;
    const onMotionChange = (event: MediaQueryListEvent) => {
      prefersReducedMotion.value = event.matches;
    };
    motionQuery.addEventListener("change", onMotionChange);
    onScopeDispose(() =>
      motionQuery.removeEventListener("change", onMotionChange)
    );
  }

  return { isLowBattery: fakeIsBattery, prefersReducedMotion, isFastMode };
}
