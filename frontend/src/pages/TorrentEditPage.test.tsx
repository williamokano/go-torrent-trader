import {
  cleanup,
  render,
  screen,
  fireEvent,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { TorrentEditPage } from "@/pages/TorrentEditPage";
import { ToastProvider } from "@/components/toast";
import { AuthContext } from "@/features/auth/AuthContextDef";
import type { AuthContextValue, User } from "@/features/auth/AuthContextDef";

const mockGET = vi.fn();

vi.mock("@/api", () => ({
  api: {
    GET: (...args: unknown[]) => mockGET(...args),
  },
}));

vi.mock("@/features/auth/token", () => ({
  getAccessToken: () => "fake-token",
}));

vi.mock("@/config", () => ({
  getConfig: () => ({ API_URL: "http://localhost:8080", SITE_NAME: "Test" }),
}));

const mockNavigate = vi.fn();

vi.mock("react-router-dom", async () => {
  const actual =
    await vi.importActual<typeof import("react-router-dom")>(
      "react-router-dom",
    );
  return { ...actual, useNavigate: () => mockNavigate };
});

const FAKE_TORRENT = {
  id: 1,
  name: "Ubuntu 24.04 LTS Desktop",
  info_hash: "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0",
  size: 4_800_000_000,
  description: "The latest Ubuntu release.",
  category_id: 2,
  category_name: "Linux ISOs",
  uploader_id: 5,
  anonymous: false,
  seeders: 42,
  leechers: 5,
  times_completed: 318,
  comments_count: 12,
  file_count: 3,
  created_at: "2026-03-05T14:30:00Z",
  updated_at: "2026-03-05T14:30:00Z",
};

const FAKE_CATEGORIES = [
  { id: 1, name: "Movies", parent_id: null, sort_order: 1 },
  { id: 2, name: "Linux ISOs", parent_id: null, sort_order: 2 },
  { id: 3, name: "Music", parent_id: null, sort_order: 3 },
];

function makeUser(overrides: Partial<User> = {}): User {
  return {
    id: 5,
    username: "testuser",
    email: "test@example.com",
    group_id: 1,
    avatar: "",
    title: "",
    info: "",
    uploaded: 0,
    downloaded: 0,
    ratio: 0,
    passkey: "",
    invites: 0,
    warned: false,
    donor: false,
    enabled: true,
    created_at: "",
    last_login: "",
    isAdmin: false,
    ...overrides,
  };
}

function makeAuthContext(user: User | null = makeUser()): AuthContextValue {
  return {
    user,
    isAuthenticated: !!user,
    isLoading: false,
    login: vi.fn(),
    logout: vi.fn(),
    register: vi.fn(),
    refreshUser: vi.fn(),
  };
}

afterEach(cleanup);

beforeEach(() => {
  vi.clearAllMocks();
  vi.restoreAllMocks();

  mockGET.mockImplementation((url: string) => {
    if (url === "/api/v1/categories") {
      return Promise.resolve({
        data: { categories: FAKE_CATEGORIES },
        error: undefined,
      });
    }
    if (url === "/api/v1/torrents/{id}") {
      return Promise.resolve({
        data: { torrent: FAKE_TORRENT },
        error: undefined,
      });
    }
    return Promise.resolve({ data: undefined, error: undefined });
  });
});

function renderEditPage(
  id = "1",
  authContext: AuthContextValue = makeAuthContext(),
) {
  return render(
    <AuthContext.Provider value={authContext}>
      <ToastProvider>
        <MemoryRouter initialEntries={[`/torrent/${id}/edit`]}>
          <Routes>
            <Route path="/torrent/:id/edit" element={<TorrentEditPage />} />
          </Routes>
        </MemoryRouter>
      </ToastProvider>
    </AuthContext.Provider>,
  );
}

describe("TorrentEditPage", () => {
  test("shows loading state initially", () => {
    mockGET.mockReturnValue(new Promise(() => {}));
    renderEditPage();
    expect(screen.getByText("Loading torrent...")).toBeInTheDocument();
  });

  test("renders pre-filled form after loading", async () => {
    renderEditPage();

    await waitFor(() => {
      expect(screen.getByLabelText("Name")).toHaveValue(
        "Ubuntu 24.04 LTS Desktop",
      );
    });

    expect(screen.getByLabelText("Description")).toHaveValue(
      "The latest Ubuntu release.",
    );
    expect(screen.getByLabelText("Category")).toHaveValue("2");
    expect(screen.getByLabelText("Upload anonymously")).not.toBeChecked();
  });

  test("renders category options from API", async () => {
    renderEditPage();

    await waitFor(() => {
      const select = screen.getByLabelText("Category") as HTMLSelectElement;
      const options = Array.from(select.options).map((o) => o.text);
      expect(options).toEqual([
        "Select a category",
        "Movies",
        "Linux ISOs",
        "Music",
      ]);
    });
  });

  test("submits updated data and navigates on success", async () => {
    const mockFetch = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          torrent: { ...FAKE_TORRENT, name: "Updated Name" },
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    );

    renderEditPage();

    await waitFor(() => {
      expect(screen.getByLabelText("Name")).toHaveValue(
        "Ubuntu 24.04 LTS Desktop",
      );
    });

    fireEvent.change(screen.getByLabelText("Name"), {
      target: { value: "Updated Name" },
    });

    fireEvent.click(screen.getByRole("button", { name: "Save Changes" }));

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledTimes(1);
    });

    const [url, options] = mockFetch.mock.calls[0];
    expect(url).toBe("http://localhost:8080/api/v1/torrents/1");
    expect(options?.method).toBe("PUT");
    expect(JSON.parse(options?.body as string)).toEqual(
      expect.objectContaining({ name: "Updated Name" }),
    );

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith("/torrent/1");
    });
  });

  test("shows error toast on API failure", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(
        JSON.stringify({ error: { message: "Permission denied" } }),
        { status: 403, headers: { "Content-Type": "application/json" } },
      ),
    );

    renderEditPage();

    await waitFor(() => {
      expect(screen.getByLabelText("Name")).toHaveValue(
        "Ubuntu 24.04 LTS Desktop",
      );
    });

    fireEvent.click(screen.getByRole("button", { name: "Save Changes" }));

    await waitFor(() => {
      expect(screen.getByText("Permission denied")).toBeInTheDocument();
    });
  });

  test("shows error for invalid torrent ID", async () => {
    renderEditPage("abc");
    await waitFor(() => {
      expect(screen.getByText("Invalid torrent ID")).toBeInTheDocument();
    });
  });

  test("shows loading state while submitting", async () => {
    let resolveUpdate: (value: Response) => void;
    vi.spyOn(globalThis, "fetch").mockReturnValueOnce(
      new Promise<Response>((resolve) => {
        resolveUpdate = resolve;
      }),
    );

    renderEditPage();

    await waitFor(() => {
      expect(screen.getByLabelText("Name")).toHaveValue(
        "Ubuntu 24.04 LTS Desktop",
      );
    });

    fireEvent.click(screen.getByRole("button", { name: "Save Changes" }));

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Saving..." })).toBeDisabled();
    });

    resolveUpdate!(
      new Response(JSON.stringify({ torrent: FAKE_TORRENT }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "Save Changes" }),
      ).not.toBeDisabled();
    });
  });

  test("cancel button navigates back to detail page", async () => {
    renderEditPage();

    await waitFor(() => {
      expect(screen.getByLabelText("Name")).toHaveValue(
        "Ubuntu 24.04 LTS Desktop",
      );
    });

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(mockNavigate).toHaveBeenCalledWith("/torrent/1");
  });

  test("shows admin controls for admin users", async () => {
    const adminUser = makeUser({ isAdmin: true });
    renderEditPage("1", makeAuthContext(adminUser));

    await waitFor(() => {
      expect(screen.getByText("Admin Controls")).toBeInTheDocument();
    });

    expect(screen.getByLabelText("Banned")).toBeInTheDocument();
    expect(screen.getByLabelText("Freeleech")).toBeInTheDocument();
  });

  test("hides admin controls for non-admin users", async () => {
    renderEditPage();

    await waitFor(() => {
      expect(screen.getByLabelText("Name")).toHaveValue(
        "Ubuntu 24.04 LTS Desktop",
      );
    });

    expect(screen.queryByText("Admin Controls")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Banned")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Freeleech")).not.toBeInTheDocument();
  });

  test("includes admin fields in submit body for admin", async () => {
    const mockFetch = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(JSON.stringify({ torrent: FAKE_TORRENT }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    const adminUser = makeUser({ isAdmin: true });
    renderEditPage("1", makeAuthContext(adminUser));

    await waitFor(() => {
      expect(screen.getByLabelText("Banned")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByLabelText("Freeleech"));
    fireEvent.click(screen.getByRole("button", { name: "Save Changes" }));

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledTimes(1);
    });

    const body = JSON.parse(mockFetch.mock.calls[0][1]?.body as string);
    expect(body).toHaveProperty("banned", false);
    expect(body).toHaveProperty("free", true);
  });

  test("shows error on fetch torrent failure", async () => {
    mockGET.mockImplementation((url: string) => {
      if (url === "/api/v1/categories") {
        return Promise.resolve({
          data: { categories: FAKE_CATEGORIES },
          error: undefined,
        });
      }
      return Promise.resolve({
        data: undefined,
        error: { error: { message: "Torrent not found" } },
      });
    });

    renderEditPage();

    await waitFor(() => {
      expect(screen.getByText("Torrent not found")).toBeInTheDocument();
    });
  });

  test("shows toast error when name is empty", async () => {
    renderEditPage();

    await waitFor(() => {
      expect(screen.getByLabelText("Name")).toHaveValue(
        "Ubuntu 24.04 LTS Desktop",
      );
    });

    fireEvent.change(screen.getByLabelText("Name"), {
      target: { value: "" },
    });

    fireEvent.click(screen.getByRole("button", { name: "Save Changes" }));

    await waitFor(() => {
      expect(screen.getByText("Name is required")).toBeInTheDocument();
    });
  });
});
