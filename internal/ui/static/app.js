// ── API Client ──
const api = {
  async get(url) {
    const res = await fetch(url);
    const json = await res.json();
    if (json.error) throw new Error(json.error);
    return json.data;
  },
  async post(url, body) {
    const res = await fetch(url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    const json = await res.json();
    if (json.error) throw new Error(json.error);
    return json.data;
  },
  async put(url, body) {
    const res = await fetch(url, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    const json = await res.json();
    if (json.error) throw new Error(json.error);
    return json.data;
  },
  async del(url) {
    const res = await fetch(url, { method: 'DELETE' });
    const json = await res.json();
    if (json.error) throw new Error(json.error);
    return json.data;
  },
};

// ── Toast ──
function showToast(message, type = 'info') {
  const container = document.getElementById('toast-container');
  const toast = document.createElement('div');
  toast.className = `toast ${type}`;
  toast.textContent = message;
  container.appendChild(toast);
  setTimeout(() => toast.remove(), 4000);
}

// ── Theme ──
function initTheme() {
  const saved = localStorage.getItem('teploy-theme') || 'dark';
  document.documentElement.setAttribute('data-theme', saved);
  return saved;
}

function toggleTheme() {
  const current = document.documentElement.getAttribute('data-theme');
  const next = current === 'dark' ? 'light' : 'dark';
  document.documentElement.setAttribute('data-theme', next);
  localStorage.setItem('teploy-theme', next);
  return next;
}

// ── Alpine.js App ──
document.addEventListener('alpine:init', () => {
  // ── Router Store ──
  Alpine.store('router', {
    page: 'projects',
    params: {},
    navigate(page, params = {}) {
      this.page = page;
      this.params = params;
    },
  });

  // ── Theme Store ──
  Alpine.store('theme', {
    mode: initTheme(),
    toggle() {
      this.mode = toggleTheme();
    },
  });

  // ── Projects Page ──
  Alpine.data('projectsPage', () => ({
    apps: [],
    groups: [],
    serverList: [],
    search: '',
    loading: true,
    deployingToGroup: null,
    deploying: false,
    deployForm: { app: '', image: '', domain: '', server: '', port: 80 },

    async init() {
      await this.load();
    },

    async load() {
      this.loading = true;
      try {
        const [apps, groups, servers] = await Promise.all([
          api.get('/api/apps'),
          api.get('/api/groups'),
          api.get('/api/config/servers').catch(() => ({})),
        ]);
        this.apps = apps;
        this.groups = groups;
        this.serverList = Object.keys(servers || {});
      } catch (e) {
        showToast(e.message, 'error');
      }
      this.loading = false;
    },

    openDeployForm(groupName) {
      this.deployingToGroup = groupName;
      this.deployForm = { app: '', image: '', domain: '', server: '', port: 80 };
    },

    async doDeploy(groupName) {
      const f = this.deployForm;
      if (!f.app || !f.image || !f.domain || !f.server) {
        showToast('All fields are required', 'error');
        return;
      }
      this.deploying = true;
      try {
        await api.post('/api/deploy', f);
        // Auto-assign the app to this group
        if (groupName) {
          await api.post(`/api/groups/${encodeURIComponent(groupName)}/apps`, { app: f.app }).catch(() => {});
        }
        showToast(`Deployed ${f.app} successfully`, 'success');
        this.deployingToGroup = null;
        this.deployForm = { app: '', image: '', domain: '', server: '', port: 80 };
        await this.load();
      } catch (e) {
        showToast(e.message, 'error');
      }
      this.deploying = false;
    },

    get filteredApps() {
      if (!this.search) return this.apps || [];
      const q = this.search.toLowerCase();
      return (this.apps || []).filter(a =>
        a.name.toLowerCase().includes(q) ||
        a.server.toLowerCase().includes(q)
      );
    },

    groupedApps() {
      const groups = (this.groups || []).map(g => {
        const projectAppNames = new Set((g.projects || []).flatMap(p => p.apps || []));
        const directApps = this.filteredApps.filter(a => (g.apps || []).includes(a.name) && !projectAppNames.has(a.name));
        const projects = (g.projects || []).map(p => ({
          ...p,
          resolvedApps: this.filteredApps.filter(a => (p.apps || []).includes(a.name)),
        }));
        return { ...g, directApps, projects, system: false };
      });
      const allAssigned = new Set((this.groups || []).flatMap(g => {
        const groupApps = g.apps || [];
        const projApps = (g.projects || []).flatMap(p => p.apps || []);
        return [...groupApps, ...projApps];
      }));
      const ungrouped = this.filteredApps.filter(a => !allAssigned.has(a.name));
      if (ungrouped.length > 0) {
        groups.push({ name: 'Ungrouped', directApps: ungrouped, projects: [], system: true });
      }
      return groups;
    },

    openApp(name, fromProject, fromGroup) {
      Alpine.store('router').navigate('app-detail', { name, fromProject: fromProject || null, fromGroup: fromGroup || null });
    },

    openProject(groupName, projectName) {
      Alpine.store('router').navigate('project-detail', { group: groupName, project: projectName });
    },

    async createProject(groupName) {
      const name = prompt('Project name:');
      if (!name) return;
      try {
        await api.post(`/api/groups/${encodeURIComponent(groupName)}/projects`, { name });
        showToast('Project created', 'success');
        await this.load();
      } catch (e) {
        showToast(e.message, 'error');
      }
    },

    async createGroup() {
      const name = prompt('Group name:');
      if (!name) return;
      try {
        await api.post('/api/groups', { name });
        showToast('Group created', 'success');
        await this.load();
      } catch (e) {
        showToast(e.message, 'error');
      }
    },

    async deleteProject(groupName, projectName) {
      if (!confirm(`Delete project "${projectName}"? Apps inside remain in the group.`)) return;
      try {
        await api.del(`/api/groups/${encodeURIComponent(groupName)}/projects/${encodeURIComponent(projectName)}`);
        showToast('Project deleted', 'success');
        await this.load();
      } catch (e) {
        showToast(e.message, 'error');
      }
    },

    async unassignFromGroup(groupName, appName) {
      if (!confirm(`Remove "${appName}" from group "${groupName}"?`)) return;
      try {
        await api.del(`/api/groups/${encodeURIComponent(groupName)}/apps/${encodeURIComponent(appName)}`);
        showToast('App removed from group', 'success');
        await this.load();
      } catch (e) {
        showToast(e.message, 'error');
      }
    },
  }));

  // ── Project Detail Page ──
  Alpine.data('projectDetailPage', () => ({
    apps: [],
    groups: [],
    serverList: [],
    loading: true,
    groupName: '',
    projectName: '',
    projectApps: [],
    deployingToProject: false,
    deploying: false,
    deployForm: { app: '', image: '', domain: '', server: '', port: 80 },

    async init() {
      this.groupName = Alpine.store('router').params.group;
      this.projectName = Alpine.store('router').params.project;
      await this.load();
    },

    async load() {
      this.loading = true;
      try {
        const [apps, groups, servers] = await Promise.all([
          api.get('/api/apps'),
          api.get('/api/groups'),
          api.get('/api/config/servers').catch(() => ({})),
        ]);
        this.apps = apps;
        this.groups = groups;
        this.serverList = Object.keys(servers || {});
        const group = (this.groups || []).find(g => g.name === this.groupName);
        const proj = group ? (group.projects || []).find(p => p.name === this.projectName) : null;
        const projAppNames = proj ? (proj.apps || []) : [];
        this.projectApps = (this.apps || []).filter(a => projAppNames.includes(a.name));
      } catch (e) {
        showToast(e.message, 'error');
      }
      this.loading = false;
    },

    openApp(name) {
      Alpine.store('router').navigate('app-detail', { name, fromProject: this.projectName, fromGroup: this.groupName });
    },

    async unassignFromProject(appName) {
      if (!confirm(`Remove "${appName}" from project "${this.projectName}"?`)) return;
      try {
        await api.del(`/api/groups/${encodeURIComponent(this.groupName)}/projects/${encodeURIComponent(this.projectName)}/apps/${encodeURIComponent(appName)}`);
        showToast('App removed from project', 'success');
        await this.load();
      } catch (e) {
        showToast(e.message, 'error');
      }
    },

    async deleteThisProject() {
      if (!confirm(`Delete project "${this.projectName}"? Apps remain in the group.`)) return;
      try {
        await api.del(`/api/groups/${encodeURIComponent(this.groupName)}/projects/${encodeURIComponent(this.projectName)}`);
        showToast('Project deleted', 'success');
        Alpine.store('router').navigate('projects');
      } catch (e) {
        showToast(e.message, 'error');
      }
    },

    openDeployForm() {
      this.deployingToProject = true;
      this.deployForm = { app: '', image: '', domain: '', server: '', port: 80 };
    },

    async doDeploy() {
      const f = this.deployForm;
      if (!f.app || !f.image || !f.domain || !f.server) {
        showToast('All fields are required', 'error');
        return;
      }
      this.deploying = true;
      try {
        await api.post('/api/deploy', f);
        // Auto-assign to the group and project
        await api.post(`/api/groups/${encodeURIComponent(this.groupName)}/apps`, { app: f.app }).catch(() => {});
        await api.post(`/api/groups/${encodeURIComponent(this.groupName)}/projects/${encodeURIComponent(this.projectName)}/apps`, { app: f.app }).catch(() => {});
        showToast(`Deployed ${f.app} successfully`, 'success');
        this.deployingToProject = false;
        this.deployForm = { app: '', image: '', domain: '', server: '', port: 80 };
        await this.load();
      } catch (e) {
        showToast(e.message, 'error');
      }
      this.deploying = false;
    },
  }));

  // ── App Detail Page ──
  Alpine.data('appDetailPage', () => ({
    tab: 'general',
    app: null,
    envVars: [],
    deployLog: [],
    accessories: [],
    loading: true,
    actionLoading: false,
    newEnvKey: '',
    newEnvValue: '',

    async init() {
      const name = Alpine.store('router').params.name;
      await this.loadStatus(name);
    },

    async loadStatus(name) {
      this.loading = true;
      try {
        this.app = await api.get(`/api/apps/${name}/status`);
      } catch (e) {
        showToast(e.message, 'error');
      }
      this.loading = false;
    },

    async loadEnv() {
      const name = Alpine.store('router').params.name;
      try {
        this.envVars = await api.get(`/api/apps/${name}/env`);
      } catch (e) {
        showToast(e.message, 'error');
      }
    },

    async loadLog() {
      const name = Alpine.store('router').params.name;
      try {
        this.deployLog = await api.get(`/api/apps/${name}/log`);
      } catch (e) {
        showToast(e.message, 'error');
      }
    },

    async loadAccessories() {
      const name = Alpine.store('router').params.name;
      try {
        this.accessories = await api.get(`/api/apps/${name}/accessories`);
      } catch (e) {
        showToast(e.message, 'error');
      }
    },

    async switchTab(t) {
      this.tab = t;
      if (t === 'env') await this.loadEnv();
      if (t === 'deploys') await this.loadLog();
      if (t === 'general') await this.loadAccessories();
      if (t === 'logs') this.$nextTick(() => this.$dispatch('start-logs'));
    },

    async doAction(action) {
      const name = Alpine.store('router').params.name;
      this.actionLoading = true;
      try {
        await api.post(`/api/apps/${name}/${action}`);
        showToast(`${action} successful`, 'success');
        await this.loadStatus(name);
      } catch (e) {
        showToast(e.message, 'error');
      }
      this.actionLoading = false;
    },

    async addEnvVar() {
      if (!this.newEnvKey) return;
      const name = Alpine.store('router').params.name;
      try {
        await api.post(`/api/apps/${name}/env`, { [this.newEnvKey]: this.newEnvValue });
        showToast('Env var added', 'success');
        this.newEnvKey = '';
        this.newEnvValue = '';
        await this.loadEnv();
      } catch (e) {
        showToast(e.message, 'error');
      }
    },

    async deleteEnvVar(key) {
      if (!confirm(`Delete ${key}?`)) return;
      const name = Alpine.store('router').params.name;
      try {
        await api.del(`/api/apps/${name}/env/${key}`);
        showToast('Env var removed', 'success');
        await this.loadEnv();
      } catch (e) {
        showToast(e.message, 'error');
      }
    },

    containerCount() {
      return (this.app?.containers || []).filter(c => c.State === 'running').length;
    },
  }));

  // ── Log Viewer Component ──
  Alpine.data('logViewer', () => ({
    ws: null,
    lines: [],
    process: 'web',
    lineCount: '100',
    paused: false,
    connected: false,
    autoScroll: true,

    init() {
      this.$el.addEventListener('start-logs', () => this.connect());
    },

    connect() {
      if (this.ws) this.ws.close();
      this.lines = [];
      const name = Alpine.store('router').params.name;
      const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
      const url = `${proto}//${location.host}/ws/logs/${name}?process=${this.process}&lines=${this.lineCount}`;
      this.ws = new WebSocket(url);
      this.ws.onopen = () => { this.connected = true; };
      this.ws.onclose = () => { this.connected = false; };
      this.ws.onmessage = (e) => {
        if (this.paused) return;
        this.lines.push(e.data);
        if (this.lines.length > 5000) this.lines = this.lines.slice(-2500);
        if (this.autoScroll) {
          this.$nextTick(() => {
            const viewer = this.$refs.logContent;
            if (viewer) viewer.scrollTop = viewer.scrollHeight;
          });
        }
      };
    },

    disconnect() {
      if (this.ws) { this.ws.close(); this.ws = null; }
    },

    clear() { this.lines = []; },

    togglePause() { this.paused = !this.paused; },

    switchProcess(p) {
      this.process = p;
      this.connect();
    },

    destroy() { this.disconnect(); },
  }));

  // ── Servers Page ──
  Alpine.data('serversPage', () => ({
    servers: [],
    loading: true,

    async init() {
      await this.load();
    },

    async load() {
      this.loading = true;
      try {
        this.servers = await api.get('/api/servers');
      } catch (e) {
        showToast(e.message, 'error');
      }
      this.loading = false;
    },

    openServer(name) {
      Alpine.store('router').navigate('server-detail', { name });
    },
  }));

  // ── Server Detail Page ──
  Alpine.data('serverDetailPage', () => ({
    status: null,
    proxy: null,
    tab: 'overview',
    loading: true,

    async init() {
      const name = Alpine.store('router').params.name;
      this.loading = true;
      try {
        this.status = await api.get(`/api/servers/${name}/status`);
      } catch (e) {
        showToast(e.message, 'error');
      }
      this.loading = false;
    },

    async switchTab(t) {
      this.tab = t;
      if (t === 'proxy' && !this.proxy) {
        await this.loadProxy();
      }
    },

    async loadProxy() {
      const name = Alpine.store('router').params.name;
      try {
        this.proxy = await api.get(`/api/servers/${name}/proxy`);
      } catch (e) {
        showToast(e.message, 'error');
      }
    },

    parsePercent(s) {
      return parseInt(s) || 0;
    },

    barClass(pct) {
      if (pct < 60) return 'low';
      if (pct < 85) return 'medium';
      return 'high';
    },
  }));

  // ── Settings Page ──
  Alpine.data('settingsPage', () => ({
    tab: 'servers',
    servers: {},
    groups: [],
    notifications: {},
    registries: [],
    loading: true,
    // Add server form
    newServer: { name: '', host: '', user: 'root', role: 'app' },
    // Edit server form (null when not editing)
    editingServer: null,
    // Add registry form
    newReg: { server: '', username: '', password: '' },
    // Add group form
    newGroupName: '',

    async init() {
      await this.loadAll();
    },

    async loadAll() {
      this.loading = true;
      try {
        [this.servers, this.groups, this.notifications, this.registries] = await Promise.all([
          api.get('/api/config/servers'),
          api.get('/api/groups'),
          api.get('/api/config/notifications').catch(() => ({})),
          api.get('/api/config/registries').catch(() => []),
        ]);
      } catch (e) {
        showToast(e.message, 'error');
      }
      this.loading = false;
    },

    serverList() {
      return Object.entries(this.servers || {}).map(([name, s]) => ({ name, ...s }));
    },

    async addServer() {
      if (!this.newServer.name || !this.newServer.host) return;
      try {
        await api.post('/api/config/servers', this.newServer);
        showToast('Server added', 'success');
        this.newServer = { name: '', host: '', user: 'root', role: 'app' };
        await this.loadAll();
      } catch (e) {
        showToast(e.message, 'error');
      }
    },

    async deleteServer(name) {
      if (!confirm(`Remove server ${name}?`)) return;
      try {
        await api.del(`/api/config/servers/${name}`);
        showToast('Server removed', 'success');
        await this.loadAll();
      } catch (e) {
        showToast(e.message, 'error');
      }
    },

    async createGroup() {
      if (!this.newGroupName) return;
      try {
        await api.post('/api/groups', { name: this.newGroupName });
        showToast('Group created', 'success');
        this.newGroupName = '';
        await this.loadAll();
      } catch (e) {
        showToast(e.message, 'error');
      }
    },

    async deleteGroup(name) {
      if (!confirm(`Delete group ${name}?`)) return;
      try {
        await api.del(`/api/groups/${encodeURIComponent(name)}`);
        showToast('Group deleted', 'success');
        await this.loadAll();
      } catch (e) {
        showToast(e.message, 'error');
      }
    },

    async renameGroup(oldName) {
      const newName = prompt('New group name:', oldName);
      if (!newName || newName === oldName) return;
      try {
        await api.put(`/api/groups/${encodeURIComponent(oldName)}`, { name: newName });
        showToast('Group renamed', 'success');
        await this.loadAll();
      } catch (e) {
        showToast(e.message, 'error');
      }
    },

    editServer(name) {
      const srv = this.servers[name] || {};
      this.editingServer = {
        originalName: name,
        name,
        host: srv.host || '',
        user: srv.user || 'root',
        role: srv.role || 'app',
      };
    },

    cancelEdit() {
      this.editingServer = null;
    },

    async saveEditServer() {
      const e = this.editingServer;
      if (!e || !e.name || !e.host) return;
      try {
        await api.put(`/api/config/servers/${encodeURIComponent(e.originalName)}`, {
          name: e.name, host: e.host, user: e.user, role: e.role,
        });
        showToast('Server updated', 'success');
        this.editingServer = null;
        await this.loadAll();
      } catch (err) {
        showToast(err.message, 'error');
      }
    },

    async saveNotifications() {
      try {
        await api.post('/api/config/notifications', this.notifications);
        showToast('Notifications saved', 'success');
      } catch (e) {
        showToast(e.message, 'error');
      }
    },

    async addRegistry() {
      if (!this.newReg.server || !this.newReg.username) return;
      try {
        await api.post('/api/config/registries', this.newReg);
        showToast('Registry added', 'success');
        this.newReg = { server: '', username: '', password: '' };
        await this.loadAll();
      } catch (e) {
        showToast(e.message, 'error');
      }
    },

    async deleteRegistry(server) {
      if (!confirm(`Remove registry ${server}?`)) return;
      try {
        await api.del(`/api/config/registries/${encodeURIComponent(server)}`);
        showToast('Registry removed', 'success');
        await this.loadAll();
      } catch (e) {
        showToast(e.message, 'error');
      }
    },
  }));
});
