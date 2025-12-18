// Global data storage for filtering/sorting
let allDockerContainers = [];
let allDockerImages = [];
let allServices = [];

let dockerLogsState = { id: null, name: '', tail: 200 };

let dockerContainerSort = { column: 'name', direction: 'asc' };
let dockerImageSort = { column: 'created', direction: 'desc' };
let serviceSort = { column: 'unit', direction: 'asc' };

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
        const stateColor = c.State === 'running' ? 'var(--accent-mem)' : 'var(--text-dim)';

        let actionsHtml = '';
        if (role === 'admin') {
            const logsBtn = `<button onclick="openDockerLogs('${c.Id}', ${JSON.stringify(name)})" style="background: rgba(255,255,255,0.08); border: 1px solid rgba(255,255,255,0.15); color: white; padding: 4px 8px; border-radius: 4px; cursor: pointer; margin-right: 5px;">Logs</button>`;
            actionsHtml =
                c.State === 'running'
                    ? `${logsBtn}<button onclick="handleDockerAction('${c.Id}', 'stop')" style="background: #ff6b6b; border: none; color: white; padding: 4px 8px; border-radius: 4px; cursor: pointer; margin-right: 5px;">Stop</button>
                         <button onclick="handleDockerAction('${c.Id}', 'restart')" style="background: var(--accent-net); border: none; color: white; padding: 4px 8px; border-radius: 4px; cursor: pointer;">Restart</button>`
                    : `${logsBtn}<button onclick="handleDockerAction('${c.Id}', 'start')" style="background: var(--accent-mem); border: none; color: white; padding: 4px 8px; border-radius: 4px; cursor: pointer; margin-right: 5px;">Start</button>
                         <button onclick="handleDockerAction('${c.Id}', 'remove')" style="background: var(--text-dim); border: none; color: white; padding: 4px 8px; border-radius: 4px; cursor: pointer;">Remove</button>`;
        } else {
            actionsHtml = '<span style="color: var(--text-dim); font-size: 0.8rem;">Read-only</span>';
        }

        const tr = document.createElement('tr');
        tr.style.borderBottom = '1px solid rgba(255,255,255,0.05)';
        tr.innerHTML = `
                    <td style="padding: 10px; font-weight: bold;">${name}</td>
                    <td style="padding: 10px; color: var(--text-dim);">${c.Image}</td>
                    <td style="padding: 10px;"><span style="color: ${stateColor}">${c.State}</span></td>
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
            actionsHtml = `<button onclick="handleDockerImageRemove('${img.Id}')" style="background: #ff6b6b; border: none; color: white; padding: 4px 8px; border-radius: 4px; cursor: pointer;">Delete</button>`;
        } else {
            actionsHtml = '<span style="color: var(--text-dim); font-size: 0.8rem;">Read-only</span>';
        }

        const tr = document.createElement('tr');
        tr.style.borderBottom = '1px solid rgba(255,255,255,0.05)';
        tr.innerHTML = `
                    <td style="padding: 10px; font-family: monospace;">${id}</td>
                    <td style="padding: 10px;">${tags}</td>
                    <td style="padding: 10px;">${size}</td>
                    <td style="padding: 10px;">${created}</td>
                    <td style="padding: 10px;">${actionsHtml}</td>
                `;
        tbody.appendChild(tr);
    });
}

async function handleDockerImageRemove(imageId) {
    if (!confirm('Are you sure you want to delete this image?')) return;
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
            alert('Error: ' + (data.error || 'Delete failed'));
        }
    } catch (err) {
        alert('Failed to delete image');
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
    if (!confirm(`Are you sure you want to ${action} this container?`)) return;
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
            alert('Error: ' + (data.error || 'Action failed'));
        }
    } catch (err) {
        alert('Failed to perform action');
    }
}

function closeDockerLogsModal() {
    const modal = document.getElementById('docker-logs-modal');
    if (modal) modal.style.display = 'none';
}

async function openDockerLogs(id, name) {
    const role = localStorage.getItem('role');
    if (role !== 'admin') {
        alert('Docker logs are restricted to admin users.');
        return;
    }

    dockerLogsState = { id, name: name || id, tail: 200 };
    const modal = document.getElementById('docker-logs-modal');
    const title = document.getElementById('docker-logs-title');
    if (title) title.innerText = `Logs: ${dockerLogsState.name}`;
    if (modal) modal.style.display = 'flex';

    await fetchDockerLogs();
}

async function fetchDockerLogs() {
    const content = document.getElementById('docker-logs-content');
    if (content) content.textContent = 'Loading...';

    if (!dockerLogsState.id) return;

    try {
        const resp = await fetch(`/api/docker/logs?id=${encodeURIComponent(dockerLogsState.id)}&tail=${dockerLogsState.tail}`);
        if (!resp.ok) {
            const msg = await readErrorMessage(resp);
            if (content) content.textContent = `Failed to load logs: ${msg}`;
            return;
        }
        const data = await resp.json();
        if (content) content.textContent = data.logs || '';
    } catch (e) {
        if (content) content.textContent = 'Failed to load logs.';
    }
}

async function loadMoreDockerLogs() {
    if (!dockerLogsState.id) return;
    dockerLogsState.tail = Math.min(dockerLogsState.tail + 200, 2000);
    await fetchDockerLogs();
}

async function openDockerPruneConfirm() {
    if (!confirm('Prune will remove stopped containers, dangling images, unused networks, and build cache. Continue?')) return;
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
                alert(msg);
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
            alert(msg);
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

        let statusColor = '#888';
        if (svc.active === 'active') statusColor = '#2ed573';
        else if (svc.active === 'failed') statusColor = '#ff4757';

        let actionsHtml = '';
        if (role === 'admin') {
            const btnStyle =
                'border: none; color: white; padding: 4px 8px; border-radius: 4px; cursor: pointer; margin-right: 5px; font-size: 0.8rem;';
            if (svc.active === 'active') {
                actionsHtml += `<button onclick="handleServiceAction('${svc.unit}', 'stop')" style="${btnStyle} background: #ff6b6b;">Stop</button>`;
                actionsHtml += `<button onclick="handleServiceAction('${svc.unit}', 'restart')" style="${btnStyle} background: var(--accent-net);">Restart</button>`;
            } else {
                actionsHtml += `<button onclick="handleServiceAction('${svc.unit}', 'start')" style="${btnStyle} background: var(--accent-mem);">Start</button>`;
            }
        } else {
            actionsHtml = '<span style="color: var(--text-dim); font-size: 0.8rem;">Read-only</span>';
        }

        tr.innerHTML = `
                    <td style="padding: 10px; font-weight: bold;">${svc.unit}</td>
                    <td style="padding: 10px;">${svc.load}</td>
                    <td style="padding: 10px;"><span style="color: ${statusColor}">${svc.active}</span></td>
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
    if (!confirm(`Are you sure you want to ${action} service ${unit}?`)) return;

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
        alert(`Failed to ${action} service: ${error.message || error}`);
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
        currentCronJobs = await response.json();

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
            actionsHtml = `<button onclick="deleteCronJob(${index})" style="background: #ff6b6b; border: none; color: white; padding: 4px 8px; border-radius: 4px; cursor: pointer; font-size: 0.8rem;">Delete</button>`;
        } else {
            actionsHtml = '<span style="color: var(--text-dim); font-size: 0.8rem;">Read-only</span>';
        }

        const scheduleHtml =
            role === 'admin'
                ? `<input type="text" value="${job.schedule}" onchange="updateCronJob(${index}, 'schedule', this.value)" style="background: transparent; border: 1px solid rgba(255,255,255,0.1); color: var(--text-main); padding: 4px; width: 100%; font-family: monospace;">`
                : `<span style="font-family: monospace;">${job.schedule}</span>`;

        const commandHtml =
            role === 'admin'
                ? `<input type="text" value="${job.command}" onchange="updateCronJob(${index}, 'command', this.value)" style="background: transparent; border: 1px solid rgba(255,255,255,0.1); color: var(--text-main); padding: 4px; width: 100%; font-family: monospace;">`
                : `<span style="font-family: monospace;">${job.command}</span>`;

        tr.innerHTML = `
                    <td style="padding: 10px;">${scheduleHtml}</td>
                    <td style="padding: 10px;">${commandHtml}</td>
                    <td style="padding: 10px;">${actionsHtml}</td>
                `;
        tbody.appendChild(tr);
    });
}

function addCronJob() {
    currentCronJobs.push({ schedule: '* * * * *', command: 'echo "New Job"' });
    renderCronJobs();
}

function deleteCronJob(index) {
    if (confirm('Delete this job?')) {
        currentCronJobs.splice(index, 1);
        renderCronJobs();
    }
}

function updateCronJob(index, field, value) {
    currentCronJobs[index][field] = value;
}

async function saveCronJobs() {
    if (!confirm('Save changes to crontab? This will overwrite the current crontab.')) return;

    try {
        const response = await fetch('/api/cron', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(currentCronJobs),
        });

        if (response.ok) {
            alert('Crontab saved successfully!');
            loadCronJobs();
        } else {
            const data = await response.json();
            alert('Error saving crontab: ' + (data.error || 'Unknown error'));
        }
    } catch (error) {
        alert('Failed to save crontab: ' + error.message);
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
                tr.innerHTML = `
                            <td style="padding: 10px;">${user.username}</td>
                            <td style="padding: 10px;"><span style="background: ${user.role === 'admin' ? 'var(--accent-cpu)' : 'var(--accent-net)'}; padding: 2px 6px; border-radius: 4px; font-size: 0.8rem;">${user.role}</span></td>
                            <td style="padding: 10px;">${new Date(user.created_at).toLocaleString()}</td>
                            <td style="padding: 10px;">${user.last_login ? new Date(user.last_login).toLocaleString() : '-'}</td>
                            <td style="padding: 10px;">
                                ${isAdmin && user.username !== 'admin' ? `<button onclick='showResetPasswordModal(${JSON.stringify(user.username)})' style="background: var(--accent-mem); border: none; color: white; padding: 4px 8px; border-radius: 4px; cursor: pointer; font-size: 0.8rem; margin-right: 8px;">Reset Password</button>` : ''}
                                ${user.username !== 'admin' ? `<button onclick='handleDeleteUser(${JSON.stringify(user.username)})' style="background: var(--accent-cpu); border: none; color: white; padding: 4px 8px; border-radius: 4px; cursor: pointer; font-size: 0.8rem;">Delete</button>` : ''}
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
            alert('Password reset successfully');
            return;
        }

        const data = await response.json().catch(() => ({}));
        alert('Error: ' + (data.error || 'Failed to reset password'));
    } catch (err) {
        alert('Error resetting password');
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
            alert('Password reset successfully');
            return;
        }

        const data = await response.json().catch(() => ({}));
        alert('Error: ' + (data.error || 'Failed to reset password'));
    } catch (err) {
        alert('Error resetting password');
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
            alert('User created successfully');
        } else {
            const data = await response.json();
            alert('Error: ' + data.error);
        }
    } catch (err) {
        alert('Failed to create user');
    }
}

async function handleDeleteUser(username) {
    if (!confirm(`Are you sure you want to delete user ${username}?`)) return;

    try {
        const response = await fetch(`/api/users?username=${username}`, {
            method: 'DELETE',
        });
        if (response.ok) {
            loadUsers();
        } else {
            alert('Failed to delete user');
        }
    } catch (err) {
        alert('Error deleting user');
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
                tr.innerHTML = `
                            <td style="padding: 10px; font-size: 0.9rem;">${new Date(log.time).toLocaleString()}</td>
                            <td style="padding: 10px;">${log.username}</td>
                            <td style="padding: 10px;">${log.action}</td>
                            <td style="padding: 10px; color: var(--text-dim);">${log.details}</td>
                            <td style="padding: 10px; font-size: 0.9rem;">${log.ip_address}</td>
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
            alert('Password changed successfully');
            document.getElementById('change-password-form').reset();
        } else {
            const data = await response.json();
            alert('Error: ' + (data.error || 'Failed to change password'));
        }
    } catch (err) {
        alert('Error changing password');
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
                alert('Failed to set power profile');
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
        .catch((err) => alert('Failed: ' + err));
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
                alert('Alert configuration saved');
            } else {
                alert('Failed to save alert configuration');
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

window.systemInfoPromise.then((data) => {
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
