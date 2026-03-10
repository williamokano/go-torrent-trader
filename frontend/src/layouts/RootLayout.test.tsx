import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, test, expect, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { ThemeProvider } from "@/themes";
import { AuthProvider } from "@/features/auth";
import { RootLayout } from "@/layouts/RootLayout";

vi.mock("@/api", () => ({
  api: {
    POST: vi.fn().mockResolvedValue({
      data: null,
      error: { error: { message: "not implemented" } },
    }),
    GET: vi.fn().mockResolvedValue({
      data: null,
      error: { error: { message: "not implemented" } },
    }),
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

  test("renders theme toggle button", () => {
    renderLayout();
    const button = document.querySelector(".header__theme-btn");
    expect(button).toBeInTheDocument();
  });

  test("renders footer with stats placeholder", () => {
    renderLayout();
    expect(screen.getByText(/Torrents:/)).toBeInTheDocument();
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
