-- User queries

-- name: UpsertUser :exec
INSERT INTO users (id, email, created_at, last_seen)
VALUES (?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
  email = excluded.email,
  last_seen = excluded.last_seen;

-- name: GetUser :one
SELECT * FROM users WHERE id = ?;

-- Category queries

-- name: CreateCategory :one
INSERT INTO categories (user_id, title) VALUES (?, ?) RETURNING *;

-- name: GetCategories :many
SELECT * FROM categories WHERE user_id = ? ORDER BY title;

-- name: GetCategory :one
SELECT * FROM categories WHERE id = ? AND user_id = ?;

-- name: UpdateCategory :exec
UPDATE categories SET title = ? WHERE id = ? AND user_id = ?;

-- name: DeleteCategory :exec
DELETE FROM categories WHERE id = ? AND user_id = ?;

-- Feed queries

-- name: CreateFeed :one
INSERT INTO feeds (user_id, category_id, url, title, site_url, description)
VALUES (?, ?, ?, ?, ?, ?) RETURNING *;

-- name: GetFeeds :many
SELECT f.*, c.title as category_title,
  (SELECT COUNT(*) FROM articles a 
   LEFT JOIN article_states s ON s.article_id = a.id AND s.user_id = f.user_id
   WHERE a.feed_id = f.id AND (s.is_read IS NULL OR s.is_read = 0)) as unread_count
FROM feeds f
LEFT JOIN categories c ON f.category_id = c.id
WHERE f.user_id = ?
ORDER BY f.title;

-- name: GetFeed :one
SELECT f.*, c.title as category_title
FROM feeds f
LEFT JOIN categories c ON f.category_id = c.id
WHERE f.id = ? AND f.user_id = ?;

-- name: GetFeedByURL :one
SELECT * FROM feeds WHERE user_id = ? AND url = ?;

-- name: UpdateFeed :exec
UPDATE feeds SET
  category_id = ?,
  title = ?,
  site_url = ?,
  description = ?,
  last_updated = ?,
  last_error = ?
WHERE id = ?;

-- name: UpdateFeedMeta :exec
UPDATE feeds SET
  title = ?,
  site_url = ?,
  description = ?,
  last_updated = ?,
  last_error = ?,
  etag = ?,
  last_modified = ?,
  error_count = ?
WHERE id = ?;

-- name: UpdateFeedDetails :exec
UPDATE feeds SET title = ?, url = ? WHERE id = ? AND user_id = ?;

-- name: DeleteFeed :exec
DELETE FROM feeds WHERE id = ? AND user_id = ?;

-- name: GetAllFeedsForRefresh :many
SELECT * FROM feeds ORDER BY last_updated ASC NULLS FIRST LIMIT ?;

-- Article queries

-- name: UpsertArticle :one
INSERT INTO articles (feed_id, guid, url, title, author, content, summary, published_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (feed_id, guid) DO UPDATE SET
  url = excluded.url,
  title = excluded.title,
  author = excluded.author,
  content = excluded.content,
  summary = excluded.summary,
  published_at = excluded.published_at
RETURNING *;

-- name: GetArticles :many
SELECT a.*, f.title as feed_title, f.site_url as feed_site_url,
  COALESCE(s.is_read, 0) as is_read,
  COALESCE(s.is_starred, 0) as is_starred
FROM articles a
JOIN feeds f ON a.feed_id = f.id
LEFT JOIN article_states s ON s.article_id = a.id AND s.user_id = ?
WHERE f.user_id = ?
ORDER BY a.published_at DESC
LIMIT ? OFFSET ?;

-- name: GetArticlesByFeed :many
SELECT a.*, f.title as feed_title, f.site_url as feed_site_url,
  COALESCE(s.is_read, 0) as is_read,
  COALESCE(s.is_starred, 0) as is_starred
FROM articles a
JOIN feeds f ON a.feed_id = f.id
LEFT JOIN article_states s ON s.article_id = a.id AND s.user_id = ?
WHERE f.id = ? AND f.user_id = ?
ORDER BY a.published_at DESC
LIMIT ? OFFSET ?;

-- name: GetArticlesByCategory :many
SELECT a.*, f.title as feed_title, f.site_url as feed_site_url,
  COALESCE(s.is_read, 0) as is_read,
  COALESCE(s.is_starred, 0) as is_starred
FROM articles a
JOIN feeds f ON a.feed_id = f.id
LEFT JOIN article_states s ON s.article_id = a.id AND s.user_id = ?
WHERE f.category_id = ? AND f.user_id = ?
ORDER BY a.published_at DESC
LIMIT ? OFFSET ?;

-- name: GetUnreadArticles :many
SELECT a.*, f.title as feed_title, f.site_url as feed_site_url,
  COALESCE(s.is_read, 0) as is_read,
  COALESCE(s.is_starred, 0) as is_starred
FROM articles a
JOIN feeds f ON a.feed_id = f.id
LEFT JOIN article_states s ON s.article_id = a.id AND s.user_id = ?
WHERE f.user_id = ? AND (s.is_read IS NULL OR s.is_read = 0)
ORDER BY a.published_at DESC
LIMIT ? OFFSET ?;

-- name: GetStarredArticles :many
SELECT a.*, f.title as feed_title, f.site_url as feed_site_url,
  COALESCE(s.is_read, 0) as is_read,
  COALESCE(s.is_starred, 0) as is_starred
FROM articles a
JOIN feeds f ON a.feed_id = f.id
JOIN article_states s ON s.article_id = a.id AND s.user_id = ?
WHERE f.user_id = ? AND s.is_starred = 1
ORDER BY s.starred_at DESC
LIMIT ? OFFSET ?;

-- name: GetArticle :one
SELECT a.*, f.title as feed_title, f.site_url as feed_site_url,
  COALESCE(s.is_read, 0) as is_read,
  COALESCE(s.is_starred, 0) as is_starred
FROM articles a
JOIN feeds f ON a.feed_id = f.id
LEFT JOIN article_states s ON s.article_id = a.id AND s.user_id = ?
WHERE a.id = ? AND f.user_id = ?;

-- name: SearchArticles :many
SELECT a.*, f.title as feed_title, f.site_url as feed_site_url,
  COALESCE(s.is_read, 0) as is_read,
  COALESCE(s.is_starred, 0) as is_starred
FROM articles a
JOIN feeds f ON a.feed_id = f.id
LEFT JOIN article_states s ON s.article_id = a.id AND s.user_id = ?
WHERE f.user_id = ? AND (a.title LIKE ? OR a.content LIKE ? OR a.summary LIKE ?)
ORDER BY a.published_at DESC
LIMIT ? OFFSET ?;

-- Article state queries

-- name: SetArticleRead :exec
INSERT INTO article_states (user_id, article_id, is_read, read_at)
VALUES (?, ?, 1, ?)
ON CONFLICT (user_id, article_id) DO UPDATE SET
  is_read = 1,
  read_at = excluded.read_at;

-- name: SetArticleUnread :exec
INSERT INTO article_states (user_id, article_id, is_read)
VALUES (?, ?, 0)
ON CONFLICT (user_id, article_id) DO UPDATE SET
  is_read = 0,
  read_at = NULL;

-- name: SetArticleStarred :exec
INSERT INTO article_states (user_id, article_id, is_starred, starred_at)
VALUES (?, ?, 1, ?)
ON CONFLICT (user_id, article_id) DO UPDATE SET
  is_starred = 1,
  starred_at = excluded.starred_at;

-- name: SetArticleUnstarred :exec
INSERT INTO article_states (user_id, article_id, is_starred)
VALUES (?, ?, 0)
ON CONFLICT (user_id, article_id) DO UPDATE SET
  is_starred = 0,
  starred_at = NULL;

-- name: MarkFeedRead :exec
INSERT INTO article_states (user_id, article_id, is_read, read_at)
SELECT ?, a.id, 1, ?
FROM articles a
WHERE a.feed_id = ?
ON CONFLICT (user_id, article_id) DO UPDATE SET
  is_read = 1,
  read_at = excluded.read_at;

-- name: MarkAllRead :exec
INSERT INTO article_states (user_id, article_id, is_read, read_at)
SELECT ?, a.id, 1, ?
FROM articles a
JOIN feeds f ON a.feed_id = f.id
WHERE f.user_id = ?
ON CONFLICT (user_id, article_id) DO UPDATE SET
  is_read = 1,
  read_at = excluded.read_at;

-- Stats queries

-- name: GetUnreadCount :one
SELECT COUNT(*) as count
FROM articles a
JOIN feeds f ON a.feed_id = f.id
LEFT JOIN article_states s ON s.article_id = a.id AND s.user_id = f.user_id
WHERE f.user_id = ? AND (s.is_read IS NULL OR s.is_read = 0);

-- name: GetTotalArticleCount :one
SELECT COUNT(*) as count
FROM articles a
JOIN feeds f ON a.feed_id = f.id
WHERE f.user_id = ?;

-- name: GetStarredCount :one
SELECT COUNT(*) as count
FROM article_states s
JOIN articles a ON s.article_id = a.id
JOIN feeds f ON a.feed_id = f.id
WHERE s.user_id = ? AND s.is_starred = 1;

-- name: PurgeOldReadArticles :execresult
DELETE FROM articles
WHERE id IN (
  SELECT a.id FROM articles a
  JOIN feeds f ON a.feed_id = f.id
  JOIN article_states s ON s.article_id = a.id AND s.user_id = f.user_id
  WHERE s.is_read = 1 
    AND s.is_starred = 0
    AND a.published_at < ?
);

-- name: CountOldReadArticles :one
SELECT COUNT(*) as count
FROM articles a
JOIN feeds f ON a.feed_id = f.id
JOIN article_states s ON s.article_id = a.id AND s.user_id = f.user_id
WHERE s.is_read = 1 
  AND s.is_starred = 0
  AND a.published_at < ?;

-- name: UpdateCategorySortOrder :exec
UPDATE categories SET sort_order = ? WHERE id = ? AND user_id = ?;

-- name: UpdateFeedSortOrder :exec
UPDATE feeds SET sort_order = ? WHERE id = ? AND user_id = ?;

-- name: UpdateFeedCategory :exec
UPDATE feeds SET category_id = ?, sort_order = ? WHERE id = ? AND user_id = ?;

-- name: GetCategoriesOrdered :many
SELECT * FROM categories WHERE user_id = ? ORDER BY sort_order ASC, title ASC;

-- name: GetFeedsOrdered :many
SELECT * FROM feeds WHERE user_id = ? ORDER BY sort_order ASC, title ASC;
