// GoRSS - Unified Application
(function() {
  'use strict';

  // ── Theme Management ──────────────────────────────────────────────────
  // Modes: 'auto' (time-based), 'light', 'dark'
  const THEME_KEY = 'gorss-theme-mode';

  function getThemeMode() {
    return localStorage.getItem(THEME_KEY) || 'auto';
  }

  function isDaytime() {
    const hour = new Date().getHours();
    return hour >= 6 && hour < 21; // 6 AM – 9 PM = day
  }

  function resolveTheme(mode) {
    if (mode === 'light') return 'light';
    if (mode === 'dark') return 'dark';
    // auto: use device local time
    return isDaytime() ? 'light' : 'dark';
  }

  function applyTheme(theme) {
    document.documentElement.setAttribute('data-theme', theme);
    // Update toggle button icon
    const btn = document.getElementById('btn-theme');
    if (btn) {
      const mode = getThemeMode();
      const icons = { auto: '🌗', light: '☀️', dark: '🌙' };
      const labels = { auto: 'Auto (time-based)', light: 'Light mode', dark: 'Dark mode' };
      btn.textContent = icons[mode];
      btn.title = labels[mode];
    }
  }

  function cycleTheme() {
    const order = ['auto', 'light', 'dark'];
    const current = getThemeMode();
    const next = order[(order.indexOf(current) + 1) % order.length];
    localStorage.setItem(THEME_KEY, next);
    applyTheme(resolveTheme(next));
  }

  // Apply theme immediately (before DOM renders rest of page)
  applyTheme(resolveTheme(getThemeMode()));

  // Re-check every 10 minutes for auto mode (catches the 6 AM / 9 PM transitions)
  setInterval(() => {
    if (getThemeMode() === 'auto') {
      applyTheme(resolveTheme('auto'));
    }
  }, 600000);

  // Custom confirm dialog (replaces native confirm())
  function showConfirm(message, title = 'Confirm') {
    return new Promise(resolve => {
      const modal = document.getElementById('modal-confirm');
      document.getElementById('confirm-title').textContent = title;
      document.getElementById('confirm-message').textContent = message;
      modal.classList.add('open');

      const ok = document.getElementById('confirm-ok');
      const cancel = document.getElementById('confirm-cancel');

      function cleanup(result) {
        modal.classList.remove('open');
        ok.removeEventListener('click', onOk);
        cancel.removeEventListener('click', onCancel);
        modal.removeEventListener('click', onBackdrop);
        document.removeEventListener('keydown', onKey);
        resolve(result);
      }
      function onOk() { cleanup(true); }
      function onCancel() { cleanup(false); }
      function onBackdrop(e) { if (e.target === modal) cleanup(false); }
      function onKey(e) {
        if (e.key === 'Escape') cleanup(false);
        if (e.key === 'Enter') cleanup(true);
      }

      ok.addEventListener('click', onOk);
      cancel.addEventListener('click', onCancel);
      modal.addEventListener('click', onBackdrop);
      document.addEventListener('keydown', onKey);
      ok.focus();
    });
  }

  // State
  let currentView = 'fresh';
  let currentFeedId = null;
  let currentCategoryId = null;
  let articles = [];
  let feeds = [];
  let categories = [];
  let selectedIndex = -1;
  let gKeyPressed = false;
  let lastKnownCounts = { total: 0, unread: 0, starred: 0, feeds: {} };
  let articlesOffset = 0;
  let articlesLoading = false;
  let articlesExhausted = false;
  const ARTICLES_PAGE_SIZE = 100;

  // DOM Elements
  const sidebar = document.getElementById('sidebar');
  const drawerOverlay = document.getElementById('drawer-overlay');
  const articlesList = document.getElementById('articles-list');
  const feedsList = document.getElementById('feeds-list');

  // Initialize - handle both cases: DOM already loaded or still loading
  console.log('GoRSS script loaded, readyState:', document.readyState);
  if (document.readyState === 'loading') {
    console.log('Adding DOMContentLoaded listener');
    document.addEventListener('DOMContentLoaded', init);
  } else {
    console.log('Calling init immediately');
    init();
  }

  async function init() {
    console.log('GoRSS init starting...');
    try {
      setupEventListeners();
      console.log('Event listeners set up');
      setupKeyboardNav();
      console.log('Keyboard nav set up');
      setupTitleEdit();
      // Load feeds, counts, and navigate in parallel for fast startup
      const [feedsOk] = await Promise.allSettled([
        loadFeeds(),
        updateCounts()
      ]);
      if (feedsOk.status === 'rejected') console.error('Failed to load feeds');
      navigateTo(currentView);
      console.log('GoRSS init complete');
    } catch (e) {
      console.error('GoRSS init error:', e);
    }
  }

  function setupEventListeners() {
    // Menu toggle (mobile)
    document.getElementById('btn-menu')?.addEventListener('click', toggleSidebar);
    drawerOverlay?.addEventListener('click', closeSidebar);

    // Navigation items
    document.querySelectorAll('.nav-item[data-view]').forEach(el => {
      el.addEventListener('click', (e) => {
        e.preventDefault();
        const view = el.dataset.view;
        navigateTo(view);
        closeSidebar();
      });
    });

    // Add feed
    document.getElementById('btn-add-feed')?.addEventListener('click', () => {
      document.getElementById('modal-add-feed').classList.add('open');
      document.querySelector('#modal-add-feed input').focus();
    });

    document.getElementById('form-add-feed')?.addEventListener('submit', handleAddFeed);

    // Import/Export
    document.getElementById('btn-import')?.addEventListener('click', () => {
      document.getElementById('modal-import').classList.add('open');
    });

    document.getElementById('form-import')?.addEventListener('submit', handleImport);

    document.getElementById('btn-export')?.addEventListener('click', () => {
      window.location.href = '/api/opml/export';
    });

    // Refresh
    document.getElementById('btn-refresh')?.addEventListener('click', handleRefresh);
    document.getElementById('btn-fab-refresh')?.addEventListener('click', handleRefresh);

    // Mark all read
    document.getElementById('btn-mark-all-read')?.addEventListener('click', handleMarkAllRead);
    document.getElementById('btn-header-mark-read')?.addEventListener('click', handleMarkAllRead);

    // Theme toggle
    document.getElementById('btn-theme')?.addEventListener('click', cycleTheme);

    // Close modals
    document.querySelectorAll('.btn-cancel').forEach(btn => {
      btn.addEventListener('click', () => {
        document.querySelectorAll('.modal').forEach(m => m.classList.remove('open'));
      });
    });

    // Scroll mark-as-read + infinite scroll
    let scrollTimeout = null;
    articlesList?.addEventListener('scroll', () => {
      if (scrollTimeout) clearTimeout(scrollTimeout);
      scrollTimeout = setTimeout(handleScrollMarkRead, 300);

      // Infinite scroll: load more when near bottom
      if (!articlesLoading && !articlesExhausted) {
        const { scrollTop, scrollHeight, clientHeight } = articlesList;
        if (scrollHeight - scrollTop - clientHeight < 300) {
          loadMoreArticles();
        }
      }
    });

    // Resize spacer on window resize
    window.addEventListener('resize', () => {
      const spacer = articlesList.querySelector('.scroll-spacer');
      if (spacer) spacer.style.height = Math.max(0, articlesList.clientHeight - 80) + 'px';
    });
  }

  function setupKeyboardNav() {
    document.addEventListener('keydown', (e) => {
      if (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA') return;

      // g + key combos
      if (gKeyPressed) {
        gKeyPressed = false;
        switch (e.key) {
          case 'a': navigateTo('all'); break;
          case 'f': navigateTo('fresh'); break;
          case 's': navigateTo('starred'); break;
        }
        return;
      }

      if (e.key === 'g') {
        gKeyPressed = true;
        setTimeout(() => gKeyPressed = false, 1000);
        return;
      }

      switch (e.key) {
        case 'j':
        case 'ArrowDown':
          e.preventDefault();
          selectArticle(selectedIndex + 1);
          break;
        case 'k':
        case 'ArrowUp':
          e.preventDefault();
          selectArticle(selectedIndex - 1);
          break;
        case 'o':
        case 'Enter':
          if (selectedIndex >= 0 && articles[selectedIndex]) {
            const link = articlesList.querySelector(`[data-index="${selectedIndex}"] a.article-btn`);
            if (link) link.click();
          }
          break;
        case 's':
          toggleStar();
          break;
        case 'u':
          toggleRead();
          break;
        case 'r':
          handleRefresh();
          break;
        case '?':
          document.getElementById('modal-help').classList.add('open');
          break;
        case 'Escape':
          document.querySelectorAll('.modal').forEach(m => m.classList.remove('open'));
          closeSidebar();
          break;
      }
    });
  }

  // Sidebar
  function toggleSidebar() {
    sidebar.classList.toggle('open');
    drawerOverlay.classList.toggle('open');
  }

  function closeSidebar() {
    sidebar.classList.remove('open');
    drawerOverlay.classList.remove('open');
  }

  // Navigation
  function updateViewTitle() {
    const titles = { all: 'All Articles', fresh: 'Unread', starred: 'Starred' };
    let title = titles[currentView] || 'Articles';
    let editable = false;
    if (currentFeedId) {
      const feed = feeds.find(f => f.id == currentFeedId);
      title = feed ? feed.title : 'Feed';
      editable = true;
    }
    if (currentCategoryId !== null && currentCategoryId !== undefined) {
      if (currentCategoryId === 0) {
        title = 'Uncategorized';
      } else {
        const cat = categories.find(c => c.id == currentCategoryId);
        title = cat ? cat.title : 'Category';
      }
      const countEl = document.querySelector(`[data-cat-count="${currentCategoryId}"]`);
      const count = countEl ? parseInt(countEl.textContent) || 0 : 0;
      title += ` (${count})`;
    }
    // Append count for top-level views
    const countIds = { all: 'count-all', fresh: 'count-fresh', starred: 'count-starred' };
    if (!currentFeedId && currentCategoryId === null && countIds[currentView]) {
      const el = document.getElementById(countIds[currentView]);
      const count = el ? parseInt(el.textContent) || 0 : 0;
      title += ` (${count})`;
    }
    const titleEl = document.getElementById('current-view');
    titleEl.textContent = title;
    titleEl.classList.toggle('editable', editable);
    titleEl.title = editable ? 'Click to rename feed' : '';
  }

  // Inline feed title editing
  function setupTitleEdit() {
    const titleEl = document.getElementById('current-view');
    const modal = document.getElementById('modal-edit-feed');
    const form = document.getElementById('form-edit-feed');
    const nameInput = document.getElementById('edit-feed-name');
    const urlInput = document.getElementById('edit-feed-url');
    const errorEl = document.getElementById('edit-feed-error');

    // Click title to open edit modal
    titleEl.addEventListener('click', () => {
      if (!titleEl.classList.contains('editable') || !currentFeedId) return;
      const feed = feeds.find(f => f.id == currentFeedId);
      if (!feed) return;

      nameInput.value = feed.title || '';
      urlInput.value = feed.url || '';
      errorEl.textContent = '';
      modal.classList.add('open');
      nameInput.focus();
      nameInput.select();
    });

    // Close modal
    modal.querySelector('.btn-cancel').addEventListener('click', () => {
      modal.classList.remove('open');
    });
    modal.addEventListener('click', (e) => {
      if (e.target === modal) modal.classList.remove('open');
    });

    // Save
    form.addEventListener('submit', async (e) => {
      e.preventDefault();
      const feed = feeds.find(f => f.id == currentFeedId);
      if (!feed) return;

      const newTitle = nameInput.value.trim();
      const newUrl = urlInput.value.trim();
      errorEl.textContent = '';

      if (!newTitle) { errorEl.textContent = 'Name is required'; return; }
      if (!newUrl) { errorEl.textContent = 'URL is required'; return; }

      // Only send if something changed
      if (newTitle === feed.title && newUrl === feed.url) {
        modal.classList.remove('open');
        return;
      }

      const saveBtn = form.querySelector('.btn-primary');
      saveBtn.disabled = true;
      saveBtn.textContent = 'Saving...';

      try {
        const body = {};
        if (newTitle !== feed.title) body.title = newTitle;
        if (newUrl !== feed.url) body.url = newUrl;

        const res = await fetch(`/api/feeds/${currentFeedId}`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(body)
        });

        if (res.ok) {
          modal.classList.remove('open');
          await loadFeeds();
          updateViewTitle();
        } else {
          const err = await res.json();
          errorEl.textContent = err.error || 'Failed to save';
        }
      } catch (err) {
        errorEl.textContent = 'Network error';
      } finally {
        saveBtn.disabled = false;
        saveBtn.textContent = 'Save';
      }
    });
  }

  function navigateTo(view, feedId = null, categoryId = null) {
    currentView = view;
    currentFeedId = feedId;
    currentCategoryId = categoryId;
    selectedIndex = -1;
    dismissNewArticlesBanner();

    // Update active state
    document.querySelectorAll('.nav-item').forEach(el => {
      el.classList.remove('active');
      if (el.dataset.view === view && !feedId && !categoryId) el.classList.add('active');
      if (feedId && el.dataset.feedId == feedId) el.classList.add('active');
    });
    document.querySelectorAll('.cat-header').forEach(el => {
      el.classList.toggle('active', categoryId && el.dataset.catId == categoryId);
    });

    // Update title
    updateViewTitle();

    loadArticles();

    // Close drawer on mobile
    if (window.innerWidth <= 768) closeSidebar();
  }

  // Load feeds
  async function loadFeeds() {
    try {
      const [feedsRes, catsRes] = await Promise.all([
        fetch('/api/feeds'),
        fetch('/api/categories')
      ]);
      feeds = await feedsRes.json();
      categories = await catsRes.json();
      renderFeeds();
    } catch (e) {
      console.error('Failed to load feeds:', e);
    }
  }

  function renderFeeds() {
    if (!feedsList) return;

    // Group feeds by category
    const catMap = new Map();
    categories.forEach(c => catMap.set(c.id, { ...c, feeds: [] }));

    // Uncategorized bucket
    const uncategorized = [];

    feeds.forEach(f => {
      const catId = f.category_id;
      if (catId && catMap.has(catId)) {
        catMap.get(catId).feeds.push(f);
      } else {
        uncategorized.push(f);
      }
    });

    let html = '';

    // Render categorized feeds
    const sortedCats = [...catMap.values()].filter(c => c.feeds.length > 0).sort((a, b) => a.title.localeCompare(b.title));
    sortedCats.forEach(cat => {
      html += `
        <div class="feed-category" draggable="true" data-drag-cat="${cat.id}">
          <div class="category-header" data-cat-id="${cat.id}">
            <span class="category-toggle">▸</span>
            <span class="cat-header" data-cat-id="${cat.id}">${escapeHtml(cat.title)}</span>
            <span class="count" data-cat-count="${cat.id}">0</span>
            <span class="cat-mark-read" data-mark-cat="${cat.id}" title="Mark all as read">✓</span>
          </div>
          <div class="category-feeds" data-cat-feeds="${cat.id}" style="display:none">
            ${cat.feeds.map(f => feedItemHtml(f)).join('')}
          </div>
        </div>`;
    });

    // Render uncategorized feeds
    if (uncategorized.length) {
      html += `
        <div class="feed-category" data-drag-cat="0">
          <div class="category-header" data-cat-id="0">
            <span class="category-toggle">▸</span>
            <span class="cat-header" data-cat-id="0">Uncategorized</span>
            <span class="count" data-cat-count="0">0</span>
            <span class="cat-mark-read" data-mark-cat="0" title="Mark all as read">✓</span>
          </div>
          <div class="category-feeds" data-cat-feeds="0" style="display:none">
            ${uncategorized.map(f => feedItemHtml(f)).join('')}
          </div>
        </div>`;
    }

    feedsList.innerHTML = html;

    // Category toggle (arrow) click
    feedsList.querySelectorAll('.category-toggle').forEach(toggle => {
      toggle.addEventListener('click', (e) => {
        e.stopPropagation();
        const header = toggle.closest('.category-header');
        const catId = header.dataset.catId;
        const feedsDiv = feedsList.querySelector(`[data-cat-feeds="${catId}"]`);
        if (feedsDiv.style.display === 'none') {
          feedsDiv.style.display = 'block';
          toggle.textContent = '▾';
        } else {
          feedsDiv.style.display = 'none';
          toggle.textContent = '▸';
        }
      });
    });

    // Category title click → show all articles in category
    feedsList.querySelectorAll('.cat-header').forEach(el => {
      el.addEventListener('click', (e) => {
        e.preventDefault();
        const catId = el.dataset.catId;
        navigateTo('category', null, parseInt(catId));
      });
    });

    // Category mark-all-read click
    feedsList.querySelectorAll('.cat-mark-read').forEach(el => {
      el.addEventListener('click', async (e) => {
        e.stopPropagation();
        const catId = el.dataset.markCat;
        const catName = catId === '0' ? 'Uncategorized' : el.closest('.category-header').querySelector('.cat-header').textContent;
        if (!await showConfirm(`Mark all articles in "${catName}" as read?`, 'Mark as Read')) return;
        try {
          await fetch(`/api/articles/mark-all-read?category_id=${catId}`, { method: 'POST' });
          await loadArticles();
          await updateCounts();
        } catch (err) {
          console.error('Mark category read failed:', err);
        }
      });
    });

    // Feed click handlers
    feedsList.querySelectorAll('.nav-item[data-feed-id]').forEach(el => {
      el.addEventListener('click', (e) => {
        e.preventDefault();
        navigateTo('feed', el.dataset.feedId);
      });
    });

    // Setup drag and drop
    setupDragDrop();
  }

  function feedItemHtml(f) {
    return `<a href="#" class="nav-item" data-feed-id="${f.id}" draggable="true" data-drag-feed="${f.id}">
      <span class="icon">📡</span>
      <span class="label">${escapeHtml(f.title || f.url)}</span>
      <span class="count" data-feed-count="${f.id}">0</span>
    </a>`;
  }

  // Build the API URL for the current view
  function buildArticlesUrl(limit, offset) {
    let url = `/api/articles?limit=${limit}&offset=${offset}`;
    if (currentView === 'fresh') url += '&view=unread';
    else if (currentView === 'starred') url += '&view=starred';
    else if (currentCategoryId !== null) url += `&category_id=${currentCategoryId}&view=unread`;
    else if (currentFeedId) url += `&feed_id=${currentFeedId}`;
    return url;
  }

  // Load articles (initial load, resets pagination)
  async function loadArticles() {
    articlesList.innerHTML = '<div class="loading">Loading...</div>';
    articlesOffset = 0;
    articlesExhausted = false;

    try {
      const res = await fetch(buildArticlesUrl(ARTICLES_PAGE_SIZE, 0));
      articles = await res.json();
      articlesOffset = articles.length;
      if (articles.length < ARTICLES_PAGE_SIZE) articlesExhausted = true;
      renderArticles();
    } catch (e) {
      articlesList.innerHTML = '<div class="loading">Failed to load articles</div>';
    }
  }

  // Load more articles (infinite scroll)
  async function loadMoreArticles() {
    if (articlesLoading || articlesExhausted) return;
    articlesLoading = true;

    // Show loading indicator at bottom
    const spacer = articlesList.querySelector('.scroll-spacer');
    const loader = document.createElement('div');
    loader.className = 'loading-more';
    loader.textContent = 'Loading more...';
    if (spacer) articlesList.insertBefore(loader, spacer);
    else articlesList.appendChild(loader);

    try {
      const res = await fetch(buildArticlesUrl(ARTICLES_PAGE_SIZE, articlesOffset));
      const newArticles = await res.json();

      if (newArticles.length === 0) {
        articlesExhausted = true;
      } else {
        // Append new articles, avoiding duplicates
        const existingIds = new Set(articles.map(a => a.id));
        const unique = newArticles.filter(a => !existingIds.has(a.id));
        articles = articles.concat(unique);
        articlesOffset += newArticles.length;
        if (newArticles.length < ARTICLES_PAGE_SIZE) articlesExhausted = true;

        // Append new article elements to the DOM (before spacer)
        const fragment = document.createDocumentFragment();
        unique.forEach((a, idx) => {
          const i = articles.length - unique.length + idx;
          const el = document.createElement('article');
          el.className = `article${a.is_read ? '' : ' unread'}`;
          el.dataset.index = i;
          el.dataset.id = a.id;
          el.innerHTML = `
            <div class="article-header">
              <div class="article-meta">
                <span class="article-feed">${escapeHtml(a.feed_title || '')}</span>
                <span class="article-time">${formatTime(a.published_at)}</span>
                ${a.author ? `<span class="article-author">by ${escapeHtml(a.author)}</span>` : ''}
                <span class="article-star${a.is_starred ? ' starred' : ''}" data-star="${a.id}">${a.is_starred ? '★' : '☆'}</span>
              </div>
              <div class="article-title">${escapeHtml(a.title)}</div>
            </div>
            <div class="article-content" id="content-${a.id}"></div>
            <div class="article-actions">
              <button class="article-btn" data-action="star" data-id="${a.id}">${a.is_starred ? '★ Unstar' : '☆ Star'}</button>
              <button class="article-btn" data-action="read" data-id="${a.id}">${a.is_read ? '● Read' : '○ Unread'}</button>
              <a class="article-btn" href="${escapeHtml(a.url)}" target="_blank" rel="noopener noreferrer">↗ Open</a>
            </div>`;
          fragment.appendChild(el);
        });
        if (spacer) articlesList.insertBefore(fragment, spacer);
        else articlesList.appendChild(fragment);
      }
    } catch (e) {
      console.error('Failed to load more articles:', e);
    } finally {
      loader.remove();
      articlesLoading = false;
    }
  }

  function renderArticles() {
    if (!articles.length) {
      articlesList.innerHTML = '<div class="loading">No articles</div>';
      return;
    }

    // Build articles without content first (content may have unclosed HTML
    // that breaks the DOM), then inject content safely via iframe sandboxing
    articlesList.innerHTML = articles.map((a, i) => `
      <article class="article${a.is_read ? '' : ' unread'}${i === selectedIndex ? ' selected' : ''}" data-index="${i}" data-id="${a.id}">
        <div class="article-header">
          <div class="article-meta">
            <span class="article-feed">${escapeHtml(a.feed_title || '')}</span>
            <span class="article-time">${formatTime(a.published_at)}</span>
            ${a.author ? `<span class="article-author">by ${escapeHtml(a.author)}</span>` : ''}
            <span class="article-star${a.is_starred ? ' starred' : ''}" data-star="${a.id}">${a.is_starred ? '★' : '☆'}</span>
          </div>
          <div class="article-title">${escapeHtml(a.title)}</div>
        </div>
        <div class="article-content" id="content-${a.id}"></div>
        <div class="article-actions">
          <button class="article-btn" data-action="star" data-id="${a.id}">${a.is_starred ? '★ Unstar' : '☆ Star'}</button>
          <button class="article-btn" data-action="read" data-id="${a.id}">${a.is_read ? '● Read' : '○ Unread'}</button>
          <a class="article-btn" href="${escapeHtml(a.url)}" target="_blank" rel="noopener noreferrer">↗ Open</a>
        </div>
      </article>
    `).join('');

    // Add scroll spacer so last article can be scrolled to top (for scroll-mark-as-read)
    const spacer = document.createElement('div');
    spacer.className = 'scroll-spacer';
    spacer.style.height = Math.max(0, articlesList.clientHeight - 80) + 'px';
    articlesList.appendChild(spacer);

    // Event handlers
    articlesList.querySelectorAll('.article-header').forEach(el => {
      el.addEventListener('click', async (e) => {
        if (e.target.classList.contains('article-star')) return;
        const article = el.closest('.article');
        const index = parseInt(article.dataset.index);
        selectArticle(index);
        article.classList.toggle('expanded');

        // Lazy-load content on first expand
        const contentEl = article.querySelector('.article-content');
        if (article.classList.contains('expanded') && !contentEl.dataset.loaded) {
          contentEl.innerHTML = '<div class="loading">Loading...</div>';
          try {
            const id = parseInt(article.dataset.id);
            const res = await fetch(`/api/articles/${id}`);
            const data = await res.json();
            const doc = new DOMParser().parseFromString(data.content || data.summary || '', 'text/html');
            contentEl.innerHTML = doc.body.innerHTML;
            contentEl.querySelectorAll('a').forEach(link => {
              link.setAttribute('target', '_blank');
              link.setAttribute('rel', 'noopener noreferrer');
            });
          } catch {
            contentEl.innerHTML = '<div class="loading">Failed to load content</div>';
          }
          contentEl.dataset.loaded = '1';
        }
      });
    });

    articlesList.querySelectorAll('.article-star').forEach(el => {
      el.addEventListener('click', async (e) => {
        e.stopPropagation();
        const id = parseInt(el.dataset.star);
        const index = articles.findIndex(a => a.id === id);
        if (index >= 0) await toggleStarAt(index);
      });
    });

    articlesList.querySelectorAll('.article-btn').forEach(btn => {
      btn.addEventListener('click', async (e) => {
        const action = btn.dataset.action;
        const id = parseInt(btn.dataset.id);
        const index = articles.findIndex(a => a.id === id);

        if (action === 'star' && index >= 0) await toggleStarAt(index);
        if (action === 'read' && index >= 0) await toggleReadAt(index);
      });
    });
  }

  // Select article
  async function selectArticle(index) {
    if (index < 0 || index >= articles.length) return;
    selectedIndex = index;

    articlesList.querySelectorAll('.article').forEach((el, i) => {
      el.classList.toggle('selected', i === index);
    });

    const el = articlesList.querySelector(`[data-index="${index}"]`);
    if (el) el.scrollIntoView({ block: 'nearest' });

    // Mark as read
    const article = articles[index];
    if (article && !article.is_read) {
      article.is_read = 1;
      el.classList.remove('unread');
      const btn = el.querySelector('[data-action="read"]');
      if (btn) btn.textContent = '● Read';
      await markRead(article.id);
      await updateCounts();
    }
  }

  // API calls
  async function markRead(id) {
    await fetch(`/api/articles/${id}/read`, { method: 'POST' });
  }

  async function markUnread(id) {
    await fetch(`/api/articles/${id}/unread`, { method: 'POST' });
  }

  async function starArticle(id) {
    await fetch(`/api/articles/${id}/star`, { method: 'POST' });
  }

  async function unstarArticle(id) {
    await fetch(`/api/articles/${id}/unstar`, { method: 'POST' });
  }

  // Toggle functions
  async function toggleStar() {
    if (selectedIndex >= 0) await toggleStarAt(selectedIndex);
  }

  async function toggleStarAt(index) {
    const article = articles[index];
    if (!article) return;

    if (article.is_starred) {
      await unstarArticle(article.id);
      article.is_starred = 0;
    } else {
      await starArticle(article.id);
      article.is_starred = 1;
    }

    const el = articlesList.querySelector(`[data-index="${index}"]`);
    const star = el.querySelector('.article-star');
    star.classList.toggle('starred', article.is_starred);
    star.textContent = article.is_starred ? '★' : '☆';

    const btn = el.querySelector('[data-action="star"]');
    if (btn) btn.textContent = article.is_starred ? '★ Unstar' : '☆ Star';

    await updateCounts();
  }

  async function toggleRead() {
    if (selectedIndex >= 0) await toggleReadAt(selectedIndex);
  }

  async function toggleReadAt(index) {
    const article = articles[index];
    if (!article) return;

    const el = articlesList.querySelector(`[data-index="${index}"]`);

    if (article.is_read) {
      await markUnread(article.id);
      article.is_read = 0;
      el.classList.add('unread');
    } else {
      await markRead(article.id);
      article.is_read = 1;
      el.classList.remove('unread');
    }

    const btn = el.querySelector('[data-action="read"]');
    if (btn) btn.textContent = article.is_read ? '● Read' : '○ Unread';

    await updateCounts();
  }

  // Scroll mark-as-read
  async function handleScrollMarkRead() {
    const listRect = articlesList.getBoundingClientRect();
    const idsToMark = [];

    articlesList.querySelectorAll('.article.unread').forEach(el => {
      const rect = el.getBoundingClientRect();
      if (rect.top < listRect.top) {
        const id = parseInt(el.dataset.id);
        const index = parseInt(el.dataset.index);
        if (articles[index] && !articles[index].is_read) {
          articles[index].is_read = 1;
          el.classList.remove('unread');
          const btn = el.querySelector('[data-action="read"]');
          if (btn) btn.textContent = '● Read';
          idsToMark.push(id);
        }
      }
    });

    if (idsToMark.length > 0) {
      await fetch('/api/articles/mark-read-batch', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ids: idsToMark })
      });
      await updateCounts();
    }
  }

  // Update counts
  async function updateCounts() {
    try {
      const res = await fetch('/api/counts');
      const data = await res.json();

      // Detect new articles by comparing with last known counts
      const prevTotal = lastKnownCounts.total;
      const newTotal = data.total || 0;
      const prevUnread = lastKnownCounts.unread;
      const newUnread = data.unread || 0;

      // Check per-feed count increases for the current view
      let newArticleCount = 0;
      if (prevTotal > 0) { // Skip first load
        if (currentFeedId && data.feeds) {
          const prev = lastKnownCounts.feeds[String(currentFeedId)] || 0;
          const now = data.feeds[String(currentFeedId)] || 0;
          if (now > prev) newArticleCount = now - prev;
        } else if (currentView === 'fresh' && newUnread > prevUnread) {
          newArticleCount = newUnread - prevUnread;
        } else if (currentView === 'all' && newTotal > prevTotal) {
          newArticleCount = newTotal - prevTotal;
        } else if (currentCategoryId !== null && data.feeds) {
          // Sum feeds in this category
          let prevCatTotal = 0, nowCatTotal = 0;
          feeds.filter(f => (f.category_id || 0) == currentCategoryId).forEach(f => {
            const id = String(f.id);
            prevCatTotal += lastKnownCounts.feeds[id] || 0;
            nowCatTotal += data.feeds[id] || 0;
          });
          if (nowCatTotal > prevCatTotal) newArticleCount = nowCatTotal - prevCatTotal;
        }
      }

      // Save current counts
      lastKnownCounts = {
        total: newTotal,
        unread: newUnread,
        starred: data.starred || 0,
        feeds: data.feeds ? { ...data.feeds } : {}
      };

      // Show banner if there are new articles
      if (newArticleCount > 0) showNewArticlesBanner(newArticleCount);

      document.getElementById('count-all').textContent = data.total || 0;
      document.getElementById('count-fresh').textContent = data.unread || 0;
      document.getElementById('count-starred').textContent = data.starred || 0;

      // Update feed counts and category totals
      if (data.feeds) {
        const catTotals = new Map();

        // Build a set of feed IDs that have unread articles
        const unreadFeedIds = new Set(Object.keys(data.feeds).map(String));

        // Update each feed's count badge and visibility
        feeds.forEach(f => {
          const id = String(f.id);
          const count = data.feeds[id] || 0;
          const el = document.querySelector(`[data-feed-count="${id}"]`);
          if (el) el.textContent = count;

          // Accumulate category totals
          const catId = f.category_id || 0;
          catTotals.set(catId, (catTotals.get(catId) || 0) + count);
        });

        // Update category counts
        feedsList.querySelectorAll('.feed-category').forEach(catEl => {
          const catId = parseInt(catEl.querySelector('.category-header')?.dataset.catId || '0');
          const total = catTotals.get(catId) || 0;
          const countEl = document.querySelector(`[data-cat-count="${catId}"]`);
          if (countEl) countEl.textContent = total;
        });
      }

      // Refresh header title after all counts are updated
      // Skip if new-articles badge is showing — the count in the title
      // would already include the new articles and look confusing next
      // to the "+N new" badge.  Title updates when the badge is clicked.
      if (pendingNewCount === 0) {
        updateViewTitle();
      }

      // Update browser tab title with unread count
      document.title = newUnread > 0 ? `GoRSS (${newUnread})` : 'GoRSS';
    } catch (e) {
      console.error('Failed to update counts:', e);
    }
  }

  // ── New Articles Badge (header) ────────────────────────────────
  const newBadge = document.getElementById('new-badge');
  let pendingNewCount = 0;

  newBadge.addEventListener('click', async () => {
    pendingNewCount = 0;
    newBadge.style.display = 'none';
    updateViewTitle();
    await loadArticles();
    articlesList.scrollTo({ top: 0, behavior: 'smooth' });
  });

  function showNewArticlesBanner(count) {
    pendingNewCount += count;
    newBadge.textContent = ` · +${pendingNewCount} new`;
    newBadge.title = 'Click to refresh';
    newBadge.style.display = 'inline';
  }

  function dismissNewArticlesBanner() {
    pendingNewCount = 0;
    newBadge.style.display = 'none';
  }

  // Poll for new articles every 30 seconds
  setInterval(() => updateCounts(), 30000);

  // Handlers
  async function handleAddFeed(e) {
    e.preventDefault();
    const form = e.target;
    const url = form.url.value.trim();
    if (!url) return;

    const btn = form.querySelector('button[type="submit"]');
    btn.disabled = true;
    btn.textContent = 'Subscribing...';

    try {
      const res = await fetch('/api/feeds', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ url })
      });

      if (res.ok) {
        form.reset();
        document.getElementById('modal-add-feed').classList.remove('open');
        await loadFeeds();
        await loadArticles();
        await updateCounts();
      } else {
        const err = await res.json();
        alert(err.error || 'Failed to subscribe');
      }
    } catch (e) {
      alert('Failed to subscribe: ' + e.message);
    } finally {
      btn.disabled = false;
      btn.textContent = 'Subscribe';
    }
  }

  async function handleImport(e) {
    e.preventDefault();
    const form = e.target;
    const file = form.file.files[0];
    if (!file) return;

    const btn = form.querySelector('button[type="submit"]');
    const result = document.getElementById('import-result');
    btn.disabled = true;
    btn.textContent = 'Importing...';
    result.textContent = '';

    try {
      const formData = new FormData();
      formData.append('file', file);

      const res = await fetch('/api/opml/import', {
        method: 'POST',
        body: formData
      });

      const data = await res.json();
      result.textContent = `Imported ${data.imported} feeds, skipped ${data.skipped}`;

      await loadFeeds();
      await loadArticles();
      await updateCounts();
    } catch (e) {
      result.textContent = 'Import failed: ' + e.message;
    } finally {
      btn.disabled = false;
      btn.textContent = 'Import';
    }
  }

  async function handleRefresh() {
    const btn = document.getElementById('btn-refresh');
    const fab = document.getElementById('btn-fab-refresh');
    if (btn) btn.textContent = '⏳';
    if (fab) fab.textContent = '⏳';

    try {
      await fetch('/api/feeds/refresh', { method: 'POST' });
      await loadArticles();
      await updateCounts();
    } catch (e) {
      console.error('Refresh failed:', e);
    } finally {
      if (btn) btn.textContent = '🔄';
      if (fab) fab.textContent = '🔄';
    }
  }

  async function handleMarkAllRead() {
    if (!await showConfirm('Mark all articles as read?', 'Mark as Read')) return;

    try {
      let url = '/api/articles/mark-all-read';
      if (currentFeedId) url += `?feed_id=${currentFeedId}`;
      else if (currentCategoryId !== null) url += `?category_id=${currentCategoryId}`;
      await fetch(url, { method: 'POST' });
      await loadArticles();
      await updateCounts();
    } catch (e) {
      console.error('Mark all read failed:', e);
    }
  }

  // Utilities
  // Drag and drop for reordering categories and feeds
  function setupDragDrop() {
    let dragItem = null;
    let dragType = null; // 'category' or 'feed'

    // Category drag
    feedsList.querySelectorAll('[data-drag-cat]').forEach(el => {
      el.addEventListener('dragstart', (e) => {
        dragItem = el;
        dragType = 'category';
        el.classList.add('dragging');
        e.dataTransfer.effectAllowed = 'move';
        e.dataTransfer.setData('text/plain', el.dataset.dragCat);
      });

      el.addEventListener('dragend', () => {
        el.classList.remove('dragging');
        feedsList.querySelectorAll('.drag-over').forEach(d => d.classList.remove('drag-over'));
        dragItem = null;
        dragType = null;
      });

      el.addEventListener('dragover', (e) => {
        e.preventDefault();
        if (dragType === 'category' && el !== dragItem) {
          el.classList.add('drag-over');
        }
        if (dragType === 'feed') {
          el.classList.add('drag-over');
        }
      });

      el.addEventListener('dragleave', () => {
        el.classList.remove('drag-over');
      });

      el.addEventListener('drop', async (e) => {
        e.preventDefault();
        el.classList.remove('drag-over');

        if (dragType === 'category' && dragItem !== el) {
          // Reorder categories
          const parent = el.parentNode;
          const items = [...parent.querySelectorAll('[data-drag-cat]')];
          const dragIdx = items.indexOf(dragItem);
          const dropIdx = items.indexOf(el);
          if (dragIdx < dropIdx) {
            parent.insertBefore(dragItem, el.nextSibling);
          } else {
            parent.insertBefore(dragItem, el);
          }
          // Save new order
          const newOrder = [...parent.querySelectorAll('[data-drag-cat]')].map((item, i) => ({
            id: parseInt(item.dataset.dragCat),
            order: i
          })).filter(item => item.id > 0);
          await fetch('/api/categories/reorder', {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(newOrder)
          });
        }

        if (dragType === 'feed') {
          // Move feed to this category
          const feedId = parseInt(dragItem.dataset.dragFeed);
          const targetCatId = parseInt(el.dataset.dragCat);
          const feedsDiv = el.querySelector('[data-cat-feeds]');
          if (feedsDiv && dragItem.parentNode !== feedsDiv) {
            feedsDiv.appendChild(dragItem);
            feedsDiv.style.display = 'block';
            el.querySelector('.category-toggle').textContent = '▾';
            // Save feed move
            const feedItems = [...feedsDiv.querySelectorAll('[data-drag-feed]')].map((item, i) => ({
              id: parseInt(item.dataset.dragFeed),
              order: i,
              category_id: targetCatId || null
            }));
            await fetch('/api/feeds/reorder', {
              method: 'PUT',
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify(feedItems)
            });
            await updateCounts();
          }
        }
      });
    });

    // Feed drag
    feedsList.querySelectorAll('[data-drag-feed]').forEach(el => {
      el.addEventListener('dragstart', (e) => {
        e.stopPropagation();
        dragItem = el;
        dragType = 'feed';
        el.classList.add('dragging');
        e.dataTransfer.effectAllowed = 'move';
        e.dataTransfer.setData('text/plain', el.dataset.dragFeed);
      });

      el.addEventListener('dragend', () => {
        el.classList.remove('dragging');
        feedsList.querySelectorAll('.drag-over').forEach(d => d.classList.remove('drag-over'));
        dragItem = null;
        dragType = null;
      });

      el.addEventListener('dragover', (e) => {
        e.preventDefault();
        if (dragType === 'feed' && el !== dragItem) {
          el.classList.add('drag-over');
        }
      });

      el.addEventListener('dragleave', () => {
        el.classList.remove('drag-over');
      });

      el.addEventListener('drop', async (e) => {
        e.preventDefault();
        e.stopPropagation();
        el.classList.remove('drag-over');

        if (dragType === 'feed' && dragItem !== el) {
          // Reorder within same category
          const parent = el.parentNode;
          const items = [...parent.querySelectorAll('[data-drag-feed]')];
          const dragIdx = items.indexOf(dragItem);
          const dropIdx = items.indexOf(el);
          if (dragIdx < dropIdx) {
            parent.insertBefore(dragItem, el.nextSibling);
          } else {
            parent.insertBefore(dragItem, el);
          }
          // Save new order
          const catEl = parent.closest('[data-drag-cat]');
          const catId = catEl ? parseInt(catEl.dataset.dragCat) : null;
          const feedItems = [...parent.querySelectorAll('[data-drag-feed]')].map((item, i) => ({
            id: parseInt(item.dataset.dragFeed),
            order: i,
            category_id: catId || null
          }));
          await fetch('/api/feeds/reorder', {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(feedItems)
          });
        }
      });
    });
  }

  function escapeHtml(str) {
    if (!str) return '';
    return str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
  }

  function formatTime(dateStr) {
    if (!dateStr) return '';
    const date = new Date(dateStr);
    const now = new Date();
    const diff = (now - date) / 1000;

    if (diff < 60) return 'Just now';
    if (diff < 3600) return Math.floor(diff / 60) + 'm ago';
    if (diff < 86400) return Math.floor(diff / 3600) + 'h ago';
    if (diff < 604800) return Math.floor(diff / 86400) + 'd ago';
    return date.toLocaleDateString();
  }
})();
