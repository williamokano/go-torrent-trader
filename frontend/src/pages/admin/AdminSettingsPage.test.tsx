// The published configuration page, read as text. A ?raw import rather than
// node:fs because this is a browser-targeted project with no @types/node, and
// pulling Node's globals into the app's type space to check a doc would be a
// poor trade.
import configureHtml from "../../../../website/configure.html?raw";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, test, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { AdminSettingsPage } from "@/pages/admin/AdminSettingsPage";
import { ToastProvider } from "@/components/toast";

const mockFetch = vi.fn();
vi.stubGlobal("fetch", mockFetch);

function renderPage() {
  return render(
    <MemoryRouter>
      <ToastProvider>
        <AdminSettingsPage />
      </ToastProvider>
    </MemoryRouter>,
  );
}

afterEach(() => {
  cleanup();
  mockFetch.mockReset();
});

describe("AdminSettingsPage", () => {
  test("renders friendly labels and descriptions for cheat and announce-log settings", async () => {
    const now = new Date().toISOString();
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => ({
        settings: [
          { key: "cheat_detection_enabled", value: "true", updated_at: now },
          { key: "announce_log_retention_days", value: "90", updated_at: now },
        ],
      }),
    });

    renderPage();

    // Previously these rendered as their raw keys; now they resolve to labels.
    expect(
      await screen.findByText("Cheat Detection Enabled"),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        /each announce is checked for impossible upload speeds/i,
      ),
    ).toBeInTheDocument();

    expect(
      await screen.findByText("Announce Log Retention (days)"),
    ).toBeInTheDocument();
    // The description has to state that the window now deletes, and that the
    // monthly totals survive it — an operator shortening this setting needs to
    // know what it costs and what it does not.
    expect(
      screen.getByText(/deletes raw rows past this window/i),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/without losing anyone's transfer totals/i),
    ).toBeInTheDocument();

    // The raw key must no longer surface as a label.
    expect(
      screen.queryByText("cheat_detection_enabled"),
    ).not.toBeInTheDocument();
  });

  // Both surfaces once told an operator that shortening the retention window
  // "reclaims disk". Expiry is a chunked DELETE (AnnounceEventRepo.DeleteOlderThan),
  // so once autovacuum sweeps them the dead rows' space goes to the table's own
  // free-space map and is reused by later announces. The database file does not
  // shrink. Someone who cut 90 days to 30 to get disk back got none of it, and
  // paid two months of history for it.
  //
  // What this guards is AGREEMENT, not today's wording. The two surfaces live in
  // different directories and are edited by different kinds of change, which is
  // exactly how they came to make the same wrong promise and would be how one
  // gets corrected without the other. Asserting a specific claim would be worse
  // than useless: #221 replaces the DELETE with a partition DROP, which does
  // return files to the filesystem, and a test demanding today's sentence would
  // then require copy that had become false.
  test("both operator-facing surfaces make the same claim about disk", async () => {
    const now = new Date().toISOString();
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => ({
        settings: [
          { key: "announce_log_retention_days", value: "90", updated_at: now },
        ],
      }),
    });

    renderPage();
    const adminDescription = normalize(
      (await screen.findByText("Announce Log Retention (days)"))
        .closest("td")
        ?.querySelector(".admin-settings__description")?.textContent,
    );

    const surfaces = {
      "admin settings page": adminDescription,
      "website/configure.html": websiteSettingDescription(
        "announce_log_retention_days",
      ),
    };

    for (const [where, text] of Object.entries(surfaces)) {
      expect(text, `${where} description is empty`).not.toHaveLength(0);
      // A tripwire on the exact sentence that shipped, nothing more. It cannot
      // enumerate every way of being wrong and does not try; the agreement check
      // below is the part that carries weight.
      expect(text.toLowerCase(), where).not.toContain("reclaims disk");
      expect(text, `${where} stopped mentioning disk at all`).toMatch(/disk/i);
    }

    // The durable assertion. Whatever either surface says about where the space
    // goes, the other has to say it too — so correcting one and forgetting the
    // other fails here, whichever one is edited and whatever the new claim is.
    expect(
      vocabulary(surfaces["admin settings page"], DISK_TERMS),
      "the two surfaces disagree about where the deleted space goes",
    ).toEqual(vocabulary(surfaces["website/configure.html"], DISK_TERMS));

    expect(
      vocabulary(surfaces["admin settings page"], FLOOR_TERMS),
      "the two surfaces disagree about the window being a floor",
    ).toEqual(vocabulary(surfaces["website/configure.html"], FLOOR_TERMS));
  });

  // An operator whose saved value is not in force has to see that on the field
  // they saved it in. Before #255 the only trace was a slog.Warn in the server
  // log — somewhere nobody looks after changing an admin setting, which is how
  // "I set 7 days" and "the site keeps 31" coexisted without anyone noticing.
  test("says so when a saved setting is not the one in force", async () => {
    const now = new Date().toISOString();
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => ({
        settings: [
          {
            key: "announce_log_retention_days",
            value: "7",
            updated_at: now,
            effective_value: "31",
            override_reason:
              "Raw announces are kept for 31 days, not 7: the shorter window is held open by automatic class promotion.",
          },
        ],
      }),
    });

    renderPage();

    expect(await screen.findByText(/In force: 31, not 7/)).toBeInTheDocument();
    expect(
      screen.getByText(/held open by automatic class promotion/),
    ).toBeInTheDocument();
  });

  // And a setting that is in force says nothing, so the panel does not cry wolf
  // on every row.
  test("says nothing when the saved setting is the one in force", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => ({
        settings: [
          {
            key: "announce_log_retention_days",
            value: "90",
            updated_at: new Date().toISOString(),
          },
        ],
      }),
    });

    renderPage();

    await screen.findByText("Announce Log Retention (days)");
    expect(screen.queryByText(/In force:/)).not.toBeInTheDocument();
  });

  // Settings render grouped under section headings (added alongside the
  // hit-and-run feature, which otherwise would have made the previously flat,
  // alphabetical-by-key table considerably harder to scan), in
  // SETTING_DEFINITIONS order rather than the alphabetical order the backend
  // returns rows in.
  test("groups settings under section headings, not alphabetically", async () => {
    const now = new Date().toISOString();
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => ({
        settings: [
          // Alphabetically, bonus_enabled would sort before hnr_enabled, which
          // would sort before registration_mode — the opposite of the expected
          // section order (Registration, then ..., then Bonus point economy,
          // then Hit-and-run last).
          { key: "hnr_enabled", value: "false", updated_at: now },
          { key: "bonus_enabled", value: "false", updated_at: now },
          { key: "registration_mode", value: "invite_only", updated_at: now },
        ],
      }),
    });

    renderPage();

    await screen.findByText("Registration");
    const headings = screen
      .getAllByRole("heading", { level: 2 })
      .map((h) => h.textContent);
    const registrationIdx = headings.indexOf("Registration");
    const bonusIdx = headings.indexOf("Bonus point economy");
    const hnrIdx = headings.indexOf("Hit-and-run");

    expect(registrationIdx).toBeGreaterThanOrEqual(0);
    expect(bonusIdx).toBeGreaterThan(registrationIdx);
    expect(hnrIdx).toBeGreaterThan(bonusIdx);
  });

  test("labels the hit-and-run settings", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => ({
        settings: [
          {
            key: "hnr_enabled",
            value: "false",
            updated_at: new Date().toISOString(),
          },
          {
            key: "hnr_seed_credit_cap_minutes",
            value: "45",
            updated_at: new Date().toISOString(),
          },
        ],
      }),
    });

    renderPage();

    expect(await screen.findByText("Hit-and-Run Enabled")).toBeInTheDocument();
    expect(screen.getByText("Seed Credit Cap (minutes)")).toBeInTheDocument();
    expect(screen.getByText(/anti-gaming guard/i)).toBeInTheDocument();
  });

  // The page renders whatever rows the API returns, so a setting without a
  // definition here shows up as its bare key — which is how an operator ends up
  // unable to tell what "chat_system_display_name" does.
  test("labels the shoutbox system name setting", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => ({
        settings: [
          {
            key: "chat_system_display_name",
            value: "System",
            updated_at: new Date().toISOString(),
          },
        ],
      }),
    });

    renderPage();

    expect(await screen.findByText("Shoutbox System Name")).toBeInTheDocument();
    expect(
      screen.getByText(/author label shown on shoutbox announcements/i),
    ).toBeInTheDocument();
    expect(screen.getByDisplayValue("System")).toBeInTheDocument();
    expect(
      screen.queryByText("chat_system_display_name"),
    ).not.toBeInTheDocument();
  });
});

function normalize(text: string | null | undefined): string {
  return (text ?? "").replace(/\s+/g, " ").trim();
}

// The terms that carry the claim about where deleted space goes. Which of these
// a description uses IS the claim, at the resolution that matters: "reused" and
// "filesystem" describe a DELETE, "partition" and "dropped" would describe what
// #221 replaces it with. Comparing the sets between the two surfaces catches a
// half-finished correction without either surface having to keep a fixed
// sentence.
// The second claim these two surfaces now duplicate: that the setting is a floor
// other features can raise, not a promise. Added in #255 to both descriptions,
// and without this the pair could drift apart again in the new direction —
// exactly what the disk vocabulary was introduced to prevent.
// Only the claim both surfaces must make: that this setting is a floor other
// features can raise. Deliberately not "class promotion" or "in force" — the
// website has to name what holds the window open because it is static, while the
// admin page reports the actual reason at runtime in the override note. A
// vocabulary that demands everything either surface says would fail on a
// difference that is correct.
const FLOOR_TERMS = ["floor", "shortest", "hold the window open"];

const DISK_TERMS = [
  "autovacuum",
  "vacuum full",
  "filesystem",
  "operating system",
  "free space",
  "reused",
  "shrink",
  "partition",
  "dropped",
  "reclaim",
];

function vocabulary(text: string, terms: string[]): string[] {
  const lower = text.toLowerCase();
  return terms.filter((term) => lower.includes(term));
}

// Pulls a setting's "What it does" cell out of the published configuration page
// by walking the real markup, so a row that is restructured or renamed fails
// loudly here instead of silently matching nothing. The key is looked up in the
// row's FIRST cell — the Setting column — so a description elsewhere that
// happens to mention the same key in <code> cannot win the match.
function websiteSettingDescription(key: string): string {
  const doc = new DOMParser().parseFromString(configureHtml, "text/html");

  const row = Array.from(doc.querySelectorAll("tr")).find((tr) => {
    const first = tr.querySelector("td");
    return first != null && normalize(first.textContent) === key;
  });
  if (!row) {
    throw new Error(`no row for ${key} in website/configure.html`);
  }
  const cells = row.querySelectorAll("td");
  if (cells.length !== 3) {
    throw new Error(
      `row for ${key} in website/configure.html has ${cells.length} cells, expected Setting/Default/What it does`,
    );
  }
  return normalize(cells[2].textContent);
}
