import { useEffect, useState } from "react";
import { getConfig } from "@/config";
import { getAccessToken } from "@/features/auth/token";
import { useAuth } from "@/features/auth";
import { useToast } from "@/components/toast";
import { Input, Textarea } from "@/components/form";
import { Modal } from "@/components/modal";
import "./settings.css";

async function apiFetch(
  path: string,
  options: RequestInit = {},
): Promise<{ data?: unknown; error?: { error?: { message?: string } } }> {
  const token = getAccessToken();
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...(token ? { Authorization: `Bearer ${token}` } : {}),
    ...((options.headers as Record<string, string>) ?? {}),
  };

  const res = await fetch(`${getConfig().API_URL}${path}`, {
    ...options,
    headers,
  });

  const body = await res.json();

  if (!res.ok) {
    return { error: body };
  }

  return { data: body };
}

export function UserSettingsPage() {
  const { user, refreshUser } = useAuth();
  const toast = useToast();

  // Profile form
  const [avatar, setAvatar] = useState("");
  const [title, setTitle] = useState("");
  const [info, setInfo] = useState("");
  const [profileSubmitting, setProfileSubmitting] = useState(false);

  // Password form
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [passwordSubmitting, setPasswordSubmitting] = useState(false);

  // Passkey
  const [passkey, setPasskey] = useState("");
  const [passkeyVisible, setPasskeyVisible] = useState(false);
  const [passkeyModalOpen, setPasskeyModalOpen] = useState(false);
  const [passkeySubmitting, setPasskeySubmitting] = useState(false);

  // Populate form from user data on mount
  useEffect(() => {
    if (user) {
      setAvatar(user.avatar ?? "");
      setTitle(user.title ?? "");
      setInfo(user.info ?? "");
      setPasskey(user.passkey ?? "");
    }
  }, [user]);

  // Refresh user data on mount to get latest
  useEffect(() => {
    refreshUser();
  }, [refreshUser]);

  async function handleProfileSubmit(e: React.FormEvent) {
    e.preventDefault();
    setProfileSubmitting(true);

    try {
      const result = await apiFetch("/api/v1/users/me/profile", {
        method: "PUT",
        body: JSON.stringify({ avatar, title, info }),
      });

      if (result.error) {
        throw new Error(
          result.error?.error?.message ?? "Failed to update profile",
        );
      }

      await refreshUser();
      toast.success("Profile updated successfully");
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : "Failed to update profile",
      );
    } finally {
      setProfileSubmitting(false);
    }
  }

  async function handlePasswordSubmit(e: React.FormEvent) {
    e.preventDefault();

    if (newPassword !== confirmPassword) {
      toast.error("New passwords do not match");
      return;
    }

    if (newPassword.length < 8) {
      toast.error("Password must be at least 8 characters");
      return;
    }

    setPasswordSubmitting(true);

    try {
      const result = await apiFetch("/api/v1/users/me/password", {
        method: "PUT",
        body: JSON.stringify({
          current_password: currentPassword,
          new_password: newPassword,
        }),
      });

      if (result.error) {
        throw new Error(
          result.error?.error?.message ?? "Failed to change password",
        );
      }

      toast.success("Password changed successfully");
      setCurrentPassword("");
      setNewPassword("");
      setConfirmPassword("");
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : "Failed to change password",
      );
    } finally {
      setPasswordSubmitting(false);
    }
  }

  async function handlePasskeyRegenerate() {
    setPasskeySubmitting(true);

    try {
      const result = await apiFetch("/api/v1/users/me/passkey", {
        method: "POST",
      });

      if (result.error) {
        throw new Error(
          result.error?.error?.message ?? "Failed to regenerate passkey",
        );
      }

      const d = result.data as { passkey?: string };
      if (d?.passkey) {
        setPasskey(d.passkey);
      }

      await refreshUser();
      toast.success("Passkey regenerated successfully");
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : "Failed to regenerate passkey",
      );
    } finally {
      setPasskeySubmitting(false);
      setPasskeyModalOpen(false);
    }
  }

  const maskedPasskey = passkey
    ? passkey.slice(0, 4) + "*".repeat(Math.max(0, passkey.length - 4))
    : "N/A";

  return (
    <div className="settings-page">
      <h1 className="settings-page__title">Settings</h1>

      {/* Profile Section */}
      <section className="settings-section">
        <h2 className="settings-section__title">Profile</h2>
        <form className="settings-section__form" onSubmit={handleProfileSubmit}>
          <Input
            label="Avatar URL"
            type="url"
            value={avatar}
            onChange={(e) => setAvatar(e.target.value)}
            placeholder="https://example.com/avatar.jpg"
          />
          <Input
            label="Title"
            type="text"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder="Your custom title"
          />
          <Textarea
            label="Bio"
            value={info}
            onChange={(e) => setInfo(e.target.value)}
            placeholder="Tell us about yourself..."
            rows={4}
          />
          <button
            type="submit"
            className="settings-section__submit"
            disabled={profileSubmitting}
          >
            {profileSubmitting ? "Saving..." : "Save Profile"}
          </button>
        </form>
      </section>

      {/* Password Section */}
      <section className="settings-section">
        <h2 className="settings-section__title">Change Password</h2>
        <form
          className="settings-section__form"
          onSubmit={handlePasswordSubmit}
        >
          <Input
            label="Current Password"
            type="password"
            value={currentPassword}
            onChange={(e) => setCurrentPassword(e.target.value)}
            required
            autoComplete="current-password"
          />
          <Input
            label="New Password"
            type="password"
            value={newPassword}
            onChange={(e) => setNewPassword(e.target.value)}
            required
            autoComplete="new-password"
          />
          <Input
            label="Confirm New Password"
            type="password"
            value={confirmPassword}
            onChange={(e) => setConfirmPassword(e.target.value)}
            required
            autoComplete="new-password"
          />
          <button
            type="submit"
            className="settings-section__submit"
            disabled={passwordSubmitting}
          >
            {passwordSubmitting ? "Changing..." : "Change Password"}
          </button>
        </form>
      </section>

      {/* Passkey Section */}
      <section className="settings-section">
        <h2 className="settings-section__title">Passkey</h2>
        <div className="settings-passkey">
          <div className="settings-passkey__current">
            <span className="settings-passkey__value">
              {passkeyVisible ? passkey || "N/A" : maskedPasskey}
            </span>
            <button
              type="button"
              className="settings-passkey__toggle"
              onClick={() => setPasskeyVisible((v) => !v)}
            >
              {passkeyVisible ? "Hide" : "Show"}
            </button>
          </div>
          <p className="settings-passkey__warning">
            Regenerating your passkey will invalidate all existing torrent
            download links. You will need to re-download any active .torrent
            files.
          </p>
          <button
            type="button"
            className="settings-passkey__regenerate"
            onClick={() => setPasskeyModalOpen(true)}
          >
            Regenerate Passkey
          </button>
        </div>
      </section>

      {/* Passkey Confirmation Modal */}
      <Modal
        isOpen={passkeyModalOpen}
        onClose={() => setPasskeyModalOpen(false)}
        title="Regenerate Passkey"
      >
        <div className="settings-modal__body">
          Are you sure you want to regenerate your passkey? This action cannot
          be undone. All existing torrent download links will stop working.
        </div>
        <div className="settings-modal__footer">
          <button
            className="settings-modal__cancel"
            onClick={() => setPasskeyModalOpen(false)}
          >
            Cancel
          </button>
          <button
            className="settings-modal__confirm"
            onClick={handlePasskeyRegenerate}
            disabled={passkeySubmitting}
          >
            {passkeySubmitting ? "Regenerating..." : "Confirm Regenerate"}
          </button>
        </div>
      </Modal>
    </div>
  );
}
