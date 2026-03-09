import { useCallback, useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { useChat } from "@/lib/useChat";
import "./chat-mod-menu.css";

interface ChatModMenuProps {
  userId: number;
  username: string;
}

export function ChatModMenu({ userId, username }: ChatModMenuProps) {
  const { deleteUserMessages, muteUser, unmuteUser } = useChat();
  const [open, setOpen] = useState(false);
  const [showMuteForm, setShowMuteForm] = useState(false);
  const [muteDuration, setMuteDuration] = useState("10");
  const [muteReason, setMuteReason] = useState("");
  const menuRef = useRef<HTMLDivElement>(null);

  // Close menu on outside click
  useEffect(() => {
    if (!open) return;
    const handleClick = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setOpen(false);
        setShowMuteForm(false);
      }
    };
    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, [open]);

  const handleDeleteAll = useCallback(async () => {
    if (!window.confirm(`Delete all chat messages from ${username}?`)) return;
    await deleteUserMessages(userId);
    setOpen(false);
  }, [userId, username, deleteUserMessages]);

  const handleMute = useCallback(async () => {
    const duration = parseInt(muteDuration, 10);
    if (!duration || duration <= 0) return;
    await muteUser(userId, duration, muteReason);
    setOpen(false);
    setShowMuteForm(false);
    setMuteDuration("10");
    setMuteReason("");
  }, [userId, muteDuration, muteReason, muteUser]);

  const handleUnmute = useCallback(async () => {
    await unmuteUser(userId);
    setOpen(false);
  }, [userId, unmuteUser]);

  return (
    <div className="chat-mod-menu" ref={menuRef}>
      <button
        className="chat-mod-menu__trigger"
        onClick={() => {
          setOpen((p) => !p);
          setShowMuteForm(false);
        }}
        title="Moderation actions"
      >
        {username}
      </button>
      {open && (
        <div className="chat-mod-menu__dropdown">
          <Link
            to={`/user/${userId}`}
            className="chat-mod-menu__item"
            onClick={() => setOpen(false)}
          >
            View profile
          </Link>
          <button className="chat-mod-menu__item" onClick={handleDeleteAll}>
            Delete all messages
          </button>
          <button
            className="chat-mod-menu__item"
            onClick={() => setShowMuteForm((p) => !p)}
          >
            Mute user
          </button>
          <button className="chat-mod-menu__item" onClick={handleUnmute}>
            Unmute user
          </button>
          {showMuteForm && (
            <div className="chat-mod-menu__mute-form">
              <input
                type="number"
                min="1"
                max="43200"
                value={muteDuration}
                onChange={(e) => setMuteDuration(e.target.value)}
                placeholder="Minutes"
                className="chat-mod-menu__mute-input"
              />
              <input
                type="text"
                value={muteReason}
                onChange={(e) => setMuteReason(e.target.value)}
                placeholder="Reason (optional)"
                className="chat-mod-menu__mute-input"
              />
              <button className="chat-mod-menu__mute-btn" onClick={handleMute}>
                Confirm mute
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
