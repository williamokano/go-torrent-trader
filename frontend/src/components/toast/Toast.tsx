import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { ReactNode } from "react";
import { ToastContext } from "./ToastContext";
import "@/components/toast/toast.css";

type ToastType = "success" | "error" | "info" | "warning";

interface Toast {
  id: string;
  type: ToastType;
  message: string;
  exiting?: boolean;
}

interface ToastProviderProps {
  children: ReactNode;
  duration?: number;
}

export function ToastProvider({
  children,
  duration = 5000,
}: ToastProviderProps) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const counterRef = useRef(0);
  const timersRef = useRef(new Set<ReturnType<typeof setTimeout>>());

  // Auto-dismiss timers outlive the provider unless they are cancelled: a toast
  // raised just before the user navigates away would still fire, setting state
  // on a tree that is gone. React no longer warns about that, so it stays
  // invisible in the browser — but under a test runner the timer lands after
  // the DOM has been torn down and takes the whole suite down with a
  // "window is not defined" that names an unrelated file.
  const schedule = useCallback((fn: () => void, ms: number) => {
    const timer = setTimeout(() => {
      timersRef.current.delete(timer);
      fn();
    }, ms);
    timersRef.current.add(timer);
  }, []);

  useEffect(() => {
    const timers = timersRef.current;
    return () => {
      timers.forEach(clearTimeout);
      timers.clear();
    };
  }, []);

  const removeToast = useCallback(
    (id: string) => {
      setToasts((prev) =>
        prev.map((t) => (t.id === id ? { ...t, exiting: true } : t)),
      );
      schedule(() => {
        setToasts((prev) => prev.filter((t) => t.id !== id));
      }, 300);
    },
    [schedule],
  );

  const addToast = useCallback(
    (type: ToastType, message: string) => {
      const id = `toast-${++counterRef.current}`;
      setToasts((prev) => [...prev, { id, type, message }]);
      schedule(() => {
        removeToast(id);
      }, duration);
    },
    [duration, removeToast, schedule],
  );

  const success = useCallback(
    (message: string) => addToast("success", message),
    [addToast],
  );
  const error = useCallback(
    (message: string) => addToast("error", message),
    [addToast],
  );
  const info = useCallback(
    (message: string) => addToast("info", message),
    [addToast],
  );
  const warning = useCallback(
    (message: string) => addToast("warning", message),
    [addToast],
  );

  // Stable identity: every callback above is already useCallback-stable, so the
  // only churn was this object literal. Without the memo useToast() returns a
  // fresh value on every provider render — and the provider re-renders on every
  // toast add and auto-dismiss — which refires any effect that depends on it.
  const value = useMemo(
    () => ({ success, error, info, warning, removeToast }),
    [success, error, info, warning, removeToast],
  );

  return (
    <ToastContext.Provider value={value}>
      {children}
      <div className="toast-container" aria-live="polite">
        {toasts.map((toast) => (
          <div
            key={toast.id}
            className={`toast toast--${toast.type}${toast.exiting ? " toast--exiting" : ""}`}
            role="status"
          >
            <span className="toast__message">{toast.message}</span>
            <button
              className="toast__close"
              onClick={() => removeToast(toast.id)}
              aria-label="Close notification"
            >
              &times;
            </button>
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}
