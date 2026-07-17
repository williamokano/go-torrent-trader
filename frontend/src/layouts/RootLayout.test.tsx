import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, test, expect, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { ThemeProvider } from "@/themes";
import { AuthProvider } from "@/features/auth";
import { RootLayout } from "@/layouts/RootLayout";
import {
  clearTokens,
  setAccessToken,
  setRefreshToken,
  setTokenExpiry,
} from "@/features/auth/token";

const mockPost = vi.fn();
const mockGet = vi.fn();

vi.mock("@/api", () => ({
  api: {
    POST: (...args: unknown[]) => mockPost(...args),
    GET: (...args: unknown[]) => mockGet(...args),
  },
}));

vi.mock("@/lib/useChat", () => ({
  useChat: () => ({
    pmUnreadCount: 0,
    setPmUnreadCount: vi.fn(),
    messages: [],
    connected: false,
    isStaff: false,
    muted: false,
    muteExpiresAt: null,
    mainChatVisible: false,
    setMainChatVisible: vi.fn(),
    sendMessage: vi.fn(),
    deleteMessage: vi.fn(),
    deleteUserMessages: vi.fn(),
    muteUser: vi.fn(),
    unmuteUser: vi.fn(),
    loadMore: vi.fn(),
  }),
}));

const sampleUser = {
  id: 1,
  username: "testuser",
  email: "test@example.com",
  group_id: 1,
  uploaded: 0,
  downloaded: 0,
  enabled: true,
  can_download: true,
  can_upload: true,
  can_chat: true,
  created_at: "2026-01-01T00:00:00Z",
};

/** Seed a valid, non-expired access token plus a refresh token. */
function seedValidSession() {
  setAccessToken("valid-access");
  setRefreshToken("valid-refresh");
  setTokenExpiry(3600); // well beyond the 5-minute refresh buffer
}

beforeEach(() => {
  clearTokens();
  localStorage.clear();
  vi.clearAllMocks();
  mockPost.mockResolvedValue({
    data: null,
    error: { error: { message: "not implemented" } },
  });
  mockGet.mockResolvedValue({
    data: null,
    error: { error: { message: "not implemented" } },
  });
});

afterEach(cleanup);

function renderLayout() {
  return render(
    <ThemeProvider>
      <AuthProvider>
        <MemoryRouter>
          <RootLayout />
        </MemoryRouter>
      </AuthProvider>
    </ThemeProvider>,
  );
}

describe("RootLayout", () => {
  test("renders header with site name", () => {
    renderLayout();
    expect(screen.getByText("TorrentTrader")).toBeInTheDocument();
  });

  test("renders navigation links and dropdown menus", () => {
    renderLayout();
    expect(screen.getByText("Home")).toBeInTheDocument();
    expect(screen.getByText("Forums")).toBeInTheDocument();
    expect(screen.getByText("Log")).toBeInTheDocument();
    // Browse/Upload are inside the "Torrents" dropdown
    const dropdownToggles = document.querySelectorAll(
      ".header__dropdown-toggle",
    );
    const toggleLabels = Array.from(dropdownToggles).map(
      (el) => el.textContent,
    );
    expect(toggleLabels.some((t) => t?.includes("Torrents"))).toBe(true);
    expect(toggleLabels.some((t) => t?.includes("Community"))).toBe(true);
  });

  test("renders login and signup for unauthenticated users", () => {
    renderLayout();
    expect(screen.getByText("Login")).toBeInTheDocument();
    expect(screen.getByText("Sign Up")).toBeInTheDocument();
  });

  test("does not render footer stats for unauthenticated visitors", () => {
    renderLayout();
    expect(screen.queryByText(/Torrents:/)).not.toBeInTheDocument();
  });

  test("shows footer stats once authenticated", async () => {
    seedValidSession();
    mockGet.mockResolvedValueOnce({
      data: { user: sampleUser },
      error: undefined,
    });

    renderLayout();

    await screen.findByText(/Torrents:/);
  });

  test("renders footer links", () => {
    renderLayout();
    expect(screen.getByText("Rules")).toBeInTheDocument();
    expect(screen.getByText("FAQ")).toBeInTheDocument();
    expect(screen.getByText("Formatting")).toBeInTheDocument();
  });

  test("shows Login and Sign Up links when not authenticated", () => {
    renderLayout();
    expect(screen.getByText("Login")).toBeInTheDocument();
    expect(screen.getByText("Sign Up")).toBeInTheDocument();
  });
});
