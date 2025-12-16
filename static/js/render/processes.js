function setProcessSort(sortType) {
    currentProcessSort = sortType;

    document.querySelectorAll('.sort-btn').forEach((btn) => btn.classList.remove('active'));
    // Relies on browser-provided global `event` for inline onclick handlers
    event.target.closest('.sort-btn').classList.add('active');

    localStorage.setItem('processSortMode', sortType);

    filterAndRenderProcesses();
}

function filterAndRenderProcesses() {
    const searchInput = document.getElementById('proc-search');
    currentProcessSearch = (searchInput ? searchInput.value : '').toLowerCase();

    let filtered = allProcesses.filter((proc) => {
        if (!currentProcessSearch) return true;
        const searchStr = currentProcessSearch;
        return (
            proc.name.toLowerCase().includes(searchStr) ||
            proc.pid.toString().includes(searchStr) ||
            (proc.username && proc.username.toLowerCase().includes(searchStr))
        );
    });

    if (currentProcessSort === 'tree') {
        renderProcessTree(filtered);
    } else {
        filtered.sort((a, b) => {
            let valA, valB;
            switch (currentProcessSort) {
                case 'cpu':
                    valA = a.cpu_percent || 0;
                    valB = b.cpu_percent || 0;
                    return valB - valA;
                case 'threads':
                    valA = a.num_threads || 0;
                    valB = b.num_threads || 0;
                    return valB - valA;
                case 'uptime':
                    valA = a.uptime_seconds || 0;
                    valB = b.uptime_seconds || 0;
                    return valB - valA;
                case 'memory':
                default:
                    valA = a.memory_percent || 0;
                    valB = b.memory_percent || 0;
                    return valB - valA;
            }
        });
        renderProcessList(filtered);
    }

    updateProcessStats();
}

function renderProcessList(processes) {
    const container = document.getElementById('process-container');
    container.innerHTML = '';

    if (processes.length === 0) {
        container.innerHTML = '<div style="padding: 20px; text-align: center; color: var(--text-dim);">No processes found</div>';
        return;
    }

    processes.forEach((proc) => {
        const div = document.createElement('div');
        div.className = 'process-item level-0';
        div.style.cursor = 'pointer';

        const cpuPercent = (proc.cpu_percent || 0).toFixed(1);
        const memPercent = (proc.memory_percent || 0).toFixed(1);
        const uptime = proc.uptime || '-';

        const role = localStorage.getItem('role');
        const isAdmin = role === 'admin';

        div.innerHTML = `
            <div class="process-info">
                <div class="process-name">[${proc.pid}] ${proc.name}</div>
                <div class="process-meta">${proc.username || '-'} • ${proc.num_threads || 0} threads • ${uptime}</div>
            </div>
            <div class="process-bars">
                <div style="display: flex; gap: 15px; align-items: center;">
                    <div style="min-width: 80px;">
                        <div style="font-size: 0.75rem; color: var(--text-dim);">CPU</div>
                        <div class="progress-bg" style="height: 4px;">
                            <div class="progress-fill" style="width: ${cpuPercent}%; background: var(--accent-cpu);"></div>
                        </div>
                        <div style="font-size: 0.75rem; color: var(--accent-cpu);">${cpuPercent}%</div>
                    </div>
                    <div style="min-width: 80px;">
                        <div style="font-size: 0.75rem; color: var(--text-dim);">Memory</div>
                        <div class="progress-bg" style="height: 4px;">
                            <div class="progress-fill" style="width: ${memPercent}%; background: var(--accent-mem);"></div>
                        </div>
                        <div style="font-size: 0.75rem; color: var(--accent-mem);">${memPercent}%</div>
                    </div>
                    ${isAdmin && proc.pid > 1 ? `
                    <div style="margin-left: 10px; display: flex; align-items: center;">
                        <button onclick="event.stopPropagation(); handleKillProcess(${proc.pid});" style="background: var(--accent-cpu); border: none; color: white; padding: 4px 10px; border-radius: 6px; cursor: pointer; font-size: 0.8rem;" title="Kill process">
                            <i class=\"fas fa-times\" style=\"margin-right: 6px;\"></i>Kill
                        </button>
                    </div>
                    ` : ''}
                </div>
            </div>
        `;

        div.addEventListener('click', () => showProcDetail(proc.pid));
        container.appendChild(div);
    });
}

function renderProcessTree(processes) {
    const container = document.getElementById('process-container');
    container.innerHTML = '';

    if (processes.length === 0) {
        container.innerHTML = '<div style="padding: 20px; text-align: center; color: var(--text-dim);">No processes found</div>';
        return;
    }

    const pidMap = {};
    processes.forEach((p) => {
        pidMap[p.pid] = p;
    });

    const rootProcesses = processes.filter((p) => !pidMap[p.ppid]);

    rootProcesses.forEach((root) => {
        renderProcessTreeNode(root, processes, pidMap, 0, container);
    });
}

function renderProcessTreeNode(proc, allProcs, pidMap, level, container) {
    const div = document.createElement('div');
    div.className = `process-item level-${Math.min(level, 4)}`;
    div.style.cursor = 'pointer';
    div.style.marginLeft = level * 20 + 'px';

    const cpuPercent = (proc.cpu_percent || 0).toFixed(1);
    const memPercent = (proc.memory_percent || 0).toFixed(1);
    const uptime = proc.uptime || '-';

    const role = localStorage.getItem('role');
    const isAdmin = role === 'admin';

    const hasChildren = proc.children && proc.children.length > 0;
    const expandIcon = hasChildren
        ? '<i class="fas fa-chevron-right" style="margin-right: 5px; transition: transform 0.2s;"></i>'
        : '<span style="margin-right: 5px; display: inline-block; width: 16px;"></span>';

    div.innerHTML = `
        <div class="process-info" style="display: flex; gap: 10px; flex-grow: 1;">
            <div style="display: flex; align-items: center;">${expandIcon}</div>
            <div style="flex: 1;">
                <div class="process-name">[${proc.pid}] ${proc.name}</div>
                <div class="process-meta">${proc.username || '-'} • ${proc.num_threads || 0} threads • ${uptime}</div>
            </div>
        </div>
        <div class="process-bars" style="display: flex; gap: 15px; align-items: center;">
            <div style="min-width: 70px;">
                <div class="progress-bg" style="height: 4px;">
                    <div class="progress-fill" style="width: ${cpuPercent}%; background: var(--accent-cpu);"></div>
                </div>
                <div style="font-size: 0.75rem; color: var(--accent-cpu);">${cpuPercent}%</div>
            </div>
            <div style="min-width: 70px;">
                <div class="progress-bg" style="height: 4px;">
                    <div class="progress-fill" style="width: ${memPercent}%; background: var(--accent-mem);"></div>
                </div>
                <div style="font-size: 0.75rem; color: var(--accent-mem);">${memPercent}%</div>
            </div>
            ${isAdmin && proc.pid > 1 ? `
            <div style="margin-left: 10px; display: flex; align-items: center;">
                <button onclick="event.stopPropagation(); handleKillProcess(${proc.pid});" style="background: var(--accent-cpu); border: none; color: white; padding: 4px 10px; border-radius: 6px; cursor: pointer; font-size: 0.8rem;" title="Kill process">
                    <i class=\"fas fa-times\" style=\"margin-right: 6px;\"></i>Kill
                </button>
            </div>
            ` : ''}
        </div>
    `;

    div.addEventListener('click', (e) => {
        if (hasChildren && e.target.closest('.fas')) {
            const childrenContainer = div.nextElementSibling;
            if (childrenContainer && childrenContainer.classList.contains('children-group')) {
                childrenContainer.style.display = childrenContainer.style.display === 'none' ? '' : 'none';
                const icon = div.querySelector('.fas');
                if (icon) icon.style.transform = childrenContainer.style.display === 'none' ? '' : 'rotate(90deg)';
            }
        } else {
            showProcDetail(proc.pid);
        }
    });

    container.appendChild(div);

    if (hasChildren && proc.children) {
        const childrenContainer = document.createElement('div');
        childrenContainer.className = 'children-group';

        proc.children.forEach((child) => {
            renderProcessTreeNode(child, allProcs, pidMap, level + 1, childrenContainer);
        });

        container.appendChild(childrenContainer);
    }
}

function showProcDetail(pid) {
    const proc = allProcesses.find((p) => p.pid === pid);
    if (!proc) return;

    const modal = document.getElementById('proc-detail-modal');
    const titleEl = document.getElementById('proc-detail-name');
    const contentEl = document.getElementById('proc-detail-content');

    const role = localStorage.getItem('role');
    const isAdmin = role === 'admin';

    titleEl.innerText = `${proc.name} (PID: ${proc.pid})`;

    contentEl.innerHTML = `
        <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 15px;">
            <div>
                <div style="font-size: 0.8rem; color: var(--text-dim);">Process ID</div>
                <div style="font-weight: bold; color: var(--accent-cpu);">${proc.pid}</div>
            </div>
            <div>
                <div style="font-size: 0.8rem; color: var(--text-dim);">Parent PID</div>
                <div style="font-weight: bold; color: var(--accent-cpu);">${proc.ppid || '-'}</div>
            </div>
            <div>
                <div style="font-size: 0.8rem; color: var(--text-dim);">User</div>
                <div style="font-weight: bold;">${proc.username || '-'}</div>
            </div>
            <div>
                <div style="font-size: 0.8rem; color: var(--text-dim);">Threads</div>
                <div style="font-weight: bold; color: var(--accent-mem);">${proc.num_threads || '-'}</div>
            </div>
            <div>
                <div style="font-size: 0.8rem; color: var(--text-dim);">CPU Usage</div>
                <div style="font-weight: bold; color: var(--accent-cpu);">${(proc.cpu_percent || 0).toFixed(1)}%</div>
            </div>
            <div>
                <div style="font-size: 0.8rem; color: var(--text-dim);">Memory Usage</div>
                <div style="font-weight: bold; color: var(--accent-mem);">${(proc.memory_percent || 0).toFixed(1)}%</div>
            </div>
            <div>
                <div style="font-size: 0.8rem; color: var(--text-dim);">Uptime</div>
                <div style="font-weight: bold;">${proc.uptime || '-'}</div>
            </div>
            <div>
                <div style="font-size: 0.8rem; color: var(--text-dim);">Status</div>
                <div style="font-weight: bold;">${proc.status || '-'}</div>
            </div>
        </div>
        <hr style="border: none; border-top: 1px solid rgba(255,255,255,0.1); margin: 15px 0;">
        <div>
            <div style="font-size: 0.8rem; color: var(--text-dim); margin-bottom: 5px;">Command Line</div>
            <div style="background: rgba(0,0,0,0.3); padding: 10px; border-radius: 4px; font-family: monospace; font-size: 0.85rem; word-break: break-all; color: #90ee90;">
                ${(proc.cmdline || '-').replace(/</g, '&lt;').replace(/>/g, '&gt;')}
            </div>
        </div>
        <div style="margin-top: 15px;">
            <div style="font-size: 0.8rem; color: var(--text-dim); margin-bottom: 5px;">Working Directory</div>
            <div style="background: rgba(0,0,0,0.3); padding: 10px; border-radius: 4px; font-family: monospace; font-size: 0.85rem; word-break: break-all;">
                ${(proc.cwd || '-').replace(/</g, '&lt;').replace(/>/g, '&gt;')}
            </div>
        </div>
        <div id="proc-io-container" style="margin-top: 15px; display: grid; grid-template-columns: 1fr 1fr; gap: 10px;">
            <div>
                <div style="font-size: 0.8rem; color: var(--text-dim);">I/O Read</div>
                <div id="proc-io-read" style="font-weight: bold; color: var(--accent-net);">Loading...</div>
            </div>
            <div>
                <div style="font-size: 0.8rem; color: var(--text-dim);">I/O Write</div>
                <div id="proc-io-write" style="font-weight: bold; color: var(--accent-net);">Loading...</div>
            </div>
        </div>

        ${isAdmin && proc.pid > 1 ? `
        <div style="margin-top: 18px; display: flex; justify-content: flex-end;">
            <button onclick="handleKillProcess(${proc.pid});" style="background: var(--accent-cpu); border: none; color: white; padding: 6px 12px; border-radius: 6px; cursor: pointer; font-size: 0.9rem;" title="Kill process">
                <i class="fas fa-times" style="margin-right: 6px;"></i>Kill Process
            </button>
        </div>
        ` : ''}
    `;

    modal.style.display = 'flex';

    // Lazy-load IO data (reduces CPU overhead by ~30-40%)
    fetchProcessIO(proc.pid);
}

async function fetchProcessIO(pid) {
    const ioReadEl = document.getElementById('proc-io-read');
    const ioWriteEl = document.getElementById('proc-io-write');

    try {
        const response = await fetch(`/api/process/io?pid=${pid}`);
        if (!response.ok) {
            ioReadEl.innerText = '-';
            ioWriteEl.innerText = '-';
            return;
        }
        const data = await response.json();
        ioReadEl.innerText = data.io_read || '-';
        ioWriteEl.innerText = data.io_write || '-';
    } catch (err) {
        ioReadEl.innerText = '-';
        ioWriteEl.innerText = '-';
    }
}

function closeProcDetail() {
    document.getElementById('proc-detail-modal').style.display = 'none';
}

async function handleKillProcess(pid) {
    const role = localStorage.getItem('role');
    if (role !== 'admin') {
        alert('Forbidden: Admin access required');
        return;
    }

    const proc = allProcesses.find((p) => p.pid === pid);
    const name = proc ? proc.name : 'unknown';
    if (!confirm(`Kill process ${name} (PID: ${pid})? This cannot be undone.`)) return;

    try {
        const response = await fetch('/api/process/kill', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ pid }),
        });

        if (response.ok) {
            closeProcDetail();
            alert('Kill signal sent. The process list will refresh shortly.');
            setTimeout(() => {
                try {
                    filterAndRenderProcesses();
                } catch (_) {}
            }, 500);
            return;
        }

        const data = await response.json();
        alert('Error: ' + (data.error || 'Kill failed'));
    } catch (err) {
        alert('Failed to kill process');
    }
}

function updateProcessStats() {
    if (allProcesses.length === 0) {
        document.getElementById('proc-total').innerText = '0';
        document.getElementById('proc-threads').innerText = '0';
        document.getElementById('proc-avg-cpu').innerText = '0%';
        document.getElementById('proc-avg-mem').innerText = '0%';
        return;
    }

    const totalThreads = allProcesses.reduce((sum, p) => sum + (p.num_threads || 0), 0);
    const avgCpu = allProcesses.reduce((sum, p) => sum + (p.cpu_percent || 0), 0) / allProcesses.length;
    const avgMem = allProcesses.reduce((sum, p) => sum + (p.memory_percent || 0), 0) / allProcesses.length;

    document.getElementById('proc-total').innerText = allProcesses.length;
    document.getElementById('proc-threads').innerText = totalThreads;
    document.getElementById('proc-avg-cpu').innerText = avgCpu.toFixed(1) + '%';
    document.getElementById('proc-avg-mem').innerText = avgMem.toFixed(1) + '%';
}

window.addEventListener('DOMContentLoaded', function () {
    const saved = localStorage.getItem('processSortMode');
    if (saved) {
        currentProcessSort = saved;
        const btn = document.querySelector(`.sort-btn[onclick="setProcessSort('${saved}')"]`);
        if (btn) {
            document.querySelectorAll('.sort-btn').forEach((b) => b.classList.remove('active'));
            btn.classList.add('active');
        }
    }

    const modal = document.getElementById('proc-detail-modal');
    if (modal) {
        document.addEventListener('keydown', function (e) {
            if (e.key === 'Escape' && modal.style.display !== 'none') {
                closeProcDetail();
            }
        });

        modal.addEventListener('click', function (e) {
            if (e.target === modal) {
                closeProcDetail();
            }
        });
    }
});
