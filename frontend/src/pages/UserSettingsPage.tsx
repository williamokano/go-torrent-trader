import { useCallback, useEffect, useState } from "react";
import { getConfig } from "@/config";
import { getAccessToken } from "@/features/auth/token";
import { useAuth } from "@/features/auth";
import { useToast } from "@/components/toast";
import { Input, Textarea } from "@/components/form";
import { Modal } from "@/components/modal";
import { formatDate, timeAgo } from "@/utils/format";
import "./settings.css";

// One row of "where am I signed in". Mirrors SessionInfo in the API: an opaque
// id and enough detail to recognise a device, and deliberately no token — the
// point of the panel is to evict an intruder, not to hand out credentials.
type SessionRow = {
  id: string;
  device_name: string;
  ip: string;
  created_at: string;
  last_active: string;
  expires_at: string;
  current: boolean;
};

async function apiFetch(
  path: string,
  options: RequestInit = {},
): Promise<{
  data?: unknown;
  status?: number;
  error?: { error?: { message?: string } };
}> {
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

  // 204 has no body at all, and an error page may not be JSON. Either would
  // throw out of res.json() and surface as "Unexpected end of JSON input".
  const body = res.status === 204 ? null : await res.json().catch(() => null);

  if (!res.ok) {
    return {
      status: res.status,
      error: (body as { error?: { message?: string } } | null) ?? {
        error: { message: `Request failed (${res.status})` },
      },
    };
  }

  return { data: body ?? undefined, status: res.status };
}

export function UserSettingsPage() {
  const { user, refreshUser, logout } = useAuth();
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

  // Sessions
  const [sessions, setSessions] = useState<SessionRow[]>([]);
  const [sessionsLoading, setSessionsLoading] = useState(true);
  const [sessionsError, setSessionsError] = useState("");
  const [revokingID, setRevokingID] = useState<string | null>(null);
  const [revokeOthersModalOpen, setRevokeOthersModalOpen] = useState(false);
  const [signOutHereSession, setSignOutHereSession] =
    useState<SessionRow | null>(null);
  const [revokeOthersSubmitting, setRevokeOthersSubmitting] = useState(false);

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

  const loadSessions = useCallback(async () => {
    setSessionsLoading(true);
    try {
      const result = await apiFetch("/api/v1/auth/sessions");
      if (result.status === 401) {
        // This page's own session is gone — revoked from another device, or
        // expired. Showing a red error box and carrying on with dead tokens
        // strands the member; send them to the login screen instead.
        await logout();
        return;
      }
      if (result.error) {
        throw new Error(
          result.error?.error?.message ?? "Failed to load sessions",
        );
      }
      const d = result.data as { sessions?: SessionRow[] } | undefined;
      setSessions(d?.sessions ?? []);
      setSessionsError("");
    } catch (err) {
      // Clear the rows as well: an error message above a list the server no
      // longer vouches for invites the member to act on stale rows.
      setSessions([]);
      setSessionsError(
        err instanceof Error ? err.message : "Failed to load sessions",
      );
    } finally {
      setSessionsLoading(false);
    }
  }, [logout]);

  useEffect(() => {
    loadSessions();
  }, [loadSessions]);

  // Revoking one session. The list is re-fetched rather than patched locally:
  // sessions change without this page's help — another device signs in, a token
  // rotates — so the server's answer is the only one worth showing after an
  // action taken because the member does not trust what they are looking at.
  async function handleRevokeSession(session: SessionRow) {
    setRevokingID(session.id);

    try {
      const result = await apiFetch(
        `/api/v1/auth/sessions/${encodeURIComponent(session.id)}`,
        {
          method: "DELETE",
        },
      );

      // 404 means that session is already gone — another tab revoked it, or it
      // expired while this list sat on screen. The member asked for it to not
      // exist and it does not, so this is a success with a stale list, not an
      // error.
      if (result.error && result.status !== 404) {
        throw new Error(
          result.error?.error?.message ?? "Failed to revoke session",
        );
      }

      if (session.current && !result.error) {
        // Revoking your own session is a logout, and the tokens this page holds
        // are dead the moment the call returns.
        toast.success("Signed out of this device");
        await logout();
        return;
      }

      toast.success(
        result.error ? "That session was already gone" : "Session revoked",
      );
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : "Failed to revoke session",
      );
    } finally {
      setRevokingID(null);
      // On every path, including failure: a member acting on this list because
      // they do not trust it must not be left looking at a row that no longer
      // reflects the server.
      await loadSessions();
    }
  }

  async function handleRevokeOtherSessions() {
    setRevokeOthersSubmitting(true);

    try {
      const result = await apiFetch("/api/v1/auth/sessions", {
        method: "DELETE",
      });

      if (result.error) {
        throw new Error(
          result.error?.error?.message ?? "Failed to sign out other devices",
        );
      }

      const d = result.data as { revoked?: number } | undefined;
      const revoked = d?.revoked ?? 0;
      toast.success(
        revoked === 1
          ? "Signed out of 1 other device"
          : `Signed out of ${revoked} other devices`,
      );
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : "Failed to sign out other devices",
      );
    } finally {
      setRevokeOthersSubmitting(false);
      setRevokeOthersModalOpen(false);
      await loadSessions();
    }
  }

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

      {/* Sessions Section */}
      <section className="settings-section">
        <h2 className="settings-section__title">Active Sessions</h2>
        <p className="settings-sessions__intro">
          Every device currently signed in as you. If you see one you do not
          recognise, revoke it and change your password.
        </p>

        {sessionsLoading ? (
          <p className="settings-sessions__status">Loading sessions...</p>
        ) : sessionsError ? (
          <p className="settings-sessions__status settings-sessions__status--error">
            {sessionsError}
          </p>
        ) : sessions.length === 0 ? (
          <p className="settings-sessions__status">No active sessions.</p>
        ) : (
          <ul className="settings-sessions__list">
            {sessions.map((session) => (
              <li key={session.id} className="settings-session">
                <div className="settings-session__details">
                  <div className="settings-session__device">
                    {session.device_name || "Unknown device"}
                    {session.current && (
                      <span className="settings-session__badge">
                        This device
                      </span>
                    )}
                  </div>
                  <div className="settings-session__meta">
                    {session.ip || "unknown address"} &middot; active{" "}
                    {timeAgo(session.last_active)} &middot; signed in{" "}
                    {formatDate(session.created_at)}
                  </div>
                </div>
                <button
                  type="button"
                  className="settings-session__revoke"
                  onClick={() =>
                    // Ending the session you are reading this from is the one
                    // action here that immediately logs the member out, and the
                    // button sits in the same place as every other row's. Ask.
                    session.current
                      ? setSignOutHereSession(session)
                      : handleRevokeSession(session)
                  }
                  disabled={revokingID !== null || sessionsLoading}
                >
                  {revokingID === session.id
                    ? "Revoking..."
                    : session.current
                      ? "Sign out"
                      : "Revoke"}
                </button>
              </li>
            ))}
          </ul>
        )}

        <button
          type="button"
          className="settings-sessions__revoke-all"
          onClick={() => setRevokeOthersModalOpen(true)}
          disabled={
            sessionsLoading ||
            sessionsError !== "" ||
            sessions.filter((s) => !s.current).length === 0
          }
        >
          Sign out of all other devices
        </button>
      </section>

      {/* Sign-out-everywhere Confirmation Modal */}
      <Modal
        isOpen={revokeOthersModalOpen}
        onClose={() => setRevokeOthersModalOpen(false)}
        title="Sign out of all other devices"
      >
        <div className="settings-modal__body">
          Every other device signed in as you will be signed out immediately.
          This device stays signed in, so you can change your password next. If
          you think someone else had access to your account, regenerate your
          passkey too — signing a device out does not stop a client that already
          has it.
        </div>
        <div className="settings-modal__footer">
          <button
            className="settings-modal__cancel"
            onClick={() => setRevokeOthersModalOpen(false)}
          >
            Cancel
          </button>
          <button
            className="settings-modal__confirm"
            onClick={handleRevokeOtherSessions}
            disabled={revokeOthersSubmitting}
          >
            {revokeOthersSubmitting ? "Signing out..." : "Sign Out Others"}
          </button>
        </div>
      </Modal>

      {/* Sign-out-this-device Confirmation Modal */}
      <Modal
        isOpen={signOutHereSession !== null}
        onClose={() => setSignOutHereSession(null)}
        title="Sign out of this device"
      >
        <div className="settings-modal__body">
          This is the device you are using now. Signing it out ends this session
          immediately and returns you to the login page.
        </div>
        <div className="settings-modal__footer">
          <button
            className="settings-modal__cancel"
            onClick={() => setSignOutHereSession(null)}
          >
            Cancel
          </button>
          <button
            className="settings-modal__confirm"
            onClick={() => {
              const session = signOutHereSession;
              setSignOutHereSession(null);
              if (session) {
                handleRevokeSession(session);
              }
            }}
            disabled={revokingID !== null}
          >
            Sign Out
          </button>
        </div>
      </Modal>

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
