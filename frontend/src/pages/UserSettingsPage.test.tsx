import {
  cleanup,
  render,
  screen,
  fireEvent,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, test, expect, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { UserSettingsPage } from "@/pages/UserSettingsPage";
import { ToastProvider } from "@/components/toast";

const mockRefreshUser = vi.fn();
const mockLogout = vi.fn();

// Two sessions: the one this page is being viewed from, and another device.
// That is the shape the sessions panel exists for — a member deciding which of
// these is not them.
const currentSession = {
  id: "session-current",
  device_name: "Firefox on Linux",
  ip: "203.0.113.7",
  created_at: "2025-01-01T00:00:00Z",
  last_active: "2025-01-02T00:00:00Z",
  expires_at: "2025-01-02T01:00:00Z",
  current: true,
};

const otherSession = {
  id: "session-other",
  device_name: "Chrome on Android",
  ip: "198.51.100.9",
  created_at: "2025-01-01T00:00:00Z",
  last_active: "2025-01-01T12:00:00Z",
  expires_at: "2025-01-01T13:00:00Z",
  current: false,
};

// The page fires several requests, and the sessions list loads on mount — so a
// mock that answers every URL the same way makes the order of those requests
// load-bearing. Route by URL instead.
function respondByUrl(
  responder: (url: string) => {
    ok?: boolean;
    status?: number;
    body?: unknown;
  },
) {
  mockFetch.mockImplementation((url: string) => {
    const res = responder(String(url));
    return Promise.resolve({
      ok: res.ok ?? true,
      status: res.status ?? (res.ok === false ? 400 : 200),
      json: () => Promise.resolve(res.body ?? {}),
    });
  });
}

vi.mock("@/features/auth/token", () => ({
  getAccessToken: () => "fake-token",
}));

vi.mock("@/config", () => ({
  getConfig: () => ({ API_URL: "http://localhost:8080", SITE_NAME: "Test" }),
}));

vi.mock("@/features/auth", () => ({
  useAuth: () => ({
    user: {
      id: 1,
      username: "testuser",
      email: "test@example.com",
      group_id: 2,
      avatar: "https://example.com/avatar.jpg",
      title: "Veteran",
      info: "Old bio",
      uploaded: 0,
      downloaded: 0,
      ratio: 1.0,
      passkey: "deadbeef1234567890abcdef",
      invites: 3,
      warned: false,
      donor: false,
      enabled: true,
      can_download: true,
      can_upload: true,
      can_chat: true,
      created_at: "2025-01-01T00:00:00Z",
      last_login: "",
      isAdmin: false,
      isStaff: false,
    },
    isAuthenticated: true,
    isLoading: false,
    login: vi.fn(),
    logout: mockLogout,
    register: vi.fn(),
    refreshUser: mockRefreshUser,
  }),
}));

const mockFetch = vi.fn();

afterEach(cleanup);

beforeEach(() => {
  vi.clearAllMocks();
  mockRefreshUser.mockResolvedValue(undefined);
  mockLogout.mockResolvedValue(undefined);
  respondByUrl((url) =>
    url.includes("/auth/sessions")
      ? { body: { sessions: [currentSession, otherSession] } }
      : { body: { user: {} } },
  );
  vi.stubGlobal("fetch", mockFetch);
});

function renderSettingsPage() {
  return render(
    <ToastProvider>
      <MemoryRouter initialEntries={["/settings"]}>
        <UserSettingsPage />
      </MemoryRouter>
    </ToastProvider>,
  );
}

describe("UserSettingsPage", () => {
  test("renders settings page title", () => {
    renderSettingsPage();
    expect(screen.getByText("Settings")).toBeInTheDocument();
  });

  test("renders profile section with pre-filled fields", () => {
    renderSettingsPage();
    expect(screen.getByLabelText("Avatar URL")).toHaveValue(
      "https://example.com/avatar.jpg",
    );
    expect(screen.getByLabelText("Title")).toHaveValue("Veteran");
    expect(screen.getByLabelText("Bio")).toHaveValue("Old bio");
  });

  test("renders password section", () => {
    renderSettingsPage();
    expect(screen.getByLabelText("Current Password")).toBeInTheDocument();
    expect(screen.getByLabelText("New Password")).toBeInTheDocument();
    expect(screen.getByLabelText("Confirm New Password")).toBeInTheDocument();
  });

  test("renders passkey section with masked value", () => {
    renderSettingsPage();
    expect(screen.getByText("Passkey")).toBeInTheDocument();
    // Passkey should be masked by default - first 4 chars visible
    expect(screen.getByText("dead********************")).toBeInTheDocument();
  });

  test("toggles passkey visibility", () => {
    renderSettingsPage();
    const showBtn = screen.getByRole("button", { name: "Show" });
    fireEvent.click(showBtn);
    expect(screen.getByText("deadbeef1234567890abcdef")).toBeInTheDocument();
    const hideBtn = screen.getByRole("button", { name: "Hide" });
    fireEvent.click(hideBtn);
    expect(screen.getByText("dead********************")).toBeInTheDocument();
  });

  test("submits profile update", async () => {
    renderSettingsPage();

    fireEvent.click(screen.getByRole("button", { name: "Save Profile" }));

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        "http://localhost:8080/api/v1/users/me/profile",
        expect.objectContaining({
          method: "PUT",
        }),
      );
    });

    // Verify the body is valid JSON with the expected shape
    const call = mockFetch.mock.calls.find(
      (c: unknown[]) =>
        c[0] === "http://localhost:8080/api/v1/users/me/profile",
    );
    expect(call).toBeDefined();
    const body = JSON.parse(call![1].body);
    expect(body).toHaveProperty("avatar");
    expect(body).toHaveProperty("title");
    expect(body).toHaveProperty("info");
  });

  test("shows success toast on profile update", async () => {
    renderSettingsPage();

    fireEvent.click(screen.getByRole("button", { name: "Save Profile" }));

    await waitFor(() => {
      expect(
        screen.getByText("Profile updated successfully"),
      ).toBeInTheDocument();
    });
  });

  test("shows error toast on profile update failure", async () => {
    respondByUrl((url) =>
      url.includes("/users/me/profile")
        ? { ok: false, body: { error: { message: "Update failed" } } }
        : { body: { sessions: [] } },
    );
    renderSettingsPage();

    fireEvent.click(screen.getByRole("button", { name: "Save Profile" }));

    await waitFor(() => {
      expect(screen.getByText("Update failed")).toBeInTheDocument();
    });
  });

  test("submits password change", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ message: "Password changed successfully" }),
    });
    renderSettingsPage();

    fireEvent.change(screen.getByLabelText("Current Password"), {
      target: { value: "oldpass123" },
    });
    fireEvent.change(screen.getByLabelText("New Password"), {
      target: { value: "newpass123" },
    });
    fireEvent.change(screen.getByLabelText("Confirm New Password"), {
      target: { value: "newpass123" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Change Password" }));

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        "http://localhost:8080/api/v1/users/me/password",
        expect.objectContaining({
          method: "PUT",
          body: JSON.stringify({
            current_password: "oldpass123",
            new_password: "newpass123",
          }),
        }),
      );
    });
  });

  test("shows error when passwords do not match", async () => {
    renderSettingsPage();

    fireEvent.change(screen.getByLabelText("Current Password"), {
      target: { value: "oldpass123" },
    });
    fireEvent.change(screen.getByLabelText("New Password"), {
      target: { value: "newpass123" },
    });
    fireEvent.change(screen.getByLabelText("Confirm New Password"), {
      target: { value: "different456" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Change Password" }));

    await waitFor(() => {
      expect(
        screen.getByText("New passwords do not match"),
      ).toBeInTheDocument();
    });
  });

  test("shows error when new password is too short", async () => {
    renderSettingsPage();

    fireEvent.change(screen.getByLabelText("Current Password"), {
      target: { value: "oldpass123" },
    });
    fireEvent.change(screen.getByLabelText("New Password"), {
      target: { value: "short" },
    });
    fireEvent.change(screen.getByLabelText("Confirm New Password"), {
      target: { value: "short" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Change Password" }));

    await waitFor(() => {
      expect(
        screen.getByText("Password must be at least 8 characters"),
      ).toBeInTheDocument();
    });
  });

  test("opens passkey confirmation modal", () => {
    renderSettingsPage();

    fireEvent.click(screen.getByRole("button", { name: "Regenerate Passkey" }));

    expect(
      screen.getByText(/Are you sure you want to regenerate your passkey/),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Confirm Regenerate" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Cancel" })).toBeInTheDocument();
  });

  test("regenerates passkey on confirm", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ passkey: "newpasskey1234" }),
    });
    renderSettingsPage();

    fireEvent.click(screen.getByRole("button", { name: "Regenerate Passkey" }));
    fireEvent.click(screen.getByRole("button", { name: "Confirm Regenerate" }));

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        "http://localhost:8080/api/v1/users/me/passkey",
        expect.objectContaining({
          method: "POST",
        }),
      );
    });

    await waitFor(() => {
      expect(
        screen.getByText("Passkey regenerated successfully"),
      ).toBeInTheDocument();
    });
  });

  test("closes passkey modal on cancel", () => {
    renderSettingsPage();

    fireEvent.click(screen.getByRole("button", { name: "Regenerate Passkey" }));
    expect(
      screen.getByText(/Are you sure you want to regenerate your passkey/),
    ).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(
      screen.queryByText(/Are you sure you want to regenerate your passkey/),
    ).not.toBeInTheDocument();
  });

  // #171: the sessions panel. A member could log in from several devices and had
  // no way to see them, let alone end one — the only remedy for "someone else is
  // signed in as me" was a password change.
  test("lists active sessions and marks the current device", async () => {
    renderSettingsPage();

    await waitFor(() => {
      expect(screen.getByText("Firefox on Linux")).toBeInTheDocument();
    });
    expect(screen.getByText("Chrome on Android")).toBeInTheDocument();
    expect(screen.getByText("This device")).toBeInTheDocument();
    expect(screen.getByText(/203\.0\.113\.7/)).toBeInTheDocument();
  });

  test("revokes another device and reloads the list", async () => {
    renderSettingsPage();

    await waitFor(() => {
      expect(screen.getByText("Chrome on Android")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: "Revoke" }));

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        "http://localhost:8080/api/v1/auth/sessions/session-other",
        expect.objectContaining({ method: "DELETE" }),
      );
    });
    await waitFor(() => {
      expect(screen.getByText("Session revoked")).toBeInTheDocument();
    });
    // The list is re-read rather than patched: the member is here precisely
    // because they do not trust what they were looking at.
    expect(
      mockFetch.mock.calls.filter(
        (c: unknown[]) =>
          c[0] === "http://localhost:8080/api/v1/auth/sessions" &&
          (c[1] as RequestInit | undefined)?.method === undefined,
      ).length,
    ).toBeGreaterThan(1);
  });

  // Revoking your own session is a logout, and the tokens this page holds are
  // dead the moment the call returns — so the page has to stop using them.
  test("revoking the current device signs the member out", async () => {
    renderSettingsPage();

    await waitFor(() => {
      expect(screen.getByText("Firefox on Linux")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: "Sign out" }));

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        "http://localhost:8080/api/v1/auth/sessions/session-current",
        expect.objectContaining({ method: "DELETE" }),
      );
    });
    await waitFor(() => {
      expect(mockLogout).toHaveBeenCalled();
    });
  });

  test("signs out of all other devices after confirming", async () => {
    respondByUrl((url) => {
      if (url.endsWith("/auth/sessions")) {
        return {
          body: { sessions: [currentSession, otherSession], revoked: 1 },
        };
      }
      return { body: { user: {} } };
    });
    renderSettingsPage();

    await waitFor(() => {
      expect(screen.getByText("Chrome on Android")).toBeInTheDocument();
    });

    fireEvent.click(
      screen.getByRole("button", { name: "Sign out of all other devices" }),
    );
    expect(
      screen.getByText(/Every other device signed in as you/),
    ).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Sign Out Others" }));

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        "http://localhost:8080/api/v1/auth/sessions",
        expect.objectContaining({ method: "DELETE" }),
      );
    });
    await waitFor(() => {
      expect(
        screen.getByText("Signed out of 1 other device"),
      ).toBeInTheDocument();
    });
    // And it must not sign the caller out: they still have a password to change.
    expect(mockLogout).not.toHaveBeenCalled();
  });

  test("the panic button is disabled when this is the only session", async () => {
    respondByUrl((url) =>
      url.includes("/auth/sessions")
        ? { body: { sessions: [currentSession] } }
        : { body: { user: {} } },
    );
    renderSettingsPage();

    await waitFor(() => {
      expect(screen.getByText("Firefox on Linux")).toBeInTheDocument();
    });
    expect(
      screen.getByRole("button", { name: "Sign out of all other devices" }),
    ).toBeDisabled();
  });

  test("shows an error when the session list cannot be loaded", async () => {
    respondByUrl((url) =>
      url.includes("/auth/sessions")
        ? { ok: false, body: { error: { message: "Sessions unavailable" } } }
        : { body: { user: {} } },
    );
    renderSettingsPage();

    await waitFor(() => {
      expect(screen.getByText("Sessions unavailable")).toBeInTheDocument();
    });
  });

  test("calls refreshUser on mount", () => {
    renderSettingsPage();
    expect(mockRefreshUser).toHaveBeenCalled();
  });
});
