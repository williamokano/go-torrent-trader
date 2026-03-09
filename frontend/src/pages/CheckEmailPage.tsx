import { Link, useLocation } from "react-router-dom";
import "./auth.css";

export function CheckEmailPage() {
  const location = useLocation();
  const email = (location.state as { email?: string })?.email;

  return (
    <div className="auth-page">
      <div className="auth-card">
        <h1 className="auth-card__title">Check Your Email</h1>
        <p
          style={{
            textAlign: "center",
            marginBottom: "var(--space-md)",
            color: "var(--color-text-secondary)",
          }}
        >
          {email
            ? `We've sent a confirmation email to ${email}. Please check your inbox and click the confirmation link.`
            : "We've sent a confirmation email to your address. Please check your inbox and click the confirmation link."}
        </p>
        <p
          style={{
            textAlign: "center",
            fontSize: "var(--text-sm)",
            color: "var(--color-text-secondary)",
          }}
        >
          The link will expire in 24 hours.
        </p>
        <p className="auth-card__footer">
          Didn&apos;t receive the email?{" "}
          <Link to="/resend-confirmation">Resend confirmation email</Link>
        </p>
        <p className="auth-card__footer">
          Already confirmed? <Link to="/login">Login</Link>
        </p>
      </div>
    </div>
  );
}
