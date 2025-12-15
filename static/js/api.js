// Global data storage for filtering/sorting
let allDockerContainers = [];
let allDockerImages = [];
let allServices = [];

let dockerContainerSort = { column: 'name', direction: 'asc' };
let dockerImageSort = { column: 'created', direction: 'desc' };
let serviceSort = { column: 'unit', direction: 'asc' };

// Docker
async function loadDockerContainers() {
    try {
        const response = await fetch('/api/docker/containers');
        if (response.ok) {
            const data = await response.json();
            allDockerContainers = data.containers || [];
            renderDockerContainers();
        }
    } catch (err) {
        console.error('Failed to load containers', err);
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
            actionsHtml =
                c.State === 'running'
                    ? `<button onclick="handleDockerAction('${c.Id}', 'stop')" style="background: #ff6b6b; border: none; color: white; padding: 4px 8px; border-radius: 4px; cursor: pointer; margin-right: 5px;">Stop</button>
                         <button onclick="handleDockerAction('${c.Id}', 'restart')" style="background: var(--accent-net); border: none; color: white; padding: 4px 8px; border-radius: 4px; cursor: pointer;">Restart</button>`
                    : `<button onclick="handleDockerAction('${c.Id}', 'start')" style="background: var(--accent-mem); border: none; color: white; padding: 4px 8px; border-radius: 4px; cursor: pointer; margin-right: 5px;">Start</button>
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
        if (response.ok) {
            const data = await response.json();
            allDockerImages = data.images || [];
            renderDockerImages();
        }
    } catch (err) {
        console.error('Failed to load images', err);
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
                                ${isAdmin && user.username !== 'admin' ? `<button onclick="handleResetPassword(${JSON.stringify(user.username)})" style="background: var(--accent-mem); border: none; color: white; padding: 4px 8px; border-radius: 4px; cursor: pointer; font-size: 0.8rem; margin-right: 8px;">Reset Password</button>` : ''}
                                ${user.username !== 'admin' ? `<button onclick="handleDeleteUser(${JSON.stringify(user.username)})" style="background: var(--accent-cpu); border: none; color: white; padding: 4px 8px; border-radius: 4px; cursor: pointer; font-size: 0.8rem;">Delete</button>` : ''}
                            </td>
                        `;
                tbody.appendChild(tr);
            });
        }
    } catch (err) {
        console.error('Failed to load users', err);
    }
}

async function handleResetPassword(username) {
    if (!username || username === 'admin') return;
    const newPassword = prompt(`Set a new password for ${username}:`);
    if (!newPassword) return;

    const token = getAuthToken();
    const headers = { 'Content-Type': 'application/json' };
    if (token) {
        headers['Authorization'] = `Bearer ${token}`;
    }

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
        document.getElementById('nav-users').style.display = 'flex';
        document.getElementById('nav-logs').style.display = 'flex';
    }
    loadPowerProfile();
    loadAlerts();
}

function loadPowerProfile() {
    fetch('/api/power/profile', {
        headers: { Authorization: `Bearer ${getAuthToken()}` },
    })
        .then((res) => res.json())
        .then((data) => {
            if (data.error || !data.available || data.available.length === 0) {
                return;
            }

            const card = document.getElementById('power-profile-card');
            const container = document.getElementById('power-profile-controls');
            card.style.display = 'block';
            container.innerHTML = '';

            const role = localStorage.getItem('role');
            const isAdmin = role === 'admin';

            data.available.forEach((profile) => {
                const btn = document.createElement('button');
                btn.innerText = profile;
                btn.style.padding = '8px 16px';
                btn.style.borderRadius = '4px';
                btn.style.border = '1px solid rgba(255,255,255,0.2)';
                btn.style.cursor = isAdmin ? 'pointer' : 'default';
                btn.style.textTransform = 'capitalize';
                btn.style.fontWeight = 'bold';
                btn.style.transition = 'all 0.2s';

                if (profile === data.current) {
                    btn.style.background = 'var(--accent-cpu)';
                    btn.style.color = 'white';
                    btn.style.borderColor = 'var(--accent-cpu)';
                } else {
                    btn.style.background = 'rgba(255,255,255,0.05)';
                    btn.style.color = 'var(--text-dim)';
                }

                if (isAdmin) {
                    btn.onclick = () => setPowerProfile(profile);
                    btn.onmouseover = () => {
                        if (profile !== data.current) {
                            btn.style.background = 'rgba(255,255,255,0.1)';
                            btn.style.color = 'var(--text-main)';
                        }
                    };
                    btn.onmouseout = () => {
                        if (profile !== data.current) {
                            btn.style.background = 'rgba(255,255,255,0.05)';
                            btn.style.color = 'var(--text-dim)';
                        }
                    };
                } else {
                    btn.disabled = true;
                    btn.style.opacity = profile === data.current ? '1' : '0.5';
                }

                container.appendChild(btn);
            });
        })
        .catch((err) => console.error('Failed to load power profile:', err));
}

function setPowerProfile(profile) {
    fetch('/api/power/profile', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
            Authorization: `Bearer ${getAuthToken()}`,
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

function loadAlerts() {
    fetch('/api/alerts', {
        headers: { Authorization: `Bearer ${getAuthToken()}` },
    })
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
            Authorization: `Bearer ${getAuthToken()}`,
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

// Fetch static info once
fetch('/api/info')
    .then((response) => response.json())
    .then((data) => {
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
    });
