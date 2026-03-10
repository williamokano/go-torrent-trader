import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, test, expect, vi } from "vitest";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { UserProfilePage } from "@/pages/UserProfilePage";

vi.mock("@/features/auth/token", () => ({
  getAccessToken: () => "fake-token",
}));

vi.mock("@/config", () => ({
  getConfig: () => ({ API_URL: "http://localhost:8080", SITE_NAME: "Test" }),
}));

const mockUser = {
  id: 42,
  username: "testuser",
  email: "test@example.com",
  group_id: 2,
  avatar: "",
  title: "",
  info: "",
  uploaded: 0,
  downloaded: 0,
  ratio: 0,
  passkey: "abc123",
  invites: 0,
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
};

vi.mock("@/features/auth", () => ({
  useAuth: () => ({
    user: mockUser,
    isAuthenticated: true,
    isLoading: false,
    login: vi.fn(),
    logout: vi.fn(),
    register: vi.fn(),
    refreshUser: vi.fn(),
  }),
}));

const FAKE_PROFILE = {
  id: 7,
  username: "jdoe",
  group_id: 2,
  avatar: "",
  title: "Power User",
  info: "I love torrents!",
  uploaded: 1073741824,
  downloaded: 536870912,
  ratio: 2.0,
  donor: true,
  created_at: "2025-06-15T10:00:00Z",
};

const mockFetch = vi.fn();

afterEach(cleanup);

beforeEach(() => {
  vi.clearAllMocks();
  mockFetch.mockResolvedValue({
    ok: true,
    json: () => Promise.resolve({ user: FAKE_PROFILE }),
  });
  vi.stubGlobal("fetch", mockFetch);
});

function renderProfilePage(id = "7") {
  return render(
    <MemoryRouter initialEntries={[`/user/${id}`]}>
      <Routes>
        <Route path="/user/:id" element={<UserProfilePage />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe("UserProfilePage", () => {
  test("shows loading state initially", () => {
    mockFetch.mockReturnValue(new Promise(() => {}));
    renderProfilePage();
    expect(screen.getByText("Loading profile...")).toBeInTheDocument();
  });

  test("renders username after loading", async () => {
    renderProfilePage();
    await waitFor(() => {
      expect(screen.getByText("jdoe")).toBeInTheDocument();
    });
  });

  test("renders user title", async () => {
    renderProfilePage();
    await waitFor(() => {
      expect(screen.getByText("Power User")).toBeInTheDocument();
    });
  });

  test("renders bio section", async () => {
    renderProfilePage();
    await waitFor(() => {
      expect(screen.getByText("I love torrents!")).toBeInTheDocument();
    });
    expect(screen.getByText("About")).toBeInTheDocument();
  });

  test("renders donor badge", async () => {
    renderProfilePage();
    await waitFor(() => {
      expect(screen.getByText("Donor")).toBeInTheDocument();
    });
  });

  test("renders upload/download stats", async () => {
    renderProfilePage();
    await waitFor(() => {
      expect(screen.getByText("1.00 GB")).toBeInTheDocument();
    });
    expect(screen.getByText("512.00 MB")).toBeInTheDocument();
  });

  test("renders ratio with good color class", async () => {
    renderProfilePage();
    await waitFor(() => {
      expect(screen.getByText("2.00")).toBeInTheDocument();
    });
    const ratioEl = screen.getByText("2.00");
    expect(ratioEl.classList.contains("profile-stat__value--good")).toBe(true);
  });

  test("renders ratio with bad color class when below 1", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ user: { ...FAKE_PROFILE, ratio: 0.5 } }),
    });
    renderProfilePage();
    await waitFor(() => {
      expect(screen.getByText("0.50")).toBeInTheDocument();
    });
    const ratioEl = screen.getByText("0.50");
    expect(ratioEl.classList.contains("profile-stat__value--bad")).toBe(true);
  });

  test("renders initials when no avatar", async () => {
    renderProfilePage();
    await waitFor(() => {
      expect(screen.getByText("J")).toBeInTheDocument();
    });
  });

  test("renders avatar image when provided", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: () =>
        Promise.resolve({
          user: { ...FAKE_PROFILE, avatar: "https://example.com/avatar.jpg" },
        }),
    });
    renderProfilePage();
    await waitFor(() => {
      const img = screen.getByAltText("jdoe's avatar");
      expect(img).toBeInTheDocument();
      expect(img).toHaveAttribute("src", "https://example.com/avatar.jpg");
    });
  });

  test("shows error on API failure", async () => {
    mockFetch.mockResolvedValue({
      ok: false,
      json: () => Promise.resolve({ error: { message: "User not found" } }),
    });
    renderProfilePage();
    await waitFor(() => {
      expect(screen.getByText("User not found")).toBeInTheDocument();
    });
  });

  test("shows error for invalid user ID", async () => {
    renderProfilePage("abc");
    await waitFor(() => {
      expect(screen.getByText("Invalid user ID")).toBeInTheDocument();
    });
    expect(mockFetch).not.toHaveBeenCalled();
  });

  test("does not render bio section when info is empty", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ user: { ...FAKE_PROFILE, info: "" } }),
    });
    renderProfilePage();
    await waitFor(() => {
      expect(screen.getByText("jdoe")).toBeInTheDocument();
    });
    expect(screen.queryByText("About")).not.toBeInTheDocument();
  });

  test("does not show Edit Profile link for other users", async () => {
    renderProfilePage("7");
    await waitFor(() => {
      expect(screen.getByText("jdoe")).toBeInTheDocument();
    });
    expect(screen.queryByText("Edit Profile")).not.toBeInTheDocument();
  });

  test("shows Edit Profile link for own profile", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ user: { ...FAKE_PROFILE, id: 42 } }),
    });
    renderProfilePage("42");
    await waitFor(() => {
      expect(screen.getByText("Edit Profile")).toBeInTheDocument();
    });
  });

  test("passes authorization header to fetch", async () => {
    renderProfilePage();
    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        "http://localhost:8080/api/v1/users/7",
        expect.objectContaining({
          headers: { Authorization: "Bearer fake-token" },
        }),
      );
    });
  });
});
