import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { Input } from "@/components/form";
import { useToast } from "@/components/toast";
import { useAuth } from "@/features/auth";
import "./auth.css";

function validateUsername(value: string): string | undefined {
  if (value.length < 3 || value.length > 20) {
    return "Username must be 3-20 characters";
  }
  if (!/^[a-zA-Z0-9_]+$/.test(value)) {
    return "Username can only contain letters, numbers, and underscores";
  }
  return undefined;
}

function validatePassword(value: string): string | undefined {
  if (value.length < 8) {
    return "Password must be at least 8 characters";
  }
  return undefined;
}

export function SignupPage() {
  const { register } = useAuth();
  const toast = useToast();
  const navigate = useNavigate();

  const [username, setUsername] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [errors, setErrors] = useState<Record<string, string>>({});

  function validate(): boolean {
    const newErrors: Record<string, string> = {};

    const usernameErr = validateUsername(username);
    if (usernameErr) newErrors.username = usernameErr;

    if (!email) {
      newErrors.email = "Email is required";
    }

    const passwordErr = validatePassword(password);
    if (passwordErr) newErrors.password = passwordErr;

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
      await register({ username, email, password });
      navigate("/", { replace: true });
    } catch (err) {
      toast.error(
        err instanceof Error
          ? err.message
          : "Registration failed. Please try again.",
      );
    } finally {
      setIsSubmitting(false);
    }
  }

  return (
    <div className="auth-page">
      <div className="auth-card">
        <h1 className="auth-card__title">Sign Up</h1>
        <form className="auth-card__form" onSubmit={handleSubmit}>
          <Input
            label="Username"
            type="text"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            error={errors.username}
            required
            autoComplete="username"
          />
          <Input
            label="Email"
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            error={errors.email}
            required
            autoComplete="email"
          />
          <Input
            label="Password"
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
            className="auth-card__submit"
            disabled={isSubmitting}
          >
            {isSubmitting ? "Creating account..." : "Sign Up"}
          </button>
        </form>
        <p className="auth-card__footer">
          Already have an account? <Link to="/login">Login</Link>
        </p>
      </div>
    </div>
  );
}
