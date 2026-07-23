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
import { AdminCategoryEditPage } from "@/pages/admin/AdminCategoryEditPage";
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
    image_url: null,
    sort_order: 1,
    metadata_schema: [{ key: "year", label: "Year", type: "number" }],
    created_at: "2024-01-01T00:00:00Z",
    updated_at: "2024-01-01T00:00:00Z",
  },
  {
    id: 2,
    name: "TV",
    slug: "tv",
    parent_id: null,
    image_url: null,
    sort_order: 2,
    metadata_schema: [],
    created_at: "2024-01-01T00:00:00Z",
    updated_at: "2024-01-01T00:00:00Z",
  },
  {
    id: 3,
    name: "4K",
    slug: "4k",
    parent_id: 1, // child of Movies
    image_url: null,
    sort_order: 1,
    metadata_schema: [{ key: "hdr", label: "HDR", type: "boolean" }],
    created_at: "2024-01-01T00:00:00Z",
    updated_at: "2024-01-01T00:00:00Z",
  },
];

// Resolves the category list and the per-category metadata-schema endpoint
// (used for inherited fields); records POST/PUT for assertions.
function mockFetch() {
  return vi.spyOn(globalThis, "fetch").mockImplementation((url, init) => {
    const method = (init?.method ?? "GET").toUpperCase();
    const u = String(url);
    if (method === "GET" && u.includes("metadata-schema")) {
      const m = u.match(/categories\/(\d+)\/metadata-schema/);
      const cat = FAKE_CATEGORIES.find((c) => String(c.id) === m?.[1]);
      return Promise.resolve({
        ok: true,
        json: async () => ({ fields: cat?.metadata_schema ?? [] }),
      } as Response);
    }
    if (method === "GET") {
      return Promise.resolve({
        ok: true,
        json: async () => ({ categories: FAKE_CATEGORIES }),
      } as Response);
    }
    return Promise.resolve({
      ok: true,
      json: async () => ({ category: { id: 1 } }),
    } as Response);
  });
}

function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <ToastProvider>
        <Routes>
          <Route path="/admin/categories" element={<div>LIST PAGE</div>} />
          <Route
            path="/admin/categories/new"
            element={<AdminCategoryEditPage />}
          />
          <Route
            path="/admin/categories/:id/edit"
            element={<AdminCategoryEditPage />}
          />
        </Routes>
      </ToastProvider>
    </MemoryRouter>,
  );
}

afterEach(cleanup);
beforeEach(() => {
  vi.clearAllMocks();
  vi.restoreAllMocks();
});

describe("AdminCategoryEditPage", () => {
  test("loads an existing category and its fields", async () => {
    mockFetch();
    renderAt("/admin/categories/1/edit");

    await waitFor(() => {
      expect(screen.getByLabelText("Name")).toHaveValue("Movies");
    });
    expect(screen.getByLabelText("Slug")).toHaveValue("movies");
    // Existing metadata field shows as a table row.
    expect(screen.getByTestId("field-row")).toHaveTextContent("Year");
  });

  test("shows not-found for a missing category", async () => {
    mockFetch();
    renderAt("/admin/categories/999/edit");

    await waitFor(() => {
      expect(screen.getByText("Category not found.")).toBeInTheDocument();
    });
  });

  test("adds a metadata field and saves it via PUT", async () => {
    const fetchSpy = mockFetch();
    renderAt("/admin/categories/1/edit");

    await waitFor(() => {
      expect(screen.getByLabelText("Name")).toHaveValue("Movies");
    });

    // Add a new field through the modal.
    fireEvent.click(screen.getByRole("button", { name: "Add Field" }));
    fireEvent.change(screen.getByLabelText("Key"), {
      target: { value: "codec" },
    });
    fireEvent.change(screen.getByLabelText("Label"), {
      target: { value: "Codec" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save Field" }));

    await waitFor(() => {
      expect(screen.getAllByTestId("field-row")).toHaveLength(2);
    });

    fireEvent.click(screen.getByRole("button", { name: "Save Category" }));

    await waitFor(() => {
      expect(screen.getByText("LIST PAGE")).toBeInTheDocument();
    });

    const putCall = fetchSpy.mock.calls.find(
      ([, init]) => (init?.method ?? "").toUpperCase() === "PUT",
    );
    expect(putCall).toBeTruthy();
    expect(putCall?.[0]).toContain("/admin/categories/1");
    const body = JSON.parse((putCall?.[1] as RequestInit).body as string);
    expect(body.metadata_schema.map((f: { key: string }) => f.key)).toEqual([
      "year",
      "codec",
    ]);
  });

  test("creates a new category via POST", async () => {
    const fetchSpy = mockFetch();
    renderAt("/admin/categories/new");

    await waitFor(() => {
      expect(screen.getByLabelText("Name")).toBeInTheDocument();
    });

    fireEvent.change(screen.getByLabelText("Name"), {
      target: { value: "Games" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save Category" }));

    await waitFor(() => {
      expect(screen.getByText("LIST PAGE")).toBeInTheDocument();
    });

    const postCall = fetchSpy.mock.calls.find(
      ([url, init]) =>
        (init?.method ?? "").toUpperCase() === "POST" &&
        String(url).endsWith("/admin/categories"),
    );
    expect(postCall).toBeTruthy();
    const body = JSON.parse((postCall?.[1] as RequestInit).body as string);
    expect(body.name).toBe("Games");
  });

  test("create page still renders if the category list fetch fails", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue({
      ok: false,
      status: 500,
      json: async () => ({}),
    } as Response);
    renderAt("/admin/categories/new");

    // A failed list fetch must not block creation (it only feeds parent options).
    await waitFor(() => {
      expect(screen.getByLabelText("Name")).toBeInTheDocument();
    });
    expect(screen.queryByText("Category not found.")).not.toBeInTheDocument();
  });

  test("shows read-only inherited fields when editing a sub-category", async () => {
    mockFetch();
    renderAt("/admin/categories/3/edit"); // 4K, child of Movies

    await waitFor(() => {
      expect(screen.getByLabelText("Name")).toHaveValue("4K");
    });
    // Inherited from Movies (parent): the "Year" field, read-only.
    const inherited = await screen.findByTestId("inherited-fields");
    expect(within(inherited).getByText("Year")).toBeInTheDocument();
    // The sub-category's own editable field is separate.
    expect(screen.getByTestId("field-row")).toHaveTextContent("HDR");
  });

  test("parent dropdown excludes the category itself and its descendants", async () => {
    mockFetch();
    renderAt("/admin/categories/1/edit"); // Movies has child 4K

    await waitFor(() => {
      expect(screen.getByLabelText("Name")).toHaveValue("Movies");
    });
    const select = screen.getByLabelText("Parent Category");
    // TV (an unrelated top-level) is selectable...
    expect(
      within(select).getByRole("option", { name: "TV" }),
    ).toBeInTheDocument();
    // ...but not Movies itself, nor its descendant 4K (would create a cycle).
    expect(
      within(select).queryByRole("option", { name: "Movies" }),
    ).not.toBeInTheDocument();
    expect(
      within(select).queryByRole("option", { name: "4K" }),
    ).not.toBeInTheDocument();
  });

  test("?parent= preselects the parent and shows its inherited fields on create", async () => {
    mockFetch();
    renderAt("/admin/categories/new?parent=1"); // create under Movies

    await waitFor(() => {
      expect(screen.getByLabelText("Parent Category")).toHaveValue("1");
    });
    const inherited = await screen.findByTestId("inherited-fields");
    expect(within(inherited).getByText("Year")).toBeInTheDocument();
  });
});
