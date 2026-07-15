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

const rules = [
  {
    group_id: 5,
    group_name: "User",
    group_level: 20,
    is_staff: false,
    min_ratio: 1,
    min_uploaded: 1000,
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
    // PUT / DELETE rule
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
  test("lists non-staff classes and excludes staff groups", async () => {
    mockApi();
    renderPage();
    expect(await screen.findByText("User")).toBeInTheDocument();
    expect(screen.getByText("Power User")).toBeInTheDocument();
    // The admin (staff) group must never appear as a ladder candidate.
    expect(screen.queryByText("Administrator")).not.toBeInTheDocument();
  });

  test("shows editable inputs for laddered classes and an add button otherwise", async () => {
    mockApi();
    renderPage();
    // User is on the ladder → has a Min Ratio input.
    expect(await screen.findByLabelText("User Min Ratio")).toBeInTheDocument();
    // Power User is not → shows an add button, no inputs.
    expect(
      screen.getByRole("button", { name: "Add to ladder" }),
    ).toBeInTheDocument();
    expect(screen.queryByLabelText("Power User Min Ratio")).toBeNull();
  });

  test("adds a class to the ladder via PUT", async () => {
    const calls = mockApi();
    const user = userEvent.setup();
    renderPage();

    await screen.findByText("Power User");
    await user.click(screen.getByRole("button", { name: "Add to ladder" }));

    await waitFor(() => {
      expect(
        calls.some(
          (c) => c.method === "PUT" && c.url.endsWith("/promotion/rules/4"),
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
