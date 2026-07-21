import {
  cleanup,
  render,
  screen,
  fireEvent,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import { AdminCategoriesPage } from "@/pages/admin/AdminCategoriesPage";
import { ToastProvider } from "@/components/toast";

vi.mock("@/features/auth/token", async () => {
  const actual = await vi.importActual<typeof import("@/features/auth/token")>(
    "@/features/auth/token",
  );
  return { ...actual, getAccessToken: () => "fake-admin-token" };
});

vi.mock("@/config", () => ({
  getConfig: () => ({ API_URL: "http://localhost:8080", SITE_NAME: "Test" }),
}));

const FAKE_CATEGORIES = [
  {
    id: 1,
    name: "Movies",
    slug: "movies",
    parent_id: null,
    sort_order: 1,
    created_at: "2024-01-01T00:00:00Z",
    updated_at: "2024-01-01T00:00:00Z",
  },
  {
    id: 2,
    name: "TV",
    slug: "tv",
    parent_id: null,
    sort_order: 2,
    created_at: "2024-01-01T00:00:00Z",
    updated_at: "2024-01-01T00:00:00Z",
  },
  {
    id: 9,
    name: "HD",
    slug: "hd",
    parent_id: 1,
    sort_order: 1,
    created_at: "2024-01-01T00:00:00Z",
    updated_at: "2024-01-01T00:00:00Z",
  },
];

afterEach(cleanup);

beforeEach(() => {
  vi.clearAllMocks();
  vi.restoreAllMocks();
});

// Renders the list page alongside stand-in edit routes so navigation from the
// list (Add / Edit) can be asserted by which sentinel page shows.
function renderPage() {
  return render(
    <MemoryRouter initialEntries={["/admin/categories"]}>
      <ToastProvider>
        <Routes>
          <Route path="/admin/categories" element={<AdminCategoriesPage />} />
          <Route
            path="/admin/categories/new"
            element={<div>NEW CATEGORY PAGE</div>}
          />
          <Route
            path="/admin/categories/:id/edit"
            element={<div>EDIT CATEGORY PAGE</div>}
          />
        </Routes>
      </ToastProvider>
    </MemoryRouter>,
  );
}

describe("AdminCategoriesPage", () => {
  test("renders categories table after fetch", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce({
      ok: true,
      json: async () => ({ categories: FAKE_CATEGORIES }),
    } as Response);

    renderPage();

    await waitFor(() => {
      expect(screen.getAllByText("Movies").length).toBeGreaterThanOrEqual(1);
    });

    expect(screen.getByText("TV")).toBeInTheDocument();
    expect(screen.getByText("HD")).toBeInTheDocument();
    expect(screen.getByText("movies")).toBeInTheDocument();
  });

  test("shows loading state", () => {
    vi.spyOn(globalThis, "fetch").mockReturnValue(new Promise(() => {}));

    renderPage();

    expect(screen.getByText("Loading...")).toBeInTheDocument();
  });

  test("shows empty state when no categories", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce({
      ok: true,
      json: async () => ({ categories: [] }),
    } as Response);

    renderPage();

    await waitFor(() => {
      expect(screen.getByText("No categories found.")).toBeInTheDocument();
    });
  });

  test("navigates to the new-category page when Add Category is clicked", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce({
      ok: true,
      json: async () => ({ categories: FAKE_CATEGORIES }),
    } as Response);

    renderPage();

    await waitFor(() => {
      expect(screen.getAllByText("Movies").length).toBeGreaterThanOrEqual(1);
    });

    fireEvent.click(screen.getByRole("button", { name: "Add Category" }));

    expect(screen.getByText("NEW CATEGORY PAGE")).toBeInTheDocument();
  });

  test("navigates to the edit page when Edit is clicked", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce({
      ok: true,
      json: async () => ({ categories: FAKE_CATEGORIES }),
    } as Response);

    renderPage();

    await waitFor(() => {
      expect(screen.getAllByText("Movies").length).toBeGreaterThanOrEqual(1);
    });

    fireEvent.click(screen.getAllByText("Edit")[0]);

    expect(screen.getByText("EDIT CATEGORY PAGE")).toBeInTheDocument();
  });

  test("shows error when delete fails due to torrents", async () => {
    const fetchSpy = vi.spyOn(globalThis, "fetch");

    fetchSpy.mockResolvedValueOnce({
      ok: true,
      json: async () => ({ categories: FAKE_CATEGORIES }),
    } as Response);

    renderPage();

    await waitFor(() => {
      expect(screen.getAllByText("Movies").length).toBeGreaterThanOrEqual(1);
    });

    const deleteButtons = screen.getAllByText("Delete");
    fireEvent.click(deleteButtons[0]);

    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByText(/Delete the "Movies"/)).toBeInTheDocument();

    fetchSpy.mockResolvedValueOnce({
      ok: false,
      status: 409,
      json: async () => ({
        error: {
          code: "conflict",
          message: "category has torrents and cannot be deleted",
        },
      }),
    } as Response);

    fireEvent.click(within(dialog).getByRole("button", { name: "Delete" }));

    await waitFor(() => {
      expect(
        screen.getAllByText("category has torrents and cannot be deleted")
          .length,
      ).toBeGreaterThanOrEqual(1);
    });
  });

  test("does not delete when confirmation dialog is cancelled", async () => {
    const fetchSpy = vi.spyOn(globalThis, "fetch");

    fetchSpy.mockResolvedValueOnce({
      ok: true,
      json: async () => ({ categories: FAKE_CATEGORIES }),
    } as Response);

    renderPage();

    await waitFor(() => {
      expect(screen.getAllByText("Movies").length).toBeGreaterThanOrEqual(1);
    });

    const deleteButtons = screen.getAllByText("Delete");
    fireEvent.click(deleteButtons[0]);

    const dialog = await screen.findByRole("dialog");
    fireEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));

    await waitFor(() => {
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    });
    expect(fetchSpy).not.toHaveBeenCalledWith(
      expect.stringContaining("/admin/categories/1"),
      expect.anything(),
    );
  });
});
