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

type TestRule = {
  group_id: number;
  required_seed_hours: number;
  required_ratio: number;
  inactivity_grace_hours: number;
  max_days_to_satisfy: number;
  clear_pricing_mode?: "fixed" | "deficit" | null;
  clear_base_points?: number | null;
  clear_points_per_gib?: number | null;
  clear_points_per_gib_deficit?: number | null;
};

// Only the User class is tracked to start; VIP has no rule (exempt).
const rules: TestRule[] = [
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

type TestStage = {
  stage: number;
  /** null = the rung follows the site-wide hnr_penalty_threshold. */
  min_active_hnr: number | null;
  min_days_in_prev: number;
  action: string;
  restriction_types: string[];
  restriction_days: number;
  message_template: string;
};

const stages: TestStage[] = [
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

type ExemptRule = {
  id: number;
  criterion: "min_seeders" | "max_size_bytes";
  threshold: number;
  enabled: boolean;
};

function mockApi(
  stagesOverride: TestStage[] = stages,
  rulesOverride: TestRule[] = rules,
  exemptRulesOverride: ExemptRule[] = [],
) {
  const calls: { method: string; url: string; body?: string }[] = [];
  mockFetch.mockImplementation((url: string, init?: FetchInit) => {
    const method = init?.method ?? "GET";
    calls.push({ method, url, body: init?.body });
    if (method === "GET" && url.endsWith("/admin/groups")) {
      return Promise.resolve({ ok: true, json: async () => ({ groups }) });
    }
    if (method === "GET" && url.endsWith("/hnr/rules")) {
      return Promise.resolve({
        ok: true,
        json: async () => ({ rules: rulesOverride }),
      });
    }
    if (method === "GET" && url.endsWith("/hnr/exempt-rules")) {
      return Promise.resolve({
        ok: true,
        json: async () => ({ rules: exemptRulesOverride }),
      });
    }
    if (url.includes("/hnr/exempt-rules")) {
      // POST / PUT / DELETE
      return Promise.resolve({
        ok: true,
        json: async () => ({ rule: { id: 99 } }),
      });
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
    if (method === "GET" && url.includes("/hnr/stats")) {
      return Promise.resolve({
        ok: true,
        json: async () => ({
          active_hnr: 3,
          monitored: 5,
          satisfied: 10,
          cleared: 2,
          waived: 1,
          breached_today: 1,
          top_offenders: [
            {
              user_id: 9,
              username: "repeat_offender",
              active_hnr: 3,
              total_records: 4,
              stage: 2,
            },
          ],
        }),
      });
    }
    if (method === "GET" && url.includes("/hnr/records")) {
      return Promise.resolve({
        ok: true,
        json: async () => ({
          records: [
            {
              id: 1,
              user_id: 9,
              username: "repeat_offender",
              torrent_id: 100,
              torrent_name: "Some Release",
              torrent_size: 1000,
              torrent_exempt: false,
              state: "hnr",
              completed_at: "2026-08-01T00:00:00Z",
              last_seen_at: "2026-08-02T00:00:00Z",
              seeded_seconds: 10,
              uploaded: 5,
            },
          ],
          total: 1,
        }),
      });
    }
    if (method === "GET" && url.endsWith("/admin/settings")) {
      return Promise.resolve({
        ok: true,
        json: async () => ({
          settings: [{ key: "hnr_penalty_threshold", value: "50" }],
        }),
      });
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
    // "User" is tracked, so it appears in both the thresholds table and the
    // clear-pricing table below it.
    expect((await screen.findAllByText("User")).length).toBeGreaterThan(0);
    expect(screen.getAllByText("VIP").length).toBeGreaterThan(0);
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

    await screen.findAllByText("User");
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

    await screen.findAllByText("User");
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

    await screen.findAllByText("User");
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

  test("sends a per-class clear-pricing override in the PUT body", async () => {
    const calls = mockApi();
    const user = userEvent.setup();
    renderPage();

    await screen.findAllByText("User");
    await user.type(screen.getByLabelText("User Base pts"), "200");
    await user.selectOptions(
      screen.getByLabelText("User clear pricing mode"),
      "deficit",
    );
    await user.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() => {
      const put = calls.find(
        (c) => c.method === "PUT" && c.url.endsWith("/hnr/rules/5"),
      );
      expect(put).toBeTruthy();
      expect(JSON.parse(put!.body!)).toMatchObject({
        clear_base_points: 200,
        clear_pricing_mode: "deficit",
      });
    });
  });

  test("renders a stored per-class clear-pricing override back into the inputs", async () => {
    mockApi(stages, [
      {
        ...rules[0],
        clear_pricing_mode: "deficit",
        clear_base_points: 150,
        clear_points_per_gib: null,
        clear_points_per_gib_deficit: 8,
      },
    ]);
    renderPage();

    expect(await screen.findByLabelText("User Base pts")).toHaveValue(150);
    expect(screen.getByLabelText("User Pts / GiB (deficit)")).toHaveValue(8);
    // An unset dimension stays blank, not 0.
    expect(screen.getByLabelText("User Pts / GiB")).toHaveValue(null);
    expect(screen.getByLabelText("User clear pricing mode")).toHaveValue(
      "deficit",
    );
  });

  test("omits a blank clear-pricing field so the class inherits the default", async () => {
    const calls = mockApi();
    const user = userEvent.setup();
    renderPage();

    await screen.findAllByText("User");
    // Change only a threshold, leave every pricing field blank.
    await user.clear(screen.getByLabelText("User Seed Hours"));
    await user.type(screen.getByLabelText("User Seed Hours"), "300");
    await user.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() => {
      const put = calls.find(
        (c) => c.method === "PUT" && c.url.endsWith("/hnr/rules/5"),
      );
      expect(put).toBeTruthy();
      const body = JSON.parse(put!.body!);
      expect(body).not.toHaveProperty("clear_pricing_mode");
      expect(body).not.toHaveProperty("clear_base_points");
      expect(body).not.toHaveProperty("clear_points_per_gib");
      expect(body).not.toHaveProperty("clear_points_per_gib_deficit");
    });
  });

  test("renders auto-exempt rules and adds a new one", async () => {
    const calls = mockApi(stages, rules, [
      { id: 1, criterion: "min_seeders", threshold: 50, enabled: true },
    ]);
    const user = userEvent.setup();
    renderPage();

    // Existing rule renders into its inputs.
    expect(await screen.findByLabelText("Rule 1 threshold")).toHaveValue(50);
    expect(screen.getByLabelText("Rule 1 enabled")).toBeChecked();

    // Add a new one.
    await user.selectOptions(
      screen.getByLabelText("New rule criterion"),
      "max_size_bytes",
    );
    await user.type(screen.getByLabelText("New rule threshold"), "4096");
    await user.click(screen.getByRole("button", { name: "Add rule" }));

    await waitFor(() => {
      const post = calls.find(
        (c) => c.method === "POST" && c.url.endsWith("/hnr/exempt-rules"),
      );
      expect(post).toBeTruthy();
      expect(JSON.parse(post!.body!)).toMatchObject({
        criterion: "max_size_bytes",
        threshold: 4096,
        enabled: true,
      });
    });
  });

  test("runs the daemon on demand", async () => {
    const calls = mockApi();
    const user = userEvent.setup();
    renderPage();

    await screen.findAllByText("User");
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
    await screen.findAllByText("User");
    expect(await screen.findByText("success")).toBeInTheDocument();
    expect(screen.getByText("schedule")).toBeInTheDocument();
  });

  test("lists penalty ladder stages with their configured action", async () => {
    mockApi();
    renderPage();
    await screen.findAllByText("User");

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
    await screen.findAllByText("User");

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
    await screen.findAllByText("User");

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

  test("a rung with no threshold of its own shows the site-wide one as its placeholder", async () => {
    const inheriting = [
      { ...stages[0] },
      { ...stages[1], min_active_hnr: null },
    ];
    mockApi(inheriting);
    renderPage();
    await screen.findAllByText("User");

    const input = screen.getByLabelText("Stage 2 min active hit-and-runs");
    expect(input).toHaveValue(null);
    expect(input).toHaveAttribute("placeholder", "50");
    // The rung that pins its own figure still shows it.
    expect(
      screen.getByLabelText("Stage 1 min active hit-and-runs"),
    ).toHaveValue(1);
  });

  test("clearing a rung's threshold saves it as null so it follows the setting", async () => {
    const calls = mockApi();
    const user = userEvent.setup();
    renderPage();
    await screen.findAllByText("User");

    const input = screen.getByLabelText("Stage 2 min active hit-and-runs");
    await user.clear(input);

    const row = input.closest("tr")!;
    await user.click(within(row).getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(
        calls.some(
          (c) => c.method === "PUT" && c.url.endsWith("/hnr/stages/2"),
        ),
      ).toBe(true);
    });
    const call = calls.find(
      (c) => c.method === "PUT" && c.url.endsWith("/hnr/stages/2"),
    );
    expect(JSON.parse(call!.body!).min_active_hnr).toBeNull();
  });

  test("saving the penalty threshold PUTs the site setting", async () => {
    const calls = mockApi();
    const user = userEvent.setup();
    renderPage();
    await screen.findAllByText("User");

    const input = screen.getByLabelText(
      "Site-wide hit-and-run penalty threshold",
    );
    await user.clear(input);
    await user.type(input, "5");
    await user.click(screen.getByRole("button", { name: "Save threshold" }));

    await waitFor(() => {
      expect(
        calls.some(
          (c) =>
            c.method === "PUT" &&
            c.url.endsWith("/admin/settings/hnr_penalty_threshold"),
        ),
      ).toBe(true);
    });
    const call = calls.find((c) =>
      c.url.endsWith("/admin/settings/hnr_penalty_threshold"),
    );
    expect(JSON.parse(call!.body!).value).toBe("5");
  });

  test("deleting a stage sends a DELETE to that stage number", async () => {
    const calls = mockApi();
    const user = userEvent.setup();
    renderPage();
    await screen.findAllByText("User");

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
    await screen.findAllByText("User");

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

  test("shows the overview stats and the top-offenders leaderboard", async () => {
    mockApi();
    renderPage();
    await screen.findAllByText("User");

    const overviewHeading = await screen.findByText("Overview");
    const overviewPanel = overviewHeading.closest(
      ".admin-panel",
    ) as HTMLElement;
    expect(within(overviewPanel).getByText("In breach")).toBeInTheDocument();

    const offenderRow = (
      await within(overviewPanel).findByText("repeat_offender")
    ).closest("tr")!;
    expect(within(offenderRow).getByText("3")).toBeInTheDocument(); // active_hnr
    expect(within(offenderRow).getByText("4")).toBeInTheDocument(); // total_records
  });

  test("lists records and filters by state", async () => {
    const calls = mockApi();
    const user = userEvent.setup();
    renderPage();
    await screen.findAllByText("User");

    const recordsHeading = await screen.findByText("Records");
    const recordsPanel = recordsHeading.closest(".admin-panel") as HTMLElement;
    const recordRow = within(recordsPanel)
      .getByText("Some Release")
      .closest("tr")!;
    expect(within(recordRow).getByText("In breach")).toBeInTheDocument();

    await user.selectOptions(
      screen.getByLabelText("Filter records by state"),
      "hnr",
    );

    await waitFor(() => {
      expect(
        calls.some((c) => c.method === "GET" && c.url.includes("state=hnr")),
      ).toBe(true);
    });
  });
});
