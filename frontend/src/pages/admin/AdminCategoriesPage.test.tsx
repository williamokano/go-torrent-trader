import {
  cleanup,
  render,
  screen,
  fireEvent,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
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

function renderPage() {
  return render(
    <MemoryRouter>
      <ToastProvider>
        <AdminCategoriesPage />
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

  test("opens create modal when Add Category is clicked", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce({
      ok: true,
      json: async () => ({ categories: FAKE_CATEGORIES }),
    } as Response);

    renderPage();

    await waitFor(() => {
      expect(screen.getAllByText("Movies").length).toBeGreaterThanOrEqual(1);
    });

    fireEvent.click(screen.getByRole("button", { name: "Add Category" }));

    await waitFor(() => {
      expect(screen.getByRole("dialog")).toBeInTheDocument();
    });
  });

  test("opens edit modal when Edit is clicked", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce({
      ok: true,
      json: async () => ({ categories: FAKE_CATEGORIES }),
    } as Response);

    renderPage();

    await waitFor(() => {
      expect(screen.getAllByText("Movies").length).toBeGreaterThanOrEqual(1);
    });

    const editButtons = screen.getAllByText("Edit");
    fireEvent.click(editButtons[0]);

    await waitFor(() => {
      expect(screen.getByRole("dialog")).toBeInTheDocument();
    });
    expect(screen.getByText("Edit Category")).toBeInTheDocument();
  });

  test("creates a category successfully", async () => {
    const fetchSpy = vi.spyOn(globalThis, "fetch");

    // Initial fetch
    fetchSpy.mockResolvedValueOnce({
      ok: true,
      json: async () => ({ categories: [] }),
    } as Response);

    renderPage();

    await waitFor(() => {
      expect(screen.getByText("No categories found.")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: "Add Category" }));

    await waitFor(() => {
      expect(screen.getByRole("dialog")).toBeInTheDocument();
    });

    // Fill in the form
    const nameInput = screen.getByLabelText("Name");
    fireEvent.change(nameInput, { target: { value: "New Category" } });

    // POST success
    fetchSpy.mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        category: {
          id: 10,
          name: "New Category",
          slug: "new-category",
          parent_id: null,
          sort_order: 0,
        },
      }),
    } as Response);

    // Re-fetch categories
    fetchSpy.mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        categories: [
          {
            id: 10,
            name: "New Category",
            slug: "new-category",
            parent_id: null,
            sort_order: 0,
            created_at: "2024-01-01T00:00:00Z",
            updated_at: "2024-01-01T00:00:00Z",
          },
        ],
      }),
    } as Response);

    fireEvent.click(screen.getByText("Save"));

    await waitFor(() => {
      expect(screen.getByText("New Category")).toBeInTheDocument();
    });
  });

  test("shows error when delete fails due to torrents", async () => {
    const fetchSpy = vi.spyOn(globalThis, "fetch");

    fetchSpy.mockResolvedValueOnce({
      ok: true,
      json: async () => ({ categories: FAKE_CATEGORIES }),
    } as Response);

    // Mock window.confirm
    vi.spyOn(window, "confirm").mockReturnValue(true);

    renderPage();

    await waitFor(() => {
      expect(screen.getAllByText("Movies").length).toBeGreaterThanOrEqual(1);
    });

    // DELETE fails with conflict
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

    const deleteButtons = screen.getAllByText("Delete");
    fireEvent.click(deleteButtons[0]);

    await waitFor(() => {
      expect(
        screen.getAllByText("category has torrents and cannot be deleted")
          .length,
      ).toBeGreaterThanOrEqual(1);
    });
  });
});
