import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { getConfig } from "@/config";
import { getAccessToken } from "@/features/auth/token";
import { Pagination } from "@/components/Pagination";
import { formatDate } from "@/utils/format";
import "./messages.css";

interface Message {
  id: number;
  sender_id: number;
  sender_username: string;
  receiver_id: number;
  receiver_username: string;
  subject: string;
  body: string;
  is_read: boolean;
  created_at: string;
}

type Tab = "inbox" | "outbox" | "compose";

const PER_PAGE = 25;

function authHeaders(): Record<string, string> {
  const token = getAccessToken();
  return token ? { Authorization: `Bearer ${token}` } : {};
}

export function MessagesPage() {
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
    null,
  );
  const [composeSubject, setComposeSubject] = useState("");
  const [composeBody, setComposeBody] = useState("");
  const [composeParentId, setComposeParentId] = useState<number | null>(null);
  const [sending, setSending] = useState(false);
  const [sendSuccess, setSendSuccess] = useState<string | null>(null);

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
        setUnreadCount(data?.unread_count ?? 0);
      }
    } catch {
      // ignore
    }
  }, []);

  const fetchMessages = useCallback(async () => {
    if (tab === "compose") return;
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
    setSelectedMessage(null);
    setError(null);
    setSendSuccess(null);
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

  const totalPages = Math.max(1, Math.ceil(total / PER_PAGE));

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
              <Link
                to={`/user/${selectedMessage.sender_id}`}
                className="messages__user-link"
              >
                {selectedMessage.sender_username}
              </Link>
            </span>
            <span>
              To:{" "}
              <Link
                to={`/user/${selectedMessage.receiver_id}`}
                className="messages__user-link"
              >
                {selectedMessage.receiver_username}
              </Link>
            </span>
            <span>{formatDate(selectedMessage.created_at)}</span>
          </div>
        </div>
        <div className="messages__detail-body">{selectedMessage.body}</div>
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
              <label htmlFor="msg-body" className="messages__form-label">
                Message
              </label>
              <textarea
                id="msg-body"
                className="messages__form-textarea"
                value={composeBody}
                onChange={(e) => setComposeBody(e.target.value)}
                required
                placeholder="Write your message..."
              />
            </div>
            <button
              type="submit"
              className="messages__form-btn"
              disabled={sending}
            >
              {sending ? "Sending..." : "Send Message"}
            </button>
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
                        <Link
                          to={`/user/${tab === "inbox" ? msg.sender_id : msg.receiver_id}`}
                          className="messages__user-link"
                        >
                          {tab === "inbox"
                            ? msg.sender_username
                            : msg.receiver_username}
                        </Link>
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
    </div>
  );
}
