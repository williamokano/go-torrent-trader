import { useCallback, useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { useChat } from "@/lib/useChat";
import "./shoutbox.css";

function formatTime(iso: string): string {
  const d = new Date(iso);
  return d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}

/**
 * Full-size shoutbox for the home page. When mounted, it signals the
 * ChatProvider that the main chat is visible, which hides the floating
 * side chat to avoid showing two chat UIs at once.
 */
export function Shoutbox() {
  const {
    messages,
    connected,
    isStaff,
    setMainChatVisible,
    sendMessage,
    deleteMessage,
    loadMore,
  } = useChat();
  const [input, setInput] = useState("");
  const messagesEndRef = useRef<HTMLDivElement>(null);

  // Signal that the main chat is visible while this component is mounted
  useEffect(() => {
    setMainChatVisible(true);
    return () => setMainChatVisible(false);
  }, [setMainChatVisible]);

  const scrollToBottom = useCallback(() => {
    requestAnimationFrame(() => {
      messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
    });
  }, []);

  // Auto-scroll when new messages arrive
  useEffect(() => {
    scrollToBottom();
  }, [messages.length, scrollToBottom]);

  const handleSend = useCallback(() => {
    const text = input.trim();
    if (!text) return;
    sendMessage(text);
    setInput("");
  }, [input, sendMessage]);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === "Enter" && !e.shiftKey) {
        e.preventDefault();
        handleSend();
      }
    },
    [handleSend],
  );

  return (
    <div className="shoutbox">
      <div className="shoutbox__header">
        <h2 className="shoutbox__title">Shoutbox</h2>
        <span
          className={`shoutbox__status shoutbox__status--${connected ? "connected" : "disconnected"}`}
        >
          {connected ? "connected" : "disconnected"}
        </span>
      </div>

      <div className="shoutbox__messages">
        {messages.length > 0 && (
          <button className="shoutbox__load-more" onClick={loadMore}>
            Load older messages
          </button>
        )}
        {messages.length === 0 && (
          <div className="shoutbox__empty">
            No messages yet. Be the first to say something!
          </div>
        )}
        {messages.map((msg) => (
          <div key={msg.id} className="shoutbox__message">
            <span className="shoutbox__message-time">
              {formatTime(msg.created_at)}
            </span>
            <Link
              to={`/user/${msg.user_id}`}
              className="shoutbox__message-user"
            >
              {msg.username}
            </Link>
            <span className="shoutbox__message-text">{msg.message}</span>
            {isStaff && (
              <button
                className="shoutbox__message-delete"
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

      <div className="shoutbox__input-area">
        <input
          className="shoutbox__input"
          type="text"
          placeholder="Type a message..."
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={handleKeyDown}
          maxLength={500}
          disabled={!connected}
        />
        <button
          className="shoutbox__send-btn"
          onClick={handleSend}
          disabled={!connected || !input.trim()}
        >
          Send
        </button>
      </div>
    </div>
  );
}
