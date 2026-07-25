import { useCallback, useEffect, useState } from "react";
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
  min_size?: number;
  freeleech_only?: boolean;
  exclude_anonymous?: boolean;
}

interface Category {
  id: number;
  name: string;
}

/** Which config keys each kind treats as write-only credentials. */
const SECRET_FIELDS: Record<string, string[]> = {
  webhook: ["hmac_secret"],
};

const KIND_LABELS: Record<string, string> = {
  chat: "Shoutbox",
  webhook: "Webhook",
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

function emptyForm(kind: string): FormState {
  return {
    kind,
    name: "",
    enabled: true,
    config: {},
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

  const fetchConnectors = useCallback(async () => {
    setLoading(true);
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
    } catch {
      toast.error("Failed to load connectors");
    } finally {
      setLoading(false);
    }
  }, [toast]);

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

  function openCreate() {
    setEditingId(null);
    setForm(emptyForm(kinds[0] ?? ""));
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
          Authorization: `Bearer ${token}`,
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
    try {
      const token = getAccessToken();
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/admin/connectors/${connector.id}`,
        {
          method: "PUT",
          headers: {
            Authorization: `Bearer ${token}`,
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
      await fetchConnectors();
    } catch {
      toast.error("Failed to update connector");
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
        toast.error(body?.error ?? "Test send failed");
      }
      await fetchConnectors();
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

  async function openDeliveries(connector: Connector) {
    setDeliveriesFor(connector);
    setDeliveriesLoading(true);
    try {
      const token = getAccessToken();
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/admin/connectors/${connector.id}/deliveries?page=1&per_page=25`,
        { headers: token ? { Authorization: `Bearer ${token}` } : {} },
      );
      if (!res.ok) {
        toast.error("Failed to load delivery log");
        return;
      }
      const data = await res.json();
      setDeliveries(data.deliveries ?? []);
    } catch {
      toast.error("Failed to load delivery log");
    } finally {
      setDeliveriesLoading(false);
    }
  }

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

  function renderKindFields() {
    switch (form.kind) {
      case "chat":
        return (
          <Textarea
            label="Message template"
            rows={2}
            placeholder="New torrent: {{.Name}} [{{.Category}}, {{.SizeHuman}}] {{.URL}}"
            value={(form.config.template as string) ?? ""}
            onChange={(e) => setConfigField("template", e.target.value)}
          />
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
            {renderSecretField("hmac_secret", "HMAC signing secret")}
          </>
        );
      default:
        return null;
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
            disabled={kinds.length === 0}
          >
            Add Connector
          </button>
        </div>
      </div>

      {loading ? (
        <p>Loading connectors...</p>
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
                    <td className="admin-muted">{kindLabel(connector.kind)}</td>
                    <td className="admin-table__center">
                      <input
                        type="checkbox"
                        className="admin-table__toggle"
                        aria-label={`Enable ${connector.name}`}
                        checked={connector.enabled}
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
                          onClick={() => openDeliveries(connector)}
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
            <p>Loading deliveries...</p>
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
            options={kinds.map((kind) => ({
              value: kind,
              label: kindLabel(kind),
            }))}
            value={form.kind}
            onChange={(e) =>
              setForm((prev) => ({
                ...prev,
                kind: e.target.value,
                config: {},
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
            label="Categories (leave empty for all)"
            multiple
            size={Math.min(6, Math.max(3, categories.length))}
            options={categories.map((category) => ({
              value: String(category.id),
              label: category.name,
            }))}
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
