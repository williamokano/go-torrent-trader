import { describe, test, expect } from "vitest";
import {
  notificationLink,
  notificationMessage,
  type Notification,
} from "./notificationDisplay";

function notif(type: string, data: Record<string, unknown>): Notification {
  return { id: 1, type, data, read: false, created_at: "2026-07-15T00:00:00Z" };
}

describe("mention notification rendering", () => {
  test("links to the publisher-provided deep-link URL", () => {
    const n = notif("mention", {
      url: "/torrent/7#comment-9",
      context_title: "Some Release",
      actor_username: "bob",
    });
    expect(notificationLink(n)).toBe("/torrent/7#comment-9");
  });

  test("message names the actor and the context", () => {
    const n = notif("mention", {
      url: "/x",
      context_title: "Some Release",
      actor_username: "bob",
    });
    expect(notificationMessage(n)).toBe('bob mentioned you in "Some Release"');
  });

  test("message omits the context when it is absent", () => {
    const n = notif("mention", { url: "/x", actor_username: "bob" });
    expect(notificationMessage(n)).toBe("bob mentioned you");
  });

  test("a mention without a url has no link", () => {
    const n = notif("mention", { actor_username: "bob" });
    expect(notificationLink(n)).toBeNull();
  });

  test("legacy forum_mention still links via topic_id", () => {
    const n = notif("forum_mention", { topic_id: 42, topic_title: "T" });
    expect(notificationLink(n)).toBe("/forums/topics/42");
  });
});
