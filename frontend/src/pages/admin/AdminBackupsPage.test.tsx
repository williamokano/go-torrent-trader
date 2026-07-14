import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { AdminBackupsPage } from "@/pages/admin/AdminBackupsPage";
import { ToastProvider } from "@/components/toast";

const mockFetch = vi.fn();
vi.stubGlobal("fetch", mockFetch);

const backup = {
  name: "backup-20260714T031500Z-a1b2c3d4.dump",
  size: 2048,
  created_at: new Date().toISOString(),
};

interface FetchInit {
  method?: string;
}

/**
 * Route the mocked fetch by URL + method. The list is served from a mutable
 * array so create/delete can change what a subsequent reload returns.
 */
function mockApi(options: {
  backups?: (typeof backup)[];
  createResponse?: unknown;
  deleteOk?: boolean;
}) {
  const state = { backups: options.backups ?? [] };

  mockFetch.mockImplementation((url: string, init?: FetchInit) => {
    const method = init?.method ?? "GET";

    if (url.includes("/download")) {
      return Promise.resolve({
        ok: true,
        blob: async () => new Blob(["PGDMP"]),
      });
    }
    if (method === "POST") {
      state.backups = [backup];
      return Promise.resolve(
        options.createResponse ?? { ok: true, json: async () => ({ backup }) },
      );
    }
    if (method === "DELETE") {
      if (options.deleteOk === false) {
        return Promise.resolve({
          ok: false,
          json: async () => ({
            error: { code: "not_found", message: "backup not found" },
          }),
        });
      }
      state.backups = [];
      return Promise.resolve({ ok: true, json: async () => ({}) });
    }
    return Promise.resolve({
      ok: true,
      json: async () => ({ backups: state.backups }),
    });
  });

  return state;
}

afterEach(cleanup);

beforeEach(() => {
  vi.clearAllMocks();
});

function renderPage() {
  return render(
    <ToastProvider>
      <MemoryRouter initialEntries={["/admin/backups"]}>
        <AdminBackupsPage />
      </MemoryRouter>
    </ToastProvider>,
  );
}

describe("AdminBackupsPage", () => {
  test("renders the page title and create button", () => {
    mockApi({});

    renderPage();

    expect(screen.getByText("Database Backups")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Create Backup" }),
    ).toBeInTheDocument();
  });

  test("shows an empty state when there are no backups", async () => {
    mockApi({});

    renderPage();

    await waitFor(() => {
      expect(screen.getByText("No backups yet.")).toBeInTheDocument();
    });
  });

  test("lists backups from the API with size and actions", async () => {
    mockApi({ backups: [backup] });

    renderPage();

    await waitFor(() => {
      expect(screen.getByText(backup.name)).toBeInTheDocument();
    });
    expect(screen.getByText("2.00 KB")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Download" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Delete" })).toBeInTheDocument();
  });

  test("fetches the admin backups endpoint on the API base URL", async () => {
    mockApi({});

    renderPage();

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining("/api/v1/admin/backups"),
        expect.anything(),
      );
    });
    // Must hit the backend, never a relative URL against the dev server.
    const [url] = mockFetch.mock.calls[0];
    expect(url).toMatch(/^https?:\/\//);
  });

  test("creates a backup with POST and reloads the list", async () => {
    const user = userEvent.setup();
    mockApi({});

    renderPage();
    await waitFor(() => {
      expect(screen.getByText("No backups yet.")).toBeInTheDocument();
    });

    await user.click(screen.getByRole("button", { name: "Create Backup" }));

    await waitFor(() => {
      expect(screen.getByText(backup.name)).toBeInTheDocument();
    });

    const postCall = mockFetch.mock.calls.find(
      (call) => call[1]?.method === "POST",
    );
    expect(postCall).toBeDefined();
    expect(postCall?.[0]).toContain("/api/v1/admin/backups");
  });

  test("surfaces the API error message when a backup is already running", async () => {
    const user = userEvent.setup();
    mockApi({
      createResponse: {
        ok: false,
        json: async () => ({
          error: {
            code: "backup_in_progress",
            message: "a backup is already in progress",
          },
        }),
      },
    });

    renderPage();
    await waitFor(() => {
      expect(screen.getByText("No backups yet.")).toBeInTheDocument();
    });

    await user.click(screen.getByRole("button", { name: "Create Backup" }));

    await waitFor(() => {
      expect(
        screen.getByText("a backup is already in progress"),
      ).toBeInTheDocument();
    });
  });

  test("asks for confirmation before deleting and calls DELETE with the encoded name", async () => {
    const user = userEvent.setup();
    mockApi({ backups: [backup] });

    renderPage();
    await waitFor(() => {
      expect(screen.getByText(backup.name)).toBeInTheDocument();
    });

    await user.click(screen.getByRole("button", { name: "Delete" }));

    const dialog = await screen.findByRole("dialog");
    expect(
      within(dialog).getByText(
        `Delete backup '${backup.name}'? This cannot be undone.`,
      ),
    ).toBeInTheDocument();

    // Nothing is deleted until the modal is confirmed.
    expect(
      mockFetch.mock.calls.some((call) => call[1]?.method === "DELETE"),
    ).toBe(false);

    await user.click(within(dialog).getByRole("button", { name: "Delete" }));

    await waitFor(() => {
      expect(screen.getByText("No backups yet.")).toBeInTheDocument();
    });

    const deleteCall = mockFetch.mock.calls.find(
      (call) => call[1]?.method === "DELETE",
    );
    expect(deleteCall?.[0]).toContain(
      `/api/v1/admin/backups/${encodeURIComponent(backup.name)}`,
    );
  });

  test("shows the API error when a delete fails", async () => {
    const user = userEvent.setup();
    mockApi({ backups: [backup], deleteOk: false });

    renderPage();
    await waitFor(() => {
      expect(screen.getByText(backup.name)).toBeInTheDocument();
    });

    await user.click(screen.getByRole("button", { name: "Delete" }));
    const dialog = await screen.findByRole("dialog");
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));

    await waitFor(() => {
      expect(screen.getByText("backup not found")).toBeInTheDocument();
    });
  });

  test("downloads a backup as a blob from the download endpoint", async () => {
    const user = userEvent.setup();
    const createObjectURL = vi.fn(() => "blob:backup");
    const revokeObjectURL = vi.fn();
    vi.stubGlobal("URL", { ...URL, createObjectURL, revokeObjectURL });

    mockApi({ backups: [backup] });

    renderPage();
    await waitFor(() => {
      expect(screen.getByText(backup.name)).toBeInTheDocument();
    });

    await user.click(screen.getByRole("button", { name: "Download" }));

    await waitFor(() => {
      expect(createObjectURL).toHaveBeenCalled();
    });
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:backup");

    const downloadCall = mockFetch.mock.calls.find((call) =>
      String(call[0]).includes("/download"),
    );
    expect(downloadCall?.[0]).toContain(
      `/api/v1/admin/backups/${encodeURIComponent(backup.name)}/download`,
    );

    vi.unstubAllGlobals();
    vi.stubGlobal("fetch", mockFetch);
  });

  test("shows an error when loading the list fails", async () => {
    mockFetch.mockRejectedValue(new Error("network down"));

    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Failed to load backups")).toBeInTheDocument();
    });
  });
});
