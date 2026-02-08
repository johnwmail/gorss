// GoRSS - Unified Application
(function() {
  'use strict';

  // State
  let currentView = 'fresh';
  let currentFeedId = null;
  let articles = [];
  let feeds = [];
  let selectedIndex = -1;
  let gKeyPressed = false;

  // DOM Elements
  const sidebar = document.getElementById('sidebar');
  const drawerOverlay = document.getElementById('drawer-overlay');
  const articlesList = document.getElementById('articles-list');
  const feedsList = document.getElementById('feeds-list');

  // Initialize
  document.addEventListener('DOMContentLoaded', init);

  async function init() {
    setupEventListeners();
    setupKeyboardNav();
    await loadFeeds();
    await updateCounts();
    navigateTo(currentView);
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

    // Close modals
    document.querySelectorAll('.btn-cancel').forEach(btn => {
      btn.addEventListener('click', () => {
        document.querySelectorAll('.modal').forEach(m => m.classList.remove('open'));
      });
    });

    // Scroll mark-as-read
    let scrollTimeout = null;
    articlesList?.addEventListener('scroll', () => {
      if (scrollTimeout) clearTimeout(scrollTimeout);
      scrollTimeout = setTimeout(handleScrollMarkRead, 300);
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
            window.open(articles[selectedIndex].url, '_blank');
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
  function navigateTo(view, feedId = null) {
    currentView = view;
    currentFeedId = feedId;
    selectedIndex = -1;

    // Update active state
    document.querySelectorAll('.nav-item').forEach(el => {
      el.classList.remove('active');
      if (el.dataset.view === view && !feedId) el.classList.add('active');
      if (feedId && el.dataset.feedId == feedId) el.classList.add('active');
    });

    // Update title
    const titles = { all: 'All Articles', fresh: 'Unread', starred: 'Starred' };
    let title = titles[view] || 'Articles';
    if (feedId) {
      const feed = feeds.find(f => f.id == feedId);
      title = feed ? feed.title : 'Feed';
    }
    document.getElementById('current-view').textContent = title;

    loadArticles();
  }

  // Load feeds
  async function loadFeeds() {
    try {
      const res = await fetch('/api/feeds');
      feeds = await res.json();
      renderFeeds();
    } catch (e) {
      console.error('Failed to load feeds:', e);
    }
  }

  function renderFeeds() {
    if (!feedsList) return;
    feedsList.innerHTML = feeds.map(f => `
      <a href="#" class="nav-item" data-feed-id="${f.id}">
        <span class="icon">📡</span>
        <span class="label">${escapeHtml(f.title || f.url)}</span>
        <span class="count" data-feed-count="${f.id}">0</span>
      </a>
    `).join('');

    feedsList.querySelectorAll('.nav-item').forEach(el => {
      el.addEventListener('click', (e) => {
        e.preventDefault();
        navigateTo('feed', el.dataset.feedId);
        closeSidebar();
      });
    });
  }

  // Load articles
  async function loadArticles() {
    articlesList.innerHTML = '<div class="loading">Loading...</div>';

    let url = '/api/articles?limit=100';
    if (currentView === 'fresh') url += '&view=unread';
    else if (currentView === 'starred') url += '&view=starred';
    else if (currentFeedId) url += `&feed_id=${currentFeedId}`;

    try {
      const res = await fetch(url);
      articles = await res.json();
      renderArticles();
    } catch (e) {
      articlesList.innerHTML = '<div class="loading">Failed to load articles</div>';
    }
  }

  function renderArticles() {
    if (!articles.length) {
      articlesList.innerHTML = '<div class="loading">No articles</div>';
      return;
    }

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
        <div class="article-content" id="content-${a.id}">${a.content || a.summary || ''}</div>
        <div class="article-actions">
          <button class="article-btn" data-action="star" data-id="${a.id}">${a.is_starred ? '★ Unstar' : '☆ Star'}</button>
          <button class="article-btn" data-action="read" data-id="${a.id}">${a.is_read ? '○ Unread' : '● Read'}</button>
          <button class="article-btn" data-action="open" data-url="${escapeHtml(a.url)}">↗ Open</button>
        </div>
      </article>
    `).join('');

    // Event handlers
    articlesList.querySelectorAll('.article-header').forEach(el => {
      el.addEventListener('click', (e) => {
        if (e.target.classList.contains('article-star')) return;
        const article = el.closest('.article');
        const index = parseInt(article.dataset.index);
        selectArticle(index);
        article.classList.toggle('expanded');
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
        if (action === 'open') window.open(btn.dataset.url, '_blank');
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
    if (btn) btn.textContent = article.is_read ? '○ Unread' : '● Read';

    await updateCounts();
  }

  // Scroll mark-as-read
  async function handleScrollMarkRead() {
    const listRect = articlesList.getBoundingClientRect();
    const promises = [];

    articlesList.querySelectorAll('.article.unread').forEach(el => {
      const rect = el.getBoundingClientRect();
      if (rect.bottom < listRect.top + 50) {
        const id = parseInt(el.dataset.id);
        const index = parseInt(el.dataset.index);
        if (articles[index] && !articles[index].is_read) {
          articles[index].is_read = 1;
          el.classList.remove('unread');
          promises.push(markRead(id));
        }
      }
    });

    if (promises.length > 0) {
      await Promise.all(promises);
      await updateCounts();
    }
  }

  // Update counts
  async function updateCounts() {
    try {
      const res = await fetch('/api/counts');
      const data = await res.json();

      document.getElementById('count-all').textContent = data.total || 0;
      document.getElementById('count-fresh').textContent = data.unread || 0;
      document.getElementById('count-starred').textContent = data.starred || 0;

      // Update feed counts
      if (data.feeds) {
        Object.entries(data.feeds).forEach(([id, count]) => {
          const el = document.querySelector(`[data-feed-count="${id}"]`);
          if (el) el.textContent = count;
        });
      }
    } catch (e) {
      console.error('Failed to update counts:', e);
    }
  }

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
    if (!confirm('Mark all articles as read?')) return;

    try {
      let url = '/api/articles/mark-all-read';
      if (currentFeedId) url += `?feed_id=${currentFeedId}`;
      await fetch(url, { method: 'POST' });
      await loadArticles();
      await updateCounts();
    } catch (e) {
      console.error('Mark all read failed:', e);
    }
  }

  // Utilities
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
