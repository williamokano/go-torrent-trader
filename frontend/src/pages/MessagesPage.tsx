import { useCallback, useEffect, useRef, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { getConfig } from "@/config";
import { getAccessToken } from "@/features/auth/token";
import { Pagination } from "@/components/Pagination";
import { MarkdownEditor } from "@/components/MarkdownEditor";
import { MarkdownRenderer } from "@/components/MarkdownRenderer";
import { useChat } from "@/lib/useChat";
import { formatDate } from "@/utils/format";
import { UsernameDisplay } from "@/components/UsernameDisplay";
import "./messages.css";

interface Message {
  id: number;
  sender_id: number;
  sender_username: string;
  receiver_id: number;
  receiver_username: string;
  subject: string;
  body: string;
  mentioned_usernames?: string[];
  is_read: boolean;
  created_at: string;
}

// A draft is a specific in-progress message (may carry a receiver/parent_id
// for a reply-in-progress) saved to finish later; a template is a reusable
// pattern with no fixed recipient, loaded to start a new compose. Both are
// returned by the same shape — see backend/internal/model/saved_message.go.
interface SavedMessageItem {
  id: number;
  kind: "draft" | "template";
  receiver_id?: number;
  receiver_username?: string;
  subject: string;
  body: string;
  parent_id?: number;
  created_at: string;
  updated_at: string;
}

type Tab = "inbox" | "outbox" | "drafts" | "templates" | "compose";
type SavedKind = "drafts" | "templates";

function isSavedKindTab(tab: Tab): tab is SavedKind {
  return tab === "drafts" || tab === "templates";
}

const PER_PAGE = 25;

function authHeaders(): Record<string, string> {
  const token = getAccessToken();
  return token ? { Authorization: `Bearer ${token}` } : {};
}

export function MessagesPage() {
  const { setPmUnreadCount } = useChat();
  const [searchParams, setSearchParams] = useSearchParams();
  const tab: Tab = (searchParams.get("tab") as Tab) || "inbox";
  const selectedMsgId = searchParams.get("msg")
    ? Number(searchParams.get("msg"))
    : null;
  const [messages, setMessages] = useState<Message[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [unreadCount, setUnreadCount] = useState(0);

  // Detail view state
  const [selectedMessage, setSelectedMessage] = useState<Message | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);

  // Compose state
  const [composeReceiver, setComposeReceiver] = useState(
    searchParams.get("to") || "",
  );
  const [composeReceiverId, setComposeReceiverId] = useState<number | null>(
    searchParams.get("to_id") ? Number(searchParams.get("to_id")) : null,
  );
  const [composeSubject, setComposeSubject] = useState("");
  const [composeBody, setComposeBody] = useState("");
  const [composeParentId, setComposeParentId] = useState<number | null>(null);
  const [sending, setSending] = useState(false);
  const [sendSuccess, setSendSuccess] = useState<string | null>(null);

  // Draft/template state (BE-7.2). When the compose form was opened by
  // editing a draft or loading a template, its id is tracked here so "Save
  // Draft"/"Save as Template" overwrite it (PUT) instead of creating a new
  // one (POST) on every click.
  const [currentDraftId, setCurrentDraftId] = useState<number | null>(null);
  const [currentTemplateId, setCurrentTemplateId] = useState<number | null>(
    null,
  );
  const [savingDraft, setSavingDraft] = useState(false);
  const [savingTemplate, setSavingTemplate] = useState(false);

  // Drafts/templates list state (separate from messages/total/page above,
  // which are shaped for inbox/outbox and keep their own pagination).
  const [savedItems, setSavedItems] = useState<SavedMessageItem[]>([]);
  const [savedTotal, setSavedTotal] = useState(0);
  const [savedPage, setSavedPage] = useState(1);

  // Sync compose receiver from URL when navigating to ?tab=compose&to=username&to_id=N
  const urlTo = searchParams.get("to") || "";
  const urlToId = searchParams.get("to_id");
  if (tab === "compose" && urlTo && urlTo !== composeReceiver) {
    setComposeReceiver(urlTo);
    setComposeReceiverId(urlToId ? Number(urlToId) : null);
  }

  // Username autocomplete state
  const [userSuggestions, setUserSuggestions] = useState<
    Array<{ id: number; username: string }>
  >([]);
  const [showSuggestions, setShowSuggestions] = useState(false);
  const [suggestionLoading, setSuggestionLoading] = useState(false);
  const suggestionRef = useRef<HTMLDivElement>(null);
  const debounceRef = useRef<ReturnType<typeof setTimeout>>(undefined);

  // Clean up debounce timer on unmount
  useEffect(() => {
    return () => clearTimeout(debounceRef.current);
  }, []);

  // Debounced username search for autocomplete
  const searchUsers = useCallback((query: string) => {
    clearTimeout(debounceRef.current);
    if (query.length < 2) {
      setUserSuggestions([]);
      setShowSuggestions(false);
      return;
    }
    setSuggestionLoading(true);
    debounceRef.current = setTimeout(async () => {
      try {
        const res = await fetch(
          `${getConfig().API_URL}/api/v1/users?search=${encodeURIComponent(query)}&per_page=8`,
          { headers: authHeaders() },
        );
        if (res.ok) {
          const data = await res.json();
          setUserSuggestions(
            (data?.users ?? []).map((u: { id: number; username: string }) => ({
              id: u.id,
              username: u.username,
            })),
          );
          setShowSuggestions(true);
        }
      } catch {
        // ignore
      } finally {
        setSuggestionLoading(false);
      }
    }, 250);
  }, []);

  const fetchUnreadCount = useCallback(async () => {
    try {
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/messages/unread-count`,
        { headers: authHeaders() },
      );
      if (res.ok) {
        const data = await res.json();
        const count = data?.unread_count ?? 0;
        setUnreadCount(count);
        setPmUnreadCount(count);
      }
    } catch {
      // ignore
    }
  }, [setPmUnreadCount]);

  const fetchMessages = useCallback(async () => {
    if (tab !== "inbox" && tab !== "outbox") return;
    setLoading(true);
    setError(null);
    setSelectedMessage(null);
    try {
      const endpoint = tab === "inbox" ? "inbox" : "outbox";
      const params = new URLSearchParams();
      params.set("page", String(page));
      params.set("per_page", String(PER_PAGE));
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/messages/${endpoint}?${params.toString()}`,
        { headers: authHeaders() },
      );
      const body = await res.json();
      if (!res.ok) {
        setError(body?.error?.message ?? "Failed to load messages");
        return;
      }
      setMessages(body?.messages ?? []);
      setTotal(body?.total ?? 0);
    } catch {
      setError("Failed to load messages");
    } finally {
      setLoading(false);
    }
  }, [tab, page]);

  useEffect(() => {
    fetchMessages();
    fetchUnreadCount();
  }, [fetchMessages, fetchUnreadCount]);

  // Drafts/templates list (BE-7.2). Same list/paginate shape as inbox/outbox,
  // but a distinct endpoint and response key per kind, so it's kept separate
  // from fetchMessages rather than overloading it with a third resource shape.
  const fetchSaved = useCallback(async () => {
    if (!isSavedKindTab(tab)) return;
    setLoading(true);
    setError(null);
    try {
      const params = new URLSearchParams();
      params.set("page", String(savedPage));
      params.set("per_page", String(PER_PAGE));
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/messages/${tab}?${params.toString()}`,
        { headers: authHeaders() },
      );
      const body = await res.json();
      if (!res.ok) {
        setError(body?.error?.message ?? `Failed to load ${tab}`);
        return;
      }
      setSavedItems(body?.[tab] ?? []);
      setSavedTotal(body?.total ?? 0);
    } catch {
      setError(`Failed to load ${tab}`);
    } finally {
      setLoading(false);
    }
  }, [tab, savedPage]);

  useEffect(() => {
    fetchSaved();
  }, [fetchSaved]);

  // Clear detail view when msg param is removed from URL
  if (!selectedMsgId && selectedMessage) {
    setSelectedMessage(null);
  }

  // Fetch message detail when msg param is in URL
  const fetchMessageDetail = useCallback(
    async (msgId: number) => {
      setDetailLoading(true);
      setError(null);
      try {
        const res = await fetch(
          `${getConfig().API_URL}/api/v1/messages/${msgId}`,
          { headers: authHeaders() },
        );
        const body = await res.json();
        if (body?.message) {
          setSelectedMessage(body.message);
          fetchUnreadCount();
          setMessages((prev) =>
            prev.map((m) => (m.id === msgId ? { ...m, is_read: true } : m)),
          );
        } else {
          setError("Message not found");
          setSearchParams((prev) => {
            const next = new URLSearchParams(prev);
            next.delete("msg");
            return next;
          });
        }
      } catch {
        setError("Failed to load message");
        setSearchParams((prev) => {
          const next = new URLSearchParams(prev);
          next.delete("msg");
          return next;
        });
      } finally {
        setDetailLoading(false);
      }
    },
    [fetchUnreadCount, setSearchParams],
  );

  useEffect(() => {
    if (selectedMsgId) {
      fetchMessageDetail(selectedMsgId);
    }
  }, [selectedMsgId, fetchMessageDetail]);

  const handleTabChange = (newTab: Tab) => {
    setPage(1);
    setSavedPage(1);
    setSelectedMessage(null);
    setError(null);
    setSendSuccess(null);
    // Switching to Compose via the tab bar (as opposed to editing a draft or
    // loading a template, which set the URL directly — see
    // loadDraftIntoCompose/loadTemplateIntoCompose) always starts fresh, so
    // any draft/template this compose session was tracking is forgotten.
    if (newTab === "compose") {
      setCurrentDraftId(null);
      setCurrentTemplateId(null);
    }
    setSearchParams({ tab: newTab });
  };

  const handleViewMessage = (id: number) => {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev);
      next.set("msg", String(id));
      return next;
    });
  };

  const handleDelete = async (id: number) => {
    try {
      const res = await fetch(`${getConfig().API_URL}/api/v1/messages/${id}`, {
        method: "DELETE",
        headers: authHeaders(),
      });
      if (!res.ok) {
        const body = await res.json();
        setError(body?.error?.message ?? "Failed to delete message");
        return;
      }
      if (selectedMessage?.id === id) {
        setSearchParams((prev) => {
          const next = new URLSearchParams(prev);
          next.delete("msg");
          return next;
        });
      }
      fetchMessages();
      fetchUnreadCount();
    } catch {
      setError("Failed to delete message");
    }
  };

  const handleSend = async (e: React.FormEvent) => {
    e.preventDefault();
    if (sending) return;
    setSending(true);
    setError(null);
    setSendSuccess(null);
    try {
      if (!composeReceiverId) {
        setError("Please select a user from the suggestions");
        return;
      }

      const reqBody: Record<string, unknown> = {
        receiver_id: composeReceiverId,
        subject: composeSubject.trim(),
        body: composeBody.trim(),
      };
      if (composeParentId) {
        reqBody.parent_id = composeParentId;
      }

      const res = await fetch(`${getConfig().API_URL}/api/v1/messages`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          ...authHeaders(),
        },
        body: JSON.stringify(reqBody),
      });
      const body = await res.json();
      if (!res.ok) {
        setError(body?.error?.message ?? "Failed to send message");
        return;
      }
      setSendSuccess("Message sent successfully!");
      setComposeReceiver("");
      setComposeReceiverId(null);
      setComposeSubject("");
      setComposeBody("");
      setComposeParentId(null);
      // The message that was drafted is now sent — leaving its draft behind
      // would just accumulate stale duplicates in the Drafts tab. This is
      // best-effort cleanup: it never blocks or fails the send itself, and a
      // template is deliberately NOT deleted here since it's meant to be
      // reused across many sends.
      if (currentDraftId) {
        const draftIdToClean = currentDraftId;
        fetch(
          `${getConfig().API_URL}/api/v1/messages/drafts/${draftIdToClean}`,
          {
            method: "DELETE",
            headers: authHeaders(),
          },
        ).catch(() => {});
      }
      setCurrentDraftId(null);
      setCurrentTemplateId(null);
    } catch {
      setError("Failed to send message");
    } finally {
      setSending(false);
    }
  };

  const handleReply = (msg: Message) => {
    setComposeReceiver(msg.sender_username);
    setComposeReceiverId(msg.sender_id);
    setComposeSubject(
      msg.subject.startsWith("Re: ") ? msg.subject : `Re: ${msg.subject}`,
    );
    setComposeBody("");
    setComposeParentId(msg.id);
    setSelectedMessage(null);
    handleTabChange("compose");
  };

  // Saves the current compose form as a draft. Overwrites currentDraftId if
  // this session already saved one, rather than piling up duplicates on
  // every click. Mirrors the backend's leniency: a draft may have any subset
  // of receiver/subject/body filled in, as long as something is there.
  const handleSaveDraft = async () => {
    if (savingDraft) return;
    if (!composeReceiverId && !composeSubject.trim() && !composeBody.trim()) {
      setError("Nothing to save yet — write something first.");
      return;
    }
    setSavingDraft(true);
    setError(null);
    setSendSuccess(null);
    try {
      const reqBody: Record<string, unknown> = {
        subject: composeSubject.trim(),
        body: composeBody.trim(),
      };
      if (composeReceiverId) reqBody.receiver_id = composeReceiverId;
      if (composeParentId) reqBody.parent_id = composeParentId;

      const url = currentDraftId
        ? `${getConfig().API_URL}/api/v1/messages/drafts/${currentDraftId}`
        : `${getConfig().API_URL}/api/v1/messages/drafts`;
      const res = await fetch(url, {
        method: currentDraftId ? "PUT" : "POST",
        headers: { "Content-Type": "application/json", ...authHeaders() },
        body: JSON.stringify(reqBody),
      });
      const body = await res.json();
      if (!res.ok || !body?.draft?.id) {
        setError(body?.error?.message ?? "Failed to save draft");
        return;
      }
      setCurrentDraftId(body.draft.id);
      setSendSuccess("Draft saved.");
    } catch {
      setError("Failed to save draft");
    } finally {
      setSavingDraft(false);
    }
  };

  // Saves the current compose body as a reusable template. Unlike a draft, a
  // template is never addressed to anyone — receiver_id/parent_id are never
  // sent, even if the compose form currently has them set (e.g. mid-reply).
  const handleSaveTemplate = async () => {
    if (savingTemplate) return;
    if (!composeBody.trim()) {
      setError("A template needs a body to be worth saving.");
      return;
    }
    setSavingTemplate(true);
    setError(null);
    setSendSuccess(null);
    try {
      const reqBody = {
        subject: composeSubject.trim(),
        body: composeBody.trim(),
      };

      const url = currentTemplateId
        ? `${getConfig().API_URL}/api/v1/messages/templates/${currentTemplateId}`
        : `${getConfig().API_URL}/api/v1/messages/templates`;
      const res = await fetch(url, {
        method: currentTemplateId ? "PUT" : "POST",
        headers: { "Content-Type": "application/json", ...authHeaders() },
        body: JSON.stringify(reqBody),
      });
      const body = await res.json();
      if (!res.ok || !body?.template?.id) {
        setError(body?.error?.message ?? "Failed to save template");
        return;
      }
      setCurrentTemplateId(body.template.id);
      setSendSuccess("Template saved.");
    } catch {
      setError("Failed to save template");
    } finally {
      setSavingTemplate(false);
    }
  };

  // Opens a saved draft back in the compose form for continued editing.
  const loadDraftIntoCompose = async (id: number) => {
    setError(null);
    try {
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/messages/drafts/${id}`,
        { headers: authHeaders() },
      );
      const body = await res.json();
      if (!res.ok || !body?.draft) {
        setError(body?.error?.message ?? "Failed to load draft");
        return;
      }
      const d = body.draft;
      setComposeReceiver(d.receiver_username ?? "");
      setComposeReceiverId(d.receiver_id ?? null);
      setComposeSubject(d.subject ?? "");
      setComposeBody(d.body ?? "");
      setComposeParentId(d.parent_id ?? null);
      setCurrentDraftId(d.id);
      setCurrentTemplateId(null);
      setSendSuccess(null);
      setSearchParams({ tab: "compose" });
    } catch {
      setError("Failed to load draft");
    }
  };

  // Loads a template into the compose form to start a new message. The
  // recipient is deliberately left blank — a template has no fixed one.
  const loadTemplateIntoCompose = async (id: number) => {
    setError(null);
    try {
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/messages/templates/${id}`,
        { headers: authHeaders() },
      );
      const body = await res.json();
      if (!res.ok || !body?.template) {
        setError(body?.error?.message ?? "Failed to load template");
        return;
      }
      const t = body.template;
      setComposeReceiver("");
      setComposeReceiverId(null);
      setComposeSubject(t.subject ?? "");
      setComposeBody(t.body ?? "");
      setComposeParentId(null);
      setCurrentTemplateId(t.id);
      setCurrentDraftId(null);
      setSendSuccess(null);
      setSearchParams({ tab: "compose" });
    } catch {
      setError("Failed to load template");
    }
  };

  const handleDeleteSaved = async (id: number) => {
    if (!isSavedKindTab(tab)) return;
    const label = tab === "drafts" ? "draft" : "template";
    try {
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/messages/${tab}/${id}`,
        { method: "DELETE", headers: authHeaders() },
      );
      if (!res.ok) {
        const body = await res.json();
        setError(body?.error?.message ?? `Failed to delete ${label}`);
        return;
      }
      fetchSaved();
    } catch {
      setError(`Failed to delete ${label}`);
    }
  };

  const totalPages = Math.max(1, Math.ceil(total / PER_PAGE));
  const savedTotalPages = Math.max(1, Math.ceil(savedTotal / PER_PAGE));

  // Detail view
  if (selectedMessage) {
    return (
      <div className="messages__detail">
        <div className="messages__detail-header">
          <h2 className="messages__detail-subject">
            {selectedMessage.subject}
          </h2>
          <div className="messages__detail-meta">
            <span>
              From:{" "}
              <UsernameDisplay
                userId={selectedMessage.sender_id}
                username={selectedMessage.sender_username}
                className="messages__user-link"
              />
            </span>
            <span>
              To:{" "}
              <UsernameDisplay
                userId={selectedMessage.receiver_id}
                username={selectedMessage.receiver_username}
                className="messages__user-link"
              />
            </span>
            <span>{formatDate(selectedMessage.created_at)}</span>
          </div>
        </div>
        <MarkdownRenderer
          content={selectedMessage.body}
          className="messages__detail-body"
          mentionedUsernames={selectedMessage.mentioned_usernames}
        />
        <div className="messages__detail-actions">
          <button
            type="button"
            className="messages__back-btn"
            onClick={() => {
              setSearchParams((prev) => {
                const next = new URLSearchParams(prev);
                next.delete("msg");
                return next;
              });
            }}
          >
            Back
          </button>
          {tab === "inbox" && (
            <button
              type="button"
              className="messages__reply-btn"
              onClick={() => handleReply(selectedMessage)}
            >
              Reply
            </button>
          )}
          <button
            type="button"
            className="messages__delete-btn"
            onClick={() => handleDelete(selectedMessage.id)}
          >
            Delete
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="messages">
      <div className="messages__header">
        <h1 className="messages__title">Messages</h1>
      </div>

      <div className="messages__tabs">
        <button
          type="button"
          className={`messages__tab${tab === "inbox" ? " messages__tab--active" : ""}`}
          onClick={() => handleTabChange("inbox")}
        >
          Inbox
          {unreadCount > 0 && (
            <span className="messages__badge">{unreadCount}</span>
          )}
        </button>
        <button
          type="button"
          className={`messages__tab${tab === "outbox" ? " messages__tab--active" : ""}`}
          onClick={() => handleTabChange("outbox")}
        >
          Outbox
        </button>
        <button
          type="button"
          className={`messages__tab${tab === "drafts" ? " messages__tab--active" : ""}`}
          onClick={() => handleTabChange("drafts")}
        >
          Drafts
        </button>
        <button
          type="button"
          className={`messages__tab${tab === "templates" ? " messages__tab--active" : ""}`}
          onClick={() => handleTabChange("templates")}
        >
          Templates
        </button>
        <button
          type="button"
          className={`messages__tab${tab === "compose" ? " messages__tab--active" : ""}`}
          onClick={() => handleTabChange("compose")}
        >
          Compose
        </button>
      </div>

      {error && <div className="messages__error">{error}</div>}

      {tab === "compose" && (
        <>
          {sendSuccess && (
            <div className="messages__success">{sendSuccess}</div>
          )}
          <form className="messages__compose-form" onSubmit={handleSend}>
            <div className="messages__form-group" ref={suggestionRef}>
              <label htmlFor="msg-receiver" className="messages__form-label">
                To
              </label>
              <div className="messages__autocomplete">
                <input
                  id="msg-receiver"
                  type="text"
                  className="messages__form-input"
                  value={composeReceiver}
                  onChange={(e) => {
                    setComposeReceiver(e.target.value);
                    setComposeReceiverId(null);
                    // A parent_id ties this compose to a specific reply
                    // thread with the *original* recipient. If the user
                    // retypes "To" (e.g. after Reply pre-filled it), that
                    // link is no longer guaranteed to point at whoever ends
                    // up selected, so drop it — otherwise a saved draft
                    // could end up addressed to someone unrelated to the
                    // thread it's still flagged as replying to.
                    setComposeParentId(null);
                    searchUsers(e.target.value);
                  }}
                  onFocus={() => {
                    if (userSuggestions.length > 0) setShowSuggestions(true);
                  }}
                  required
                  placeholder="Search username..."
                  autoComplete="off"
                />
                {suggestionLoading && (
                  <span className="messages__autocomplete-loading">...</span>
                )}
                {showSuggestions && userSuggestions.length > 0 && (
                  <ul className="messages__autocomplete-list">
                    {userSuggestions.map((u) => (
                      <li key={u.id}>
                        <button
                          type="button"
                          className="messages__autocomplete-item"
                          onClick={() => {
                            setComposeReceiver(u.username);
                            setComposeReceiverId(u.id);
                            setShowSuggestions(false);
                          }}
                        >
                          {u.username}
                        </button>
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            </div>
            <div className="messages__form-group">
              <label htmlFor="msg-subject" className="messages__form-label">
                Subject
              </label>
              <input
                id="msg-subject"
                type="text"
                className="messages__form-input"
                value={composeSubject}
                onChange={(e) => setComposeSubject(e.target.value)}
                required
                placeholder="Message subject"
              />
            </div>
            <div className="messages__form-group">
              <MarkdownEditor
                id="msg-body"
                label="Message"
                value={composeBody}
                onChange={setComposeBody}
                placeholder="Write your message..."
              />
            </div>
            <div className="messages__form-actions">
              <button
                type="submit"
                className="messages__form-btn"
                disabled={sending || !composeBody.trim()}
              >
                {sending ? "Sending..." : "Send Message"}
              </button>
              <button
                type="button"
                className="messages__form-btn messages__form-btn--secondary"
                disabled={savingDraft}
                onClick={handleSaveDraft}
              >
                {savingDraft
                  ? "Saving..."
                  : currentDraftId
                    ? "Update Draft"
                    : "Save Draft"}
              </button>
              <button
                type="button"
                className="messages__form-btn messages__form-btn--secondary"
                disabled={savingTemplate || !composeBody.trim()}
                onClick={handleSaveTemplate}
              >
                {savingTemplate
                  ? "Saving..."
                  : currentTemplateId
                    ? "Update Template"
                    : "Save as Template"}
              </button>
            </div>
          </form>
        </>
      )}

      {(tab === "inbox" || tab === "outbox") && (
        <>
          {loading ? (
            <div className="messages__loading">Loading messages...</div>
          ) : messages.length === 0 ? (
            <div className="messages__empty">
              {tab === "inbox"
                ? "Your inbox is empty."
                : "No sent messages yet."}
            </div>
          ) : (
            <>
              {detailLoading && (
                <div className="messages__loading">Loading message...</div>
              )}
              <table className="messages__table">
                <thead>
                  <tr>
                    <th>{tab === "inbox" ? "From" : "To"}</th>
                    <th>Subject</th>
                    <th>Date</th>
                    <th style={{ textAlign: "right" }}>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {messages.map((msg) => (
                    <tr
                      key={msg.id}
                      className={
                        tab === "inbox" && !msg.is_read
                          ? "messages__row--unread"
                          : ""
                      }
                    >
                      <td>
                        <UsernameDisplay
                          userId={
                            tab === "inbox" ? msg.sender_id : msg.receiver_id
                          }
                          username={
                            tab === "inbox"
                              ? msg.sender_username
                              : msg.receiver_username
                          }
                          className="messages__user-link"
                        />
                      </td>
                      <td>
                        <button
                          type="button"
                          className="messages__subject-link"
                          onClick={() => handleViewMessage(msg.id)}
                        >
                          {msg.subject}
                        </button>
                      </td>
                      <td>{formatDate(msg.created_at)}</td>
                      <td style={{ textAlign: "right" }}>
                        <button
                          type="button"
                          className="messages__delete-btn"
                          onClick={() => handleDelete(msg.id)}
                        >
                          Delete
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>

              {totalPages > 1 && (
                <Pagination
                  currentPage={page}
                  totalPages={totalPages}
                  onPageChange={setPage}
                />
              )}
            </>
          )}
        </>
      )}

      {isSavedKindTab(tab) && (
        <>
          {loading ? (
            <div className="messages__loading">
              Loading {tab === "drafts" ? "drafts" : "templates"}...
            </div>
          ) : savedItems.length === 0 ? (
            <div className="messages__empty">
              {tab === "drafts"
                ? "No saved drafts."
                : "No saved templates yet."}
            </div>
          ) : (
            <>
              <table className="messages__table">
                <thead>
                  <tr>
                    {tab === "drafts" && <th>To</th>}
                    <th>Subject</th>
                    <th>Updated</th>
                    <th style={{ textAlign: "right" }}>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {savedItems.map((item) => (
                    <tr key={item.id}>
                      {tab === "drafts" && (
                        <td>
                          {item.receiver_id && item.receiver_username ? (
                            <UsernameDisplay
                              userId={item.receiver_id}
                              username={item.receiver_username}
                              className="messages__user-link"
                            />
                          ) : (
                            <span className="messages__saved-empty">
                              (no recipient yet)
                            </span>
                          )}
                        </td>
                      )}
                      <td>
                        <button
                          type="button"
                          className="messages__subject-link"
                          onClick={() =>
                            tab === "drafts"
                              ? loadDraftIntoCompose(item.id)
                              : loadTemplateIntoCompose(item.id)
                          }
                        >
                          {item.subject || (
                            <span className="messages__saved-empty">
                              {tab === "drafts"
                                ? "(no subject)"
                                : "(untitled template)"}
                            </span>
                          )}
                        </button>
                      </td>
                      <td>{formatDate(item.updated_at)}</td>
                      <td style={{ textAlign: "right" }}>
                        <button
                          type="button"
                          className="messages__edit-btn"
                          onClick={() =>
                            tab === "drafts"
                              ? loadDraftIntoCompose(item.id)
                              : loadTemplateIntoCompose(item.id)
                          }
                        >
                          {tab === "drafts" ? "Edit" : "Use"}
                        </button>
                        <button
                          type="button"
                          className="messages__delete-btn"
                          onClick={() => handleDeleteSaved(item.id)}
                        >
                          Delete
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>

              {savedTotalPages > 1 && (
                <Pagination
                  currentPage={savedPage}
                  totalPages={savedTotalPages}
                  onPageChange={setSavedPage}
                />
              )}
            </>
          )}
        </>
      )}
    </div>
  );
}
