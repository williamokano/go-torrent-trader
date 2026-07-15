import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { AdminPromotionPage } from "@/pages/admin/AdminPromotionPage";
import { ToastProvider } from "@/components/toast";

const mockFetch = vi.fn();
vi.stubGlobal("fetch", mockFetch);

interface FetchInit {
  method?: string;
  body?: string;
}

const groups = [
  { id: 5, name: "User", level: 20, is_admin: false, is_moderator: false },
  {
    id: 4,
    name: "Power User",
    level: 40,
    is_admin: false,
    is_moderator: false,
  },
  {
    id: 1,
    name: "Administrator",
    level: 100,
    is_admin: true,
    is_moderator: false,
  },
];

// Only the User class is on the ladder to start.
const rules = [
  {
    group_id: 5,
    min_ratio: 1,
    min_uploaded: 0,
    min_age_days: 0,
    min_torrents: 0,
    min_seed_hours: 0,
  },
];

function mockApi() {
  const calls: { method: string; url: string; body?: string }[] = [];
  mockFetch.mockImplementation((url: string, init?: FetchInit) => {
    const method = init?.method ?? "GET";
    calls.push({ method, url, body: init?.body });
    if (method === "GET" && url.endsWith("/admin/groups")) {
      return Promise.resolve({ ok: true, json: async () => ({ groups }) });
    }
    if (method === "GET" && url.endsWith("/promotion/rules")) {
      return Promise.resolve({ ok: true, json: async () => ({ rules }) });
    }
    if (url.endsWith("/promotion/run")) {
      return Promise.resolve({
        ok: true,
        json: async () => ({ skipped: false, promoted: 2, demoted: 1 }),
      });
    }
    return Promise.resolve({ ok: true, json: async () => ({}) });
  });
  return calls;
}

function renderPage() {
  return render(
    <MemoryRouter>
      <ToastProvider>
        <AdminPromotionPage />
      </ToastProvider>
    </MemoryRouter>,
  );
}

beforeEach(() => mockFetch.mockReset());
afterEach(() => {
  cleanup();
  mockFetch.mockReset();
});

describe("AdminPromotionPage", () => {
  test("lists non-staff classes and excludes staff", async () => {
    mockApi();
    renderPage();
    expect(await screen.findByText("User")).toBeInTheDocument();
    expect(screen.getByText("Power User")).toBeInTheDocument();
    expect(screen.queryByText("Administrator")).not.toBeInTheDocument();
  });

  test("enables threshold inputs only for classes on the ladder", async () => {
    mockApi();
    renderPage();
    // User is on the ladder → editable.
    expect(await screen.findByLabelText("User Min Ratio")).toBeEnabled();
    // Power User is off the ladder → present but disabled until toggled on.
    expect(screen.getByLabelText("Power User Min Ratio")).toBeDisabled();
  });

  test("shows the save bar only after a change", async () => {
    mockApi();
    const user = userEvent.setup();
    renderPage();

    await screen.findByText("User");
    expect(
      screen.queryByRole("button", { name: "Save changes" }),
    ).not.toBeInTheDocument();

    await user.click(screen.getByLabelText("Include Power User in ladder"));
    expect(
      await screen.findByRole("button", { name: "Save changes" }),
    ).toBeInTheDocument();
    expect(screen.getByText("1 unsaved change")).toBeInTheDocument();
  });

  test("batches adding a class and editing another into one save", async () => {
    const calls = mockApi();
    const user = userEvent.setup();
    renderPage();

    await screen.findByText("User");
    // Add Power User to the ladder.
    await user.click(screen.getByLabelText("Include Power User in ladder"));
    // Edit the User class threshold.
    const ratio = screen.getByLabelText("User Min Ratio");
    await user.clear(ratio);
    await user.type(ratio, "2");

    expect(screen.getByText("2 unsaved changes")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() => {
      expect(
        calls.filter(
          (c) => c.method === "PUT" && c.url.includes("/promotion/rules/"),
        ).length,
      ).toBe(2);
    });
    expect(
      calls.some((c) => c.method === "PUT" && c.url.endsWith("/rules/4")),
    ).toBe(true);
    expect(
      calls.some((c) => c.method === "PUT" && c.url.endsWith("/rules/5")),
    ).toBe(true);
  });

  test("removes a class from the ladder with DELETE", async () => {
    const calls = mockApi();
    const user = userEvent.setup();
    renderPage();

    await screen.findByText("User");
    await user.click(screen.getByLabelText("Include User in ladder")); // toggle off
    await user.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() => {
      expect(
        calls.some(
          (c) => c.method === "DELETE" && c.url.endsWith("/promotion/rules/5"),
        ),
      ).toBe(true);
    });
  });

  test("runs the engine on demand", async () => {
    const calls = mockApi();
    const user = userEvent.setup();
    renderPage();

    await screen.findByText("User");
    await user.click(screen.getByRole("button", { name: "Run now" }));

    await waitFor(() => {
      expect(
        calls.some(
          (c) => c.method === "POST" && c.url.endsWith("/promotion/run"),
        ),
      ).toBe(true);
    });
  });
});
