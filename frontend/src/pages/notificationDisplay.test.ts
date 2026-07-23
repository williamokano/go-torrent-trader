import { describe, test, expect } from "vitest";
import {
  notificationLink,
  notificationMessage,
  groupLink,
  groupMessage,
  type Notification,
  type NotificationGroup,
} from "./notificationDisplay";

function notif(
  type: string,
  data: Record<string, unknown>,
  overrides: Partial<Notification> = {},
): Notification {
  return {
    id: 1,
    type,
    data,
    read: false,
    created_at: "2026-07-15T00:00:00Z",
    ...overrides,
  };
}

function group(overrides: Partial<NotificationGroup> = {}): NotificationGroup {
  return {
    key: "topic_reply:4",
    type: "topic_reply",
    count: 1,
    unread: true,
    last_actors: [],
    latest_created_at: "2026-07-15T00:00:00Z",
    data: {},
    notifications: [],
    ...overrides,
  };
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

  test("moderation_message links to the torrent and reads sensibly", () => {
    const n = notif("moderation_message", {
      actor_username: "carol",
      torrent_id: 12,
      torrent_name: "Big Buck Bunny",
    });
    expect(notificationLink(n)).toBe("/torrent/12");
    expect(notificationMessage(n)).toBe(
      'carol posted in the moderation review of "Big Buck Bunny"',
    );
  });

  test("moderation_decision reads approved / rejected and links to the torrent", () => {
    const approved = notif("moderation_decision", {
      torrent_id: 12,
      torrent_name: "Big Buck Bunny",
      decision: "approved",
    });
    expect(notificationLink(approved)).toBe("/torrent/12");
    expect(notificationMessage(approved)).toBe(
      'Your torrent "Big Buck Bunny" was approved',
    );

    const rejected = notif("moderation_decision", {
      torrent_id: 12,
      torrent_name: "Big Buck Bunny",
      decision: "rejected",
    });
    expect(notificationMessage(rejected)).toBe(
      'Your torrent "Big Buck Bunny" was rejected',
    );
  });
});

describe("notification group rendering (BE-9.14)", () => {
  test("a singleton group (count 1) renders exactly like its one notification", () => {
    const n = notif("mention", { actor_username: "bob", context_title: "X" });
    const g = group({ type: "mention", count: 1, notifications: [n] });
    expect(groupMessage(g)).toBe(notificationMessage(n));
    expect(groupLink(g)).toBe(notificationLink(n));
  });

  test("a collapsed group names the count, topic, and actors", () => {
    const g = group({
      count: 5,
      data: { topic_id: 4, topic_title: "Release Discussion" },
      last_actors: ["carol", "bob", "alice"],
    });
    expect(groupMessage(g)).toBe(
      '5 new replies in "Release Discussion" from carol, bob, and alice',
    );
  });

  test("drops the 'new' label once every reply in the group has been read", () => {
    const g = group({
      count: 5,
      unread: false,
      data: { topic_id: 4, topic_title: "Release Discussion" },
      last_actors: ["carol", "bob", "alice"],
    });
    expect(groupMessage(g)).toBe(
      '5 replies in "Release Discussion" from carol, bob, and alice',
    );
  });

  test("a collapsed group links to the topic", () => {
    const g = group({ count: 3, data: { topic_id: 9 } });
    expect(groupLink(g)).toBe("/forums/topics/9");
  });

  test("falls back to a generic topic label when the title is missing", () => {
    const g = group({ count: 2, data: {}, last_actors: ["alice"] });
    expect(groupMessage(g)).toBe('2 new replies in "a topic" from alice');
  });

  test("formats two actors with 'and', three+ with an Oxford comma", () => {
    expect(
      groupMessage(group({ count: 2, last_actors: ["alice", "bob"] })),
    ).toContain("alice and bob");
    expect(
      groupMessage(group({ count: 3, last_actors: ["alice", "bob", "carol"] })),
    ).toContain("alice, bob, and carol");
  });

  test("falls back to a neutral phrase when no actors were recorded", () => {
    expect(groupMessage(group({ count: 4, last_actors: [] }))).toContain(
      "several people",
    );
  });

  test("a collapsed group without a topic_id has no link", () => {
    const g = group({ count: 2, data: {} });
    expect(groupLink(g)).toBeNull();
  });
});
