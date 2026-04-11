const API_BASE = '';

async function api(path, opts = {}) {
  const res = await fetch(API_BASE + path, opts);
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}

async function loadDashboard() {
  try {
    const stats = await api('/admin/stats');
    renderStats(stats);
  } catch (e) {
    console.error('Failed to load stats:', e);
  }
}

async function issueToken(e) {
  e.preventDefault();
  const fd = new FormData(e.target);
  const body = Object.fromEntries(fd);
  try {
    const result = await api('/admin/issue-token', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify(body),
    });
    document.getElementById('token-result').textContent = result.token;
  } catch (err) {
    document.getElementById('token-result').textContent = 'Error: ' + err.message;
  }
}

async function refreshAll() {
  await loadDashboard();
}

function renderStats(stats) {
  const grid = document.getElementById('stats-grid');
  if (!grid) return;
  grid.innerHTML = Object.entries(stats).map(([name, s]) => `
    <div class="stat-card">
      <div class="label">${name}</div>
      <div class="value" style="color:${s.is_healthy ? 'var(--green)' : 'var(--red)'}">${(s.health_score * 100).toFixed(0)}%</div>
      <div class="sub">${s.total_success} 成功 / ${s.total_fail} 失败 | ${s.avg_latency_ms}ms</div>
    </div>
  `).join('');
}

// Page routing
document.querySelectorAll('.nav-item').forEach(el => {
  el.addEventListener('click', e => {
    e.preventDefault();
    const page = el.dataset.page;
    document.querySelectorAll('.page').forEach(p => p.classList.remove('active'));
    document.querySelectorAll('.nav-item').forEach(n => n.classList.remove('active'));
    document.getElementById('page-' + page)?.classList.add('active');
    el.classList.add('active');
    if (page === 'dashboard') loadDashboard();
  });
});

loadDashboard();
