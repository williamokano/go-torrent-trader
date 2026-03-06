import { act, render, screen, cleanup } from "@testing-library/react";
import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { renderHook } from "@testing-library/react";
import { ToastProvider } from "@/components/toast/Toast";
import { useToast } from "@/components/toast/useToast";

describe("Toast", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
    cleanup();
    document.body.innerHTML = "";
  });

  it("useToast throws outside provider", () => {
    expect(() => {
      renderHook(() => useToast());
    }).toThrow("useToast must be used within a ToastProvider");
  });

  it("toast.success adds a toast", () => {
    function TestComponent() {
      const toast = useToast();
      return (
        <button onClick={() => toast.success("Operation succeeded")}>
          Add Success
        </button>
      );
    }

    render(
      <ToastProvider>
        <TestComponent />
      </ToastProvider>,
    );

    act(() => {
      screen.getByText("Add Success").click();
    });

    expect(screen.getByText("Operation succeeded")).toBeInTheDocument();
  });

  it("toasts auto-dismiss after duration", () => {
    function TestComponent() {
      const toast = useToast();
      return (
        <button onClick={() => toast.info("Temporary message")}>
          Add Info
        </button>
      );
    }

    render(
      <ToastProvider duration={3000}>
        <TestComponent />
      </ToastProvider>,
    );

    act(() => {
      screen.getByText("Add Info").click();
    });

    expect(screen.getByText("Temporary message")).toBeInTheDocument();

    // Advance past the duration + exit animation
    act(() => {
      vi.advanceTimersByTime(3300);
    });

    expect(screen.queryByText("Temporary message")).not.toBeInTheDocument();
  });
});
