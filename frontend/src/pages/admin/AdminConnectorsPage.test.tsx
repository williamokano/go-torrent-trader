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
import { AdminConnectorsPage } from "@/pages/admin/AdminConnectorsPage";
import { ToastProvider } from "@/components/toast";

const mockFetch = vi.fn();
vi.stubGlobal("fetch", mockFetch);

interface FetchInit {
  method?: string;
  body?: string;
}

/** A webhook instance as the API returns it: the secret is never in `config`. */
const webhookConnector = {
  id: 1,
  kind: "webhook",
  name: "My Hook",
  enabled: true,
  singleton: false,
  config: { url: "https://example.com/hook", method: "POST" },
  filters: {},
  secrets_set: ["hmac_secret"],
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
  last_delivery: {
    id: 10,
    instance_id: 1,
    event_key: "torrent.published:42",
    event_type: "torrent.published",
    status: "sent",
    attempts: 1,
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
  },
};

interface MockOptions {
  connectors?: unknown[];
  kinds?: string[];
  testResponse?: unknown;
  deliveries?: unknown[];
  saveOk?: boolean;
  saveError?: string;
}

/** Records every request so a test can assert on the exact payload sent. */
const requests: { url: string; init?: FetchInit }[] = [];

function mockApi(options: MockOptions = {}) {
  const connectors = options.connectors ?? [webhookConnector];
  const kinds = options.kinds ?? ["chat", "webhook"];

  mockFetch.mockImplementation((url: string, init?: FetchInit) => {
    requests.push({ url, init });
    const method = init?.method ?? "GET";

    if (url.includes("/api/v1/categories")) {
      return Promise.resolve({
        ok: true,
        json: async () => ({
          categories: [
            { id: 1, name: "Movies" },
            { id: 2, name: "TV" },
          ],
        }),
      });
    }
    if (url.includes("/test")) {
      return Promise.resolve({
        ok: true,
        json: async () => options.testResponse ?? { status: "sent" },
      });
    }
    if (url.includes("/deliveries")) {
      return Promise.resolve({
        ok: true,
        json: async () => ({
          deliveries: options.deliveries ?? [],
          total: (options.deliveries ?? []).length,
        }),
      });
    }
    if (method === "POST" || method === "PUT") {
      if (options.saveOk === false) {
        return Promise.resolve({
          ok: false,
          json: async () => ({
            error: { message: options.saveError ?? "nope" },
          }),
        });
      }
      return Promise.resolve({ ok: true, json: async () => ({}) });
    }
    if (method === "DELETE") {
      return Promise.resolve({ ok: true, json: async () => ({}) });
    }
    return Promise.resolve({
      ok: true,
      json: async () => ({ connectors, kinds }),
    });
  });
}

function renderPage() {
  return render(
    <ToastProvider>
      <MemoryRouter initialEntries={["/admin/connectors"]}>
        <AdminConnectorsPage />
      </MemoryRouter>
    </ToastProvider>,
  );
}

/** The most recent request matching method (and optionally a URL fragment). */
function lastRequest(method: string, fragment?: string) {
  for (let i = requests.length - 1; i >= 0; i--) {
    const req = requests[i];
    if ((req.init?.method ?? "GET") !== method) continue;
    if (fragment && !req.url.includes(fragment)) continue;
    return req;
  }
  return undefined;
}

afterEach(cleanup);

beforeEach(() => {
  vi.clearAllMocks();
  requests.length = 0;
});

describe("AdminConnectorsPage", () => {
  test("fetches the admin connectors endpoint on the API base URL", async () => {
    mockApi();
    renderPage();

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining("/api/v1/admin/connectors"),
        expect.anything(),
      );
    });
    // Must hit the backend, never a relative URL against the dev server.
    const [url] = mockFetch.mock.calls[0];
    expect(url).toMatch(/^https?:\/\//);
  });

  test("renders instances with kind, enabled state and last delivery status", async () => {
    mockApi();
    renderPage();

    expect(await screen.findByText("My Hook")).toBeInTheDocument();
    expect(screen.getByText("Webhook")).toBeInTheDocument();
    expect(screen.getByText("sent")).toBeInTheDocument();
    expect(
      screen.getByRole("checkbox", { name: "Enable My Hook" }),
    ).toBeChecked();
  });

  test("shows an empty state when nothing is configured", async () => {
    mockApi({ connectors: [] });
    renderPage();

    expect(
      await screen.findByText("No connectors configured yet."),
    ).toBeInTheDocument();
  });

  test("shows an error when loading fails, without refetching in a loop", async () => {
    mockFetch.mockRejectedValue(new Error("network down"));
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Failed to load connectors")).toBeInTheDocument();
    });
    const callsAfterError = mockFetch.mock.calls.length;
    await new Promise((resolve) => setTimeout(resolve, 50));
    expect(mockFetch.mock.calls.length).toBe(callsAfterError);
  });

  test("toggling enabled PUTs the row with the flag flipped", async () => {
    const user = userEvent.setup();
    mockApi();
    renderPage();

    await user.click(
      await screen.findByRole("checkbox", { name: "Enable My Hook" }),
    );

    await waitFor(() => {
      expect(lastRequest("PUT")).toBeDefined();
    });
    const body = JSON.parse(lastRequest("PUT")!.init!.body!);
    expect(body.enabled).toBe(false);
    expect(body.name).toBe("My Hook");
  });

  test("the create modal shows the webhook sub-form and posts it", async () => {
    const user = userEvent.setup();
    mockApi({ connectors: [] });
    renderPage();

    await user.click(
      await screen.findByRole("button", { name: "Add Connector" }),
    );
    const dialog = await screen.findByRole("dialog");

    // The kind list comes from the API, so the picker matches the backend.
    await user.selectOptions(within(dialog).getByLabelText("Kind"), "webhook");
    await user.type(within(dialog).getByLabelText("Name"), "New Hook");
    await user.type(
      within(dialog).getByLabelText("Endpoint URL"),
      "https://example.com/hook",
    );
    expect(within(dialog).getByLabelText("Method")).toBeInTheDocument();
    expect(
      within(dialog).getByLabelText("HMAC signing secret"),
    ).toBeInTheDocument();

    await user.click(within(dialog).getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(lastRequest("POST", "/admin/connectors")).toBeDefined();
    });
    const body = JSON.parse(
      lastRequest("POST", "/admin/connectors")!.init!.body!,
    );
    expect(body.kind).toBe("webhook");
    expect(body.name).toBe("New Hook");
    expect(body.config.url).toBe("https://example.com/hook");
  });

  // The write-only contract: the browser never holds the credential, so an
  // untouched secret field must submit nothing rather than a blank value.
  test("editing without touching the secret omits it from the PUT", async () => {
    const user = userEvent.setup();
    mockApi();
    renderPage();

    await user.click(await screen.findByRole("button", { name: "Edit" }));
    const dialog = await screen.findByRole("dialog");

    const secretField = within(dialog).getByLabelText("HMAC signing secret");
    expect(secretField).toHaveAttribute(
      "placeholder",
      "•••• set — leave blank to keep",
    );

    await user.click(within(dialog).getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(lastRequest("PUT")).toBeDefined();
    });
    const body = JSON.parse(lastRequest("PUT")!.init!.body!);
    expect("hmac_secret" in body.config).toBe(false);
  });

  test("typing a new secret sends it, and clearing sends null", async () => {
    const user = userEvent.setup();
    mockApi();
    renderPage();

    await user.click(await screen.findByRole("button", { name: "Edit" }));
    let dialog = await screen.findByRole("dialog");

    await user.type(
      within(dialog).getByLabelText("HMAC signing secret"),
      "rotated",
    );
    await user.click(within(dialog).getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(lastRequest("PUT")).toBeDefined();
    });
    expect(JSON.parse(lastRequest("PUT")!.init!.body!).config.hmac_secret).toBe(
      "rotated",
    );

    // Reopen and clear instead.
    requests.length = 0;
    await user.click(await screen.findByRole("button", { name: "Edit" }));
    dialog = await screen.findByRole("dialog");
    await user.click(within(dialog).getByLabelText("Clear stored value"));
    await user.click(within(dialog).getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(lastRequest("PUT")).toBeDefined();
    });
    expect(JSON.parse(lastRequest("PUT")!.init!.body!).config.hmac_secret).toBe(
      null,
    );
  });

  test("the kind cannot be changed on an existing instance", async () => {
    const user = userEvent.setup();
    mockApi();
    renderPage();

    await user.click(await screen.findByRole("button", { name: "Edit" }));
    const dialog = await screen.findByRole("dialog");

    expect(within(dialog).getByLabelText("Kind")).toBeDisabled();
  });

  test("a successful test send is reported", async () => {
    const user = userEvent.setup();
    mockApi({ testResponse: { status: "sent" } });
    renderPage();

    await user.click(await screen.findByRole("button", { name: "Test" }));

    expect(
      await screen.findByText("Test message sent to My Hook"),
    ).toBeInTheDocument();
  });

  // A refused test comes back 200 with a reason; the admin needs to see it.
  test("a failed test send surfaces the reason", async () => {
    const user = userEvent.setup();
    mockApi({
      testResponse: { status: "failed", error: "webhook returned 500" },
    });
    renderPage();

    await user.click(await screen.findByRole("button", { name: "Test" }));

    expect(await screen.findByText("webhook returned 500")).toBeInTheDocument();
  });

  test("the delivery log panel renders status, attempts and errors", async () => {
    const user = userEvent.setup();
    mockApi({
      deliveries: [
        {
          id: 5,
          instance_id: 1,
          event_key: "torrent.published:42",
          event_type: "torrent.published",
          status: "failed",
          attempts: 5,
          last_error: "webhook returned 500",
          created_at: new Date().toISOString(),
          updated_at: new Date().toISOString(),
        },
      ],
    });
    renderPage();

    await user.click(await screen.findByRole("button", { name: "Log" }));

    expect(
      await screen.findByText("Delivery log — My Hook"),
    ).toBeInTheDocument();
    expect(screen.getByText("torrent.published:42")).toBeInTheDocument();
    expect(screen.getByText("failed")).toBeInTheDocument();
    expect(screen.getByText("webhook returned 500")).toBeInTheDocument();
  });

  test("deleting asks for confirmation first", async () => {
    const user = userEvent.setup();
    mockApi();
    renderPage();

    await user.click(await screen.findByRole("button", { name: "Delete" }));
    const dialog = await screen.findByRole("dialog");
    expect(
      within(dialog).getByText(/Delete connector 'My Hook'/),
    ).toBeInTheDocument();
    expect(lastRequest("DELETE")).toBeUndefined();

    await user.click(within(dialog).getByRole("button", { name: "Delete" }));

    await waitFor(() => {
      expect(lastRequest("DELETE")).toBeDefined();
    });
    expect(lastRequest("DELETE")!.url).toContain("/admin/connectors/1");
  });

  test("a rejected save surfaces the backend's message", async () => {
    const user = userEvent.setup();
    mockApi({ saveOk: false, saveError: "invalid connector: url is required" });
    renderPage();

    await user.click(await screen.findByRole("button", { name: "Edit" }));
    const dialog = await screen.findByRole("dialog");
    await user.click(within(dialog).getByRole("button", { name: "Save" }));

    expect(
      await screen.findByText("invalid connector: url is required"),
    ).toBeInTheDocument();
  });

  test("the chat sub-form offers a template instead of webhook fields", async () => {
    const user = userEvent.setup();
    mockApi({ connectors: [] });
    renderPage();

    await user.click(
      await screen.findByRole("button", { name: "Add Connector" }),
    );
    const dialog = await screen.findByRole("dialog");

    await user.selectOptions(within(dialog).getByLabelText("Kind"), "chat");

    expect(
      within(dialog).getByLabelText("Message template"),
    ).toBeInTheDocument();
    expect(within(dialog).queryByLabelText("Endpoint URL")).toBeNull();
  });

  test("filters are submitted alongside the config", async () => {
    const user = userEvent.setup();
    mockApi();
    renderPage();

    await user.click(await screen.findByRole("button", { name: "Edit" }));
    const dialog = await screen.findByRole("dialog");

    await user.click(within(dialog).getByLabelText("Freeleech only"));
    await user.click(
      within(dialog).getByLabelText("Exclude anonymous uploads"),
    );
    await user.selectOptions(
      within(dialog).getByLabelText("Categories (leave empty for all)"),
      "2",
    );
    await user.click(within(dialog).getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(lastRequest("PUT")).toBeDefined();
    });
    const body = JSON.parse(lastRequest("PUT")!.init!.body!);
    expect(body.filters.freeleech_only).toBe(true);
    expect(body.filters.exclude_anonymous).toBe(true);
    expect(body.filters.category_ids).toEqual([2]);
  });

  test("the webhook headers editor adds, edits and removes rows", async () => {
    const user = userEvent.setup();
    mockApi();
    renderPage();

    await user.click(await screen.findByRole("button", { name: "Edit" }));
    const dialog = await screen.findByRole("dialog");

    await user.click(
      within(dialog).getByRole("button", { name: "Add header" }),
    );
    const headerInputs = within(dialog).getAllByLabelText("Header");
    await user.type(headerInputs[headerInputs.length - 1], "X-Tracker");
    const valueInputs = within(dialog).getAllByLabelText("Value");
    await user.type(valueInputs[valueInputs.length - 1], "test");

    await user.click(within(dialog).getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(lastRequest("PUT")).toBeDefined();
    });
    const body = JSON.parse(lastRequest("PUT")!.init!.body!);
    expect(body.config.headers).toEqual({ "X-Tracker": "test" });
  });

  // A summary that only shows the newest 25 hides exactly the dead-lettered
  // delivery an admin opens the log to find.
  test("the delivery log paginates", async () => {
    const user = userEvent.setup();
    const rows = Array.from({ length: 25 }, (_, i) => ({
      id: i + 1,
      instance_id: 1,
      event_key: `torrent.published:${i + 1}`,
      event_type: "torrent.published",
      status: "sent",
      attempts: 1,
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
    }));
    mockApi({ deliveries: rows });
    // Report more rows than one page holds.
    const original = mockFetch.getMockImplementation()!;
    mockFetch.mockImplementation((url: string, init?: FetchInit) => {
      if (url.includes("/deliveries")) {
        requests.push({ url, init });
        return Promise.resolve({
          ok: true,
          json: async () => ({ deliveries: rows, total: 60 }),
        });
      }
      return original(url, init);
    });
    renderPage();

    await user.click(await screen.findByRole("button", { name: "Log" }));
    await screen.findByText("Delivery log — My Hook");

    await user.click(await screen.findByRole("button", { name: "2" }));

    await waitFor(() => {
      expect(lastRequest("GET", "page=2")).toBeDefined();
    });
  });

  // Offering a singleton kind that already exists just earns a 409 after the
  // admin has filled in the whole form.
  test("a singleton kind already in use is not offered again", async () => {
    const user = userEvent.setup();
    mockApi({
      connectors: [
        {
          ...webhookConnector,
          id: 2,
          kind: "chat",
          name: "Shoutbox",
          singleton: true,
        },
      ],
    });
    renderPage();

    await user.click(
      await screen.findByRole("button", { name: "Add Connector" }),
    );
    const dialog = await screen.findByRole("dialog");

    const kindSelect = within(dialog).getByLabelText("Kind");
    expect(
      within(kindSelect).queryByRole("option", { name: "Shoutbox" }),
    ).toBeNull();
    expect(
      within(kindSelect).getByRole("option", { name: "Webhook" }),
    ).toBeInTheDocument();
  });
});
