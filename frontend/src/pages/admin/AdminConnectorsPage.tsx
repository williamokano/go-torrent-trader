import { useCallback, useEffect, useRef, useState } from "react";
import { getAccessToken } from "@/features/auth/token";
import { getConfig } from "@/config";
import { useToast } from "@/components/toast";
import { ConfirmModal, Modal } from "@/components/modal";
import {
  ByteSizeInput,
  Checkbox,
  Input,
  Select,
  Textarea,
} from "@/components/form";
import { Pagination } from "@/components/Pagination";
import { buildCategoryOptions } from "@/utils/categories";
import { timeAgo } from "@/utils/format";
import "./admin-ui.css";
import "./admin-connectors.css";

/** One delivery-log row. */
interface Delivery {
  id: number;
  instance_id: number;
  event_key: string;
  event_type: string;
  status: string;
  attempts: number;
  last_error?: string;
  next_attempt_at?: string;
  created_at: string;
  updated_at: string;
}

/**
 * A connector instance as the API returns it. `config` never contains a secret
 * — the keys listed in `secrets_set` are stripped server-side and only their
 * names come back, which is what drives the "leave blank to keep" fields.
 */
interface Connector {
  id: number;
  kind: string;
  name: string;
  enabled: boolean;
  singleton: boolean;
  config: Record<string, unknown>;
  filters: Filters;
  secrets_set: string[];
  created_at: string;
  updated_at: string;
  last_delivery?: Delivery;
}

interface Filters {
  event_types?: string[];
  category_ids?: number[];
  /** "include" (default) delivers only these categories; "exclude" delivers everything else. */
  category_mode?: "include" | "exclude";
  min_size?: number;
  freeleech_only?: boolean;
  exclude_anonymous?: boolean;
}

interface Category {
  id: number;
  name: string;
  parent_id: number | null;
  sort_order: number;
}

/** One field an admin's message template may reference, described by the API. */
interface TemplateField {
  token: string;
  description: string;
  example: string;
}

/** One IRC target channel, optionally narrowed to some categories. */
interface IrcChannel {
  name: string;
  categories?: number[];
}

const DELIVERIES_PER_PAGE = 25;

/** Which config keys each kind treats as write-only credentials. */
const SECRET_FIELDS: Record<string, string[]> = {
  webhook: ["hmac_secret"],
  irc: ["sasl_pass", "nickserv_pass"],
  // The Discord webhook URL is itself the credential — anyone holding it can
  // post to the channel — so it is write-only like any other secret.
  discord: ["webhook_url"],
  telegram: ["bot_token"],
};

const KIND_LABELS: Record<string, string> = {
  chat: "Shoutbox",
  webhook: "Webhook",
  irc: "IRC",
  sse: "Live Feed",
  discord: "Discord",
  telegram: "Telegram",
};

/** How often the IRC connection status is refreshed while the page is open. */
const STATUS_POLL_MS = 10_000;

/** One instance's lifecycle state on the node that served the request. */
interface ManagerStatus {
  state: string;
  since?: string;
  last_error?: string;
}

/**
 * Maps a connection state onto a badge. Anything that means "not currently
 * announcing" is red, because that is the question an admin is asking — a
 * standby node reporting not_owner is healthy for the cluster but is not
 * delivering from here.
 */
function stateBadgeClass(state: string): string {
  switch (state) {
    case "connected":
      return "admin-badge admin-badge--ok";
    case "connecting":
    case "acquiring":
    case "reconnecting":
      return "admin-badge admin-badge--warn";
    case "error":
    case "not_owner":
      return "admin-badge admin-badge--danger";
    default:
      return "admin-badge admin-badge--muted";
  }
}

const STATE_LABELS: Record<string, string> = {
  connected: "connected",
  connecting: "connecting",
  acquiring: "acquiring",
  reconnecting: "reconnecting",
  not_owner: "another node",
  stopped: "stopped",
  error: "error",
};

function kindLabel(kind: string): string {
  return KIND_LABELS[kind] ?? kind;
}

/** Maps a delivery status onto one of the shared badge modifiers. */
function statusBadgeClass(status: string): string {
  switch (status) {
    case "sent":
      return "admin-badge admin-badge--ok";
    case "coalesced":
      return "admin-badge admin-badge--accent";
    case "failed":
      return "admin-badge admin-badge--danger";
    default:
      return "admin-badge admin-badge--warn";
  }
}

interface FormState {
  kind: string;
  name: string;
  enabled: boolean;
  config: Record<string, unknown>;
  filters: Filters;
  /** Freshly typed secrets, keyed by config field. */
  secrets: Record<string, string>;
  /** Secrets the admin ticked "clear" on, which submit as explicit null. */
  clearSecrets: Record<string, boolean>;
}

/**
 * Defaults a kind needs in its config from the start.
 *
 * A field that only defaults at render time looks filled in but submits
 * nothing, so the admin sees "6697" in the Port box and a validation error
 * saying the port is 0.
 */
function defaultConfig(kind: string): Record<string, unknown> {
  switch (kind) {
    case "irc":
      return { port: 6697, tls: true, channels: [] };
    case "telegram":
      return { chat_ids: [] };
    default:
      return {};
  }
}

function emptyForm(kind: string): FormState {
  return {
    kind,
    name: "",
    enabled: true,
    config: defaultConfig(kind),
    filters: {},
    secrets: {},
    clearSecrets: {},
  };
}

/**
 * Builds the config payload for a save.
 *
 * A secret field is only included when the admin actually typed one (replace)
 * or ticked "clear" (explicit null). Leaving it untouched omits the key
 * entirely, which the backend reads as "keep what is stored" — the browser
 * never holds the credential, so it cannot resubmit it.
 */
function buildConfig(form: FormState): Record<string, unknown> {
  const config: Record<string, unknown> = { ...form.config };
  for (const field of SECRET_FIELDS[form.kind] ?? []) {
    delete config[field];
    if (form.clearSecrets[field]) {
      config[field] = null;
    } else if (form.secrets[field]) {
      config[field] = form.secrets[field];
    }
  }
  return config;
}

export function AdminConnectorsPage() {
  const toast = useToast();
  const [connectors, setConnectors] = useState<Connector[]>([]);
  const [kinds, setKinds] = useState<string[]>([]);
  const [categories, setCategories] = useState<Category[]>([]);
  const [loading, setLoading] = useState(true);
  const [modalOpen, setModalOpen] = useState(false);
  const [editingId, setEditingId] = useState<number | null>(null);
  const [form, setForm] = useState<FormState>(emptyForm(""));
  const [saving, setSaving] = useState(false);
  const [testingId, setTestingId] = useState<number | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Connector | null>(null);
  const [deleteLoading, setDeleteLoading] = useState(false);
  const [deliveriesFor, setDeliveriesFor] = useState<Connector | null>(null);
  const [deliveries, setDeliveries] = useState<Delivery[]>([]);
  const [deliveriesLoading, setDeliveriesLoading] = useState(false);
  const [deliveriesPage, setDeliveriesPage] = useState(1);
  const [deliveriesTotal, setDeliveriesTotal] = useState(0);
  const deliveriesRequestRef = useRef(0);
  const [togglingId, setTogglingId] = useState<number | null>(null);
  const [statuses, setStatuses] = useState<Record<string, ManagerStatus>>({});
  const [templateFields, setTemplateFields] = useState<TemplateField[]>([]);

  const fetchConnectors = useCallback(
    async (showLoading = true) => {
      // A background refresh after a toggle should not blank the table out.
      if (showLoading) setLoading(true);
      try {
        const token = getAccessToken();
        const res = await fetch(
          `${getConfig().API_URL}/api/v1/admin/connectors`,
          {
            headers: token ? { Authorization: `Bearer ${token}` } : {},
          },
        );
        if (!res.ok) {
          toast.error("Failed to load connectors");
          return;
        }
        const data = await res.json();
        setConnectors(data.connectors ?? []);
        setKinds(data.kinds ?? []);
        setTemplateFields(data.template_fields ?? []);
      } catch {
        toast.error("Failed to load connectors");
      } finally {
        if (showLoading) setLoading(false);
      }
    },
    [toast],
  );

  const fetchCategories = useCallback(async () => {
    try {
      const token = getAccessToken();
      const res = await fetch(`${getConfig().API_URL}/api/v1/categories`, {
        headers: token ? { Authorization: `Bearer ${token}` } : {},
      });
      if (!res.ok) return;
      const data = await res.json();
      setCategories(data.categories ?? []);
    } catch {
      // Categories only power an optional filter; the page is still usable.
    }
  }, []);

  useEffect(() => {
    fetchConnectors();
  }, [fetchConnectors]);

  useEffect(() => {
    fetchCategories();
  }, [fetchCategories]);

  const fetchStatuses = useCallback(async () => {
    try {
      const token = getAccessToken();
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/admin/connectors/status`,
        { headers: token ? { Authorization: `Bearer ${token}` } : {} },
      );
      if (!res.ok) return;
      const data = await res.json();
      setStatuses(data.statuses ?? {});
    } catch {
      // Status is a nicety on top of the list; a failed poll just leaves the
      // previous badge in place rather than nagging the admin.
    }
  }, []);

  // Only poll while there is a connection-holding instance to report on.
  const hasPersistent = connectors.some((c) => c.kind === "irc");
  useEffect(() => {
    if (!hasPersistent) return;
    fetchStatuses();
    const timer = setInterval(fetchStatuses, STATUS_POLL_MS);
    return () => clearInterval(timer);
  }, [hasPersistent, fetchStatuses]);

  /** Kinds that can still be created: a singleton already in use is spent. */
  function availableKinds(): string[] {
    return kinds.filter(
      (kind) => !connectors.some((c) => c.kind === kind && c.singleton),
    );
  }

  function openCreate() {
    setEditingId(null);
    setForm(emptyForm(availableKinds()[0] ?? ""));
    setModalOpen(true);
  }

  function openEdit(connector: Connector) {
    setEditingId(connector.id);
    setForm({
      kind: connector.kind,
      name: connector.name,
      enabled: connector.enabled,
      config: { ...connector.config },
      filters: { ...connector.filters },
      secrets: {},
      clearSecrets: {},
    });
    setModalOpen(true);
  }

  function closeModal() {
    setModalOpen(false);
    setEditingId(null);
    setForm(emptyForm(""));
  }

  async function handleSave() {
    setSaving(true);
    try {
      const token = getAccessToken();
      const url = editingId
        ? `${getConfig().API_URL}/api/v1/admin/connectors/${editingId}`
        : `${getConfig().API_URL}/api/v1/admin/connectors`;
      const res = await fetch(url, {
        method: editingId ? "PUT" : "POST",
        headers: {
          ...(token ? { Authorization: `Bearer ${token}` } : {}),
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          kind: form.kind,
          name: form.name,
          enabled: form.enabled,
          config: buildConfig(form),
          filters: form.filters,
        }),
      });
      if (!res.ok) {
        const body = await res.json().catch(() => null);
        toast.error(body?.error?.message ?? "Failed to save connector");
        return;
      }
      toast.success(editingId ? "Connector updated" : "Connector created");
      closeModal();
      await fetchConnectors();
    } catch {
      toast.error("Failed to save connector");
    } finally {
      setSaving(false);
    }
  }

  /**
   * The enable toggle is a whole-row PUT with the flag flipped. Secrets are not
   * in the payload, so the backend keeps the stored ones.
   */
  async function handleToggle(connector: Connector) {
    if (togglingId !== null) return;
    setTogglingId(connector.id);
    try {
      const token = getAccessToken();
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/admin/connectors/${connector.id}`,
        {
          method: "PUT",
          headers: {
            ...(token ? { Authorization: `Bearer ${token}` } : {}),
            "Content-Type": "application/json",
          },
          body: JSON.stringify({
            name: connector.name,
            enabled: !connector.enabled,
            config: connector.config,
            filters: connector.filters,
          }),
        },
      );
      if (!res.ok) {
        const body = await res.json().catch(() => null);
        toast.error(body?.error?.message ?? "Failed to update connector");
        return;
      }
      await fetchConnectors(false);
    } catch {
      toast.error("Failed to update connector");
    } finally {
      setTogglingId(null);
    }
  }

  async function handleTest(connector: Connector) {
    setTestingId(connector.id);
    try {
      const token = getAccessToken();
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/admin/connectors/${connector.id}/test`,
        {
          method: "POST",
          headers: token ? { Authorization: `Bearer ${token}` } : {},
        },
      );
      const body = await res.json().catch(() => null);
      if (!res.ok) {
        toast.error(body?.error?.message ?? "Test send failed");
        return;
      }
      // A refused test is a successful request with a "failed" answer, so the
      // reason lives in the payload rather than the status code.
      if (body?.status === "sent") {
        toast.success(`Test message sent to ${connector.name}`);
      } else {
        toast.error(body?.error || "Test send failed");
      }
      await fetchConnectors(false);
    } catch {
      toast.error("Test send failed");
    } finally {
      setTestingId(null);
    }
  }

  async function handleDeleteConfirm() {
    if (!deleteTarget) return;
    setDeleteLoading(true);
    try {
      const token = getAccessToken();
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/admin/connectors/${deleteTarget.id}`,
        {
          method: "DELETE",
          headers: token ? { Authorization: `Bearer ${token}` } : {},
        },
      );
      if (!res.ok) {
        toast.error("Failed to delete connector");
        return;
      }
      toast.success("Connector deleted");
      setDeleteTarget(null);
      await fetchConnectors();
    } catch {
      toast.error("Failed to delete connector");
    } finally {
      setDeleteLoading(false);
    }
  }

  const openDeliveries = useCallback(
    async (connector: Connector, page: number) => {
      setDeliveriesFor(connector);
      setDeliveriesPage(page);
      // Clear first: otherwise a failed or slow fetch leaves the previous
      // connector's rows sitting under the new connector's heading.
      setDeliveries([]);
      setDeliveriesTotal(0);
      setDeliveriesLoading(true);
      // Two quick clicks can resolve out of order; only the latest wins.
      const request = ++deliveriesRequestRef.current;
      try {
        const token = getAccessToken();
        const res = await fetch(
          `${getConfig().API_URL}/api/v1/admin/connectors/${connector.id}/deliveries?page=${page}&per_page=${DELIVERIES_PER_PAGE}`,
          { headers: token ? { Authorization: `Bearer ${token}` } : {} },
        );
        if (request !== deliveriesRequestRef.current) return;
        if (!res.ok) {
          toast.error("Failed to load delivery log");
          return;
        }
        const data = await res.json();
        setDeliveries(data.deliveries ?? []);
        setDeliveriesTotal(data.total ?? 0);
      } catch {
        if (request === deliveriesRequestRef.current) {
          toast.error("Failed to load delivery log");
        }
      } finally {
        if (request === deliveriesRequestRef.current) {
          setDeliveriesLoading(false);
        }
      }
    },
    [toast],
  );

  function setConfigField(key: string, value: unknown) {
    setForm((prev) => ({ ...prev, config: { ...prev.config, [key]: value } }));
  }

  function setFilterField(key: keyof Filters, value: unknown) {
    setForm((prev) => ({
      ...prev,
      filters: { ...prev.filters, [key]: value },
    }));
  }

  const editing = editingId
    ? connectors.find((c) => c.id === editingId)
    : undefined;
  const secretsSet = editing?.secrets_set ?? [];

  function renderSecretField(field: string, label: string) {
    const alreadySet = secretsSet.includes(field);
    return (
      <div className="admin-connectors__secret" key={field}>
        <Input
          label={label}
          type="password"
          autoComplete="new-password"
          value={form.secrets[field] ?? ""}
          disabled={form.clearSecrets[field]}
          placeholder={
            alreadySet ? "•••• set — leave blank to keep" : "Not set"
          }
          onChange={(e) =>
            setForm((prev) => ({
              ...prev,
              secrets: { ...prev.secrets, [field]: e.target.value },
            }))
          }
        />
        {alreadySet && (
          <Checkbox
            label="Clear stored value"
            checked={form.clearSecrets[field] ?? false}
            onChange={(e) =>
              setForm((prev) => ({
                ...prev,
                clearSecrets: {
                  ...prev.clearSecrets,
                  [field]: e.target.checked,
                },
              }))
            }
          />
        )}
      </div>
    );
  }

  /**
   * Free-form request headers. They are stored and returned like any other
   * config value — deliberately not treated as secrets in v1 — so the hint
   * steers credentials towards the HMAC secret instead.
   */
  function renderHeadersEditor() {
    const headers = (form.config.headers as Record<string, string>) ?? {};
    const entries = Object.entries(headers);

    function writeHeaders(next: Record<string, string>) {
      setConfigField("headers", next);
    }

    return (
      <div className="admin-connectors__headers">
        <span className="form-label">Extra request headers</span>
        <p className="admin-muted">
          Stored and shown in plain text. For authentication prefer the HMAC
          secret below, which is write-only.
        </p>
        {entries.map(([name, value], index) => (
          <div className="admin-connectors__header-row" key={index}>
            <Input
              label="Header"
              value={name}
              onChange={(e) => {
                const next: Record<string, string> = {};
                entries.forEach(([n, v], i) => {
                  next[i === index ? e.target.value : n] = v;
                });
                writeHeaders(next);
              }}
            />
            <Input
              label="Value"
              value={value}
              onChange={(e) =>
                writeHeaders({ ...headers, [name]: e.target.value })
              }
            />
            <button
              type="button"
              className="admin-btn admin-btn--ghost admin-btn--sm"
              aria-label={`Remove header ${name}`}
              onClick={() => {
                const next = { ...headers };
                delete next[name];
                writeHeaders(next);
              }}
            >
              Remove
            </button>
          </div>
        ))}
        <button
          type="button"
          className="admin-btn admin-btn--ghost admin-btn--sm"
          onClick={() => writeHeaders({ ...headers, "": "" })}
        >
          Add header
        </button>
      </div>
    );
  }

  /**
   * Channel rows, each optionally narrowed to some categories — one connection
   * can feed #movies and #tv from the same instance, which is why this routing
   * lives in the config rather than in the instance-level filters.
   */
  function renderChannelsEditor() {
    const channels = (form.config.channels as IrcChannel[]) ?? [];

    function writeChannels(next: IrcChannel[]) {
      setConfigField("channels", next);
    }

    return (
      <div className="admin-connectors__channels">
        <span className="form-label">Channels</span>
        {channels.map((channel, index) => (
          <div className="admin-connectors__channel-row" key={index}>
            <Input
              label="Channel"
              placeholder="#announce"
              value={channel.name}
              onChange={(e) =>
                writeChannels(
                  channels.map((c, i) =>
                    i === index ? { ...c, name: e.target.value } : c,
                  ),
                )
              }
            />
            <Select
              label="Categories (empty = all)"
              multiple
              size={Math.min(4, Math.max(2, categories.length))}
              options={buildCategoryOptions(categories)}
              value={(channel.categories ?? []).map(String)}
              onChange={(e) =>
                writeChannels(
                  channels.map((c, i) =>
                    i === index
                      ? {
                          ...c,
                          categories: Array.from(
                            e.target.selectedOptions,
                            (option) => Number(option.value),
                          ),
                        }
                      : c,
                  ),
                )
              }
            />
            <button
              type="button"
              className="admin-btn admin-btn--ghost admin-btn--sm"
              aria-label={`Remove channel ${channel.name || index + 1}`}
              onClick={() =>
                writeChannels(channels.filter((_, i) => i !== index))
              }
            >
              Remove
            </button>
          </div>
        ))}
        <button
          type="button"
          className="admin-btn admin-btn--ghost admin-btn--sm"
          onClick={() =>
            writeChannels([...channels, { name: "", categories: [] }])
          }
        >
          Add channel
        </button>
      </div>
    );
  }

  /**
   * Telegram chat ids. Kept as strings because group ids are negative and
   * exceed a 32-bit int, and because "@channelusername" is also valid.
   */
  function renderChatIDsEditor() {
    const chatIDs = (form.config.chat_ids as string[]) ?? [];

    function writeChatIDs(next: string[]) {
      setConfigField("chat_ids", next);
    }

    return (
      <div className="admin-connectors__chats">
        <span className="form-label">Chat IDs</span>
        {chatIDs.map((chatID, index) => (
          <div className="admin-connectors__chat-row" key={index}>
            <Input
              label={`Chat ${index + 1}`}
              placeholder="-1001234567890 or @channelname"
              value={chatID}
              onChange={(e) =>
                writeChatIDs(
                  chatIDs.map((c, i) => (i === index ? e.target.value : c)),
                )
              }
            />
            <button
              type="button"
              className="admin-btn admin-btn--ghost admin-btn--sm"
              aria-label={`Remove chat ${index + 1}`}
              onClick={() =>
                writeChatIDs(chatIDs.filter((_, i) => i !== index))
              }
            >
              Remove
            </button>
          </div>
        ))}
        <button
          type="button"
          className="admin-btn admin-btn--ghost admin-btn--sm"
          onClick={() => writeChatIDs([...chatIDs, ""])}
        >
          Add chat
        </button>
      </div>
    );
  }

  /**
   * The list of things a template may interpolate, with what each one renders
   * to. The fields come from the API rather than from a copy here: they are
   * defined by the backend's render context, and a stale local list would keep
   * promising a field that no longer exists.
   */
  function renderTemplateHelp() {
    if (templateFields.length === 0) return null;
    return (
      <details className="admin-connectors__template-help">
        <summary>Available fields</summary>
        <table>
          <thead>
            <tr>
              <th>Field</th>
              <th>Meaning</th>
              <th>Renders as</th>
            </tr>
          </thead>
          <tbody>
            {templateFields.map((field) => (
              <tr key={field.token}>
                <td>
                  <code>{field.token}</code>
                </td>
                <td>{field.description}</td>
                <td className="admin-muted">{field.example}</td>
              </tr>
            ))}
          </tbody>
        </table>
        <p className="admin-muted">
          Leave the template empty to use the built-in line. When the rate limit
          collapses a burst into one summary, that summary uses its own wording
          and ignores this template.
        </p>
      </details>
    );
  }

  function renderKindFields() {
    switch (form.kind) {
      case "chat":
        return (
          <>
            <p className="admin-muted admin-connectors__filter-hint">
              Several shoutbox instances are allowed, each with its own wording.
              They all post to the same shoutbox, so give them non-overlapping
              category filters unless you want a torrent announced twice.
            </p>
            <Textarea
              label="Message template"
              rows={2}
              placeholder="New torrent: {{.Name}} [{{.Category}}, {{.SizeHuman}}] {{.URL}}"
              value={(form.config.template as string) ?? ""}
              onChange={(e) => setConfigField("template", e.target.value)}
            />
            {renderTemplateHelp()}
          </>
        );
      case "irc":
        return (
          <>
            <Input
              label="Server"
              placeholder="irc.libera.chat"
              value={(form.config.server as string) ?? ""}
              onChange={(e) => setConfigField("server", e.target.value)}
            />
            <Input
              label="Port"
              type="number"
              min={1}
              max={65535}
              value={String(form.config.port ?? 6697)}
              onChange={(e) => setConfigField("port", Number(e.target.value))}
            />
            <Checkbox
              label="Use TLS"
              checked={(form.config.tls as boolean) ?? false}
              onChange={(e) => setConfigField("tls", e.target.checked)}
            />
            <Input
              label="Nick"
              placeholder="announcebot"
              value={(form.config.nick as string) ?? ""}
              onChange={(e) => setConfigField("nick", e.target.value)}
            />
            <Input
              label="SASL username"
              value={(form.config.sasl_user as string) ?? ""}
              onChange={(e) => setConfigField("sasl_user", e.target.value)}
            />
            {renderSecretField("sasl_pass", "SASL password")}
            {renderSecretField("nickserv_pass", "NickServ password")}
            {renderChannelsEditor()}
            <Textarea
              label="Message template"
              rows={2}
              placeholder="[{{.Category}}] {{.Name}} [{{.SizeHuman}}] — {{.URL}}"
              value={(form.config.template as string) ?? ""}
              onChange={(e) => setConfigField("template", e.target.value)}
            />
            {renderTemplateHelp()}
          </>
        );
      case "webhook":
        return (
          <>
            <Input
              label="Endpoint URL"
              type="url"
              placeholder="https://example.com/hook"
              value={(form.config.url as string) ?? ""}
              onChange={(e) => setConfigField("url", e.target.value)}
            />
            <Select
              label="Method"
              options={[
                { value: "POST", label: "POST" },
                { value: "PUT", label: "PUT" },
              ]}
              value={(form.config.method as string) ?? "POST"}
              onChange={(e) => setConfigField("method", e.target.value)}
            />
            {renderHeadersEditor()}
            {renderSecretField("hmac_secret", "HMAC signing secret")}
          </>
        );
      case "discord":
        return (
          <>
            {renderSecretField("webhook_url", "Discord webhook URL")}
            <Input
              label="Bot username (optional)"
              value={(form.config.username as string) ?? ""}
              onChange={(e) => setConfigField("username", e.target.value)}
            />
            <Input
              label="Avatar URL (optional)"
              type="url"
              value={(form.config.avatar_url as string) ?? ""}
              onChange={(e) => setConfigField("avatar_url", e.target.value)}
            />
            <Textarea
              label="Embed description template"
              rows={2}
              placeholder="{{.Name}}"
              value={(form.config.template as string) ?? ""}
              onChange={(e) => setConfigField("template", e.target.value)}
            />
            {renderTemplateHelp()}
          </>
        );
      case "telegram":
        return (
          <>
            {renderSecretField("bot_token", "Bot token")}
            {renderChatIDsEditor()}
            <Textarea
              label="Message template (HTML)"
              rows={3}
              placeholder="<b>{{.Name}}</b>"
              value={(form.config.template as string) ?? ""}
              onChange={(e) => setConfigField("template", e.target.value)}
            />
            {renderTemplateHelp()}
          </>
        );
      case "sse":
        // Deliberately config-less: who sees the feed is decided by
        // authentication, and what appears in it by the filters below.
        return (
          <p className="admin-muted">
            The live feed has nothing to configure. Use the filters below to
            narrow what appears in it.
          </p>
        );
      default:
        // A kind the backend registered but this build has no editor for. Saying
        // so beats silently offering an empty form that creates a broken
        // instance.
        return (
          <p className="admin-muted">
            This build has no configuration editor for “{form.kind}”.
          </p>
        );
    }
  }

  return (
    <div>
      <div className="admin-page-header">
        <div>
          <h1 className="admin-page-header__title">Notification Connectors</h1>
          <p className="admin-page-header__desc">
            Announce newly published torrents to the shoutbox and to external
            services. Deliveries are queued, retried and rate-limited, and never
            hold up an approval.
          </p>
        </div>
        <div className="admin-page-header__actions">
          <button
            className="admin-btn admin-btn--primary"
            onClick={openCreate}
            disabled={availableKinds().length === 0}
          >
            Add Connector
          </button>
        </div>
      </div>

      {loading ? (
        <p role="status">Loading connectors...</p>
      ) : connectors.length === 0 ? (
        <div className="admin-panel">
          <p className="admin-empty">No connectors configured yet.</p>
        </div>
      ) : (
        <div className="admin-panel">
          <div className="admin-table-scroll">
            <table className="admin-table">
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Kind</th>
                  <th>Enabled</th>
                  <th>Last delivery</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                {connectors.map((connector) => (
                  <tr key={connector.id}>
                    <td className="admin-table__name">{connector.name}</td>
                    <td className="admin-muted">
                      {kindLabel(connector.kind)}
                      {statuses[String(connector.id)] && (
                        <>
                          {" "}
                          <span
                            className={stateBadgeClass(
                              statuses[String(connector.id)].state,
                            )}
                            title={
                              statuses[String(connector.id)].last_error ||
                              undefined
                            }
                          >
                            {STATE_LABELS[
                              statuses[String(connector.id)].state
                            ] ?? statuses[String(connector.id)].state}
                          </span>
                        </>
                      )}
                    </td>
                    <td className="admin-table__toggle">
                      <input
                        type="checkbox"
                        aria-label={`Enable ${connector.name}`}
                        checked={connector.enabled}
                        disabled={togglingId !== null}
                        onChange={() => handleToggle(connector)}
                      />
                    </td>
                    <td>
                      {connector.last_delivery ? (
                        <span className="admin-connectors__status">
                          <span
                            className={statusBadgeClass(
                              connector.last_delivery.status,
                            )}
                          >
                            {connector.last_delivery.status}
                          </span>
                          <span className="admin-muted">
                            {timeAgo(connector.last_delivery.created_at)}
                          </span>
                        </span>
                      ) : (
                        <span className="admin-muted">never</span>
                      )}
                    </td>
                    <td className="admin-table__actions">
                      <div className="admin-connectors__actions">
                        <button
                          className="admin-btn admin-btn--ghost admin-btn--sm"
                          onClick={() => handleTest(connector)}
                          disabled={testingId === connector.id}
                        >
                          {testingId === connector.id ? "Testing..." : "Test"}
                        </button>
                        <button
                          className="admin-btn admin-btn--ghost admin-btn--sm"
                          onClick={() => openDeliveries(connector, 1)}
                        >
                          Log
                        </button>
                        <button
                          className="admin-btn admin-btn--ghost admin-btn--sm"
                          onClick={() => openEdit(connector)}
                        >
                          Edit
                        </button>
                        <button
                          className="admin-btn admin-btn--danger admin-btn--sm"
                          onClick={() => setDeleteTarget(connector)}
                        >
                          Delete
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {deliveriesFor && (
        <div className="admin-panel admin-connectors__log">
          <div className="admin-connectors__log-header">
            <h2 className="admin-panel__title">
              Delivery log — {deliveriesFor.name}
            </h2>
            <button
              className="admin-btn admin-btn--ghost admin-btn--sm"
              onClick={() => setDeliveriesFor(null)}
            >
              Close
            </button>
          </div>
          {deliveriesLoading ? (
            <p role="status">Loading deliveries...</p>
          ) : deliveries.length === 0 ? (
            <p className="admin-empty">No deliveries recorded yet.</p>
          ) : (
            <div className="admin-table-scroll">
              <table className="admin-table">
                <thead>
                  <tr>
                    <th>Event</th>
                    <th>Status</th>
                    <th>Attempts</th>
                    <th>Error</th>
                    <th>When</th>
                  </tr>
                </thead>
                <tbody>
                  {deliveries.map((delivery) => (
                    <tr key={delivery.id}>
                      <td className="admin-num">{delivery.event_key}</td>
                      <td>
                        <span className={statusBadgeClass(delivery.status)}>
                          {delivery.status}
                        </span>
                      </td>
                      <td className="admin-num">{delivery.attempts}</td>
                      <td className="admin-connectors__error">
                        {delivery.last_error ?? ""}
                      </td>
                      <td
                        className="admin-muted"
                        title={new Date(delivery.created_at).toLocaleString()}
                      >
                        {timeAgo(delivery.created_at)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
          {deliveriesTotal > DELIVERIES_PER_PAGE && (
            <div className="admin-connectors__log-pagination">
              <Pagination
                currentPage={deliveriesPage}
                totalPages={Math.ceil(deliveriesTotal / DELIVERIES_PER_PAGE)}
                onPageChange={(page) => openDeliveries(deliveriesFor, page)}
              />
            </div>
          )}
        </div>
      )}

      <Modal
        isOpen={modalOpen}
        onClose={closeModal}
        title={editingId ? "Edit Connector" : "Add Connector"}
        closeOnEscape={false}
        closeOnDismissClick={false}
      >
        <div className="admin-modal-form">
          <Select
            label="Kind"
            // The kind decides how the stored config is interpreted, so it is
            // fixed once an instance exists.
            disabled={editingId !== null}
            // A singleton kind that already has its one instance is filtered
            // out rather than offered and then rejected with a 409 after the
            // admin has filled in the whole form. When editing, the instance's
            // own kind stays listed so the (disabled) picker still shows it.
            options={(editingId !== null ? kinds : availableKinds()).map(
              (kind) => ({ value: kind, label: kindLabel(kind) }),
            )}
            value={form.kind}
            onChange={(e) =>
              setForm((prev) => ({
                ...prev,
                kind: e.target.value,
                config: defaultConfig(e.target.value),
                secrets: {},
                clearSecrets: {},
              }))
            }
          />
          <Input
            label="Name"
            value={form.name}
            onChange={(e) =>
              setForm((prev) => ({ ...prev, name: e.target.value }))
            }
          />
          <Checkbox
            label="Enabled"
            checked={form.enabled}
            onChange={(e) =>
              setForm((prev) => ({ ...prev, enabled: e.target.checked }))
            }
          />

          {renderKindFields()}

          <Input
            label="Rate limit (messages per minute)"
            type="number"
            min={1}
            value={String(form.config.rate_per_min ?? "")}
            placeholder="20"
            onChange={(e) =>
              setConfigField(
                "rate_per_min",
                e.target.value === "" ? undefined : Number(e.target.value),
              )
            }
          />

          <h3 className="admin-panel__title admin-panel__title--tight">
            Filters
          </h3>
          <Select
            label="Category filter mode"
            options={[
              { value: "include", label: "Only the categories selected below" },
              { value: "exclude", label: "Everything except those selected" },
            ]}
            value={form.filters.category_mode ?? "include"}
            onChange={(e) =>
              setFilterField(
                "category_mode",
                e.target.value as "include" | "exclude",
              )
            }
          />
          <Select
            label={
              form.filters.category_mode === "exclude"
                ? "Categories to exclude (leave empty to exclude none)"
                : "Categories to include (leave empty for all)"
            }
            multiple
            size={Math.min(6, Math.max(3, categories.length))}
            options={buildCategoryOptions(categories)}
            value={(form.filters.category_ids ?? []).map(String)}
            onChange={(e) =>
              setFilterField(
                "category_ids",
                Array.from(e.target.selectedOptions, (option) =>
                  Number(option.value),
                ),
              )
            }
          />
          <p className="admin-muted admin-connectors__filter-hint">
            Selecting a parent category covers everything under it.
          </p>
          <ByteSizeInput
            label="Minimum size"
            value={form.filters.min_size ?? 0}
            onChange={(bytes) => setFilterField("min_size", bytes)}
          />
          <Checkbox
            label="Freeleech only"
            checked={form.filters.freeleech_only ?? false}
            onChange={(e) => setFilterField("freeleech_only", e.target.checked)}
          />
          <Checkbox
            label="Exclude anonymous uploads"
            checked={form.filters.exclude_anonymous ?? false}
            onChange={(e) =>
              setFilterField("exclude_anonymous", e.target.checked)
            }
          />

          <div className="admin-modal-form__actions">
            <button className="admin-btn admin-btn--ghost" onClick={closeModal}>
              Cancel
            </button>
            <button
              className="admin-btn admin-btn--primary"
              onClick={handleSave}
              disabled={saving || !form.name.trim() || !form.kind}
            >
              {saving ? "Saving..." : "Save"}
            </button>
          </div>
        </div>
      </Modal>

      <ConfirmModal
        isOpen={deleteTarget !== null}
        title="Delete Connector"
        message={
          deleteTarget
            ? `Delete connector '${deleteTarget.name}'? Its delivery history goes with it. This cannot be undone.`
            : ""
        }
        confirmLabel="Delete"
        danger
        loading={deleteLoading}
        onConfirm={handleDeleteConfirm}
        onCancel={() => setDeleteTarget(null)}
      />
    </div>
  );
}
