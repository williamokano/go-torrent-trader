import { useCallback, useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { useAuth } from "@/features/auth";
import { getAccessToken } from "@/features/auth/token";
import { getConfig } from "@/config";
import {
  chatSocket,
  type ChatMessage,
  type ChatListener,
} from "@/lib/ChatSocket";
import "./chat.css";

function formatTime(iso: string): string {
  const d = new Date(iso);
  return d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}

export function Chat() {
  const { user, isAuthenticated } = useAuth();
  const [collapsed, setCollapsed] = useState(true);
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [input, setInput] = useState("");
  const [connected, setConnected] = useState(false);
  const messagesEndRef = useRef<HTMLDivElement>(null);

  const scrollToBottom = useCallback(() => {
    requestAnimationFrame(() => {
      messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
    });
  }, []);

  // Subscribe to the singleton ChatSocket
  useEffect(() => {
    if (!isAuthenticated) return;

    chatSocket.connect();

    const onEvent: ChatListener = (event) => {
      switch (event.type) {
        case "connected":
          setConnected(true);
          break;
        case "disconnected":
          setConnected(false);
          break;
        case "backfill":
          setMessages(event.messages);
          setTimeout(scrollToBottom, 50);
          break;
        case "message":
          setMessages((prev) => [...prev, event.message]);
          setTimeout(scrollToBottom, 50);
          break;
        case "delete":
          setMessages((prev) => prev.filter((m) => m.id !== event.id));
          break;
      }
    };

    chatSocket.addListener(onEvent);

    return () => {
      chatSocket.removeListener(onEvent);
      // Don't disconnect here — the singleton stays alive across
      // React remounts. Only disconnect on logout (below).
    };
  }, [isAuthenticated, scrollToBottom]);

  // Disconnect when user logs out
  useEffect(() => {
    if (!isAuthenticated) {
      chatSocket.disconnect();
    }
    // No setState here — the listener handles connected state,
    // and messages will be replaced on next backfill.
  }, [isAuthenticated]);

  const sendMessage = useCallback(() => {
    const text = input.trim();
    if (!text) return;
    chatSocket.send(text);
    setInput("");
  }, [input]);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === "Enter" && !e.shiftKey) {
        e.preventDefault();
        sendMessage();
      }
    },
    [sendMessage],
  );

  const deleteMessage = useCallback(async (id: number) => {
    const token = getAccessToken();
    if (!token) return;
    try {
      await fetch(`${getConfig().API_URL}/api/v1/chat/${id}`, {
        method: "DELETE",
        headers: { Authorization: `Bearer ${token}` },
      });
    } catch {
      // Network error — ignore.
    }
  }, []);

  const loadMore = useCallback(async () => {
    if (messages.length === 0) return;
    const oldestId = messages[0].id;
    const token = getAccessToken();
    if (!token) return;
    try {
      const resp = await fetch(
        `${getConfig().API_URL}/api/v1/chat/history?before_id=${oldestId}&limit=50`,
        { headers: { Authorization: `Bearer ${token}` } },
      );
      if (!resp.ok) return;
      const data = (await resp.json()) as { messages: ChatMessage[] };
      if (data.messages?.length > 0) {
        setMessages((prev) => [...data.messages, ...prev]);
      }
    } catch {
      // Network error — ignore.
    }
  }, [messages]);

  if (!isAuthenticated) return null;

  const isStaff = user?.isStaff ?? false;

  return (
    <div className={`chat chat--${collapsed ? "collapsed" : "expanded"}`}>
      <div className="chat__header" onClick={() => setCollapsed((p) => !p)}>
        <span>
          <span className="chat__header-title">Shoutbox</span>
          <span
            className={`chat__header-status chat__header-status--${connected ? "connected" : "disconnected"}`}
          >
            {connected ? "connected" : "disconnected"}
          </span>
        </span>
        <span className="chat__header-toggle">
          {collapsed ? "\u25B2" : "\u25BC"}
        </span>
      </div>

      {!collapsed && (
        <>
          <div className="chat__messages">
            {messages.length > 0 && (
              <button className="chat__load-more" onClick={loadMore}>
                Load older messages
              </button>
            )}
            {messages.length === 0 && (
              <div className="chat__empty">
                No messages yet. Be the first to say something!
              </div>
            )}
            {messages.map((msg) => (
              <div key={msg.id} className="chat__message">
                <span className="chat__message-time">
                  {formatTime(msg.created_at)}
                </span>
                <Link
                  to={`/user/${msg.user_id}`}
                  className="chat__message-user"
                >
                  {msg.username}
                </Link>
                <span className="chat__message-text">{msg.message}</span>
                {isStaff && (
                  <button
                    className="chat__message-delete"
                    onClick={() => deleteMessage(msg.id)}
                    title="Delete message"
                  >
                    x
                  </button>
                )}
              </div>
            ))}
            <div ref={messagesEndRef} />
          </div>

          <div className="chat__input-area">
            <input
              className="chat__input"
              type="text"
              placeholder="Type a message..."
              value={input}
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={handleKeyDown}
              maxLength={500}
              disabled={!connected}
            />
            <button
              className="chat__send-btn"
              onClick={sendMessage}
              disabled={!connected || !input.trim()}
            >
              Send
            </button>
          </div>
        </>
      )}
    </div>
  );
}
