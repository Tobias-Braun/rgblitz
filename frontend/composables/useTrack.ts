type EventData = Record<string, string | number | boolean | undefined>;

declare global {
  interface Window {
    umami?: {
      track: (name: string, data?: EventData) => void;
      identify: (data: EventData) => void;
    };
  }
}

/**
 * Thin wrapper around the self-hosted Umami tracker. Every call is a silent
 * no-op during SSR, in local dev (no script configured) and when the script is
 * blocked. Event payloads must never contain player names, IDs or search
 * queries — see docs/07-analytics.md.
 */
export function useTrack() {
  function track(name: string, data?: EventData) {
    if (import.meta.client) window.umami?.track(name, data);
  }

  // The tracker is loaded with `defer`, so it may not exist yet at mount.
  function identify(data: EventData) {
    if (!import.meta.client) return;
    if (window.umami) return window.umami.identify(data);
    document
      .querySelector("script[data-website-id]")
      ?.addEventListener("load", () => window.umami?.identify(data), { once: true });
  }

  return { track, identify };
}
