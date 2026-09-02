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
    stages_advanced: 1,
    stages_decayed: 0,
  },
];

const stages = [
  {
    stage: 1,
    min_active_hnr: 1,
    min_days_in_prev: 0,
    action: "notify",
    restriction_types: [],
    restriction_days: 0,
    message_template: "{{username}} has {{count}} active hit-and-runs.",
  },
  {
    stage: 2,
    min_active_hnr: 3,
    min_days_in_prev: 5,
    action: "restrict",
    restriction_types: ["download"],
    restriction_days: 7,
    message_template: "Restricted.",
  },
];

function mockApi(stagesOverride = stages) {
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
    if (method === "GET" && url.endsWith("/hnr/stages")) {
      return Promise.resolve({
        ok: true,
        json: async () => ({ stages: stagesOverride }),
      });
    }
    if (url.includes("/hnr/stages/")) {
      return Promise.resolve({ ok: true, json: async () => ({ stage: {} }) });
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

  test("lists penalty ladder stages with their configured action", async () => {
    mockApi();
    renderPage();
    await screen.findByText("User");

    expect(await screen.findByText("Penalty ladder")).toBeInTheDocument();
    expect(screen.getByLabelText("Stage 1 action")).toHaveValue("notify");
    expect(screen.getByLabelText("Stage 2 action")).toHaveValue("restrict");
    // Only the restrict stage shows the restriction-type checkboxes.
    expect(
      screen.getByLabelText("Stage 2 min active hit-and-runs"),
    ).toHaveValue(3);
  });

  test("restriction type checkboxes only appear for the restrict action", async () => {
    mockApi();
    renderPage();
    await screen.findByText("User");

    // Stage 1 is "notify" — no restriction checkboxes, just the explainer.
    const stage1Row = screen.getByLabelText("Stage 1 action").closest("tr")!;
    expect(
      within(stage1Row).getByText("only for restrict"),
    ).toBeInTheDocument();

    // Stage 2 is "restrict" with download pre-checked.
    const stage2Row = screen.getByLabelText("Stage 2 action").closest("tr")!;
    const downloadCheckbox = within(stage2Row).getByRole("checkbox", {
      name: "Download",
    });
    expect(downloadCheckbox).toBeChecked();
  });

  test("saving an edited stage sends a PUT to that stage number", async () => {
    const calls = mockApi();
    const user = userEvent.setup();
    renderPage();
    await screen.findByText("User");

    const dwellInput = screen.getByLabelText("Stage 1 dwell days");
    await user.clear(dwellInput);
    await user.type(dwellInput, "2");

    const stage1Row = dwellInput.closest("tr")!;
    await user.click(within(stage1Row).getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(
        calls.some(
          (c) => c.method === "PUT" && c.url.endsWith("/hnr/stages/1"),
        ),
      ).toBe(true);
    });
    const call = calls.find(
      (c) => c.method === "PUT" && c.url.endsWith("/hnr/stages/1"),
    );
    const body = JSON.parse(call!.body!);
    expect(body.min_days_in_prev).toBe(2);
  });

  test("deleting a stage sends a DELETE to that stage number", async () => {
    const calls = mockApi();
    const user = userEvent.setup();
    renderPage();
    await screen.findByText("User");

    const stage2Row = screen.getByLabelText("Stage 2 action").closest("tr")!;
    await user.click(within(stage2Row).getByRole("button", { name: "Delete" }));

    await waitFor(() => {
      expect(
        calls.some(
          (c) => c.method === "DELETE" && c.url.endsWith("/hnr/stages/2"),
        ),
      ).toBe(true);
    });
  });

  test("adding a new stage sends a PUT to the entered stage number", async () => {
    const calls = mockApi();
    const user = userEvent.setup();
    renderPage();
    await screen.findByText("User");

    await user.type(screen.getByLabelText("New stage number"), "3");
    await user.click(screen.getByRole("button", { name: "Add stage" }));

    await waitFor(() => {
      expect(
        calls.some(
          (c) => c.method === "PUT" && c.url.endsWith("/hnr/stages/3"),
        ),
      ).toBe(true);
    });
  });
});
