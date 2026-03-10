import {
  cleanup,
  render,
  screen,
  fireEvent,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { RSSBuilderPage } from "@/pages/RSSBuilderPage";
import { ToastProvider } from "@/components/toast";
import { clearTokens } from "@/features/auth/token";

const mockGET = vi.fn();

vi.mock("@/api", () => ({
  api: {
    GET: (...args: unknown[]) => mockGET(...args),
  },
}));

const FAKE_CATEGORIES = [
  { id: 1, name: "Movies", parent_id: null, sort_order: 1 },
  { id: 2, name: "TV", parent_id: null, sort_order: 2 },
  { id: 3, name: "Music", parent_id: null, sort_order: 3 },
];

vi.mock("@/config", () => ({
  getConfig: () => ({ API_URL: "http://localhost:8080", SITE_NAME: "Test" }),
}));

const mockUser = {
  id: 1,
  username: "testuser",
  email: "test@example.com",
  group_id: 5,
  avatar: "",
  title: "",
  info: "",
  uploaded: 0,
  downloaded: 0,
  ratio: 0,
  passkey: "abc123passkey",
  invites: 0,
  warned: false,
  donor: false,
  enabled: true,
  can_download: true,
  can_upload: true,
  can_chat: true,
  created_at: "",
  last_login: "",
  isAdmin: false,
  isStaff: false,
  permissions: undefined,
};

const mockUseAuth = vi.fn();

vi.mock("@/features/auth", () => ({
  useAuth: () => mockUseAuth(),
}));

afterEach(cleanup);

beforeEach(() => {
  clearTokens();
  localStorage.clear();
  vi.clearAllMocks();
  vi.restoreAllMocks();
  mockGET.mockResolvedValue({
    data: { categories: FAKE_CATEGORIES },
    error: undefined,
  });
  mockUseAuth.mockReturnValue({
    user: mockUser,
    isAuthenticated: true,
    isLoading: false,
  });
});

function renderPage() {
  return render(
    <ToastProvider>
      <MemoryRouter initialEntries={["/rss"]}>
        <RSSBuilderPage />
      </MemoryRouter>
    </ToastProvider>,
  );
}

describe("RSSBuilderPage", () => {
  test("renders page title and description", () => {
    renderPage();
    expect(screen.getByText("RSS Feed")).toBeInTheDocument();
    expect(
      screen.getByText(/Use this URL in your torrent client or RSS reader/),
    ).toBeInTheDocument();
  });

  test("renders category dropdown", async () => {
    renderPage();
    await waitFor(() => {
      const select = screen.getByLabelText("Category") as HTMLSelectElement;
      const options = Array.from(select.options).map((o) => o.text);
      expect(options).toEqual(["All categories", "Movies", "TV", "Music"]);
    });
  });

  test("displays feed URL with passkey", () => {
    renderPage();
    const urlInput = screen.getByLabelText("RSS feed URL") as HTMLInputElement;
    expect(urlInput.value).toBe(
      "http://localhost:8080/api/v1/rss?passkey=abc123passkey",
    );
  });

  test("updates URL when category is selected", async () => {
    renderPage();

    await waitFor(() => {
      const select = screen.getByLabelText("Category") as HTMLSelectElement;
      expect(select.options.length).toBeGreaterThan(1);
    });

    fireEvent.change(screen.getByLabelText("Category"), {
      target: { value: "2" },
    });

    const urlInput = screen.getByLabelText("RSS feed URL") as HTMLInputElement;
    expect(urlInput.value).toBe(
      "http://localhost:8080/api/v1/rss?passkey=abc123passkey&cat=2",
    );
  });

  test("shows copy button", () => {
    renderPage();
    expect(
      screen.getByRole("button", { name: "Copy URL" }),
    ).toBeInTheDocument();
  });

  test("copies URL to clipboard on button click", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.assign(navigator, {
      clipboard: { writeText },
    });

    renderPage();
    fireEvent.click(screen.getByRole("button", { name: "Copy URL" }));

    await waitFor(() => {
      expect(writeText).toHaveBeenCalledWith(
        "http://localhost:8080/api/v1/rss?passkey=abc123passkey",
      );
    });

    expect(screen.getByRole("button", { name: "Copied!" })).toBeInTheDocument();
  });

  test("shows warning about passkey", () => {
    renderPage();
    expect(
      screen.getByText(/This URL contains your personal passkey/),
    ).toBeInTheDocument();
  });

  test("shows message when user has no passkey", () => {
    mockUseAuth.mockReturnValue({
      user: { ...mockUser, passkey: "" },
      isAuthenticated: true,
      isLoading: false,
    });

    renderPage();
    expect(
      screen.getByText(/You need a passkey to use RSS feeds/),
    ).toBeInTheDocument();
  });

  test("URL input is read-only", () => {
    renderPage();
    const urlInput = screen.getByLabelText("RSS feed URL") as HTMLInputElement;
    expect(urlInput.readOnly).toBe(true);
  });
});
