import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, test, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { ThemeProvider } from "@/themes";
import { AuthProvider } from "@/features/auth";
import { RootLayout } from "@/layouts/RootLayout";
import { HomePage } from "@/pages/HomePage";

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

test("renders app with routing", () => {
  render(
    <ThemeProvider>
      <AuthProvider>
        <MemoryRouter initialEntries={["/"]}>
          <RootLayout />
        </MemoryRouter>
      </AuthProvider>
    </ThemeProvider>,
  );
  screen.getByText("TorrentTrader");
});

test("renders home page content at root route", () => {
  render(
    <ThemeProvider>
      <AuthProvider>
        <MemoryRouter initialEntries={["/"]}>
          <HomePage />
        </MemoryRouter>
      </AuthProvider>
    </ThemeProvider>,
  );
  screen.getByText("Welcome to TorrentTrader");
});
