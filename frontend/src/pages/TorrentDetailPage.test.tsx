import {
  cleanup,
  render,
  screen,
  fireEvent,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, test, expect, vi } from "vitest";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { TorrentDetailPage } from "@/pages/TorrentDetailPage";
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
  category_id: 1,
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
  mockGET.mockResolvedValue({
    data: { torrent: FAKE_TORRENT },
    error: undefined,
  });
});

function renderDetailPage(
  id = "1",
  authContext: AuthContextValue = makeAuthContext(),
) {
  return render(
    <AuthContext.Provider value={authContext}>
      <ToastProvider>
        <MemoryRouter initialEntries={[`/torrent/${id}`]}>
          <Routes>
            <Route path="/torrent/:id" element={<TorrentDetailPage />} />
          </Routes>
        </MemoryRouter>
      </ToastProvider>
    </AuthContext.Provider>,
  );
}

describe("TorrentDetailPage", () => {
  test("shows loading state initially", () => {
    mockGET.mockReturnValue(new Promise(() => {}));
    renderDetailPage();
    expect(screen.getByText("Loading torrent...")).toBeInTheDocument();
  });

  test("renders torrent name after loading", async () => {
    renderDetailPage();
    await waitFor(() => {
      expect(screen.getByText("Ubuntu 24.04 LTS Desktop")).toBeInTheDocument();
    });
  });

  test("renders info hash", async () => {
    renderDetailPage();
    await waitFor(() => {
      expect(
        screen.getByText("a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0"),
      ).toBeInTheDocument();
    });
  });

  test("renders category label", async () => {
    renderDetailPage();
    await waitFor(() => {
      expect(screen.getByText("Linux ISOs")).toBeInTheDocument();
    });
  });

  test("renders seeders and leechers stats", async () => {
    renderDetailPage();
    await waitFor(() => {
      expect(screen.getByText("Seeders")).toBeInTheDocument();
    });
    expect(screen.getByText("Leechers")).toBeInTheDocument();
    expect(screen.getByText("42")).toBeInTheDocument();
    expect(screen.getByText("5")).toBeInTheDocument();
  });

  test("renders times completed stat", async () => {
    renderDetailPage();
    await waitFor(() => {
      expect(screen.getByText("Completed")).toBeInTheDocument();
    });
    expect(screen.getByText("318")).toBeInTheDocument();
  });

  test("renders file count", async () => {
    renderDetailPage();
    await waitFor(() => {
      expect(screen.getByText("3")).toBeInTheDocument();
    });
  });

  test("renders description when present", async () => {
    renderDetailPage();
    await waitFor(() => {
      expect(
        screen.getByText("The latest Ubuntu release."),
      ).toBeInTheDocument();
    });
    expect(screen.getByText("Description")).toBeInTheDocument();
  });

  test("does not render description section when absent", async () => {
    mockGET.mockResolvedValue({
      data: { torrent: { ...FAKE_TORRENT, description: undefined } },
      error: undefined,
    });
    renderDetailPage();
    await waitFor(() => {
      expect(screen.getByText("Ubuntu 24.04 LTS Desktop")).toBeInTheDocument();
    });
    expect(screen.queryByText("Description")).not.toBeInTheDocument();
  });

  test("renders download button", async () => {
    renderDetailPage();
    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "Download .torrent" }),
      ).toBeInTheDocument();
    });
  });

  test("shows error on API failure", async () => {
    mockGET.mockResolvedValue({
      data: undefined,
      error: { error: { message: "Torrent not found" } },
    });
    renderDetailPage();
    await waitFor(() => {
      expect(screen.getByText("Torrent not found")).toBeInTheDocument();
    });
  });

  test("shows error for invalid torrent ID", async () => {
    renderDetailPage("abc");
    await waitFor(() => {
      expect(screen.getByText("Invalid torrent ID")).toBeInTheDocument();
    });
    expect(mockGET).not.toHaveBeenCalled();
  });

  test("renders health indicator", async () => {
    renderDetailPage();
    await waitFor(() => {
      expect(screen.getByText("Ubuntu 24.04 LTS Desktop")).toBeInTheDocument();
    });
    const healthDot = document.querySelector(".torrent-detail__health");
    expect(healthDot).not.toBeNull();
    expect(healthDot?.classList.contains("torrent-detail__health--good")).toBe(
      true,
    );
  });

  test("passes authorization header to API", async () => {
    renderDetailPage();
    await waitFor(() => {
      expect(mockGET).toHaveBeenCalledWith(
        "/api/v1/torrents/{id}",
        expect.objectContaining({
          headers: { Authorization: "Bearer fake-token" },
        }),
      );
    });
  });

  // Edit/Delete button tests

  test("shows edit and delete buttons for torrent owner", async () => {
    renderDetailPage();
    await waitFor(() => {
      expect(screen.getByText("Edit")).toBeInTheDocument();
    });
    expect(screen.getByRole("button", { name: "Delete" })).toBeInTheDocument();
  });

  test("shows edit and delete buttons for admin", async () => {
    const adminContext = makeAuthContext(makeUser({ id: 999, isAdmin: true }));
    renderDetailPage("1", adminContext);
    await waitFor(() => {
      expect(screen.getByText("Edit")).toBeInTheDocument();
    });
    expect(screen.getByRole("button", { name: "Delete" })).toBeInTheDocument();
  });

  test("hides edit and delete buttons for non-owner non-admin", async () => {
    const otherUserContext = makeAuthContext(
      makeUser({ id: 999, isAdmin: false }),
    );
    renderDetailPage("1", otherUserContext);
    await waitFor(() => {
      expect(screen.getByText("Ubuntu 24.04 LTS Desktop")).toBeInTheDocument();
    });
    expect(screen.queryByText("Edit")).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Delete" }),
    ).not.toBeInTheDocument();
  });

  test("edit button links to edit page", async () => {
    renderDetailPage();
    await waitFor(() => {
      expect(screen.getByText("Edit")).toBeInTheDocument();
    });
    const editLink = screen.getByText("Edit").closest("a");
    expect(editLink).toHaveAttribute("href", "/torrent/1/edit");
  });

  test("delete button opens confirmation modal", async () => {
    renderDetailPage();
    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "Delete" }),
      ).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: "Delete" }));

    await waitFor(() => {
      expect(screen.getByText("Delete Torrent")).toBeInTheDocument();
    });
    expect(
      screen.getByText(/Are you sure you want to delete this torrent/),
    ).toBeInTheDocument();
    expect(screen.getByLabelText("Reason for deletion")).toBeInTheDocument();
  });

  test("delete calls API and redirects on success", async () => {
    const mockFetch = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(new Response(null, { status: 204 }));

    renderDetailPage();
    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "Delete" }),
      ).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: "Delete" }));

    await waitFor(() => {
      expect(screen.getByText("Delete Torrent")).toBeInTheDocument();
    });

    fireEvent.change(screen.getByLabelText("Reason for deletion"), {
      target: { value: "Duplicate upload" },
    });

    // Click the modal's delete/confirm button
    const confirmButtons = screen.getAllByRole("button", { name: "Delete" });
    const modalConfirmBtn = confirmButtons.find((btn) =>
      btn.classList.contains("torrent-detail__delete-modal-confirm"),
    );
    fireEvent.click(modalConfirmBtn!);

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledTimes(1);
    });

    const [url, options] = mockFetch.mock.calls[0];
    expect(url).toBe("http://localhost:8080/api/v1/torrents/1");
    expect(options?.method).toBe("DELETE");
    expect(JSON.parse(options?.body as string)).toEqual({
      reason: "Duplicate upload",
    });

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith("/browse");
    });
  });

  // Report button tests

  test("shows report button for logged-in user", async () => {
    renderDetailPage();
    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "Report" }),
      ).toBeInTheDocument();
    });
  });

  test("hides report button when not logged in", async () => {
    const anonContext = makeAuthContext(null);
    renderDetailPage("1", anonContext);
    await waitFor(() => {
      expect(screen.getByText("Ubuntu 24.04 LTS Desktop")).toBeInTheDocument();
    });
    expect(
      screen.queryByRole("button", { name: "Report" }),
    ).not.toBeInTheDocument();
  });

  test("report button opens report modal", async () => {
    renderDetailPage();
    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "Report" }),
      ).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: "Report" }));

    await waitFor(() => {
      expect(screen.getByText("Report Torrent")).toBeInTheDocument();
    });
    expect(
      screen.getByText(/Please describe why you are reporting/),
    ).toBeInTheDocument();
    expect(screen.getByLabelText("Reason")).toBeInTheDocument();
  });

  test("report submits to POST /api/v1/reports", async () => {
    const mockFetch = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(new Response(null, { status: 201 }));

    renderDetailPage();
    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "Report" }),
      ).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: "Report" }));

    await waitFor(() => {
      expect(screen.getByText("Report Torrent")).toBeInTheDocument();
    });

    fireEvent.change(screen.getByLabelText("Reason"), {
      target: { value: "Fake content" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Submit Report" }));

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledTimes(1);
    });

    const [url, options] = mockFetch.mock.calls[0];
    expect(url).toBe("http://localhost:8080/api/v1/reports");
    expect(options?.method).toBe("POST");
    expect(JSON.parse(options?.body as string)).toEqual({
      torrent_id: 1,
      reason: "Fake content",
    });
  });

  test("report shows error toast on API failure", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(JSON.stringify({ error: { message: "Already reported" } }), {
        status: 409,
        headers: { "Content-Type": "application/json" },
      }),
    );

    renderDetailPage();
    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "Report" }),
      ).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: "Report" }));

    await waitFor(() => {
      expect(screen.getByText("Report Torrent")).toBeInTheDocument();
    });

    fireEvent.change(screen.getByLabelText("Reason"), {
      target: { value: "Bad content" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Submit Report" }));

    await waitFor(() => {
      expect(screen.getByText("Already reported")).toBeInTheDocument();
    });
  });

  test("delete shows error toast on API failure", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(
        JSON.stringify({ error: { message: "Permission denied" } }),
        { status: 403, headers: { "Content-Type": "application/json" } },
      ),
    );

    renderDetailPage();
    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "Delete" }),
      ).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: "Delete" }));

    await waitFor(() => {
      expect(screen.getByText("Delete Torrent")).toBeInTheDocument();
    });

    const confirmButtons = screen.getAllByRole("button", { name: "Delete" });
    const modalConfirmBtn = confirmButtons.find((btn) =>
      btn.classList.contains("torrent-detail__delete-modal-confirm"),
    );
    fireEvent.click(modalConfirmBtn!);

    await waitFor(() => {
      expect(screen.getByText("Permission denied")).toBeInTheDocument();
    });
  });
});
