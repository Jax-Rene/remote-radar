const DEFAULT_SUMMARY_LIMIT = 120;
export const PAGE_SIZE = 20;

export function truncateText(text = '', maxLength = DEFAULT_SUMMARY_LIMIT) {
  const value = typeof text === 'string' ? text.trim() : '';
  if (!value) {
    return { text: '', tooltip: '' };
  }
  if (value.length <= maxLength) {
    return { text: value, tooltip: value };
  }
  const cutoff = Math.max(0, maxLength - 1);
  const clipped = value.slice(0, cutoff).trimEnd();
  return { text: `${clipped}…`, tooltip: value };
}

export function extractTags(tagMap) {
  if (!tagMap || typeof tagMap !== 'object') {
    return [];
  }
  const tags = [];
  for (const key of Object.keys(tagMap)) {
    if (tagMap[key]) {
      tags.push(key);
    }
  }
  return tags.sort((a, b) => a.localeCompare(b, 'zh-CN'));
}

export function renderJobRow(job) {
  const tr = document.createElement('tr');

  const titleCell = document.createElement('td');
  titleCell.textContent = job.title || '未命名';
  tr.appendChild(titleCell);

  const summaryCell = document.createElement('td');
  summaryCell.className = 'summary-cell';
  const raw = job.raw_attributes || {};
  const baseSummary = job.summary || raw.summary || raw.full_title || raw.excerpt || '';
  const summary = truncateText(baseSummary, 100);
  summaryCell.textContent = summary.text || '暂无描述';
  summaryCell.title = summary.tooltip || '';
  tr.appendChild(summaryCell);

  const publishedCell = document.createElement('td');
  publishedCell.className = 'published-cell';
  const published = formatPublishedAt(job.published_at);
  publishedCell.textContent = published || '未知';
  publishedCell.title = published || '';
  tr.appendChild(publishedCell);

  const tagsCell = document.createElement('td');
  tagsCell.className = 'tags-cell';
  const tagList = extractTags(job.tags);
  if (tagList.length === 0) {
    tagsCell.textContent = '—';
  } else {
    tagList.forEach((tag) => {
      const span = document.createElement('span');
      span.className = 'tag';
      span.textContent = tag;
      tagsCell.appendChild(span);
    });
  }
  tr.appendChild(tagsCell);

  const sourceCell = document.createElement('td');
  sourceCell.textContent = job.source || '';
  tr.appendChild(sourceCell);

  const linkCell = document.createElement('td');
  if (job.url) {
    const anchor = document.createElement('a');
    anchor.href = job.url;
    anchor.textContent = '查看';
    anchor.target = '_blank';
    anchor.rel = 'noopener noreferrer';
    linkCell.appendChild(anchor);
  } else {
    linkCell.textContent = '无链接';
  }
  tr.appendChild(linkCell);

  return tr;
}

export function formatPublishedAt(input) {
  if (!input) {
    return '';
  }
  const date = new Date(input);
  if (Number.isNaN(date.getTime())) {
    return '';
  }
  return date.toLocaleString('zh-CN', { hour12: false });
}

export async function fetchJobs(page = 1, limit = PAGE_SIZE) {
  const params = new URLSearchParams({ page: String(page), limit: String(limit) });
  const res = await fetch(`/api/jobs?${params.toString()}`);
  if (!res.ok) {
    throw new Error(`fetch jobs failed: ${res.status}`);
  }
  const hasMoreHeader = res.headers.get('x-has-more');
  const totalHeader = res.headers.get('x-total');
  const total = Number(totalHeader || '0');
  const hasMore = hasMoreHeader === 'true' || page * limit < total;
  const body = await res.json();
  return { jobs: body, hasMore, page, total };
}

export async function refreshJobs(tbody, options = {}) {
  const page = options.page ?? 1;
  const limit = options.limit ?? PAGE_SIZE;
  const { jobs, hasMore, total } = await fetchJobs(page, limit);
  tbody.innerHTML = '';
  jobs.forEach((job) => {
    tbody.appendChild(renderJobRow(job));
  });
  return { jobs, hasMore, page, limit, total };
}

export async function handleManualRefresh(button, reloadFn) {
  button.disabled = true;
  button.textContent = '刷新中...';
  try {
    await fetch('/api/refresh', { method: 'POST' });
    await reloadFn();
  } finally {
    button.disabled = false;
    button.textContent = 'Refresh';
  }
}

export function initJobsTable({ tableBodyId = 'jobs', refreshBtnId = 'refresh' } = {}) {
  const tbody = document.getElementById(tableBodyId);
  const refreshButton = document.getElementById(refreshBtnId);
  const prevBtn = document.getElementById('prev-page');
  const nextBtn = document.getElementById('next-page');
  const indicator = document.getElementById('page-indicator');
  const totalIndicator = document.getElementById('total-indicator');
  if (!tbody) {
    throw new Error('jobs table body not found');
  }
  if (!refreshButton) {
    throw new Error('refresh button not found');
  }
  if (!prevBtn || !nextBtn || !indicator || !totalIndicator) {
    throw new Error('pagination controls not found');
  }

  const state = { page: 1, limit: PAGE_SIZE, hasMore: false, total: 0 };

  function updatePaginationControls() {
    prevBtn.disabled = state.page <= 1;
    const totalPages = Math.max(1, Math.ceil(state.total / state.limit));
    nextBtn.disabled = state.page >= totalPages;
    indicator.textContent = `第 ${state.page} / ${totalPages} 页`;
    totalIndicator.textContent = `共 ${state.total} 条记录`;
  }

  async function loadPage(targetPage) {
    const { hasMore, total } = await refreshJobs(tbody, { page: targetPage, limit: state.limit });
    state.page = targetPage;
    state.hasMore = hasMore;
    state.total = total;
    updatePaginationControls();
  }

  refreshButton.addEventListener('click', () =>
    handleManualRefresh(refreshButton, () => loadPage(state.page)).catch((err) => console.error(err)),
  );

  prevBtn.addEventListener('click', () => {
    if (state.page > 1) {
      loadPage(state.page - 1).catch((err) => console.error(err));
    }
  });

  nextBtn.addEventListener('click', () => {
    const totalPages = Math.max(1, Math.ceil(state.total / state.limit));
    if (state.page < totalPages) {
      loadPage(state.page + 1).catch((err) => console.error(err));
    }
  });

  loadPage(state.page).catch((err) => console.error(err));
}

if (typeof window !== 'undefined') {
  window.addEventListener('DOMContentLoaded', () => {
    initJobsTable();
  });
}
