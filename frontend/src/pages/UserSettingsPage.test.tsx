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
      created_at: "2025-01-01T00:00:00Z",
      last_login: "",
      isAdmin: false,
    },
    isAuthenticated: true,
    isLoading: false,
    login: vi.fn(),
    logout: vi.fn(),
    register: vi.fn(),
    refreshUser: mockRefreshUser,
  }),
}));

const mockFetch = vi.fn();

afterEach(cleanup);

beforeEach(() => {
  vi.clearAllMocks();
  mockRefreshUser.mockResolvedValue(undefined);
  mockFetch.mockResolvedValue({
    ok: true,
    json: () => Promise.resolve({ user: {} }),
  });
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
    mockFetch.mockResolvedValueOnce({
      ok: false,
      json: () => Promise.resolve({ error: { message: "Update failed" } }),
    });
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

  test("calls refreshUser on mount", () => {
    renderSettingsPage();
    expect(mockRefreshUser).toHaveBeenCalled();
  });
});
