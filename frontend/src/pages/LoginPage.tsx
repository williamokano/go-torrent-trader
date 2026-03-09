import { useState } from "react";
import { Link, useLocation, useNavigate } from "react-router-dom";
import { Input } from "@/components/form";
import { useToast } from "@/components/toast";
import { ApiError, useAuth } from "@/features/auth";
import "./auth.css";

export function LoginPage() {
  const { login } = useAuth();
  const toast = useToast();
  const navigate = useNavigate();
  const location = useLocation();

  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [emailNotConfirmed, setEmailNotConfirmed] = useState(false);

  const from = (location.state as { from?: string })?.from || "/";

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setIsSubmitting(true);
    setEmailNotConfirmed(false);

    try {
      await login(username, password);
      navigate(from, { replace: true });
    } catch (err) {
      if (err instanceof ApiError && err.code === "email_not_confirmed") {
        setEmailNotConfirmed(true);
      } else {
        const msg =
          err instanceof Error
            ? err.message
            : "Login failed. Please try again.";
        toast.error(msg);
      }
    } finally {
      setIsSubmitting(false);
    }
  }

  return (
    <div className="auth-page">
      <div className="auth-card">
        <h1 className="auth-card__title">Login</h1>
        {emailNotConfirmed && (
          <p className="auth-card__notice">
            Please confirm your email address before logging in.{" "}
            <Link to="/resend-confirmation">Resend confirmation email</Link>
          </p>
        )}
        <form className="auth-card__form" onSubmit={handleSubmit}>
          <Input
            label="Username"
            type="text"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            required
            autoComplete="username"
          />
          <Input
            label="Password"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
            autoComplete="current-password"
          />
          <button
            type="submit"
            className="auth-card__submit"
            disabled={isSubmitting}
          >
            {isSubmitting ? "Logging in..." : "Login"}
          </button>
        </form>
        <p className="auth-card__footer">
          <Link to="/forgot-password">Forgot password?</Link>
        </p>
        <p className="auth-card__footer">
          Don&apos;t have an account? <Link to="/signup">Sign up</Link>
        </p>
      </div>
    </div>
  );
}
