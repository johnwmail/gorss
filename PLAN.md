# GoRSS Implementation Plan

## Phase 1: Core Data Model & Database ✅

- [x] Database schema design
  - users, categories, feeds, articles, article_states tables
  - Indexes for performance
- [x] sqlc queries for CRUD operations
- [x] Run migrations and verify schema

## Phase 2: Feed Management ✅

- [x] Feed subscription (add feed URL)
- [x] Feed fetching with gofeed library
- [x] Parse RSS/Atom and store articles
- [x] Category management (create)
- [ ] Assign feeds to categories (UI pending)
- [x] Unsubscribe from feeds

## Phase 3: Article Display & Reading ✅

- [x] Main UI layout (sidebar + article list)
  - Left sidebar: Special views + Categories + Feeds
  - Right area: Article list with expanded content
- [x] Article list views:
  - All articles
  - Fresh (unread)
  - Starred
  - By feed
  - By category (pending)
- [x] Article content display (expanded inline, like TT-RSS)
- [x] Unread counts on feeds/categories

## Phase 4: Article State Management ✅

- [x] Mark article as read/unread
- [x] Star/unstar articles
- [x] Mark feed as read
- [ ] Mark all as read (API exists, UI pending)

## Phase 5: Keyboard Navigation (Vi-style) ✅

- [x] `j` / `↓` - Next article
- [x] `k` / `↑` - Previous article  
- [x] `o` / `Enter` - Open article in new tab
- [x] `s` - Toggle star
- [x] `u` - Toggle read/unread
- [x] `r` - Refresh feeds
- [x] `?` - Show keyboard shortcuts help
- [x] `g a` - Go to All articles
- [x] `g f` - Go to Fresh articles
- [x] `g s` - Go to Starred articles

## Phase 6: Background Feed Refresh ✅

- [x] Goroutine for periodic feed updates
- [x] Configurable refresh interval
- [x] Update feed error status
- [x] Manual refresh trigger

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
