document.addEventListener('alpine:init', () => {
  Alpine.store('theme', {
    isDark: localStorage.getItem('teploy-theme') !== 'light',
    toggle() {
      this.isDark = !this.isDark;
      localStorage.setItem('teploy-theme', this.isDark ? 'dark' : 'light');
      document.documentElement.dataset.theme = this.isDark ? 'dark' : 'light';
    },
  });

  Alpine.store('api', {
    async get(path) {
      const res = await fetch('/api' + path);
      if (!res.ok) throw new Error(await res.text());
      return res.json();
    },
    async post(path, body) {
      const res = await fetch('/api' + path, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body || {}) });
      if (!res.ok) throw new Error(await res.text());
      return res.json();
    },
  });

  Alpine.store('toast', {
    items: [],
    show(msg, type = 'info') {
      const id = Date.now();
      this.items.push({ id, message: msg, type });
      setTimeout(() => this.items = this.items.filter(t => t.id !== id), 4000);
    },
    success(msg) { this.show(msg, 'success'); },
    error(msg) { this.show(msg, 'error'); },
  });

  Alpine.store('nav', {
    page: 'overview',
    detail: null,
    go(page, detail = null) {
      this.page = page;
      this.detail = detail;
    },
  });

  Alpine.store('search', {
    query: '',
  });

  Alpine.store('servers', {
    list: [],
    loading: true,
    async fetch() {
      try {
        this.list = await Alpine.store('api').get('/servers');
      } catch (e) {
        console.error('Failed to fetch servers:', e);
      } finally {
        this.loading = false;
      }
    },
  });

  Alpine.store('deployments', {
    all: [],
    recent: [],
    loading: true,
    async fetch() {
      try {
        const groups = await Alpine.store('api').get('/groups');
        const all = [];
        for (const g of groups) {
          const projects = await Alpine.store('api').get('/groups/' + encodeURIComponent(g.name) + '/projects');
          all.push(...projects);
        }
        this.all = all;
        this.recent = all.slice(0, 6);
      } catch (e) {
        console.error('Failed to fetch deployments:', e);
      } finally {
        this.loading = false;
      }
    },
  });

  Alpine.store('projects', {
    byGroup: {},
    async fetch() {
      try {
        const groups = await Alpine.store('api').get('/groups');
        this.byGroup = {};
        for (const g of groups) {
          const projects = await Alpine.store('api').get('/groups/' + encodeURIComponent(g.name) + '/projects');
          this.byGroup[g.name] = projects;
        }
      } catch (e) {
        console.error('Failed to fetch projects:', e);
      }
    },
  });

  Alpine.store('groups', {
    list: [],
    async fetch() {
      try {
        this.list = await Alpine.store('api').get('/groups');
        await Alpine.store('projects').fetch();
      } catch (e) {
        console.error('Failed to fetch groups:', e);
      }
    },
    async create(name) {
      await Alpine.store('api').post('/groups', { name });
      await this.fetch();
      Alpine.store('toast').success('Group created');
    },
    async delete(name) {
      await Alpine.store('api').post('/groups/' + encodeURIComponent(name), {}, 'DELETE');
      await this.fetch();
      Alpine.store('toast').success('Group deleted');
    },
  });
});

function init() {
  const isDark = localStorage.getItem('teploy-theme') !== 'light';
  document.documentElement.dataset.theme = isDark ? 'dark' : 'light';
  Alpine.store('servers').fetch();
  Alpine.store('deployments').fetch();
  Alpine.store('groups').fetch();
  Alpine.store('projects').fetch();
  setInterval(() => {
    Alpine.store('servers').fetch();
    Alpine.store('deployments').fetch();
    Alpine.store('groups').fetch();
    Alpine.store('projects').fetch();
  }, 5000);
}

function overviewPage() {
  return {};
}

function deploymentDetail() {
  return {
    status: null,
    containers: [],
    envVars: [],
    deployLogs: [],
    loading: true,
    actionLoading: null,
    viewLogs: false,
    showEnv: {},

    async load() {
      this.loading = true;
      try {
        const project = this.$store.nav.detail?.project || this.$store.nav.detail?.name;
        if (!project) {
          Alpine.store('toast').error('No project selected');
          return;
        }
        const [status, env, logs] = await Promise.allSettled([
          Alpine.store('api').get('/apps/' + encodeURIComponent(project) + '/status'),
          Alpine.store('api').get('/apps/' + encodeURIComponent(project) + '/env'),
          Alpine.store('api').get('/apps/' + encodeURIComponent(project) + '/log'),
        ]);
        if (status.status === 'fulfilled') {
          this.status = status.value;
          this.containers = status.value.containers || [];
        }
        if (env.status === 'fulfilled') this.envVars = env.value || [];
        if (logs.status === 'fulfilled') this.deployLogs = logs.value || [];
      } catch (e) {
        Alpine.store('toast').error('Failed to load deployment');
      } finally {
        this.loading = false;
      }
    },

    async doAction(act) {
      if (this.actionLoading) return;
      this.actionLoading = act;
      try {
        const project = this.$store.nav.detail?.project || this.$store.nav.detail?.name;
        const result = await Alpine.store('api').post('/apps/' + encodeURIComponent(project) + '/' + act);
        if (result.success) {
          Alpine.store('toast').success(act + ' completed');
          await this.load();
        } else {
          Alpine.store('toast').error(result.error || 'Failed');
        }
      } catch (e) {
        Alpine.store('toast').error(e.message);
      } finally {
        this.actionLoading = null;
      }
    },

    toggleEnv(key) {
      this.showEnv[key] = !this.showEnv[key];
    },

    maskedValue(key, value) {
      return this.showEnv[key] ? value : '••••••••';
    },
  };
}

function serverDetail() {
  return {
    info: null,
    loading: true,

    async load() {
      this.loading = true;
      try {
        this.info = await Alpine.store('api').get('/servers/' + encodeURIComponent(this.$store.nav.detail) + '/status');
      } catch (e) {
        Alpine.store('toast').error('Failed to load server');
      } finally {
        this.loading = false;
      }
    },
  };
}

function settingsPage() {
  return {
    newGroup: '',
    webhook_url: '',
    saving: false,

    async load() {
      try {
        const data = await Alpine.store('api').get('/settings/notifications');
        this.webhook_url = data.webhook_url || '';
      } catch (e) {
        console.error('Failed to load settings:', e);
      }
    },

    async createGroup() {
      if (!this.newGroup.trim()) return;
      await Alpine.store('groups').create(this.newGroup.trim());
      this.newGroup = '';
    },

    async deleteGroup(name) {
      if (confirm('Delete group ' + name + '?')) {
        await Alpine.store('api').post('/groups/' + encodeURIComponent(name), {}, 'DELETE');
        await Alpine.store('groups').fetch();
      }
    },

    async saveSettings() {
      this.saving = true;
      try {
        await Alpine.store('api').post('/settings/notifications', { webhook_url: this.webhook_url });
        Alpine.store('toast').success('Saved');
      } catch (e) {
        Alpine.store('toast').error('Failed to save');
      } finally {
        this.saving = false;
      }
    },
  };
}

function liveLogs() {
  return {
    ws: null,
    lines: [],
    paused: false,
    connected: false,

    connect(appName, process, numLines) {
      this.disconnect();
      this.lines = [];
      this.connected = false;
      const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
      let url = proto + '//' + location.host + '/ws/logs/' + encodeURIComponent(appName);
      if (process || numLines) {
        const params = [];
        if (process) params.push('process=' + encodeURIComponent(process));
        if (numLines) params.push('lines=' + numLines);
        url += '?' + params.join('&');
      }

      this.ws = new WebSocket(url);
      this.ws.onopen = () => { this.connected = true; };
      this.ws.onmessage = (e) => {
        if (this.paused) return;
        this.lines.push(e.data);
        if (this.lines.length > 1000) this.lines = this.lines.slice(-500);
        this.$nextTick?.(() => {
          const el = document.querySelector('.log-viewer');
          if (el) el.scrollTop = el.scrollHeight;
        });
      };
      this.ws.onclose = () => { this.connected = false; };
      this.ws.onerror = () => { this.connected = false; };
    },

    disconnect() {
      if (this.ws) {
        this.ws.close();
        this.ws = null;
      }
    },

    clear() {
      this.lines = [];
    },
  };
}

// Helpers
function statusColor(state) {
  if (state === 'running') return 'green';
  if (state === 'exited' || state === 'dead') return 'red';
  if (state === 'unknown') return 'gray';
  return 'yellow';
}

function shortHash(hash) {
  return hash ? hash.substring(0, 7) : '—';
}

// Parse resource values like "4.2GB" to numbers
function parseResourceValue(str) {
  if (!str) return 0;
  const value = parseFloat(str);
  return isNaN(value) ? 0 : value;
}

// Calculate usage percentage from "used" and "total" strings
function usagePercent(used, total) {
  const usedNum = parseResourceValue(used);
  const totalNum = parseResourceValue(total);
  if (totalNum === 0) return 0;
  const percent = Math.round((usedNum / totalNum) * 100);
  return Math.min(100, Math.max(0, percent));
}

// Get color class based on usage percentage
function usageColor(percent) {
  if (percent >= 80) return 'critical';
  if (percent >= 60) return 'warning';
  return '';
}

// For deployments page projects loading
async function loadProjects(groupName) {
  this.loading = true;
  try {
    this.projects = await Alpine.store('api').get('/groups/' + encodeURIComponent(groupName) + '/projects');
  } catch (e) {
    this.projects = [];
  } finally {
    this.loading = false;
  }
}

// Auto-initialize when Alpine is ready
document.addEventListener('alpine:init', () => {
  setTimeout(() => init(), 100);
});
