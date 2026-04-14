# Feature/Improvement Tracker

## High Priority

### #1 Add tests for `feed.go`
- **Status**: Pending
- **Priority**: 🔴 High
- **Description**: `feed.go` has 0% test coverage. This is the most critical untested component handling feed fetching, HTTP conditional GET, background refresh, error backoff, and auto-purge.
- **Tasks**:
  - [ ] Mock HTTP responses for feed fetching
  - [ ] Test `FetchConditional` (200, 304, error cases)
  - [ ] Test `filterOldItems` date filtering
  - [ ] Test `shouldSkipFeed` backoff logic
  - [ ] Test `refreshAllFeeds` with multiple feeds
  - [ ] Test `purgeOldArticles`
  - [ ] Test backup scheduling logic

### #2 Expose article search
- **Status**: ✅ Completed
- **Priority**: 🔴 High
- **Description**: Added search API endpoint and UI for searching articles by title, content, or summary.
- **Implementation**:
  - Added `GET /api/articles/search?q=<query>` endpoint in `handlers.go`
  - Added search input in main header in `app.html` template
  - Added search styling in `app.css`
  - Added search JavaScript with debounced input in `app.js`
  - Uses existing `SearchArticles` SQL query from `visitors.sql`

### #3 Show feed errors in UI
- **Status**: ✅ Completed
- **Priority**: 🔴 High
- **Description**: Feed errors now displayed in sidebar with warning icon and error message on hover.
- **Implementation**:
  - Modified `feedItemHtml()` to check `error_count` and `last_error` fields
  - Shows ⚠️ icon instead of 📡 when feed has errors
  - Adds `feed-error` CSS class for styling
  - Error icon pulses with CSS animation to draw attention
  - Tooltip shows error message on hover

### #4 Pre-compile templates
- **Status**: ✅ Completed
- **Priority**: 🔴 High
- **Description**: Templates are now parsed once at startup instead of on every request, improving performance.
- **Implementation**:
  - Added `templates` map to `Server` struct
  - Added `precompileTemplates()` method called during `New()`
  - Updated `renderTemplate()` to use cached templates
  - Templates parsed: `app.html`, `welcome.html`

## Medium Priority

### #5 Feed favicon fetching
- **Status**: Pending
- **Priority**: 🟡 Medium
- **Description**: Currently all feeds show a generic emoji. Fetch and display feed icons for better visual scanning.
- **Tasks**:
  - [ ] Add `favicon_url` column to feeds table (migration)
  - [ ] Fetch favicon from feed site URL on subscribe
  - [ ] Store favicon locally or cache URL
  - [ ] Display favicon in sidebar next to feed title
  - [ ] Fallback to emoji if no favicon

### #6 Add CSRF protection
- **Status**: Pending
- **Priority**: 🟡 Medium
- **Description**: Currently relies only on SameSite cookies. Add CSRF tokens for mutation endpoints.
- **Tasks**:
  - [ ] Generate CSRF token on session creation
  - [ ] Add CSRF token to form submissions
  - [ ] Add CSRF middleware to validate tokens
  - [ ] Skip CSRF for proxy auth mode (uses headers)

### #7 Rate limiting
- **Status**: Pending
- **Priority**: 🟡 Medium
- **Description**: No rate limiting on API endpoints. Add basic rate limiting to prevent abuse.
- **Tasks**:
  - [ ] Add in-memory rate limiter (token bucket or sliding window)
  - [ ] Apply to all `/api/*` endpoints
  - [ ] Return 429 Too Many Requests with retry-after
  - [ ] Make limits configurable via env vars

### #8 Improve `resolveCategoryMap` and `importSingleFeed`
- **Status**: Pending
- **Priority**: 🟡 Medium
- **Description**: These have 0% coverage and are complex. Simplify or add comprehensive tests.
- **Tasks**:
  - [ ] Add tests for `resolveCategoryMap` (nil, empty, existing, new categories)
  - [ ] Add tests for `importSingleFeed` (all cases)
  - [ ] Refactor if needed for clarity
  - [ ] Target 90%+ coverage

### #9 Mark-read delay
- **Status**: Pending
- **Priority**: 🟡 Medium
- **Description**: Scrolling past articles marks them immediately. Add a configurable delay to prevent accidental marks.
- **Tasks**:
  - [ ] Add delay setting (default 500ms)
  - [ ] Use setTimeout to delay mark-read API call
  - [ ] Cancel delay if user scrolls back
  - [ ] Make configurable in UI or localStorage

## Nice-to-Have

### #10 Tags/labels system
- **Status**: Pending
- **Priority**: 🟢 Nice-to-Have
- **Description**: Beyond starred, allow users to tag articles for custom organization.
- **Tasks**:
  - [ ] Add `tags` table and `article_tags` junction table
  - [ ] Add API for tag CRUD
  - [ ] Add UI for adding/removing tags on articles
  - [ ] Add filter by tag in sidebar

### #11 Article sharing/export
- **Status**: Pending
- **Priority**: 🟢 Nice-to-Have
- **Description**: Add ability to share individual articles via URL or export.
- **Tasks**:
  - [ ] Add "Share" button on article view
  - [ ] Generate shareable link (public URL or copy content)
  - [ ] Add export selected articles (JSON/HTML)
  - [ ] Add copy article URL to clipboard

### #12 Multi-user management UI
- **Status**: Pending
- **Priority**: 🟢 Nice-to-Have
- **Description**: Infrastructure exists (users table) but no admin interface for managing users.
- **Tasks**:
  - [ ] Add admin settings page
  - [ ] List users with last seen, created date
  - [ ] Delete user with all data
  - [ ] Rename user/email
  - [ ] Only accessible in proxy auth mode

### #13 API documentation
- **Status**: Pending
- **Priority**: 🟢 Nice-to-Have
- **Description**: No OpenAPI/Swagger spec. Add documentation for the API endpoints.
- **Tasks**:
  - [ ] Create OpenAPI 3.0 spec
  - [ ] Document all endpoints with request/response examples
  - [ ] Add to `/api/docs` route
  - [ ] Use swagger-ui or redoc for rendering

### #14 PWA support
- **Status**: Pending
- **Priority**: 🟢 Nice-to-Have
- **Description**: Add service worker and manifest.json for installable web app experience.
- **Tasks**:
  - [ ] Add `manifest.json` with app metadata
  - [ ] Add service worker for offline caching
  - [ ] Cache static assets for offline reading
  - [ ] Add install prompt
  - [ ] Add app icons for various sizes
