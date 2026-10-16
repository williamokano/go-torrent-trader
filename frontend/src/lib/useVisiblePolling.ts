import { useEffect, useRef } from "react";

/**
 * Calls `poll` now and then every `intervalMs`, but only while the page is
 * visible. A hidden tab skips its ticks, and catches up with a single call when
 * it is shown again — unless the last call is still fresh, so flicking between
 * tabs does not turn into a request per flick.
 *
 * Every signed-in member's header polls through this, so what it saves scales
 * with how many tabs are left open in the background.
 *
 * `poll` may change between renders; the latest one is always called, without
 * restarting the timer.
 */
export function useVisiblePolling(
  poll: () => void,
  intervalMs: number,
  enabled: boolean,
): void {
  const pollRef = useRef(poll);
  useEffect(() => {
    pollRef.current = poll;
  });

  useEffect(() => {
    if (!enabled) return;

    let lastRun = -Infinity;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const visible = () => document.visibilityState !== "hidden";

    // A timeout re-armed after every poll rather than a fixed interval, so a
    // catch-up poll on becoming visible restarts the cadence instead of being
    // followed seconds later by a tick that was already due.
    const schedule = () => {
      clearTimeout(timer);
      timer = setTimeout(tick, intervalMs);
    };
    const run = () => {
      lastRun = Date.now();
      pollRef.current();
      schedule();
    };
    const tick = () => {
      if (visible()) run();
      else schedule();
    };
    const onVisibilityChange = () => {
      if (visible() && Date.now() - lastRun >= intervalMs) run();
    };

    // In development, StrictMode mounts effects twice, so the first poll runs
    // twice there. That is React checking the cleanup below, not a bug.
    if (visible()) run();
    else schedule();
    document.addEventListener("visibilitychange", onVisibilityChange);

    return () => {
      clearTimeout(timer);
      document.removeEventListener("visibilitychange", onVisibilityChange);
    };
  }, [enabled, intervalMs]);
}
