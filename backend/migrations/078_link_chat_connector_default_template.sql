-- +goose Up
-- The shoutbox connector's default line now links the torrent name instead of
-- trailing a bare URL (see internal/connector/chat.DefaultTemplate). Instances
-- created before this stored the old shared default *verbatim*, so the code
-- change alone would not reach them.
--
-- Only rows holding exactly that string are rewritten: a hand-written template
-- is the admin's wording and must survive a deploy. Rows with an empty template
-- are deliberately left empty — they already track whatever the code default is,
-- and pinning one here would take that away.
UPDATE notification_connectors
SET config = jsonb_set(
        config,
        '{template}',
        to_jsonb('New torrent: {{.Link}} — {{.Category}}, {{.SizeHuman}}'::text),
        true
    ),
    updated_at = NOW()
WHERE kind = 'chat'
  AND config->>'template' = 'New torrent: {{.Name}} [{{.Category}}, {{.SizeHuman}}] {{.URL}}';

-- +goose Down
UPDATE notification_connectors
SET config = jsonb_set(
        config,
        '{template}',
        to_jsonb('New torrent: {{.Name}} [{{.Category}}, {{.SizeHuman}}] {{.URL}}'::text),
        true
    ),
    updated_at = NOW()
WHERE kind = 'chat'
  AND config->>'template' = 'New torrent: {{.Link}} — {{.Category}}, {{.SizeHuman}}';
