import { useCallback, useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { useAuth } from "@/features/auth";
import { useChat } from "@/lib/useChat";
import { ConfirmModal } from "@/components/modal";
import { ChatModMenu } from "./ChatModMenu";
import "./chat.css";

function formatTime(iso: string): string {
  const d = new Date(iso);
  return d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}

export function Chat() {
  const { isAuthenticated } = useAuth();
  const {
    messages,
    connected,
    isStaff,
    muted,
    muteExpiresAt,
    chatSuspended,
    mainChatVisible,
    sendMessage,
    deleteMessage,
    loadMore,
  } = useChat();
  const [collapsed, setCollapsed] = useState(true);
  const [input, setInput] = useState("");
  const [deletingMsgId, setDeletingMsgId] = useState<number | null>(null);
  const [muteRemainingText, setMuteRemainingText] = useState("");
  const messagesEndRef = useRef<HTMLDivElement>(null);

  // Update mute remaining time display periodically.
  useEffect(() => {
    if (!muted || !muteExpiresAt) return;
    const computeText = () => {
      const ms = new Date(muteExpiresAt).getTime() - Date.now();
      if (ms <= 0) return "";
      const mins = Math.ceil(ms / 60000);
      return mins === 1 ? "1 minute" : `${mins} minutes`;
    };
    // Use requestAnimationFrame to schedule the initial update outside the
    // synchronous effect body, satisfying the set-state-in-effect lint rule.
    const raf = requestAnimationFrame(() => {
      setMuteRemainingText(computeText());
    });
    const interval = setInterval(() => {
      setMuteRemainingText(computeText());
    }, 30000);
    return () => {
      cancelAnimationFrame(raf);
      clearInterval(interval);
      setMuteRemainingText("");
    };
  }, [muted, muteExpiresAt]);

  const scrollToBottom = useCallback(() => {
    requestAnimationFrame(() => {
      messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
    });
  }, []);

  const handleSend = useCallback(() => {
    const text = input.trim();
    if (!text) return;
    sendMessage(text);
    setInput("");
    setTimeout(scrollToBottom, 50);
  }, [input, sendMessage, scrollToBottom]);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === "Enter" && !e.shiftKey) {
        e.preventDefault();
        handleSend();
      }
    },
    [handleSend],
  );

  // Don't render if not authenticated or if the main page shoutbox is visible
  if (!isAuthenticated || mainChatVisible) return null;

  return (
    <div className={`chat chat--${collapsed ? "collapsed" : "expanded"}`}>
      <div className="chat__header" onClick={() => setCollapsed((p) => !p)}>
        <span>
          <span className="chat__header-title">Shoutbox</span>
          <span
            className={`chat__header-status chat__header-status--${connected ? "connected" : "disconnected"}`}
            title={connected ? "Connected" : "Disconnected"}
          >
            {connected ? "\uD83D\uDFE2" : "\uD83D\uDD34"}
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
                {isStaff ? (
                  <ChatModMenu userId={msg.user_id} username={msg.username} />
                ) : (
                  <Link
                    to={`/user/${msg.user_id}`}
                    className="chat__message-user"
                  >
                    {msg.username}
                  </Link>
                )}
                <span className="chat__message-text">{msg.message}</span>
                {isStaff && (
                  <button
                    className="chat__message-delete"
                    onClick={() => setDeletingMsgId(msg.id)}
                    title="Delete message"
                  >
                    x
                  </button>
                )}
              </div>
            ))}
            <div ref={messagesEndRef} />
          </div>

          {muted && muteRemainingText && (
            <div className="chat__muted-notice">
              You are muted. (expires in {muteRemainingText})
            </div>
          )}
          <div className="chat__input-area">
            <input
              className="chat__input"
              type="text"
              placeholder={chatSuspended ? "Chat suspended" : muted ? "You are muted" : "Type a message..."}
              value={input}
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={handleKeyDown}
              maxLength={500}
              disabled={!connected || muted || chatSuspended}
            />
            <button
              className="chat__send-btn"
              onClick={handleSend}
              disabled={!connected || !input.trim() || muted || chatSuspended}
            >
              Send
            </button>
          </div>
        </>
      )}
      <ConfirmModal
        isOpen={deletingMsgId !== null}
        title="Delete Message"
        message="Are you sure you want to delete this message?"
        confirmLabel="Delete"
        danger
        onConfirm={() => {
          if (deletingMsgId !== null) deleteMessage(deletingMsgId);
          setDeletingMsgId(null);
        }}
        onCancel={() => setDeletingMsgId(null)}
      />
    </div>
  );
}
