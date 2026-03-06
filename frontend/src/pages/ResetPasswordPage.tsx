import { useState } from "react";
import { Link, useSearchParams, useNavigate } from "react-router-dom";
import { Input } from "@/components/form";
import { useToast } from "@/components/toast";
import { api } from "@/api";
import "./recovery.css";

export function ResetPasswordPage() {
  const toast = useToast();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const token = searchParams.get("token") ?? "";

  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [success, setSuccess] = useState(false);

  function validate(): boolean {
    const newErrors: Record<string, string> = {};

    if (password.length < 8) {
      newErrors.password = "Password must be at least 8 characters";
    }

    if (password !== confirmPassword) {
      newErrors.confirmPassword = "Passwords do not match";
    }

    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!validate()) return;

    setIsSubmitting(true);

    try {
      const { error } = await (
        api as {
          POST: (
            url: string,
            opts: object,
          ) => Promise<{
            data?: unknown;
            error?: { error?: { message?: string } };
          }>;
        }
      ).POST("/api/v1/auth/reset-password", {
        body: { token, password },
      });

      if (error) {
        const message =
          error.error?.message ??
          "Failed to reset password. The link may be invalid or expired.";
        toast.error(message);
        return;
      }

      setSuccess(true);
      setTimeout(() => navigate("/login", { replace: true }), 3000);
    } catch {
      toast.error("Failed to reset password. Please try again.");
    } finally {
      setIsSubmitting(false);
    }
  }

  if (success) {
    return (
      <div className="recovery-page">
        <div className="recovery-card">
          <h1 className="recovery-card__title">Password Reset</h1>
          <p className="recovery-card__success">
            Your password has been reset. Redirecting to login...
          </p>
          <p className="recovery-card__footer">
            <Link to="/login">Go to login</Link>
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="recovery-page">
      <div className="recovery-card">
        <h1 className="recovery-card__title">Reset Password</h1>
        <form className="recovery-card__form" onSubmit={handleSubmit}>
          <Input
            label="New Password"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            error={errors.password}
            required
            autoComplete="new-password"
          />
          <Input
            label="Confirm Password"
            type="password"
            value={confirmPassword}
            onChange={(e) => setConfirmPassword(e.target.value)}
            error={errors.confirmPassword}
            required
            autoComplete="new-password"
          />
          <button
            type="submit"
            className="recovery-card__submit"
            disabled={isSubmitting}
          >
            {isSubmitting ? "Resetting..." : "Reset Password"}
          </button>
        </form>
        <p className="recovery-card__footer">
          <Link to="/login">Back to login</Link>
        </p>
      </div>
    </div>
  );
}
