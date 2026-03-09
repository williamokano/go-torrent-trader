import { useState } from "react";
import { Link } from "react-router-dom";
import { Input } from "@/components/form";
import { getConfig } from "@/config";
import "./auth.css";

export function ResendConfirmationPage() {
  const [email, setEmail] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setMessage("");
    setError("");

    if (!email.trim()) {
      setError("Email is required");
      return;
    }

    setIsSubmitting(true);
    try {
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/auth/resend-confirmation`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ email: email.trim() }),
        },
      );

      const body = await res.json().catch(() => null);

      if (res.ok) {
        setMessage(
          body?.message ||
            "If this email has a pending confirmation, a new link has been sent.",
        );
      } else if (res.status === 429) {
        setError(
          body?.error?.message ||
            "Please wait 5 minutes before requesting another confirmation email.",
        );
      } else if (res.status === 409) {
        setError(body?.error?.message || "This account is already confirmed.");
      } else {
        setError(
          body?.error?.message || "Failed to resend confirmation email.",
        );
      }
    } catch {
      setError("Network error. Please try again.");
    } finally {
      setIsSubmitting(false);
    }
  }

  return (
    <div className="auth-page">
      <div className="auth-card">
        <h1 className="auth-card__title">Resend Confirmation</h1>
        <p
          style={{
            textAlign: "center",
            marginBottom: "var(--space-md)",
            fontSize: "var(--text-sm)",
            color: "var(--color-text-secondary)",
          }}
        >
          Enter your email address and we will send a new confirmation link.
        </p>

        {message && (
          <p
            style={{
              color: "var(--color-success, #22c55e)",
              textAlign: "center",
              marginBottom: "var(--space-md)",
            }}
          >
            {message}
          </p>
        )}

        {error && (
          <p
            style={{
              color: "var(--color-error, #ef4444)",
              textAlign: "center",
              marginBottom: "var(--space-md)",
            }}
          >
            {error}
          </p>
        )}

        <form className="auth-card__form" onSubmit={handleSubmit}>
          <Input
            label="Email"
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            required
            autoComplete="email"
          />
          <button
            type="submit"
            className="auth-card__submit"
            disabled={isSubmitting}
          >
            {isSubmitting ? "Sending..." : "Resend Confirmation Email"}
          </button>
        </form>

        <p className="auth-card__footer">
          Already confirmed? <Link to="/login">Login</Link>
        </p>
      </div>
    </div>
  );
}
