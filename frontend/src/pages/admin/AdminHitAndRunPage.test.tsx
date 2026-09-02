import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { AdminHitAndRunPage } from "@/pages/admin/AdminHitAndRunPage";
import { ToastProvider } from "@/components/toast";

const mockFetch = vi.fn();
vi.stubGlobal("fetch", mockFetch);

interface FetchInit {
  method?: string;
  body?: string;
}

const groups = [
  { id: 5, name: "User", level: 20, is_admin: false, is_moderator: false },
  { id: 6, name: "VIP", level: 60, is_admin: false, is_moderator: false },
  {
    id: 1,
    name: "Administrator",
    level: 100,
    is_admin: true,
    is_moderator: false,
  },
];

// Only the User class is tracked to start; VIP has no rule (exempt).
const rules = [
  {
    group_id: 5,
    required_seed_hours: 240,
    required_ratio: 1,
    inactivity_grace_hours: 48,
    max_days_to_satisfy: 30,
  },
];

const runs = [
  {
    id: 1,
    started_at: "2026-08-01T00:00:00Z",
    finished_at: "2026-08-01T00:00:05Z",
    status: "success",
    trigger: "schedule",
    scanned: 10,
    breached: 2,
    satisfied: 1,
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
    if (method === "GET" && url.endsWith("/hnr/rules")) {
      return Promise.resolve({ ok: true, json: async () => ({ rules }) });
    }
    if (method === "GET" && url.includes("/hnr/runs")) {
      return Promise.resolve({ ok: true, json: async () => ({ runs }) });
    }
    if (url.endsWith("/hnr/run")) {
      return Promise.resolve({
        ok: true,
        json: async () => ({
          skipped: false,
          scanned: 5,
          breached: 1,
          satisfied: 0,
        }),
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
        <AdminHitAndRunPage />
      </ToastProvider>
    </MemoryRouter>,
  );
}

beforeEach(() => mockFetch.mockReset());
afterEach(() => {
  cleanup();
  mockFetch.mockReset();
});

describe("AdminHitAndRunPage", () => {
  test("lists non-staff classes and excludes staff", async () => {
    mockApi();
    renderPage();
    expect(await screen.findByText("User")).toBeInTheDocument();
    expect(screen.getByText("VIP")).toBeInTheDocument();
    expect(screen.queryByText("Administrator")).not.toBeInTheDocument();
  });

  test("enables threshold inputs only for tracked classes", async () => {
    mockApi();
    renderPage();
    // User has a rule → editable.
    expect(await screen.findByLabelText("User Ratio")).toBeEnabled();
    // VIP has no rule → present but disabled until toggled on.
    expect(screen.getByLabelText("VIP Ratio")).toBeDisabled();
  });

  test("shows the save bar only after a change", async () => {
    mockApi();
    const user = userEvent.setup();
    renderPage();

    await screen.findByText("User");
    expect(
      screen.queryByRole("button", { name: "Save changes" }),
    ).not.toBeInTheDocument();

    await user.click(screen.getByLabelText("Track VIP for hit-and-run"));
    expect(
      await screen.findByRole("button", { name: "Save changes" }),
    ).toBeInTheDocument();
    expect(screen.getByText("1 unsaved change")).toBeInTheDocument();
  });

  test("saves a new tracked class with PUT", async () => {
    const calls = mockApi();
    const user = userEvent.setup();
    renderPage();

    await screen.findByText("User");
    await user.click(screen.getByLabelText("Track VIP for hit-and-run"));
    await user.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() => {
      expect(
        calls.some((c) => c.method === "PUT" && c.url.endsWith("/hnr/rules/6")),
      ).toBe(true);
    });
  });

  test("removes a tracked class with DELETE", async () => {
    const calls = mockApi();
    const user = userEvent.setup();
    renderPage();

    await screen.findByText("User");
    await user.click(screen.getByLabelText("Track User for hit-and-run")); // toggle off
    await user.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() => {
      expect(
        calls.some(
          (c) => c.method === "DELETE" && c.url.endsWith("/hnr/rules/5"),
        ),
      ).toBe(true);
    });
  });

  test("runs the daemon on demand", async () => {
    const calls = mockApi();
    const user = userEvent.setup();
    renderPage();

    await screen.findByText("User");
    await user.click(screen.getByRole("button", { name: "Run now" }));

    await waitFor(() => {
      expect(
        calls.some((c) => c.method === "POST" && c.url.endsWith("/hnr/run")),
      ).toBe(true);
    });
  });

  test("shows the recent run log", async () => {
    mockApi();
    renderPage();
    await screen.findByText("User");
    expect(await screen.findByText("success")).toBeInTheDocument();
    expect(screen.getByText("schedule")).toBeInTheDocument();
  });
});
