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
  test("builds a comment deep-link with the page from structured data", () => {
    const n = notif("mention", {
      source: "torrent_comment",
      torrent_id: 7,
      comment_id: 9,
      page: 3,
      context_title: "Some Release",
      actor_username: "bob",
    });
    expect(notificationLink(n)).toBe("/torrent/7?page=3#comment-9");
  });

  test("builds a forum deep-link and omits page 1", () => {
    const n = notif("mention", {
      source: "forum_post",
      topic_id: 4,
      post_id: 8,
      page: 1,
      context_title: "A Topic",
      actor_username: "bob",
    });
    expect(notificationLink(n)).toBe("/forums/topics/4#post-8");
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

  test("a mention without link data has no link", () => {
    const n = notif("mention", { actor_username: "bob" });
    expect(notificationLink(n)).toBeNull();
  });

  test("legacy forum_mention still links via topic_id", () => {
    const n = notif("forum_mention", { topic_id: 42, topic_title: "T" });
    expect(notificationLink(n)).toBe("/forums/topics/42");
  });
});
