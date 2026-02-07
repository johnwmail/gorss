# GoRSS Implementation Plan

## Phase 1: Core Data Model & Database (CURRENT)

- [x] Database schema design
  - users, categories, feeds, articles, article_states tables
  - Indexes for performance
- [x] sqlc queries for CRUD operations
- [ ] Run migrations and verify schema

## Phase 2: Feed Management

- [ ] Feed subscription (add feed URL)
- [ ] Feed fetching with gofeed library
- [ ] Parse RSS/Atom and store articles
- [ ] Category management (create, rename, delete)
- [ ] Assign feeds to categories
- [ ] Unsubscribe from feeds

## Phase 3: Article Display & Reading

- [ ] Main UI layout (sidebar + article list)
  - Left sidebar: Special views + Categories + Feeds
  - Right area: Article list with expanded content
- [ ] Article list views:
  - All articles
  - Fresh (unread)
  - Starred
  - By feed
  - By category
- [ ] Article content display (expanded inline, like TT-RSS)
- [ ] Unread counts on feeds/categories

## Phase 4: Article State Management

- [ ] Mark article as read/unread
- [ ] Star/unstar articles
- [ ] Mark feed as read
- [ ] Mark all as read

## Phase 5: Keyboard Navigation (Vi-style)

- [ ] `j` / `n` - Next article
- [ ] `k` / `p` - Previous article  
- [ ] `o` / `Enter` - Open article in new tab
- [ ] `s` - Toggle star
- [ ] `u` - Toggle read/unread
- [ ] `r` - Refresh feeds
- [ ] `?` - Show keyboard shortcuts help
- [ ] `g a` - Go to All articles
- [ ] `g f` - Go to Fresh articles
- [ ] `g s` - Go to Starred articles

## Phase 6: Background Feed Refresh

- [ ] Goroutine for periodic feed updates
- [ ] Configurable refresh interval
- [ ] Update feed error status
- [ ] Manual refresh trigger

## Phase 7: Search & Filters

- [ ] Full-text search in articles
- [ ] Filter by date range
- [ ] Sort options (newest first, oldest first)

## Phase 8: Polish & UX

- [ ] Loading states
- [ ] Error handling & display
- [ ] Feed favicon display
- [ ] Relative time display ("5 min ago")
- [ ] Keyboard shortcut help modal

## Phase 9: Container Deployment

- [x] Dockerfile (multi-stage build)
- [x] docker-compose.yml
- [ ] Health check endpoint
- [ ] Graceful shutdown
- [ ] Environment variable configuration
- [ ] Volume mount for SQLite persistence

---

## TT-RSS Features Reference (from exploration)

### UI Layout
- Left sidebar with collapsible sections
- "Special" section: All articles, Fresh, Starred, Published, Archived, Recently read
- Categories with unread counts (e.g., "Security 95")
- Feeds listed under categories
- Article list shows: checkbox, star icon, title, author, source, time
- Expanded article content inline

### Keyboard Shortcuts (observed)
- `j` - Select/move to next article
- `k` - Select/move to previous article
- Article selection shows "1 article selected" in toolbar

### Toolbar Options
- Select dropdown
- Adaptive view toggle
- Sort: "Newest first"
- "Mark as read" dropdown
- Menu (≡) for settings/preferences

### Color Scheme
- Sidebar: Light gray (#f0f0f0)
- Selected/unread: Light orange/peach background
- Links: Teal/green (#2a9d8f)
- Source badges: Colored labels (orange for OSNews, etc.)

---

## Database Schema Summary

```sql
users(id, email, created_at, last_seen)
categories(id, user_id, title)
feeds(id, user_id, category_id, url, title, site_url, description, last_updated, last_error)
articles(id, feed_id, guid, url, title, author, content, summary, published_at)
article_states(user_id, article_id, is_read, is_starred, read_at, starred_at)
```

## API Endpoints Plan

```
GET  /                     - Main app (article list)
GET  /api/feeds            - List feeds
POST /api/feeds            - Subscribe to feed
DEL  /api/feeds/:id        - Unsubscribe
GET  /api/articles         - List articles (with filters)
POST /api/articles/:id/read      - Mark read
POST /api/articles/:id/unread    - Mark unread  
POST /api/articles/:id/star      - Star
POST /api/articles/:id/unstar    - Unstar
POST /api/feeds/:id/mark-read    - Mark feed read
POST /api/refresh          - Trigger feed refresh
GET  /api/categories       - List categories
POST /api/categories       - Create category
```
