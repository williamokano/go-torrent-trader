import { useState } from "react";
import { Link } from "react-router-dom";
import { Input } from "@/components/form";
import { api } from "@/api";
import "./recovery.css";

export function ForgotPasswordPage() {
  const [email, setEmail] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [submitted, setSubmitted] = useState(false);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setIsSubmitting(true);

    try {
      await (
        api as { POST: (url: string, opts: object) => Promise<unknown> }
      ).POST("/api/v1/auth/forgot-password", { body: { email } });
    } catch {
      // Silently ignore — we always show the same message
      // to prevent email enumeration
    } finally {
      setIsSubmitting(false);
      setSubmitted(true);
    }
  }

  if (submitted) {
    return (
      <div className="recovery-page">
        <div className="recovery-card">
          <h1 className="recovery-card__title">Check Your Email</h1>
          <p className="recovery-card__success">
            If this email exists, a reset link has been sent. Check your inbox.
          </p>
          <p className="recovery-card__footer">
            <Link to="/login">Back to login</Link>
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="recovery-page">
      <div className="recovery-card">
        <h1 className="recovery-card__title">Forgot Password</h1>
        <form className="recovery-card__form" onSubmit={handleSubmit}>
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
            className="recovery-card__submit"
            disabled={isSubmitting}
          >
            {isSubmitting ? "Sending..." : "Send Reset Link"}
          </button>
        </form>
        <p className="recovery-card__footer">
          <Link to="/login">Back to login</Link>
        </p>
      </div>
    </div>
  );
}
