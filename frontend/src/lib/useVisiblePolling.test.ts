import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { renderHook } from "@testing-library/react";
import { useVisiblePolling } from "./useVisiblePolling";

const MINUTE = 60_000;
let visibility: DocumentVisibilityState = "visible";

function setVisibility(state: DocumentVisibilityState) {
  visibility = state;
  document.dispatchEvent(new Event("visibilitychange"));
}

beforeEach(() => {
  vi.useFakeTimers();
  visibility = "visible";
  Object.defineProperty(document, "visibilityState", {
    configurable: true,
    get: () => visibility,
  });
});

afterEach(() => {
  vi.useRealTimers();
  // Drop the getter so later tests in this worker see jsdom's own property.
  delete (document as { visibilityState?: unknown }).visibilityState;
});

describe("useVisiblePolling", () => {
  test("polls at once and then on every interval", () => {
    const poll = vi.fn();
    renderHook(() => useVisiblePolling(poll, 5 * MINUTE, true));
    expect(poll).toHaveBeenCalledTimes(1);

    vi.advanceTimersByTime(5 * MINUTE);
    expect(poll).toHaveBeenCalledTimes(2);
    vi.advanceTimersByTime(5 * MINUTE);
    expect(poll).toHaveBeenCalledTimes(3);
  });

  test("does nothing while disabled", () => {
    const poll = vi.fn();
    renderHook(() => useVisiblePolling(poll, 5 * MINUTE, false));
    vi.advanceTimersByTime(20 * MINUTE);
    expect(poll).not.toHaveBeenCalled();
  });

  test("skips ticks while the tab is hidden", () => {
    const poll = vi.fn();
    renderHook(() => useVisiblePolling(poll, 5 * MINUTE, true));
    setVisibility("hidden");

    vi.advanceTimersByTime(30 * MINUTE);
    expect(poll).toHaveBeenCalledTimes(1);
  });

  test("catches up once when a stale tab is shown again", () => {
    const poll = vi.fn();
    renderHook(() => useVisiblePolling(poll, 5 * MINUTE, true));
    setVisibility("hidden");
    vi.advanceTimersByTime(12 * MINUTE);

    setVisibility("visible");
    expect(poll).toHaveBeenCalledTimes(2);
  });

  test("a catch-up poll restarts the cadence", () => {
    const poll = vi.fn();
    renderHook(() => useVisiblePolling(poll, 5 * MINUTE, true));
    setVisibility("hidden");
    vi.advanceTimersByTime(12 * MINUTE);
    setVisibility("visible");
    expect(poll).toHaveBeenCalledTimes(2);

    // The tick that was due at 15 minutes must not land three minutes after
    // the catch-up; the next poll is a full interval later.
    vi.advanceTimersByTime(4 * MINUTE);
    expect(poll).toHaveBeenCalledTimes(2);
    vi.advanceTimersByTime(MINUTE);
    expect(poll).toHaveBeenCalledTimes(3);
  });

  test("does not refetch when a tab is shown again while still fresh", () => {
    const poll = vi.fn();
    renderHook(() => useVisiblePolling(poll, 5 * MINUTE, true));
    setVisibility("hidden");
    vi.advanceTimersByTime(MINUTE);

    setVisibility("visible");
    expect(poll).toHaveBeenCalledTimes(1);
  });

  test("a tab opened in the background fetches when first shown", () => {
    visibility = "hidden";
    const poll = vi.fn();
    renderHook(() => useVisiblePolling(poll, 5 * MINUTE, true));
    expect(poll).not.toHaveBeenCalled();

    setVisibility("visible");
    expect(poll).toHaveBeenCalledTimes(1);
  });

  test("stops polling on unmount", () => {
    const poll = vi.fn();
    const { unmount } = renderHook(() =>
      useVisiblePolling(poll, 5 * MINUTE, true),
    );
    unmount();
    vi.advanceTimersByTime(20 * MINUTE);
    setVisibility("visible");
    expect(poll).toHaveBeenCalledTimes(1);
  });
});
