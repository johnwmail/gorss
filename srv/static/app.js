// GoRSS Client Application
(function() {
  'use strict';

  // State
  let currentView = 'all';
  let currentFeedId = null;
  let articles = [];
  let selectedIndex = -1;
  let gKeyPressed = false;

  // DOM Elements
  const articlesList = document.getElementById('articles-list');
  const feedsList = document.getElementById('feeds-list');
  const currentViewEl = document.getElementById('current-view');
  const modalAddFeed = document.getElementById('modal-add-feed');
  const modalHelp = document.getElementById('modal-help');

  // Initialize
  document.addEventListener('DOMContentLoaded', init);

  function init() {
    loadArticles();
    setupEventListeners();
    setupKeyboardNav();
    updateCounts();
  }

  // Event Listeners
  function setupEventListeners() {
    // Navigation items
    document.querySelectorAll('.nav-item[data-view]').forEach(el => {
      el.addEventListener('click', (e) => {
        e.preventDefault();
        setActiveNav(el);
        currentView = el.dataset.view;
        currentFeedId = null;
        updateViewTitle();
        loadArticles();
      });
    });

    // Feed items
    document.querySelectorAll('.nav-item[data-feed-id]').forEach(el => {
      el.addEventListener('click', (e) => {
        e.preventDefault();
        setActiveNav(el);
        currentView = 'feed';
        currentFeedId = el.dataset.feedId;
        currentViewEl.textContent = el.querySelector('.label').textContent;
        loadArticles();
      });
    });

    // Add feed button
    document.getElementById('btn-add-feed').addEventListener('click', () => {
      modalAddFeed.classList.add('open');
      modalAddFeed.querySelector('input').focus();
    });

    // Add feed form
    document.getElementById('form-add-feed').addEventListener('submit', handleAddFeed);

    // Import OPML button
    document.getElementById('btn-import').addEventListener('click', () => {
      document.getElementById('modal-import').classList.add('open');
    });

    // Export OPML button
    document.getElementById('btn-export').addEventListener('click', () => {
      window.location.href = '/api/opml/export';
    });

    // Import form
    document.getElementById('form-import').addEventListener('submit', handleImport);

    // Refresh button
    document.getElementById('btn-refresh').addEventListener('click', handleRefresh);

    // Mark all read
    document.getElementById('btn-mark-all-read').addEventListener('click', handleMarkAllRead);

    // Mark as read on scroll (TT-RSS style)
    const articleList = document.getElementById('articles-list');
    if (articleList) {
      articleList.addEventListener('scroll', handleScrollMarkRead);
    }

    // Close modals
    document.querySelectorAll('[data-close-modal]').forEach(el => {
      el.addEventListener('click', () => {
        document.querySelectorAll('.modal').forEach(m => m.classList.remove('open'));
      });
    });

    // Close modal on backdrop click
    document.querySelectorAll('.modal').forEach(modal => {
      modal.addEventListener('click', (e) => {
        if (e.target === modal) modal.classList.remove('open');
      });
    });
  }

  // Keyboard Navigation
  function setupKeyboardNav() {
    document.addEventListener('keydown', (e) => {
      // Ignore if typing in input
      if (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA') return;

      // Handle 'g' prefix commands
      if (gKeyPressed) {
        gKeyPressed = false;
        switch (e.key) {
          case 'a': navigateTo('all'); break;
          case 'f': navigateTo('fresh'); break;
          case 's': navigateTo('starred'); break;
        }
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
        case 'g':
          gKeyPressed = true;
          setTimeout(() => gKeyPressed = false, 1000);
          break;
        case '?':
          modalHelp.classList.add('open');
          break;
        case 'Escape':
          document.querySelectorAll('.modal').forEach(m => m.classList.remove('open'));
          break;
      }
    });
  }

  function navigateTo(view) {
    const el = document.querySelector(`.nav-item[data-view="${view}"]`);
    if (el) el.click();
  }

  function setActiveNav(el) {
    document.querySelectorAll('.nav-item').forEach(n => n.classList.remove('active'));
    el.classList.add('active');
  }

  function updateViewTitle() {
    const titles = {
      'all': 'All Articles',
      'fresh': 'Fresh',
      'starred': 'Starred'
    };
    currentViewEl.textContent = titles[currentView] || 'Articles';
  }

  // API Functions
  async function loadArticles() {
    articlesList.innerHTML = '<div class="loading">Loading...</div>';
    selectedIndex = -1;

    let url = '/api/articles?limit=100';
    if (currentView === 'fresh') url += '&view=unread';
    else if (currentView === 'starred') url += '&view=starred';
    else if (currentFeedId) url += '&feed_id=' + currentFeedId;

    try {
      const res = await fetch(url);
      articles = await res.json();
      renderArticles();
    } catch (err) {
      articlesList.innerHTML = '<div class="loading">Error loading articles</div>';
    }
  }

  function renderArticles() {
    if (!articles || articles.length === 0) {
      articlesList.innerHTML = '<div class="loading">No articles</div>';
      return;
    }

    articlesList.innerHTML = articles.map((a, i) => `
      <article class="article${a.is_read ? '' : ' unread'}${i === selectedIndex ? ' selected' : ''}" data-index="${i}" data-id="${a.id}">
        <div class="article-header">
          <span class="article-star${a.is_starred ? ' starred' : ''}" data-action="star">${a.is_starred ? '★' : '☆'}</span>
          <div class="article-meta">
            <div class="article-title"><a href="${escapeHtml(a.url)}" target="_blank">${escapeHtml(a.title || 'Untitled')}</a></div>
            <div class="article-info">
              <span class="article-feed">${escapeHtml(a.feed_title || 'Unknown')}</span>
              ${a.author ? `<span class="article-author">${escapeHtml(a.author)}</span>` : ''}
              <span class="article-time">${formatTime(a.published_at)}</span>
            </div>
          </div>
        </div>
        <div class="article-content">${a.content || a.summary || ''}</div>
      </article>
    `).join('');

    // Add click handlers
    articlesList.querySelectorAll('.article').forEach(el => {
      el.addEventListener('click', (e) => {
        if (e.target.dataset.action === 'star') {
          toggleStarAt(parseInt(el.dataset.index));
        } else {
          selectArticle(parseInt(el.dataset.index));
        }
      });
    });
  }

  async function selectArticle(index) {
    if (index < 0 || index >= articles.length) return;
    selectedIndex = index;

    // Update UI
    articlesList.querySelectorAll('.article').forEach((el, i) => {
      el.classList.toggle('selected', i === index);
    });

    // Scroll into view
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

  async function markRead(id) {
    await fetch(`/api/articles/${id}/read`, { method: 'POST' });
  }

  // Mark articles as read when scrolled past (TT-RSS style)
  let scrollMarkReadTimeout = null;
  function handleScrollMarkRead() {
    if (scrollMarkReadTimeout) clearTimeout(scrollMarkReadTimeout);
    scrollMarkReadTimeout = setTimeout(async () => {
      const articleList = document.getElementById('articles-list');
      if (!articleList) return;
      
      const articleElements = articleList.querySelectorAll('.article.unread');
      const listRect = articleList.getBoundingClientRect();
      const markPromises = [];
      
      articleElements.forEach((el) => {
        const rect = el.getBoundingClientRect();
        // If article is scrolled above the visible area (user has scrolled past it)
        if (rect.bottom < listRect.top + 50) {
          const articleId = parseInt(el.dataset.id);
          const index = parseInt(el.dataset.index);
          if (articleId) {
            markPromises.push(markRead(articleId));
            el.classList.remove('unread');
            // Update local state
            if (articles[index]) {
              articles[index].is_read = 1;
            }
          }
        }
      });
      
      if (markPromises.length > 0) {
        await Promise.all(markPromises);
        await updateCounts();
      }
    }, 300); // Debounce 300ms
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
    await updateCounts();
  }

  async function toggleRead() {
    if (selectedIndex < 0) return;
    const article = articles[selectedIndex];
    if (!article) return;

    const el = articlesList.querySelector(`[data-index="${selectedIndex}"]`);
    if (article.is_read) {
      await markUnread(article.id);
      article.is_read = 0;
      el.classList.add('unread');
    } else {
      await markRead(article.id);
      article.is_read = 1;
      el.classList.remove('unread');
    }
    await updateCounts();
  }

  async function updateCounts() {
    try {
      const res = await fetch('/api/counts');
      const data = await res.json();
      document.getElementById('count-all').textContent = data.total || 0;
      document.getElementById('count-fresh').textContent = data.unread || 0;
      document.getElementById('count-starred').textContent = data.starred || 0;
      
      // Update per-feed counts
      if (data.feeds) {
        document.querySelectorAll('[data-feed-id]').forEach(el => {
          const feedId = el.dataset.feedId;
          const countEl = el.querySelector('.count');
          const count = data.feeds[feedId] || 0;
          if (count > 0) {
            if (countEl) {
              countEl.textContent = count;
            } else {
              const span = document.createElement('span');
              span.className = 'count';
              span.textContent = count;
              el.appendChild(span);
            }
          } else if (countEl) {
            countEl.remove();
          }
        });
      }
    } catch (e) {}
  }

  async function handleImport(e) {
    e.preventDefault();
    const form = e.target;
    const fileInput = form.querySelector('input[type="file"]');
    if (!fileInput.files.length) return;

    const btn = form.querySelector('button[type="submit"]');
    btn.disabled = true;
    btn.textContent = 'Importing...';

    const formData = new FormData();
    formData.append('file', fileInput.files[0]);

    try {
      const res = await fetch('/api/opml/import', {
        method: 'POST',
        body: formData
      });

      const data = await res.json();
      if (!res.ok) {
        alert(data.error || 'Failed to import');
        return;
      }

      const resultDiv = document.getElementById('import-result');
      resultDiv.style.display = 'block';
      resultDiv.textContent = `Imported ${data.imported} feeds, skipped ${data.skipped} (total: ${data.total})`;

      // Reload after 2 seconds
      setTimeout(() => window.location.reload(), 2000);
    } catch (err) {
      alert('Error: ' + err.message);
    } finally {
      btn.disabled = false;
      btn.textContent = 'Import';
    }
  }

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

      if (!res.ok) {
        const data = await res.json();
        alert(data.error || 'Failed to subscribe');
        return;
      }

      // Reload page to show new feed
      window.location.reload();
    } catch (err) {
      alert('Error: ' + err.message);
    } finally {
      btn.disabled = false;
      btn.textContent = 'Subscribe';
    }
  }

  async function handleRefresh() {
    const btn = document.getElementById('btn-refresh');
    btn.disabled = true;
    btn.textContent = '⏳';

    try {
      await fetch('/api/refresh', { method: 'POST' });
      setTimeout(() => {
        loadArticles();
        updateCounts();
        btn.disabled = false;
        btn.textContent = '🔄';
      }, 2000);
    } catch (err) {
      btn.disabled = false;
      btn.textContent = '🔄';
    }
  }

  async function handleMarkAllRead() {
    if (!confirm('Mark all articles as read?')) return;
    // TODO: implement mark all read API
    loadArticles();
  }

  // Utilities
  function escapeHtml(str) {
    if (!str) return '';
    return str.replace(/[&<>"']/g, c => ({
      '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
    }[c]));
  }

  function formatTime(dateStr) {
    if (!dateStr) return '';
    const date = new Date(dateStr);
    const now = new Date();
    const diff = (now - date) / 1000;

    if (diff < 60) return 'just now';
    if (diff < 3600) return Math.floor(diff / 60) + ' min ago';
    if (diff < 86400) return Math.floor(diff / 3600) + ' hours ago';
    if (diff < 604800) return Math.floor(diff / 86400) + ' days ago';

    return date.toLocaleDateString();
  }
})();
