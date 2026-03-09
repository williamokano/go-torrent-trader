import { useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { getConfig } from "@/config";
import "./auth.css";

type ConfirmState = "idle" | "loading" | "success" | "error";

export function ConfirmEmailPage() {
  const [searchParams] = useSearchParams();
  const token = searchParams.get("token");

  const [state, setState] = useState<ConfirmState>(() =>
    token ? "idle" : "error",
  );
  const [errorMessage, setErrorMessage] = useState(() =>
    token ? "" : "No confirmation token provided.",
  );

  async function handleConfirm() {
    if (!token) return;
    setState("loading");

    try {
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/auth/confirm-email`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ token }),
        },
      );
      if (res.ok) {
        setState("success");
      } else {
        const body = await res.json().catch(() => null);
        setState("error");
        setErrorMessage(
          body?.error?.message || "Invalid or expired confirmation link.",
        );
      }
    } catch {
      setState("error");
      setErrorMessage("Failed to confirm email. Please try again.");
    }
  }

  return (
    <div className="auth-page">
      <div className="auth-card">
        <h1 className="auth-card__title">Email Confirmation</h1>

        {state === "idle" && (
          <>
            <p>Click the button below to confirm your email address.</p>
            <button
              className="auth-card__submit"
              onClick={handleConfirm}
              type="button"
            >
              Confirm Email
            </button>
          </>
        )}

        {state === "loading" && <p>Confirming your email address...</p>}

        {state === "success" && (
          <>
            <p>Your email has been confirmed. You can now log in.</p>
            <p className="auth-card__footer">
              <Link to="/login">Go to Login</Link>
            </p>
          </>
        )}

        {state === "error" && (
          <>
            <p>{errorMessage}</p>
            <p className="auth-card__footer">
              <Link to="/resend-confirmation">Resend confirmation email</Link>
            </p>
          </>
        )}
      </div>
    </div>
  );
}
