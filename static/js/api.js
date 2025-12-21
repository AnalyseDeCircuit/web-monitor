// Global data storage for filtering/sorting
let allDockerContainers = [];
let allDockerImages = [];
let allServices = [];

let dockerLogsState = { id: null, name: '', tail: 200 };

let dockerContainerSort = { column: 'name', direction: 'asc' };
let dockerImageSort = { column: 'created', direction: 'desc' };
let serviceSort = { column: 'unit', direction: 'asc' };

// Plugins
async function loadPlugins() {
    try {
        const response = await fetch('/api/plugins/list');
        if (!response.ok) return [];
        return await response.json();
    } catch (e) {
        console.error("Failed to load plugins", e);
        return [];
    }
}

async function togglePlugin(name, enabled) {
    try {
        const response = await fetch('/api/plugins/action', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ name, enabled })
        });
        if (!response.ok) {
            const msg = await readErrorMessage(response);
            throw new Error(msg);
        }
        return true;
    } catch (e) {
        console.error("Failed to toggle plugin", e);
        throw e;
    }
}

async function installPlugin(name) {
    const response = await fetch('/api/plugins/install', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name })
    });
    if (!response.ok) {
        const msg = await readErrorMessage(response);
        throw new Error(msg);
    }
    return await response.json();
}

async function uninstallPlugin(name, removeData = false) {
    const response = await fetch('/api/plugins/uninstall', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name, removeData })
    });
    if (!response.ok) {
        const msg = await readErrorMessage(response);
        throw new Error(msg);
    }
    return await response.json();
}

async function getPluginManifest(name) {
    const response = await fetch(`/api/plugins/manifest?name=${encodeURIComponent(name)}`);
    if (!response.ok) {
        const msg = await readErrorMessage(response);
        throw new Error(msg);
    }
    return await response.json();
}

// Docker
function dockerShowTableError(tbodyId, message, colspan) {
    try {
        const tbody = document.getElementById(tbodyId);
        if (!tbody) return;
        tbody.innerHTML = `<tr><td colspan="${colspan}" style="padding: 10px; text-align: center; color: var(--text-dim);">${message}</td></tr>`;
    } catch (e) {
        // no-op
    }
}

async function readErrorMessage(response) {
    try {
        const ct = (response.headers.get('Content-Type') || '').toLowerCase();
        if (ct.includes('application/json')) {
            const data = await response.json();
            if (data && typeof data.error === 'string') return data.error;
            return JSON.stringify(data);
        }
        const text = await response.text();
        return text || `${response.status} ${response.statusText}`;
    } catch (e) {
        return `${response.status} ${response.statusText}`;
    }
}

async function loadDockerContainers() {
    try {
        const response = await fetch('/api/docker/containers');
        if (!response.ok) {
            const msg = await readErrorMessage(response);
            if (response.status === 401) {
                dockerShowTableError('docker-containers-body', 'Unauthorized (please log in again)', 6);
                return;
            }
            dockerShowTableError('docker-containers-body', `Failed to load containers: ${msg}`, 6);
            return;
        }
        const data = await response.json();
        allDockerContainers = data.containers || [];
        // Show prune button only for admin (fallback to /api/session if role not cached)
        const pruneBtn = document.getElementById('docker-prune-btn');
        const pruneHint = document.getElementById('docker-prune-hint');
        if (pruneBtn && pruneHint) {
            let role = localStorage.getItem('role');
            if (!role) {
                try {
                    const sessionRes = await fetch('/api/session');
                    if (sessionRes.ok) {
                        const sessionData = await sessionRes.json();
                        if (sessionData && sessionData.role) {
                            role = sessionData.role;
                            localStorage.setItem('role', role);
                            if (sessionData.username) localStorage.setItem('username', sessionData.username);
                        }
                    }
                } catch (_) {
                    // ignore
                }
            }
            const isAdmin = role === 'admin';
            pruneBtn.style.display = isAdmin ? 'inline-flex' : 'none';
            pruneHint.style.display = isAdmin ? 'block' : 'none';
        }
        renderDockerContainers();
        updateDockerSummary();
    } catch (err) {
        console.error('Failed to load containers', err);
        dockerShowTableError('docker-containers-body', 'Failed to load containers: network error', 6);
    }
}

function renderDockerContainers() {
    const tbody = document.getElementById('docker-containers-body');
    tbody.innerHTML = '';

    const searchTerm = document.getElementById('docker-container-search').value.toLowerCase();

    let filtered = allDockerContainers.filter((c) => {
        const name = c.Names ? c.Names[0].replace('/', '') : c.Id.substring(0, 12);
        return (
            name.toLowerCase().includes(searchTerm) ||
            (c.Image && c.Image.toLowerCase().includes(searchTerm)) ||
            (c.State && c.State.toLowerCase().includes(searchTerm))
        );
    });

    filtered.sort((a, b) => {
        let valA, valB;
        if (dockerContainerSort.column === 'name') {
            valA = a.Names ? a.Names[0].replace('/', '') : a.Id;
            valB = b.Names ? b.Names[0].replace('/', '') : b.Id;
        } else if (dockerContainerSort.column === 'state') {
            valA = a.State;
            valB = b.State;
        } else {
            return 0;
        }

        if (valA < valB) return dockerContainerSort.direction === 'asc' ? -1 : 1;
        if (valA > valB) return dockerContainerSort.direction === 'asc' ? 1 : -1;
        return 0;
    });

    if (filtered.length === 0) {
        tbody.innerHTML =
            '<tr><td colspan="6" style="padding: 10px; text-align: center; color: var(--text-dim);">No containers found</td></tr>';
        return;
    }

    const role = localStorage.getItem('role');

    filtered.forEach((c) => {
        const name = c.Names ? c.Names[0].replace('/', '') : c.Id.substring(0, 12);
        const ports = c.Ports ? c.Ports.map((p) => `${p.PrivatePort}->${p.PublicPort}/${p.Type}`).join(', ') : '';
        const stateClass = c.State === 'running' ? 'running' : 
                          c.State === 'paused' ? 'paused' : 
                          c.State === 'restarting' ? 'restarting' : 'exited';

        let actionsHtml = '';
        if (role === 'admin') {
            const logsBtn = `<button onclick="openDockerLogsById('${c.Id}')" class="btn btn-secondary btn-sm" style="margin-right: 5px;">Logs</button>`;
            actionsHtml =
                c.State === 'running'
                    ? `${logsBtn}<button onclick="handleDockerAction('${c.Id}', 'stop')" class="btn btn-danger btn-sm" style="margin-right: 5px;">Stop</button>
                         <button onclick="handleDockerAction('${c.Id}', 'restart')" class="btn btn-info btn-sm">Restart</button>`
                    : `${logsBtn}<button onclick="handleDockerAction('${c.Id}', 'start')" class="btn btn-success btn-sm" style="margin-right: 5px;">Start</button>
                         <button onclick="handleDockerAction('${c.Id}', 'remove')" class="btn btn-secondary btn-sm">Remove</button>`;
        } else {
            actionsHtml = '<span style="color: var(--text-dim); font-size: 0.8rem;">Read-only</span>';
        }

        const tr = document.createElement('tr');
        tr.style.borderBottom = '1px solid rgba(255,255,255,0.05)';
        tr.innerHTML = `
                    <td style="padding: 10px; font-weight: bold;">${name}</td>
                    <td style="padding: 10px; color: var(--text-dim);">${c.Image}</td>
                    <td style="padding: 10px;"><span class="container-status ${stateClass}">${c.State}</span></td>
                    <td style="padding: 10px; font-size: 0.85rem;">${c.Status}</td>
                    <td style="padding: 10px; font-size: 0.85rem;">${ports}</td>
                    <td style="padding: 10px;">
                        ${actionsHtml}
                    </td>
                `;
        tbody.appendChild(tr);
    });
}

function setDockerContainerSort(column) {
    if (dockerContainerSort.column === column) {
        dockerContainerSort.direction = dockerContainerSort.direction === 'asc' ? 'desc' : 'asc';
    } else {
        dockerContainerSort.column = column;
        dockerContainerSort.direction = 'asc';
    }
    renderDockerContainers();
}

async function loadDockerImages() {
    try {
        const response = await fetch('/api/docker/images');
        if (!response.ok) {
            const msg = await readErrorMessage(response);
            if (response.status === 401) {
                dockerShowTableError('docker-images-body', 'Unauthorized (please log in again)', 5);
                return;
            }
            dockerShowTableError('docker-images-body', `Failed to load images: ${msg}`, 5);
            return;
        }
        const data = await response.json();
        allDockerImages = data.images || [];
        renderDockerImages();
        updateDockerSummary();
    } catch (err) {
        console.error('Failed to load images', err);
        dockerShowTableError('docker-images-body', 'Failed to load images: network error', 5);
    }
}

function updateDockerSummary() {
    try {
        const running = allDockerContainers.filter((c) => c.State === 'running').length;
        const total = allDockerContainers.length;
        const imageCount = allDockerImages.length;
        const imageSizeBytes = allDockerImages.reduce((acc, img) => acc + (img.Size || 0), 0);

        const containersEl = document.getElementById('docker-summary-containers');
        const imagesEl = document.getElementById('docker-summary-images');
        const sizeEl = document.getElementById('docker-summary-size');

        if (containersEl) containersEl.innerText = `${running} / ${total} running`;
        if (imagesEl) imagesEl.innerText = `${imageCount} images`;
        if (sizeEl) sizeEl.innerText = formatSize(imageSizeBytes);
    } catch (e) {
        // non-blocking UI helper
    }
}

function renderDockerImages() {
    const tbody = document.getElementById('docker-images-body');
    tbody.innerHTML = '';

    const searchTerm = document.getElementById('docker-image-search').value.toLowerCase();

    let filtered = allDockerImages.filter((img) => {
        const id = img.Id.substring(7, 19);
        return (
            id.toLowerCase().includes(searchTerm) ||
            (img.RepoTags && img.RepoTags.some((t) => t.toLowerCase().includes(searchTerm)))
        );
    });

    filtered.sort((a, b) => {
        let valA, valB;
        if (dockerImageSort.column === 'id') {
            valA = a.Id;
            valB = b.Id;
        } else if (dockerImageSort.column === 'size') {
            valA = a.Size;
            valB = b.Size;
        } else if (dockerImageSort.column === 'created') {
            valA = a.Created;
            valB = b.Created;
        } else {
            return 0;
        }

        if (valA < valB) return dockerImageSort.direction === 'asc' ? -1 : 1;
        if (valA > valB) return dockerImageSort.direction === 'asc' ? 1 : -1;
        return 0;
    });

    if (filtered.length === 0) {
        tbody.innerHTML =
            '<tr><td colspan="5" style="padding: 10px; text-align: center; color: var(--text-dim);">No images found</td></tr>';
        return;
    }

    const role = localStorage.getItem('role');

    filtered.forEach((img) => {
        const id = img.Id.substring(7, 19);
        const size = (img.Size / 1024 / 1024).toFixed(2) + ' MB';
        const created = new Date(img.Created * 1000).toLocaleDateString();
        const tags = img.RepoTags ? img.RepoTags.join(', ') : '<none>';

        let actionsHtml = '';
        if (role === 'admin') {
            actionsHtml = `<button onclick="handleDockerImageRemove('${img.Id}')" class="btn btn-danger btn-sm">Delete</button>`;
        } else {
            actionsHtml = '<span style="color: var(--text-dim); font-size: 0.8rem;">Read-only</span>';
        }

        const tr = document.createElement('tr');
        tr.style.borderBottom = '1px solid rgba(255,255,255,0.05)';
        tr.innerHTML = `
                    <td style="padding: 10px;" class="mono-text">${id}</td>
                    <td style="padding: 10px;">${tags}</td>
                    <td style="padding: 10px;">${size}</td>
                    <td style="padding: 10px;">${created}</td>
                    <td style="padding: 10px;">${actionsHtml}</td>
                `;
        tbody.appendChild(tr);
    });
}

async function handleDockerImageRemove(imageId) {
    if (!await appConfirm('Are you sure you want to delete this image?')) return;
    try {
        const response = await fetch('/api/docker/image/remove', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ id: imageId }),
        });
        if (response.ok) {
            loadDockerImages();
        } else {
            const data = await response.json();
            appError('Error: ' + (data.error || 'Delete failed'));
        }
    } catch (err) {
        appError('Failed to delete image');
    }
}

function setDockerImageSort(column) {
    if (dockerImageSort.column === column) {
        dockerImageSort.direction = dockerImageSort.direction === 'asc' ? 'desc' : 'asc';
    } else {
        dockerImageSort.column = column;
        dockerImageSort.direction = 'asc';
        if (column === 'size' || column === 'created') dockerImageSort.direction = 'desc';
    }
    renderDockerImages();
}

async function handleDockerAction(id, action) {
    if (!await appConfirm(`Are you sure you want to ${action} this container?`)) return;
    try {
        const response = await fetch('/api/docker/action', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ id, action }),
        });
        if (response.ok) {
            loadDockerContainers();
        } else {
            const data = await response.json();
            appError('Error: ' + (data.error || 'Action failed'));
        }
    } catch (err) {
        appError('Failed to perform action');
    }
}

function closeDockerLogsModal() {
    const modal = document.getElementById('docker-logs-modal');
    if (modal) modal.style.display = 'none';
}

async function openDockerLogsById(id) {
    const role = localStorage.getItem('role');
    if (role !== 'admin') {
        appError('Docker logs are restricted to admin users.');
        return;
    }

    if (!id || id.trim() === '') {
        appError('Invalid container ID');
        return;
    }

    // 从 ID 中提取显示名称
    const displayName = id.substring(0, 12);
    await openDockerLogs(id, displayName);
}

async function openDockerLogs(id, name) {
    const role = localStorage.getItem('role');
    if (role !== 'admin') {
        appError('Docker logs are restricted to admin users.');
        return;
    }

    if (!id || id.trim() === '') {
        appError('Invalid container ID');
        return;
    }

    dockerLogsState = { id: id.trim(), name: (name || id).substring(0, 50), tail: 200 };
    const modal = document.getElementById('docker-logs-modal');
    const title = document.getElementById('docker-logs-title');
    if (title) title.innerText = `Logs: ${dockerLogsState.name}`;
    if (modal) modal.style.display = 'flex';

    await fetchDockerLogs();
}

async function fetchDockerLogs() {
    const content = document.getElementById('docker-logs-content');
    if (content) content.textContent = 'Loading...';

    if (!dockerLogsState.id || dockerLogsState.id.trim() === '') {
        if (content) content.textContent = 'Error: No container ID specified';
        return;
    }

    try {
        const url = `/api/docker/logs?id=${encodeURIComponent(dockerLogsState.id)}&tail=${dockerLogsState.tail}`;
        console.log('[Docker] Fetching logs from:', url);
        
        const resp = await fetch(url);
        console.log('[Docker] Response status:', resp.status);
        
        if (!resp.ok) {
            const msg = await readErrorMessage(resp);
            console.error('[Docker] Error fetching logs:', msg);
            if (content) content.textContent = `Failed to load logs: ${msg}`;
            return;
        }
        
        const data = await resp.json();
        console.log('[Docker] Logs loaded, length:', data.logs ? data.logs.length : 0);
        
        if (content) {
            if (!data.logs || data.logs.trim() === '') {
                content.textContent = '(No logs available)';
            } else {
                content.textContent = data.logs;
                // Auto-scroll to bottom
                setTimeout(() => {
                    content.parentElement.scrollTop = content.parentElement.scrollHeight;
                }, 100);
            }
        }
    } catch (e) {
        console.error('[Docker] Exception fetching logs:', e);
        if (content) content.textContent = `Failed to load logs: ${e.message}`;
    }
}

async function loadMoreDockerLogs() {
    if (!dockerLogsState.id) return;
    dockerLogsState.tail = Math.min(dockerLogsState.tail + 200, 2000);
    await fetchDockerLogs();
}

async function openDockerPruneConfirm() {
    if (!await appConfirm('Prune will remove stopped containers, dangling images, unused networks, and build cache. Continue?')) return;
    await handleDockerPrune();
}

async function handleDockerPrune() {
    const resultBox = document.getElementById('docker-prune-result');
    if (resultBox) {
        resultBox.style.display = 'block';
        resultBox.className = 'result-box pending';
        resultBox.style.border = '1px solid rgba(255,182,0,0.3)';
        resultBox.textContent = 'Pruning...';
    }

    try {
        const response = await fetch('/api/docker/prune', { method: 'POST' });
        if (response.ok) {
            const data = await response.json();
            const reclaimed = data.result && (data.result.SpaceReclaimed || data.result.space_reclaimed || data.result.spaceReclaimed);
            const containersDeleted = data.result && (data.result.ContainersDeleted || (data.result.Containers && data.result.Containers.length));
            const imagesDeleted = data.result && (data.result.ImagesDeleted || (data.result.Images && data.result.Images.length));
            const volumesDeleted = data.result && (data.result.VolumesDeleted || (data.result.Volumes && data.result.Volumes.length));

            let summary = 'Prune completed';
            if (reclaimed !== undefined) summary += ` • Space reclaimed: ${formatSize(reclaimed)}`;
            if (containersDeleted) summary += ` • Containers: ${containersDeleted}`;
            if (imagesDeleted) summary += ` • Images: ${imagesDeleted}`;
            if (volumesDeleted) summary += ` • Volumes: ${volumesDeleted}`;

            if (resultBox) {
                resultBox.style.display = 'block';
                resultBox.className = 'result-box success';
                resultBox.style.border = '1px solid rgba(0,180,0,0.3)';
                resultBox.textContent = summary;
            }
            // Refresh containers/images after prune
            loadDockerContainers();
            loadDockerImages();
        } else {
            const data = await response.json().catch(() => ({}));
            const msg = data.error || 'Prune failed';
            if (resultBox) {
                resultBox.style.display = 'block';
                resultBox.className = 'result-box error';
                resultBox.style.border = '1px solid rgba(255,0,0,0.3)';
                resultBox.textContent = msg;
            } else {
                appError(msg);
            }
        }
    } catch (err) {
        const msg = 'Prune failed: ' + err.message;
        if (resultBox) {
            resultBox.style.display = 'block';
            resultBox.className = 'result-box error';
            resultBox.style.border = '1px solid rgba(255,0,0,0.3)';
            resultBox.textContent = msg;
        } else {
            appError(msg);
        }
    }
}

// Systemd
async function loadServices() {
    const tbody = document.getElementById('services-body');
    tbody.innerHTML = '<tr><td colspan="6" style="text-align:center; padding: 20px;">Loading...</td></tr>';

    try {
        const response = await fetch('/api/systemd/services');
        if (!response.ok) throw new Error('Failed to fetch services');
        const services = await response.json();
        allServices = services || [];
        renderServices();
    } catch (err) {
        console.error('Failed to load services', err);
        tbody.innerHTML =
            '<tr><td colspan="6" style="text-align:center; padding: 20px; color: #ff4757;">Failed to load services</td></tr>';
    }
}

function renderServices() {
    const tbody = document.getElementById('services-body');
    tbody.innerHTML = '';

    const searchTerm = document.getElementById('service-search').value.toLowerCase();

    let filtered = allServices.filter((svc) => {
        return (
            svc.unit.toLowerCase().includes(searchTerm) ||
            (svc.description && svc.description.toLowerCase().includes(searchTerm))
        );
    });

    filtered.sort((a, b) => {
        let valA, valB;
        if (serviceSort.column === 'unit') {
            valA = a.unit;
            valB = b.unit;
        } else if (serviceSort.column === 'active') {
            valA = a.active;
            valB = b.active;
        } else {
            return 0;
        }

        if (valA < valB) return serviceSort.direction === 'asc' ? -1 : 1;
        if (valA > valB) return serviceSort.direction === 'asc' ? 1 : -1;
        return 0;
    });

    if (filtered.length === 0) {
        tbody.innerHTML = '<tr><td colspan="6" style="text-align:center; padding: 20px;">No services found</td></tr>';
        return;
    }

    const role = localStorage.getItem('role');

    filtered.forEach((svc) => {
        const tr = document.createElement('tr');
        tr.style.borderBottom = '1px solid rgba(255,255,255,0.05)';

        const statusClass = svc.active === 'active' ? 'active' : 
                           svc.active === 'failed' ? 'failed' : 'inactive';
        const unitClass = svc.active === 'active' ? 'service-unit-active' : '';

        let actionsHtml = '';
        if (role === 'admin') {
            if (svc.active === 'active') {
                actionsHtml += `<button onclick="handleServiceAction('${svc.unit}', 'stop')" class="btn btn-danger btn-sm" style="margin-right: 5px;">Stop</button>`;
                actionsHtml += `<button onclick="handleServiceAction('${svc.unit}', 'restart')" class="btn btn-info btn-sm">Restart</button>`;
            } else {
                actionsHtml += `<button onclick="handleServiceAction('${svc.unit}', 'start')" class="btn btn-success btn-sm">Start</button>`;
            }
        } else {
            actionsHtml = '<span style="color: var(--text-dim); font-size: 0.8rem;">Read-only</span>';
        }

        tr.innerHTML = `
                    <td style="padding: 10px;" class="${unitClass}">${svc.unit}</td>
                    <td style="padding: 10px;">${svc.load}</td>
                    <td style="padding: 10px;"><span class="service-status ${statusClass}">${svc.active}</span></td>
                    <td style="padding: 10px;">${svc.sub}</td>
                    <td style="padding: 10px; color: var(--text-dim); font-size: 0.85rem;">${svc.description}</td>
                    <td style="padding: 10px;">${actionsHtml}</td>
                `;
        tbody.appendChild(tr);
    });
}

function setServiceSort(column) {
    if (serviceSort.column === column) {
        serviceSort.direction = serviceSort.direction === 'asc' ? 'desc' : 'asc';
    } else {
        serviceSort.column = column;
        serviceSort.direction = 'asc';
    }
    renderServices();
}

async function handleServiceAction(unit, action) {
    if (!await appConfirm(`Are you sure you want to ${action} service ${unit}?`)) return;

    try {
        const response = await fetch('/api/systemd/action', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ unit, action }),
        });

        if (!response.ok) {
            const data = await response.json();
            throw new Error(data.error || 'Action failed');
        }

        loadServices();
    } catch (error) {
        console.error('Error performing action:', error);
        appError(`Failed to ${action} service: ${error.message || error}`);
    }
}

// Cron
let currentCronJobs = [];

async function loadCronJobs() {
    const tbody = document.getElementById('cron-body');
    tbody.innerHTML = '<tr><td colspan="3" style="text-align:center; padding: 20px;">Loading...</td></tr>';

    try {
        const response = await fetch('/api/cron');
        if (!response.ok) throw new Error('Failed to fetch cron jobs');
        const data = await response.json();
        currentCronJobs = Array.isArray(data) ? data : [];

        renderCronJobs();

        const role = localStorage.getItem('role');
        if (role === 'admin') {
            document.getElementById('btn-add-cron').style.display = 'inline-block';
            document.getElementById('btn-save-cron').style.display = 'inline-block';
        } else {
            document.getElementById('btn-add-cron').style.display = 'none';
            document.getElementById('btn-save-cron').style.display = 'none';
        }
    } catch (error) {
        console.error('Error loading cron jobs:', error);
        tbody.innerHTML = `<tr><td colspan="3" style="text-align:center; color: #ff4757; padding: 20px;">Error: ${error.message}</td></tr>`;
    }
}

function renderCronJobs() {
    const tbody = document.getElementById('cron-body');
    tbody.innerHTML = '';

    if (!currentCronJobs || currentCronJobs.length === 0) {
        tbody.innerHTML =
            '<tr><td colspan="3" style="text-align:center; padding: 20px; color: var(--text-dim);">No cron jobs found</td></tr>';
        return;
    }

    const role = localStorage.getItem('role');

    currentCronJobs.forEach((job, index) => {
        const tr = document.createElement('tr');
        tr.style.borderBottom = '1px solid rgba(255,255,255,0.05)';

        let actionsHtml = '';
        if (role === 'admin') {
            actionsHtml = `<button onclick="deleteCronJob(${index})" class="btn btn-danger btn-sm">Delete</button>`;
        } else {
            actionsHtml = '<span style="color: var(--text-dim); font-size: 0.8rem;">Read-only</span>';
        }

        const scheduleHtml =
            role === 'admin'
                ? `<input type="text" value="${job.schedule}" onchange="updateCronJob(${index}, 'schedule', this.value)" class="mono-text cron-schedule" style="background: transparent; border: 1px solid rgba(255,255,255,0.1); color: var(--text-main); padding: 4px 8px; width: 100%;">`
                : `<span class="cron-schedule">${job.schedule}</span>`;

        const commandHtml =
            role === 'admin'
                ? `<input type="text" value="${job.command}" onchange="updateCronJob(${index}, 'command', this.value)" class="mono-text" style="background: transparent; border: 1px solid rgba(255,255,255,0.1); color: var(--text-main); padding: 4px 8px; width: 100%;">`
                : `<span class="mono-text">${job.command}</span>`;

        tr.innerHTML = `
                    <td style="padding: 10px;">${scheduleHtml}</td>
                    <td style="padding: 10px;">${commandHtml}</td>
                    <td style="padding: 10px;">${actionsHtml}</td>
                `;
        tbody.appendChild(tr);
    });
}

function addCronJob() {
    if (!Array.isArray(currentCronJobs)) {
        currentCronJobs = [];
    }
    currentCronJobs.push({ schedule: '* * * * *', command: 'echo "New Job"' });
    renderCronJobs();
}

async function deleteCronJob(index) {
    if (await appConfirm('Delete this job?')) {
        currentCronJobs.splice(index, 1);
        renderCronJobs();
    }
}

function updateCronJob(index, field, value) {
    currentCronJobs[index][field] = value;
}

async function saveCronJobs() {
    if (!await appConfirm('Save changes to crontab? This will overwrite the current crontab.')) return;

    try {
        const response = await fetch('/api/cron', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(currentCronJobs),
        });

        if (response.ok) {
            appSuccess('Crontab saved successfully!');
            loadCronJobs();
        } else {
            const data = await response.json();
            appError('Error saving crontab: ' + (data.error || 'Unknown error'));
        }
    } catch (error) {
        appError('Failed to save crontab: ' + error.message);
    }
}

// Users
async function loadUsers() {
    try {
        const response = await fetch('/api/users');
        if (response.ok) {
            const data = await response.json();
            const tbody = document.getElementById('users-table-body');
            tbody.innerHTML = '';
            const currentRole = localStorage.getItem('role');
            const isAdmin = currentRole === 'admin';
            data.users.forEach((user) => {
                const tr = document.createElement('tr');
                tr.style.borderBottom = '1px solid rgba(255,255,255,0.05)';
                const roleClass = user.role === 'admin' ? 'admin' : 'user';
                tr.innerHTML = `
                            <td style="padding: 10px;">${user.username}</td>
                            <td style="padding: 10px;"><span class="role-badge ${roleClass}">${user.role}</span></td>
                            <td style="padding: 10px;">${new Date(user.created_at).toLocaleString()}</td>
                            <td style="padding: 10px;">${user.last_login ? new Date(user.last_login).toLocaleString() : '-'}</td>
                            <td style="padding: 10px;">
                                ${isAdmin && user.username !== 'admin' ? `<button onclick='showResetPasswordModal(${JSON.stringify(user.username)})' class="btn btn-success btn-sm" style="margin-right: 8px;">Reset Password</button>` : ''}
                                ${user.username !== 'admin' ? `<button onclick='handleDeleteUser(${JSON.stringify(user.username)})' class="btn btn-danger btn-sm">Delete</button>` : ''}
                            </td>
                        `;
                tbody.appendChild(tr);
            });
        }
    } catch (err) {
        console.error('Failed to load users', err);
    }
}

function showResetPasswordModal(username) {
    if (!username || username === 'admin') return;

    const modal = document.getElementById('reset-password-modal');
    const usernameField = document.getElementById('reset-password-username');
    const usernameText = document.getElementById('reset-password-username-text');
    const passwordInput = document.getElementById('reset-password-new');

    if (!modal || !usernameField || !usernameText || !passwordInput) {
        // Fallback: if modal is missing for any reason, try prompt.
        handleResetPassword(username);
        return;
    }

    usernameField.value = username;
    usernameText.textContent = username;
    passwordInput.value = '';
    modal.style.display = 'flex';
    setTimeout(() => passwordInput.focus(), 0);
}

function hideResetPasswordModal() {
    const modal = document.getElementById('reset-password-modal');
    const usernameField = document.getElementById('reset-password-username');
    const passwordInput = document.getElementById('reset-password-new');

    if (usernameField) usernameField.value = '';
    if (passwordInput) passwordInput.value = '';
    if (modal) modal.style.display = 'none';
}

async function handleResetPasswordSubmit(e) {
    e.preventDefault();

    const usernameField = document.getElementById('reset-password-username');
    const passwordInput = document.getElementById('reset-password-new');
    const username = usernameField ? usernameField.value : '';
    const newPassword = passwordInput ? passwordInput.value : '';

    if (!username || username === 'admin') return;
    if (!newPassword) return;

    const headers = { 'Content-Type': 'application/json' };

    try {
        const response = await fetch('/api/password', {
            method: 'POST',
            headers,
            body: JSON.stringify({ username, new_password: newPassword }),
        });

        if (response.ok) {
            hideResetPasswordModal();
            appSuccess('Password reset successfully');
            return;
        }

        const data = await response.json().catch(() => ({}));
        appError('Error: ' + (data.error || 'Failed to reset password'));
    } catch (err) {
        appError('Error resetting password');
    }
}

// Backward-compatible prompt-based flow (some deployments may not include the modal)
async function handleResetPassword(username) {
    if (!username || username === 'admin') return;
    const newPassword = prompt(`Set a new password for ${username}:`);
    if (!newPassword) return;

    const headers = { 'Content-Type': 'application/json' };

    try {
        const response = await fetch('/api/password', {
            method: 'POST',
            headers,
            body: JSON.stringify({ username, new_password: newPassword }),
        });

        if (response.ok) {
            appSuccess('Password reset successfully');
            return;
        }

        const data = await response.json().catch(() => ({}));
        appError('Error: ' + (data.error || 'Failed to reset password'));
    } catch (err) {
        appError('Error resetting password');
    }
}

function showAddUserModal() {
    document.getElementById('add-user-modal').style.display = 'flex';
}

async function handleAddUser(e) {
    e.preventDefault();
    const username = document.getElementById('new-username').value;
    const password = document.getElementById('new-password').value;
    const role = document.getElementById('new-role').value;

    try {
        const response = await fetch('/api/users', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ username, password, role }),
        });
        if (response.ok) {
            document.getElementById('add-user-modal').style.display = 'none';
            document.getElementById('new-username').value = '';
            document.getElementById('new-password').value = '';
            loadUsers();
            appSuccess('User created successfully');
        } else {
            const data = await response.json();
            appError('Error: ' + data.error);
        }
    } catch (err) {
        appError('Failed to create user');
    }
}

async function handleDeleteUser(username) {
    if (!await appConfirm(`Are you sure you want to delete user ${username}?`)) return;

    try {
        const response = await fetch(`/api/users?username=${username}`, {
            method: 'DELETE',
        });
        if (response.ok) {
            loadUsers();
        } else {
            appError('Failed to delete user');
        }
    } catch (err) {
        appError('Error deleting user');
    }
}

// Logs
async function loadLogs() {
    try {
        const response = await fetch('/api/logs');
        if (response.ok) {
            const data = await response.json();
            const tbody = document.getElementById('logs-table-body');
            tbody.innerHTML = '';
            data.logs.forEach((log) => {
                const tr = document.createElement('tr');
                tr.style.borderBottom = '1px solid rgba(255,255,255,0.05)';
                // Determine action class for styling
                const actionLower = (log.action || '').toLowerCase();
                let actionClass = '';
                if (actionLower.includes('login')) actionClass = 'login';
                else if (actionLower.includes('logout')) actionClass = 'logout';
                else if (actionLower.includes('delete') || actionLower.includes('remove') || actionLower.includes('kill')) actionClass = 'delete';
                else if (actionLower.includes('create') || actionLower.includes('add')) actionClass = 'create';
                else if (actionLower.includes('update') || actionLower.includes('change') || actionLower.includes('modify') || actionLower.includes('reset')) actionClass = 'modify';
                tr.innerHTML = `
                            <td style="padding: 10px; font-size: 0.9rem;">${new Date(log.time).toLocaleString()}</td>
                            <td style="padding: 10px;">${log.username}</td>
                            <td style="padding: 10px;"><span class="log-action ${actionClass}">${log.action}</span></td>
                            <td style="padding: 10px; color: var(--text-dim);">${log.details}</td>
                            <td style="padding: 10px; font-size: 0.9rem;" class="mono-text">${log.ip_address}</td>
                        `;
                tbody.appendChild(tr);
            });
        }
    } catch (err) {
        console.error('Failed to load logs', err);
    }
}

// Profile
function loadProfile() {
    const username = localStorage.getItem('username') || '-';
    const role = localStorage.getItem('role') || '-';

    document.getElementById('profile-username').innerText = username;
    document.getElementById('profile-role').innerText = role;
    
    // Load font preference
    const fontSelect = document.getElementById('font-select');
    if (fontSelect) {
        const savedFont = localStorage.getItem('fontFamily') || 'jetbrains-mono';
        fontSelect.value = savedFont;
    }

    // Show admin card and load settings if user is admin
    const adminCard = document.getElementById('system-admin-card');
    if (adminCard) {
        if (role === 'admin') {
            adminCard.style.display = 'block';
            loadSystemSettings();
        } else {
            adminCard.style.display = 'none';
        }
    }

    // Load profile data from server
    loadProfileData();
    loadProfileSessions();
    loadProfileLoginHistory();
    loadProfilePreferences();
}

// Load full profile data including security info and permissions
async function loadProfileData() {
    try {
        const response = await fetch('/api/profile');
        if (!response.ok) return;

        const data = await response.json();

        // Update security info
        const lastPwdChange = document.getElementById('profile-last-pwd-change');
        const lastFailedLogin = document.getElementById('profile-last-failed-login');
        
        if (lastPwdChange) {
            lastPwdChange.innerText = data.last_password_change 
                ? new Date(data.last_password_change).toLocaleString() 
                : 'Never';
        }
        if (lastFailedLogin) {
            if (data.last_failed_login) {
                const time = new Date(data.last_failed_login).toLocaleString();
                const ip = data.last_failed_login_ip || 'Unknown IP';
                lastFailedLogin.innerHTML = `${time} <span style="color: var(--text-dim); font-size: 0.8rem;">(${ip})</span>`;
            } else {
                lastFailedLogin.innerText = 'None';
            }
        }

        // Render permissions
        renderPermissions(data.permissions);
    } catch (e) {
        console.error('Failed to load profile data:', e);
    }
}

// Render role and permissions card
function renderPermissions(permissions) {
    const container = document.getElementById('profile-permissions-container');
    if (!container || !permissions) return;

    const role = localStorage.getItem('role') || 'user';
    const roleColors = {
        admin: 'var(--accent-cpu)',
        user: 'var(--accent-mem)'
    };

    let html = `
        <div style="display: flex; gap: 30px; flex-wrap: wrap;">
            <div style="flex: 1; min-width: 200px;">
                <div style="display: flex; align-items: center; gap: 12px; margin-bottom: 15px;">
                    <div style="width: 50px; height: 50px; background: ${roleColors[role] || 'var(--accent-cpu)'}; border-radius: 10px; display: flex; align-items: center; justify-content: center;">
                        <i class="fas ${role === 'admin' ? 'fa-crown' : 'fa-user'}" style="color: white; font-size: 1.2rem;"></i>
                    </div>
                    <div>
                        <div style="font-size: 1.2rem; font-weight: bold; text-transform: capitalize;">${role}</div>
                        <div style="color: var(--text-dim); font-size: 0.85rem;">${permissions.description || ''}</div>
                    </div>
                </div>
            </div>
            <div style="flex: 2; min-width: 300px;">
                <div style="font-size: 0.9rem; color: var(--text-dim); margin-bottom: 10px;">Permission Matrix</div>
                <div style="display: grid; grid-template-columns: repeat(auto-fill, minmax(160px, 1fr)); gap: 8px;">
    `;

    // Icon mapping based on permission text keywords
    const getPermIcon = (text) => {
        const lower = text.toLowerCase();
        if (lower.includes('docker')) return 'fa-docker';
        if (lower.includes('container')) return 'fa-box';
        if (lower.includes('service') || lower.includes('systemd')) return 'fa-cogs';
        if (lower.includes('cron') || lower.includes('job')) return 'fa-clock';
        if (lower.includes('process')) return 'fa-microchip';
        if (lower.includes('user')) return 'fa-users-cog';
        if (lower.includes('log')) return 'fa-file-alt';
        if (lower.includes('metric') || lower.includes('monitor')) return 'fa-chart-line';
        if (lower.includes('setting') || lower.includes('config')) return 'fa-sliders-h';
        if (lower.includes('password')) return 'fa-key';
        if (lower.includes('view') || lower.includes('read')) return 'fa-eye';
        if (lower.includes('kill') || lower.includes('stop')) return 'fa-power-off';
        if (lower.includes('create') || lower.includes('add')) return 'fa-plus-circle';
        if (lower.includes('delete') || lower.includes('remove')) return 'fa-trash-alt';
        if (lower.includes('edit') || lower.includes('modify')) return 'fa-edit';
        if (lower.includes('admin')) return 'fa-crown';
        return 'fa-check';
    };

    // Render "Can Do" permissions
    if (permissions.can_do && permissions.can_do.length > 0) {
        html += `<div style="grid-column: 1 / -1; font-size: 0.8rem; color: #2ed573; margin-top: 5px; font-weight: 500;"><i class="fas fa-check-circle"></i> Allowed Actions</div>`;
        permissions.can_do.forEach(perm => {
            const icon = getPermIcon(perm);
            html += `
                <div style="display: flex; align-items: center; gap: 8px; padding: 8px 12px; background: rgba(46, 213, 115, 0.1); border-radius: 6px; border: 1px solid rgba(46, 213, 115, 0.2);">
                    <i class="fas ${icon}" style="color: #2ed573; font-size: 0.85rem; flex-shrink: 0;"></i>
                    <span style="font-size: 0.8rem; line-height: 1.3;">${perm}</span>
                </div>
            `;
        });
    }

    // Render "Cannot Do" permissions
    if (permissions.cannot_do && permissions.cannot_do.length > 0) {
        html += `<div style="grid-column: 1 / -1; font-size: 0.8rem; color: #ff4757; margin-top: 10px; font-weight: 500;"><i class="fas fa-times-circle"></i> Restricted Actions</div>`;
        permissions.cannot_do.forEach(perm => {
            const icon = getPermIcon(perm);
            html += `
                <div style="display: flex; align-items: center; gap: 8px; padding: 8px 12px; background: rgba(255, 71, 87, 0.08); border-radius: 6px; border: 1px solid rgba(255, 71, 87, 0.15);">
                    <i class="fas ${icon}" style="color: #ff4757; font-size: 0.85rem; opacity: 0.7; flex-shrink: 0;"></i>
                    <span style="font-size: 0.8rem; line-height: 1.3; opacity: 0.7;">${perm}</span>
                </div>
            `;
        });
    }

    // Fallback for old API format (allowed/denied)
    if (permissions.allowed) {
        permissions.allowed.forEach(perm => {
            const icon = getPermIcon(perm);
            html += `
                <div style="display: flex; align-items: center; gap: 8px; padding: 8px 12px; background: rgba(46, 213, 115, 0.1); border-radius: 6px; border: 1px solid rgba(46, 213, 115, 0.2);">
                    <i class="fas ${icon}" style="color: #2ed573; font-size: 0.85rem;"></i>
                    <span style="font-size: 0.85rem;">${perm.replace(/_/g, ' ')}</span>
                </div>
            `;
        });
    }
    if (permissions.denied) {
        permissions.denied.forEach(perm => {
            const icon = getPermIcon(perm);
            html += `
                <div style="display: flex; align-items: center; gap: 8px; padding: 8px 12px; background: rgba(255, 71, 87, 0.1); border-radius: 6px; border: 1px solid rgba(255, 71, 87, 0.2);">
                    <i class="fas ${icon}" style="color: #ff4757; font-size: 0.85rem;"></i>
                    <span style="font-size: 0.85rem; text-decoration: line-through; opacity: 0.7;">${perm.replace(/_/g, ' ')}</span>
                </div>
            `;
        });
    }

    html += '</div></div></div>';
    container.innerHTML = html;
}

// Load active sessions
async function loadProfileSessions() {
    const container = document.getElementById('profile-sessions-container');
    if (!container) return;

    try {
        const response = await fetch('/api/profile/sessions');
        if (!response.ok) throw new Error('Failed to load sessions');

        const data = await response.json();
        const sessions = data.sessions || [];

        if (sessions.length === 0) {
            container.innerHTML = '<div style="padding: 20px; text-align: center; color: var(--text-dim);">No active sessions</div>';
            return;
        }

        // Show revoke button if more than 1 session
        const revokeBtn = document.getElementById('btn-revoke-others');
        if (revokeBtn) {
            revokeBtn.style.display = sessions.length > 1 ? 'inline-block' : 'none';
        }

        let html = '';
        sessions.forEach(session => {
            const isCurrentSession = session.is_current;
            const createdAt = new Date(session.created_at).toLocaleString();
            const lastActive = session.last_active ? new Date(session.last_active).toLocaleString() : createdAt;
            const deviceClass = isCurrentSession ? 'current' : 'other';

            html += `
                <div style="display: flex; align-items: center; gap: 15px; padding: 15px; background: rgba(0,0,0,0.2); border-radius: 8px; ${isCurrentSession ? 'border: 1px solid var(--accent-mem);' : ''}">
                    <div style="width: 45px; height: 45px; background: rgba(255,255,255,0.08); border-radius: 10px; display: flex; align-items: center; justify-content: center;">
                        <i class="fas ${getDeviceIcon(session.device_type)}" style="color: var(--text-dim); font-size: 1.1rem;"></i>
                    </div>
                    <div style="flex: 1;">
                        <div style="display: flex; align-items: center; gap: 10px; margin-bottom: 4px;">
                            <span class="session-device ${deviceClass}" style="font-weight: bold;">${session.browser || 'Unknown Browser'}</span>
                            ${isCurrentSession ? '<span style="background: var(--accent-mem); color: white; padding: 2px 8px; border-radius: 10px; font-size: 0.7rem;">Current</span>' : ''}
                        </div>
                        <div style="font-size: 0.85rem; color: var(--text-dim);">
                            ${session.os || 'Unknown OS'} • <span class="mono-text">${session.ip_address || 'Unknown IP'}</span>
                        </div>
                        <div style="font-size: 0.8rem; color: var(--text-dim); margin-top: 4px;">
                            Last active: ${lastActive}
                        </div>
                    </div>
                    ${!isCurrentSession ? `
                        <button onclick="revokeSession('${session.session_id}')" class="btn btn-outline-danger btn-sm">
                            <i class="fas fa-sign-out-alt"></i>
                        </button>
                    ` : ''}
                </div>
            `;
        });

        container.innerHTML = html;
    } catch (e) {
        console.error('Failed to load sessions:', e);
        container.innerHTML = '<div style="padding: 20px; text-align: center; color: var(--text-dim);">Failed to load sessions</div>';
    }
}

function getDeviceIcon(deviceType) {
    switch (deviceType) {
        case 'desktop': return 'fa-desktop';
        case 'mobile': return 'fa-mobile-alt';
        case 'tablet': return 'fa-tablet-alt';
        default: return 'fa-globe';
    }
}

// Revoke a specific session
async function revokeSession(sessionId) {
    if (!await appConfirm('Are you sure you want to logout this session?')) return;

    try {
        const response = await fetch(`/api/profile/sessions?id=${encodeURIComponent(sessionId)}`, {
            method: 'DELETE'
        });

        if (response.ok) {
            loadProfileSessions();
        } else {
            const data = await response.json();
            appError('Failed to revoke session: ' + (data.error || 'Unknown error'));
        }
    } catch (e) {
        console.error('Failed to revoke session:', e);
        appError('Failed to revoke session');
    }
}

// Revoke all other sessions
async function revokeOtherSessions() {
    if (!await appConfirm('Are you sure you want to logout all other sessions? This will sign out all devices except this one.')) return;

    try {
        const response = await fetch('/api/profile/sessions', {
            method: 'DELETE'
        });

        if (response.ok) {
            loadProfileSessions();
            appSuccess('All other sessions have been logged out');
        } else {
            const data = await response.json();
            appError('Failed to revoke sessions: ' + (data.error || 'Unknown error'));
        }
    } catch (e) {
        console.error('Failed to revoke sessions:', e);
        appError('Failed to revoke sessions');
    }
}

// Load login history
async function loadProfileLoginHistory() {
    const container = document.getElementById('profile-login-history-container');
    if (!container) return;

    try {
        const response = await fetch('/api/profile/login-history');
        if (!response.ok) throw new Error('Failed to load login history');

        const data = await response.json();
        const records = data.records || [];

        if (records.length === 0) {
            container.innerHTML = '<div style="padding: 20px; text-align: center; color: var(--text-dim);">No login history</div>';
            return;
        }

        let html = `
            <table style="width: 100%; border-collapse: collapse; font-size: 0.9rem;">
                <thead>
                    <tr style="border-bottom: 1px solid rgba(255,255,255,0.1);">
                        <th style="text-align: left; padding: 10px; color: var(--text-dim); font-weight: normal;">Time</th>
                        <th style="text-align: left; padding: 10px; color: var(--text-dim); font-weight: normal;">IP Address</th>
                        <th style="text-align: left; padding: 10px; color: var(--text-dim); font-weight: normal;">Device</th>
                        <th style="text-align: left; padding: 10px; color: var(--text-dim); font-weight: normal;">Status</th>
                    </tr>
                </thead>
                <tbody>
        `;

        records.forEach(record => {
            const time = new Date(record.time).toLocaleString();
            const statusColor = record.success ? '#2ed573' : '#ff4757';
            const statusIcon = record.success ? 'fa-check-circle' : 'fa-times-circle';
            const statusText = record.success ? 'Success' : 'Failed';

            html += `
                <tr style="border-bottom: 1px solid rgba(255,255,255,0.05);">
                    <td style="padding: 12px 10px;">${time}</td>
                    <td style="padding: 12px 10px;" class="mono-text">${record.ip_address || '-'}</td>
                    <td style="padding: 12px 10px;">
                        <div>${record.browser || 'Unknown'}</div>
                        <div style="font-size: 0.8rem; color: var(--text-dim);">${record.os || ''}</div>
                    </td>
                    <td style="padding: 12px 10px;">
                        <span style="display: inline-flex; align-items: center; gap: 6px; color: ${statusColor};">
                            <i class="fas ${statusIcon}"></i> ${statusText}
                        </span>
                    </td>
                </tr>
            `;
        });

        html += '</tbody></table>';
        container.innerHTML = html;
    } catch (e) {
        console.error('Failed to load login history:', e);
        container.innerHTML = '<div style="padding: 20px; text-align: center; color: var(--text-dim);">Failed to load login history</div>';
    }
}

// Load user preferences
async function loadProfilePreferences() {
    try {
        const response = await fetch('/api/profile/preferences');
        if (!response.ok) return;

        const data = await response.json();
        const prefs = data.preferences;
        if (!prefs) return;

        // Alert subscriptions
        if (prefs.alert_preferences) {
            const alerts = prefs.alert_preferences;
            setCheckbox('pref-alert-cpu', alerts.cpu_alerts);
            setCheckbox('pref-alert-memory', alerts.memory_alerts);
            setCheckbox('pref-alert-disk', alerts.disk_alerts);
            setCheckbox('pref-alert-network', alerts.network_alerts);
            setCheckbox('pref-alert-docker', alerts.docker_alerts);

            // Notification methods
            setCheckbox('pref-notify-panel', alerts.notify_panel);
            setCheckbox('pref-notify-email', alerts.notify_email);
            setCheckbox('pref-notify-webhook', alerts.notify_webhook);

            // Quiet hours
            setCheckbox('pref-dnd-enabled', alerts.quiet_hours_enabled);
            setValue('pref-dnd-start', alerts.quiet_hours_start || '22:00');
            setValue('pref-dnd-end', alerts.quiet_hours_end || '08:00');
        }
    } catch (e) {
        console.error('Failed to load preferences:', e);
    }
}

function setCheckbox(id, value) {
    const el = document.getElementById(id);
    if (el) el.checked = !!value;
}

function setValue(id, value) {
    const el = document.getElementById(id);
    if (el && value) el.value = value;
}

// Save alert preferences
async function saveProfileAlertPreferences() {
    const prefs = {
        alert_preferences: {
            cpu_alerts: document.getElementById('pref-alert-cpu')?.checked || false,
            memory_alerts: document.getElementById('pref-alert-memory')?.checked || false,
            disk_alerts: document.getElementById('pref-alert-disk')?.checked || false,
            network_alerts: document.getElementById('pref-alert-network')?.checked || false,
            docker_alerts: document.getElementById('pref-alert-docker')?.checked || false,
            notify_panel: document.getElementById('pref-notify-panel')?.checked || false,
            notify_email: document.getElementById('pref-notify-email')?.checked || false,
            notify_webhook: document.getElementById('pref-notify-webhook')?.checked || false,
            quiet_hours_enabled: document.getElementById('pref-dnd-enabled')?.checked || false,
            quiet_hours_start: document.getElementById('pref-dnd-start')?.value || '22:00',
            quiet_hours_end: document.getElementById('pref-dnd-end')?.value || '08:00'
        }
    };

    try {
        const response = await fetch('/api/profile/preferences', {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(prefs)
        });

        if (response.ok) {
            // Visual feedback
            const card = document.querySelector('#page-profile .card:last-child');
            if (card) {
                const oldBorder = card.style.borderColor;
                card.style.borderColor = 'var(--accent-mem)';
                setTimeout(() => { card.style.borderColor = oldBorder; }, 500);
            }
        } else {
            const data = await response.json();
            appError('Failed to save preferences: ' + (data.error || 'Unknown error'));
        }
    } catch (e) {
        console.error('Failed to save preferences:', e);
        appError('Failed to save preferences');
    }
}

// Load system settings from server
async function loadSystemSettings() {
    try {
        const response = await fetch('/api/settings');
        if (!response.ok) return;
        
        const data = await response.json();
        const select = document.getElementById('monitoring-mode-select');
        if (select && data.monitoringMode) {
            select.value = data.monitoringMode;
        }
    } catch (e) {
        console.error('Failed to load system settings:', e);
    }
}

// Handle monitoring mode change
async function handleMonitoringModeChange() {
    const select = document.getElementById('monitoring-mode-select');
    if (!select) return;
    
    const mode = select.value;
    
    try {
        const response = await fetch('/api/settings', {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ monitoringMode: mode })
        });
        
        if (!response.ok) {
            const data = await response.json();
            appError('Failed to save: ' + (data.error || 'Unknown error'));
            // Revert to previous value
            loadSystemSettings();
            return;
        }
        
        // Show brief confirmation
        const card = document.getElementById('system-admin-card');
        if (card) {
            const oldBg = card.style.borderColor;
            card.style.borderColor = 'var(--accent-mem)';
            setTimeout(() => { card.style.borderColor = oldBg; }, 500);
        }
    } catch (e) {
        console.error('Failed to update settings:', e);
        appError('Failed to save settings');
        loadSystemSettings();
    }
}

// Font switching
function handleFontChange() {
    const fontSelect = document.getElementById('font-select');
    if (!fontSelect) return;
    
    const font = fontSelect.value;
    localStorage.setItem('fontFamily', font);
    applyFont(font);
}

function applyFont(font) {
    const root = document.documentElement;
    if (font === 'intel-one-mono') {
        root.setAttribute('data-font', 'intel-one-mono');
    } else {
        root.removeAttribute('data-font');
    }
}

// Initialize font on page load
function initFont() {
    const savedFont = localStorage.getItem('fontFamily') || 'jetbrains-mono';
    applyFont(savedFont);
}

async function handleChangePassword(e) {
    e.preventDefault();
    const oldPassword = document.getElementById('cp-old').value;
    const newPassword = document.getElementById('cp-new').value;

    try {
        const response = await fetch('/api/password', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ old_password: oldPassword, new_password: newPassword }),
        });

        if (response.ok) {
            appSuccess('Password changed successfully');
            document.getElementById('change-password-form').reset();
        } else {
            const data = await response.json();
            appError('Error: ' + (data.error || 'Failed to change password'));
        }
    } catch (err) {
        appError('Error changing password');
    }
}

// Role-dependent UI & alerts/power
function checkRole() {
    const role = localStorage.getItem('role');
    if (role === 'admin') {
        const navUsers = document.getElementById('nav-users');
        const navLogs = document.getElementById('nav-logs');
        if (navUsers) navUsers.style.display = 'flex';
        if (navLogs) navLogs.style.display = 'flex';
    }
    loadGuiStatus();
    
    if (window.systemInfoPromise) {
        window.systemInfoPromise.then(info => {
             if (!info.enabled_modules || info.enabled_modules.power !== false) {
                 loadPowerProfile();
             }
        });
    } else {
        loadPowerProfile();
    }

    loadAlerts();
}

function loadPowerProfile() {
    fetch('/api/power/profile')
        .then((res) => res.json())
        .then((data) => {
            const card = document.getElementById('power-profile-card');
            const container = document.getElementById('power-profile-controls');
            const hint = document.getElementById('power-profile-hint');
            if (!card || !container || !hint) return;
            card.style.display = 'block';
            container.innerHTML = '';

            const role = localStorage.getItem('role');
            const isAdmin = role === 'admin';
            const supported = data && data.supported !== false;
            const profiles = (data && data.available) || [];
            const hasProfiles = profiles.length > 0;

            const errMsg = data && data.error ? data.error : (supported ? '' : 'powerprofilesctl not detected or not supported on this device');
            if (errMsg) {
                hint.style.display = 'block';
                hint.textContent = errMsg;
            } else {
                hint.style.display = 'none';
                hint.textContent = '';
            }

            if (!supported || !hasProfiles) {
                const msg = document.createElement('div');
                msg.style.color = 'var(--text-dim)';
                msg.style.fontSize = '0.9rem';
                msg.textContent = 'No power profiles detected';
                container.appendChild(msg);
                return;
            }

            // Profile display name mapping
            const profileNames = {
                'performance': '🚀 Performance',
                'balanced': '⚖️ Balanced',
                'power-saver': '🔋 Power Saver'
            };

            profiles.forEach((profile) => {
                const btn = document.createElement('button');
                btn.className = 'power-profile-btn' + (profile === data.current ? ' active' : '');
                btn.innerText = profileNames[profile] || profile;
                btn.title = profile === data.current ? 'Current mode' : (isAdmin ? 'Click to switch' : 'Admin only');

                if (isAdmin) {
                    btn.onclick = () => setPowerProfile(profile);
                } else {
                    btn.disabled = true;
                    if (profile !== data.current) {
                        btn.style.opacity = '0.5';
                    }
                }

                container.appendChild(btn);
            });

            // Add admin hint for non-admins
            if (!isAdmin && hasProfiles) {
                const hint = document.createElement('span');
                hint.style.fontSize = '0.8rem';
                hint.style.color = 'var(--text-dim)';
                hint.style.marginLeft = '8px';
                hint.innerText = '(Admin only)';
                container.appendChild(hint);
            }
        })
        .catch((err) => console.error('Failed to load power profile:', err));
}

function setPowerProfile(profile) {
    fetch('/api/power/profile', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
        },
        body: JSON.stringify({ profile: profile }),
    })
        .then((res) => {
            if (res.ok) {
                loadPowerProfile();
            } else {
                appError('Failed to set power profile');
            }
        })
        .catch((err) => console.error('Error setting profile:', err));
}

function loadGuiStatus() {
    fetch('/api/gui/status')
        .then((res) => res.json())
        .then((data) => {
            const statusEl = document.getElementById('gui-status-text');
            const btn = document.getElementById('gui-toggle-btn');
            const targetEl = document.getElementById('gui-default-target');
            const adminHint = document.getElementById('gui-admin-hint');
            if (!statusEl || !btn) return;

            const role = localStorage.getItem('role');
            const isAdmin = role === 'admin';

            if (!data || data.supported === false) {
                statusEl.innerText = 'Display manager not detected';
                statusEl.className = 'gui-status-badge';
                btn.style.display = 'none';
                if (targetEl) targetEl.style.display = 'none';
                if (adminHint) adminHint.style.display = 'none';
                return;
            }

            statusEl.innerText = data.running ? '✓ Running' : '○ Stopped';
            statusEl.className = 'gui-status-badge ' + (data.running ? 'running' : 'stopped');

            if (targetEl) {
                if (data.default_target) {
                    targetEl.style.display = 'inline-flex';
                    // Friendly target names
                    const targetMap = {
                        'graphical.target': '🖥️ Graphical',
                        'multi-user.target': '💻 Multi-User'
                    };
                    targetEl.innerText = targetMap[data.default_target] || data.default_target;
                    targetEl.title = 'System default target: ' + data.default_target;
                } else {
                    targetEl.style.display = 'none';
                }
            }

            btn.style.display = isAdmin ? 'inline-flex' : 'none';
            btn.textContent = data.running ? '⏹ Stop GUI' : '▶ Start GUI';
            btn.title = data.running ? 'Stop display-manager service' : 'Start display-manager service';
            btn.dataset.nextAction = data.running ? 'stop' : 'start';

            if (adminHint) {
                adminHint.style.display = isAdmin ? 'none' : 'inline';
            }
        })
        .catch((err) => console.error('Failed to load GUI status:', err));
}

function toggleGuiSession() {
    const btn = document.getElementById('gui-toggle-btn');
    if (!btn || !btn.dataset.nextAction) return;
    const action = btn.dataset.nextAction;

    fetch('/api/gui/action', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ action }),
    })
        .then((res) => {
            if (!res.ok) return res.json().then((d) => Promise.reject(d.error || 'Failed'));
            return res.json();
        })
        .then(() => loadGuiStatus())
        .catch((err) => appError('Failed: ' + err));
}

function loadAlerts() {
    fetch('/api/alerts')
        .then((res) => res.json())
        .then((data) => {
            document.getElementById('alert-enabled').checked = data.enabled;
            document.getElementById('alert-webhook').value = data.webhook_url || '';
            document.getElementById('alert-cpu').value = data.cpu_threshold || 90;
            document.getElementById('alert-mem').value = data.mem_threshold || 90;
            document.getElementById('alert-disk').value = data.disk_threshold || 90;
        })
        .catch((err) => console.error('Failed to load alerts:', err));
}

function saveAlerts() {
    const config = {
        enabled: document.getElementById('alert-enabled').checked,
        webhook_url: document.getElementById('alert-webhook').value,
        cpu_threshold: parseFloat(document.getElementById('alert-cpu').value),
        mem_threshold: parseFloat(document.getElementById('alert-mem').value),
        disk_threshold: parseFloat(document.getElementById('alert-disk').value),
    };

    fetch('/api/alerts', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
        },
        body: JSON.stringify(config),
    })
        .then((res) => {
            if (res.ok) {
                appSuccess('Alert configuration saved');
            } else {
                appError('Failed to save alert configuration');
            }
        })
        .catch((err) => console.error('Error saving alerts:', err));
}

document.addEventListener('DOMContentLoaded', () => {
    const loadMoreBtn = document.getElementById('docker-logs-load-more');
    if (loadMoreBtn) {
        loadMoreBtn.addEventListener('click', (e) => {
            e.preventDefault();
            loadMoreDockerLogs();
        });
    }
});

// Fetch static info once
window.systemInfoPromise = fetch('/api/info')
    .then((response) => response.json());

window.systemInfoPromise.then(async (data) => {
        // Store system info globally for ASCII logo detection
        window.systemInfo = data;

        // Render ASCII logo (async)
        const asciiLogoEl = document.getElementById('ascii-logo');
        if (asciiLogoEl) {
            // Avoid a blank area if the logo script failed to load
            if (!asciiLogoEl.innerHTML) asciiLogoEl.textContent = 'Loading...';

            if (typeof getFormattedASCIILogo === 'function') {
                try {
                    asciiLogoEl.innerHTML = await getFormattedASCIILogo();
                } catch (error) {
                    console.warn('Failed to render ASCII logo:', error);
                    asciiLogoEl.textContent = 'Linux';
                }
            } else {
                // Script not loaded / cached stale page
                asciiLogoEl.textContent = 'Linux';
            }
        }

        // Render system information
        const infoList = document.getElementById('sys-info-list');
        infoList.innerHTML = `
            <div style="color: var(--accent-cpu); font-weight: bold; margin-bottom: 5px;">${data.header}</div>
            <div style="color: var(--text-dim); margin-bottom: 10px;">--------------------------</div>
            <div><span class="sys-key">OS:</span><span class="sys-val">${data.os}</span></div>
            <div><span class="sys-key">Kernel:</span><span class="sys-val">${data.kernel}</span></div>
            <div><span class="sys-key">Uptime:</span><span class="sys-val">${data.uptime}</span></div>
            <div><span class="sys-key">Shell:</span><span class="sys-val">${data.shell}</span></div>
            <div><span class="sys-key">CPU:</span><span class="sys-val">${data.cpu}</span></div>
            <div><span class="sys-key">GPU:</span><span class="sys-val">${data.gpu}</span></div>
            <div><span class="sys-key">Memory:</span><span class="sys-val">${data.memory}</span></div>
            <div><span class="sys-key">Swap:</span><span class="sys-val">${data.swap}</span></div>
            <div><span class="sys-key">Disk (/):</span><span class="sys-val">${data.disk}</span></div>
            <div><span class="sys-key">Local IP:</span><span class="sys-val">${data.ip}</span></div>
            <div><span class="sys-key">Locale:</span><span class="sys-val">${data.locale}</span></div>
        `;

        if (data.enabled_modules) {
            updateUIForModules(data.enabled_modules);
        }
    });

function updateUIForModules(modules) {
    const hideSidebarItem = (onclickPattern) => {
        const el = document.querySelector(`.nav-item[onclick*="${onclickPattern}"]`);
        if (el) el.style.display = 'none';
    };

    const hideCard = (childId) => {
        const el = document.getElementById(childId);
        if (el) {
            const card = el.closest('.card');
            if (card) card.style.display = 'none';
        }
    };

    if (modules.cpu === false) {
        hideSidebarItem("switchPage('cpu')");
        hideCard('cpu-freq');
    }
    if (modules.memory === false) {
        hideSidebarItem("switchPage('memory')");
        hideCard('mem-text');
    }
    if (modules.disk === false) {
        hideSidebarItem("switchPage('storage')");
        hideCard('disk-container');
    }
    if (modules.gpu === false) {
        hideSidebarItem("switchPage('gpu')");
        hideCard('general-gpu-name');
    }
    if (modules.network === false) {
        hideSidebarItem("toggleNetworkSubmenu()");
        hideCard('net-interface-select');
    }
    if (modules.ssh === false) {
        hideSidebarItem("switchPage('ssh')");
        hideCard('general-ssh-status');
    }
    if (modules.docker === false) {
        hideSidebarItem("switchPage('docker')");
    }
    if (modules.systemd === false) {
        hideSidebarItem("switchPage('services')");
    }
    if (modules.cron === false) {
        hideSidebarItem("switchPage('cron')");
    }
    if (modules.power === false) {
        const powerCard = document.getElementById('power-profile-card');
        if (powerCard) powerCard.style.display = 'none';
    }
    if (modules.sensors === false) {
        hideCard('sensors-container');
    }
    if (modules.system === false) {
        hideSidebarItem("switchPage('processes')");
    }
}

// ============================================================================
//  NEW ALERTS SYSTEM
// ============================================================================

// Alert state
let alertRules = [];
let alertConfig = null;
let alertHistoryOffset = 0;
const ALERT_HISTORY_LIMIT = 20;

// Load all alert data
async function loadAlertsPage() {
    try {
        await Promise.all([
            loadAlertSummary(),
            loadAlertRules(),
            loadAlertConfig(),
            loadActiveAlerts(),
            loadAlertHistory()
        ]);
    } catch (e) {
        console.error('Failed to load alerts page:', e);
    }
}

// Load summary
async function loadAlertSummary() {
    try {
        const res = await fetch('/api/alerts/summary');
        if (!res.ok) return;
        const data = await res.json();
        
        document.getElementById('alerts-total-rules').textContent = data.total_rules || 0;
        document.getElementById('alerts-enabled-rules').textContent = data.enabled_rules || 0;
        document.getElementById('alerts-firing-count').textContent = data.firing_alerts || 0;
        document.getElementById('alerts-today-events').textContent = data.today_events || 0;
        
        // Update firing badge visibility
        const firingCount = data.firing_alerts || 0;
        const badge = document.getElementById('active-alerts-badge');
        if (badge) {
            badge.textContent = firingCount;
            badge.style.display = firingCount > 0 ? 'inline-block' : 'none';
        }
    } catch (e) {
        console.error('Failed to load alert summary:', e);
    }
}

// Load rules
async function loadAlertRules() {
    try {
        const res = await fetch('/api/alerts/rules');
        if (!res.ok) return;
        alertRules = await res.json();
        renderAlertRules();
    } catch (e) {
        console.error('Failed to load alert rules:', e);
    }
}

// Render rules table
function renderAlertRules() {
    const tbody = document.getElementById('rules-tbody');
    if (!tbody) return;
    
    if (!alertRules || alertRules.length === 0) {
        tbody.innerHTML = '<tr><td colspan="7" class="empty-state"><i class="fa-solid fa-inbox"></i> No rules</td></tr>';
        return;
    }
    
    const metricNames = {
        'cpu': 'CPU',
        'memory': 'Memory',
        'disk': 'Disk',
        'swap': 'Swap',
        'load1': 'Load (1m)',
        'load5': 'Load (5m)',
        'load15': 'Load (15m)'
    };
    
    tbody.innerHTML = alertRules.map(rule => `
        <tr>
            <td>
                <label class="switch switch-sm">
                    <input type="checkbox" ${rule.enabled ? 'checked' : ''} 
                           onchange="toggleAlertRule('${rule.id}', this.checked)">
                    <span class="slider round"></span>
                </label>
            </td>
            <td>
                <div class="rule-name">${escapeHtml(rule.name)}</div>
                ${rule.description ? `<div class="rule-desc">${escapeHtml(rule.description)}</div>` : ''}
            </td>
            <td>${metricNames[rule.metric] || rule.metric}</td>
            <td class="condition">${rule.operator} ${rule.threshold}${rule.metric.startsWith('load') ? '' : '%'}</td>
            <td>${rule.duration || '1m'}</td>
            <td><span class="badge badge-${rule.severity}">${rule.severity}</span></td>
            <td>
                <button class="btn btn-xs btn-secondary" onclick="editAlertRule('${rule.id}')" title="Edit">
                    <i class="fa-solid fa-pen"></i>
                </button>
                ${!rule.builtin ? `
                <button class="btn btn-xs btn-danger" onclick="deleteAlertRule('${rule.id}')" title="Delete">
                    <i class="fa-solid fa-trash"></i>
                </button>
                ` : ''}
            </td>
        </tr>
    `).join('');
}

// Toggle rule enable/disable
async function toggleAlertRule(ruleId, enabled) {
    try {
        const action = enabled ? 'enable' : 'disable';
        const res = await fetch(`/api/alerts/rules/${ruleId}/${action}`, { method: 'POST' });
        if (!res.ok) {
            const msg = await readErrorMessage(res);
            throw new Error(msg);
        }
        appSuccess(`Rule ${enabled ? 'enabled' : 'disabled'}`);
        loadAlertSummary();
    } catch (e) {
        appError('Operation failed: ' + e.message);
        loadAlertRules(); // Reload to reset checkbox
    }
}

// Edit rule (placeholder - could open modal)
function editAlertRule(ruleId) {
    const rule = alertRules.find(r => r.id === ruleId);
    if (!rule) return;
    
    // For now, just show an alert with rule details
    // In a full implementation, this would open an edit modal
    appAlert(
        `<div style="text-align: left;">
            <p><strong>Rule Name:</strong> ${escapeHtml(rule.name)}</p>
            <p><strong>Metric:</strong> ${rule.metric}</p>
            <p><strong>Condition:</strong> ${rule.operator} ${rule.threshold}</p>
            <p><strong>Duration:</strong> ${rule.duration}</p>
            <p><strong>Severity:</strong> ${rule.severity}</p>
            <p style="color: var(--text-dim); font-size: 0.85rem; margin-top: 10px;">
                Note: Edit feature coming soon
            </p>
        </div>`,
        { title: `Edit Rule: ${rule.name}` }
    );
}

// Delete rule
async function deleteAlertRule(ruleId) {
    const rule = alertRules.find(r => r.id === ruleId);
    if (!rule) return;
    
    const confirmed = await appConfirm(`Delete rule "${rule.name}"?`);
    if (!confirmed) return;
    
    try {
        const res = await fetch(`/api/alerts/rules/${ruleId}`, { method: 'DELETE' });
        if (!res.ok) {
            const msg = await readErrorMessage(res);
            throw new Error(msg);
        }
        appSuccess('Rule deleted');
        loadAlertRules();
        loadAlertSummary();
    } catch (e) {
        appError('Delete failed: ' + e.message);
    }
}

// Enable preset
async function enablePreset(presetId) {
    try {
        const res = await fetch(`/api/alerts/presets/${presetId}/enable`, { method: 'POST' });
        if (!res.ok) {
            const msg = await readErrorMessage(res);
            throw new Error(msg);
        }
        appSuccess('Preset enabled');
        loadAlertRules();
        loadAlertSummary();
    } catch (e) {
        appError('Enable failed: ' + e.message);
    }
}

// Disable all rules
async function disableAllAlertRules() {
    const confirmed = await appConfirm('Disable all alert rules?');
    if (!confirmed) return;
    
    try {
        const res = await fetch('/api/alerts/disable-all', { method: 'POST' });
        if (!res.ok) {
            const msg = await readErrorMessage(res);
            throw new Error(msg);
        }
        appSuccess('All rules disabled');
        loadAlertRules();
        loadAlertSummary();
        loadActiveAlerts();
    } catch (e) {
        appError('Operation failed: ' + e.message);
    }
}

// Load config
async function loadAlertConfig() {
    try {
        const res = await fetch('/api/alerts/config');
        if (!res.ok) return;
        alertConfig = await res.json();
        
        // Populate UI
        const globalEnabled = document.getElementById('alerts-global-enabled');
        if (globalEnabled) globalEnabled.checked = alertConfig.enabled;
        
        const notifyResolved = document.getElementById('notify-on-resolved');
        if (notifyResolved) notifyResolved.checked = alertConfig.notify_on_resolved !== false;
        
        // Parse channels
        const channels = alertConfig.channels || [];
        
        const dashboardCh = channels.find(c => c.type === 'dashboard');
        const webhookCh = channels.find(c => c.type === 'webhook');
        
        const dashboardEl = document.getElementById('channel-dashboard');
        if (dashboardEl) dashboardEl.checked = dashboardCh?.enabled || false;
        
        const webhookEl = document.getElementById('channel-webhook');
        const webhookUrlEl = document.getElementById('webhook-url');
        if (webhookEl) webhookEl.checked = webhookCh?.enabled || false;
        if (webhookUrlEl) webhookUrlEl.value = webhookCh?.config?.url || '';
        
    } catch (e) {
        console.error('Failed to load alert config:', e);
    }
}

// Save config
async function saveAlertConfig() {
    try {
        const config = {
            enabled: document.getElementById('alerts-global-enabled')?.checked || false,
            notify_on_resolved: document.getElementById('notify-on-resolved')?.checked || false,
            global_silence_period: '5m',
            channels: [
                {
                    type: 'dashboard',
                    enabled: document.getElementById('channel-dashboard')?.checked || false,
                    config: {}
                },
                {
                    type: 'webhook',
                    enabled: document.getElementById('channel-webhook')?.checked || false,
                    config: {
                        url: document.getElementById('webhook-url')?.value || ''
                    }
                }
            ]
        };
        
        const res = await fetch('/api/alerts/config', {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(config)
        });
        
        if (!res.ok) {
            const msg = await readErrorMessage(res);
            throw new Error(msg);
        }
        
        appSuccess('Config saved');
    } catch (e) {
        appError('Save failed: ' + e.message);
    }
}

// Load active alerts
async function loadActiveAlerts() {
    try {
        const res = await fetch('/api/alerts/active');
        if (!res.ok) return;
        const alerts = await res.json();
        
        const container = document.getElementById('active-alerts-list');
        if (!container) return;
        
        if (!alerts || alerts.length === 0) {
            container.innerHTML = `
                <div class="empty-state">
                    <i class="fa-solid fa-check-circle" style="color: var(--accent-mem);"></i>
                    <span>No active alerts</span>
                </div>
            `;
            return;
        }
        
        container.innerHTML = alerts.map(alert => `
            <div class="alert-item ${alert.severity}">
                <div class="alert-item-icon">
                    ${alert.severity === 'critical' ? '🔴' : '🟡'}
                </div>
                <div class="alert-item-content">
                    <div class="alert-item-name">${escapeHtml(alert.rule_name)}</div>
                    <div class="alert-item-detail">
                        ${alert.metric}: <span class="alert-item-value">${alert.value.toFixed(1)}${alert.metric.startsWith('load') ? '' : '%'}</span>
                        ${alert.operator} ${alert.threshold}${alert.metric.startsWith('load') ? '' : '%'}
                    </div>
                </div>
                <div class="alert-item-duration">${alert.duration}</div>
            </div>
        `).join('');
        
    } catch (e) {
        console.error('Failed to load active alerts:', e);
    }
}

// Load alert history
async function loadAlertHistory(reset = true) {
    try {
        if (reset) alertHistoryOffset = 0;
        
        const filter = document.getElementById('history-filter')?.value || '';
        let url = `/api/alerts/history?limit=${ALERT_HISTORY_LIMIT}&offset=${alertHistoryOffset}`;
        if (filter) url += `&status=${filter}`;
        
        const res = await fetch(url);
        if (!res.ok) return;
        const data = await res.json();
        
        const container = document.getElementById('alert-history-list');
        const pagination = document.getElementById('history-pagination');
        if (!container) return;
        
        if (!data.events || data.events.length === 0) {
            if (reset) {
                container.innerHTML = `
                    <div class="empty-state">
                        <i class="fa-solid fa-inbox"></i>
                        <span>No history records</span>
                    </div>
                `;
            }
            if (pagination) pagination.style.display = 'none';
            return;
        }
        
        const html = data.events.map(event => {
            const time = new Date(event.fired_at);
            const timeStr = time.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit' });
            const statusIcon = event.status === 'firing' ? '🔴' : '✅';
            
            return `
                <div class="history-item ${event.status}">
                    <div class="history-item-time">${timeStr}</div>
                    <div class="history-item-status">${statusIcon}</div>
                    <div class="history-item-name">${escapeHtml(event.rule_name)}</div>
                    <div class="history-item-value">${event.value.toFixed(1)}${event.metric.startsWith('load') ? '' : '%'}</div>
                </div>
            `;
        }).join('');
        
        if (reset) {
            container.innerHTML = html;
        } else {
            container.insertAdjacentHTML('beforeend', html);
        }
        
        // Show/hide pagination
        if (pagination) {
            pagination.style.display = data.total > alertHistoryOffset + data.events.length ? 'block' : 'none';
        }
        
    } catch (e) {
        console.error('Failed to load alert history:', e);
    }
}

// Load more history
function loadMoreHistory() {
    alertHistoryOffset += ALERT_HISTORY_LIMIT;
    loadAlertHistory(false);
}

// Test notification
async function testAlertNotification(channel = 'webhook') {
    try {
        const res = await fetch(`/api/alerts/test?channel=${channel}`, { method: 'POST' });
        if (!res.ok) {
            const msg = await readErrorMessage(res);
            throw new Error(msg);
        }
        appSuccess('Test notification sent');
    } catch (e) {
        appError('Send failed: ' + e.message);
    }
}

// Helper: escape HTML
function escapeHtml(str) {
    if (!str) return '';
    return str.replace(/&/g, '&amp;')
              .replace(/</g, '&lt;')
              .replace(/>/g, '&gt;')
              .replace(/"/g, '&quot;')
              .replace(/'/g, '&#039;');
}
