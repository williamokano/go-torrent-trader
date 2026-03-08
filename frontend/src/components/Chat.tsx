import { useCallback, useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { useAuth } from "@/features/auth";
import { getAccessToken } from "@/features/auth/token";
import { getConfig } from "@/config";
import "./chat.css";

interface ChatMessage {
  id: number;
  user_id: number;
  username: string;
  message: string;
  created_at: string;
}

type WSMessage =
  | { type: "backfill"; messages: ChatMessage[] }
  | ({ type: "message" } & ChatMessage)
  | { type: "delete"; id: number }
  | { type: "error"; message: string };

function formatTime(iso: string): string {
  const d = new Date(iso);
  return d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}

function getWebSocketURL(): string {
  const apiUrl = getConfig().API_URL;
  return apiUrl.replace(/^http/, "ws") + "/ws/chat";
}

export function Chat() {
  const { user, isAuthenticated } = useAuth();
  const [collapsed, setCollapsed] = useState(true);
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [input, setInput] = useState("");
  const [connected, setConnected] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const messagesContainerRef = useRef<HTMLDivElement>(null);
  const reconnectTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(
    null,
  );
  const reconnectDelayRef = useRef(1000);
  const shouldReconnectRef = useRef(true);
  const connectRef = useRef<() => void>(() => {});

  const scrollToBottom = useCallback(() => {
    requestAnimationFrame(() => {
      messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
    });
  }, []);

  // Single effect for WebSocket lifecycle — avoids React Strict Mode
  // double-mount creating two connections by closing any existing one
  // before connecting, and cleaning up fully on unmount.
  useEffect(() => {
    if (!isAuthenticated) return;

    shouldReconnectRef.current = true;

    function connect() {
      // Close any existing connection first
      if (wsRef.current) {
        wsRef.current.close();
        wsRef.current = null;
      }

      const token = getAccessToken();
      if (!token) return;

      const url = `${getWebSocketURL()}?token=${encodeURIComponent(token)}`;
      const ws = new WebSocket(url);
      wsRef.current = ws;

      ws.onopen = () => {
        setConnected(true);
        reconnectDelayRef.current = 1000;
      };

      ws.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data as string) as WSMessage;

          switch (data.type) {
            case "backfill":
              setMessages(data.messages);
              setTimeout(scrollToBottom, 50);
              break;
            case "message":
              setMessages((prev) => [
                ...prev,
                {
                  id: data.id,
                  user_id: data.user_id,
                  username: data.username,
                  message: data.message,
                  created_at: data.created_at,
                },
              ]);
              setTimeout(scrollToBottom, 50);
              break;
            case "delete":
              setMessages((prev) => prev.filter((m) => m.id !== data.id));
              break;
            case "error":
              break;
          }
        } catch {
          // Ignore malformed messages.
        }
      };

      ws.onclose = () => {
        setConnected(false);
        wsRef.current = null;

        if (shouldReconnectRef.current) {
          const delay = reconnectDelayRef.current;
          reconnectTimeoutRef.current = setTimeout(() => {
            reconnectDelayRef.current = Math.min(delay * 2, 30000);
            connect();
          }, delay);
        }
      };

      ws.onerror = () => {
        // onclose will fire after onerror, which handles reconnection.
      };
    }

    connectRef.current = connect;
    connect();

    return () => {
      shouldReconnectRef.current = false;
      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current);
      }
      if (wsRef.current) {
        wsRef.current.close();
        wsRef.current = null;
      }
    };
  }, [isAuthenticated, scrollToBottom]);

  const sendMessage = useCallback(() => {
    const text = input.trim();
    if (!text || !wsRef.current || wsRef.current.readyState !== WebSocket.OPEN)
      return;

    wsRef.current.send(JSON.stringify({ type: "message", text }));
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
        {
          headers: { Authorization: `Bearer ${token}` },
        },
      );
      if (!resp.ok) return;

      const data = (await resp.json()) as { messages: ChatMessage[] };
      const older = data.messages;
      if (older && older.length > 0) {
        setMessages((prev) => [...older, ...prev]);
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
          <div className="chat__messages" ref={messagesContainerRef}>
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
