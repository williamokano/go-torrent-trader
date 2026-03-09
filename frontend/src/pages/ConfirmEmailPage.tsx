import { useEffect, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { getConfig } from "@/config";
import "./auth.css";

type ConfirmState = "loading" | "success" | "error";

export function ConfirmEmailPage() {
  const [searchParams] = useSearchParams();
  const token = searchParams.get("token");

  const [state, setState] = useState<ConfirmState>(() =>
    token ? "loading" : "error",
  );
  const [errorMessage, setErrorMessage] = useState(() =>
    token ? "" : "No confirmation token provided.",
  );

  useEffect(() => {
    if (!token) return;

    let cancelled = false;

    async function confirmEmail() {
      try {
        const res = await fetch(
          `${getConfig().API_URL}/api/v1/auth/confirm-email?token=${encodeURIComponent(token!)}`,
        );
        if (cancelled) return;
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
        if (!cancelled) {
          setState("error");
          setErrorMessage("Failed to confirm email. Please try again.");
        }
      }
    }

    confirmEmail();

    return () => {
      cancelled = true;
    };
  }, [token]);

  return (
    <div className="auth-page">
      <div className="auth-card">
        <h1 className="auth-card__title">Email Confirmation</h1>

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
