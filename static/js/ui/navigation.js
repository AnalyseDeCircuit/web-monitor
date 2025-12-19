function toggleMobileMenu() {
    const sidebar = document.getElementById('sidebar');
    const mobileBtn = document.getElementById('mobile-menu-btn');

    sidebar.classList.toggle('mobile-open');
    mobileBtn.classList.toggle('active');
}

function closeMobileMenu() {
    const sidebar = document.getElementById('sidebar');
    const mobileBtn = document.getElementById('mobile-menu-btn');

    if (window.innerWidth <= 768) {
        sidebar.classList.remove('mobile-open');
        mobileBtn.classList.remove('active');
    }
}

function toggleSidebar() {
    const sidebar = document.getElementById('sidebar');
    const toggle = document.getElementById('sidebar-toggle');
    sidebar.classList.toggle('collapsed');

    const isCollapsed = sidebar.classList.contains('collapsed');
    localStorage.setItem('sidebarCollapsed', isCollapsed);

    if (isCollapsed) {
        toggle.innerHTML = '<i class="fas fa-chevron-right"></i>';
    } else {
        toggle.innerHTML = '<i class="fas fa-chevron-left"></i>';
    }
}

window.addEventListener('DOMContentLoaded', function () {
    // Apply saved font preference immediately
    if (typeof initFont === 'function') {
        initFont();
    }
    
    checkRole();
    initPlugins();

    const isCollapsed = localStorage.getItem('sidebarCollapsed') === 'true';
    if (isCollapsed) {
        const sidebar = document.getElementById('sidebar');
        sidebar.classList.add('collapsed');
        const toggle = document.getElementById('sidebar-toggle');
        toggle.innerHTML = '<i class="fas fa-chevron-right"></i>';
    }

    window.addEventListener('resize', () => {
        if (window.innerWidth > 768) {
            closeMobileMenu();
        }
    });
});

function toggleNetworkSubmenu() {
    const submenu = document.getElementById('network-submenu');
    const icon = document.getElementById('net-submenu-icon');
    if (submenu.style.display === 'none') {
        submenu.style.display = 'flex';
        icon.classList.remove('fa-chevron-down');
        icon.classList.add('fa-chevron-up');
    } else {
        submenu.style.display = 'none';
        icon.classList.remove('fa-chevron-up');
        icon.classList.add('fa-chevron-down');
    }
}

function togglePluginsSubmenu() {
    const submenu = document.getElementById('plugins-submenu');
    const icon = document.getElementById('plugins-submenu-icon');
    if (submenu.style.display === 'none') {
        submenu.style.display = 'flex';
        icon.classList.remove('fa-chevron-down');
        icon.classList.add('fa-chevron-up');
    } else {
        submenu.style.display = 'none';
        icon.classList.remove('fa-chevron-up');
        icon.classList.add('fa-chevron-down');
    }
}

function switchPage(pageId) {
    closeMobileMenu();

    document.querySelectorAll('.page').forEach((el) => el.classList.remove('active'));
    
    // For plugin pages, use the shared plugin-container page
    const targetPageId = pageId.startsWith('plugin-') ? 'plugin-container' : pageId;
    const pageEl = document.getElementById('page-' + targetPageId);
    if (pageEl) {
        pageEl.classList.add('active');
    }

    document.querySelectorAll('.nav-item').forEach((el) => el.classList.remove('active'));

    const navItem = document.querySelector(`.nav-item[onclick*="'${pageId}'"]`);
    if (navItem) {
        navItem.classList.add('active');
        if (['net-traffic', 'ssh'].includes(pageId)) {
            document.getElementById('network-submenu').style.display = 'flex';
            document.getElementById('net-submenu-icon').classList.remove('fa-chevron-down');
            document.getElementById('net-submenu-icon').classList.add('fa-chevron-up');
        }
        if (pageId.startsWith('plugin-') || pageId === 'plugins') {
            const submenu = document.getElementById('plugins-submenu');
            const icon = document.getElementById('plugins-submenu-icon');
            if (submenu && icon) {
                submenu.style.display = 'flex';
                icon.classList.remove('fa-chevron-down');
                icon.classList.add('fa-chevron-up');
            }
        }
    }

    const titles = {
        general: 'General Dashboard',
        cpu: 'CPU Analysis',
        memory: 'Memory Analysis',
        processes: 'Process Manager',
        storage: 'Storage Manager',
        'net-traffic': 'Network Traffic',
        gpu: 'GPU Monitor',
        docker: 'Docker Management',
        services: 'System Services',
        ssh: 'SSH Monitor',
        cron: 'Cron Jobs',
        alerts: 'Alert Settings',
        plugins: 'Plugins',
        users: 'User Management',
        logs: 'Operation Logs',
        profile: 'My Profile',
    };
    
    // Handle plugin pages
    const pluginContainer = document.getElementById('plugin-container');

    if (pageId.startsWith('plugin-')) {
        const pluginName = pageId.replace('plugin-', '');
        document.getElementById('page-title').innerText = pluginName.charAt(0).toUpperCase() + pluginName.slice(1);
        
        if (pluginContainer) {
            // Clear previous content
            pluginContainer.innerHTML = '';
            
            const iframe = document.createElement('iframe');
            iframe.style.width = '100%';
            iframe.style.height = '100%';
            iframe.style.border = 'none';
            iframe.style.borderRadius = '8px';
            
            // Determine theme
            const isDark = document.body.classList.contains('dark-mode'); // Assuming dark-mode class on body? Or check style?
            // Actually style.css uses :root variables, but usually there is a class or localStorage
            // Let's check localStorage or body class
            const theme = document.body.classList.contains('light-mode') ? 'light' : 'dark';
            
            iframe.src = `/api/plugins/${pluginName}/?theme=${theme}`;
            pluginContainer.appendChild(iframe);
        }
    } else if (pageId === 'plugins') {
        document.getElementById('page-title').innerText = 'Plugins';
    } else if (titles[pageId]) {
        document.getElementById('page-title').innerText = titles[pageId];
    }

    // Dynamic WebSocket topic subscription based on page
    // Pages needing full process list: processes, memory (has process table)
    // Other pages only need top 10 processes (lightweight)
    if (typeof updateWebSocketTopics === 'function') {
        if (pageId === 'processes' || pageId === 'memory') {
            updateWebSocketTopics(['processes', 'net_detail']);
        } else if (pageId === 'net-traffic' || pageId === 'ssh') {
            updateWebSocketTopics(['top_processes', 'net_detail']);
        } else {
            // General, CPU, Storage, GPU, etc. - only need top processes
            updateWebSocketTopics(['top_processes']);
        }
    }

    if (pageId === 'gpu' && lastData && lastData.gpu) {
        requestAnimationFrame(() => renderGPUs(lastData.gpu));
    }

    if (pageId === 'docker') {
        loadDockerContainers();
        loadDockerImages();
    }
    if (pageId === 'services') loadServices();
    if (pageId === 'cron') loadCronJobs();
    if (pageId === 'users') loadUsers();
    if (pageId === 'logs') loadLogs();
    if (pageId === 'profile') loadProfile();
    if (pageId === 'plugins') loadPluginsPage();
}

async function initPlugins() {
    const plugins = await loadPlugins();
    const pluginsGroup = document.getElementById('nav-plugins-group');
    const pluginsSubmenu = document.getElementById('plugins-submenu');
    
    if (!pluginsGroup || !pluginsSubmenu) return;

    // Always show the Plugins group so users can access the read-only list.
    pluginsGroup.style.display = 'flex';

    // Resolve role (best-effort)
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

    // Clear existing dynamic plugins (keep the first child which is the Plugins page)
    while (pluginsSubmenu.children.length > 1) {
        pluginsSubmenu.removeChild(pluginsSubmenu.lastChild);
    }

    // Add enabled plugins
    (plugins || []).forEach(p => {
        if (p.enabled) {
            const div = document.createElement('div');
            div.className = 'nav-item';
            div.setAttribute('onclick', `switchPage('plugin-${p.name}')`);
            div.style.fontSize = '0.9rem';
            
            let icon = 'fa-puzzle-piece';
            if (p.name.includes('shell')) icon = 'fa-terminal';
            if (p.name.includes('file')) icon = 'fa-folder-open';
            
            div.innerHTML = `<i class="fas ${icon}"></i> <span>${p.name}</span>`;
            pluginsSubmenu.appendChild(div);
        }
    });
}

async function loadPluginsPage() {
    const plugins = await loadPlugins();
    renderPluginManager(plugins);
}

function renderPluginManager(plugins) {
    const container = document.getElementById('page-plugins');
    if (!container) return;

    const role = localStorage.getItem('role');

    plugins = plugins || [];
    const isAdmin = role === 'admin';
    
    let html = `
    <div class="grid-container">
        <div class="card" style="grid-column: 1 / -1;">
            <div class="card-title" style="margin-bottom: 20px;">
                <i class="fas fa-plug" style="margin-right: 8px; color: var(--accent-cpu);"></i>
                ${isAdmin ? 'Plugin Management' : 'Installed Plugins'}
            </div>
            ${isAdmin ? '' : '<div style="color: var(--text-dim); margin-bottom: 12px;">Read-only list (admin can enable/disable).</div>'}
            <div style="overflow-x: auto;">
                <table style="width: 100%; border-collapse: collapse;">
                    <thead>
                        <tr style="border-bottom: 1px solid rgba(255,255,255,0.1);">
                            <th style="padding: 12px; text-align: left; color: var(--text-dim); font-weight: 500;">Name</th>
                            <th style="padding: 12px; text-align: left; color: var(--text-dim); font-weight: 500;">Status</th>
                            <th style="padding: 12px; text-align: left; color: var(--text-dim); font-weight: 500;">Runtime</th>
                            ${isAdmin ? '<th style="padding: 12px; text-align: left; color: var(--text-dim); font-weight: 500;">Action</th>' : ''}
                        </tr>
                    </thead>
                    <tbody>
    `;
    
    if (plugins.length === 0) {
        html += `
            <tr>
                <td colspan="${isAdmin ? 4 : 3}" style="padding: 30px; text-align: center; color: var(--text-dim);">
                    <i class="fas fa-inbox" style="font-size: 2rem; margin-bottom: 10px; display: block;"></i>
                    No plugins installed
                </td>
            </tr>
        `;
    } else {
        plugins.forEach(p => {
            const statusColor = p.enabled ? 'var(--accent-mem)' : 'var(--text-dim)';
            const statusIcon = p.enabled ? 'fa-check-circle' : 'fa-times-circle';
            const statusText = p.enabled ? 'Enabled' : 'Disabled';
            const runtimeColor = p.running ? 'var(--accent-mem)' : 'var(--text-dim)';
            const runtimeIcon = p.running ? 'fa-circle' : 'fa-circle';
            const runtimeText = p.running ? 'Running' : 'Stopped';

            const btnClass = p.enabled ? 'background: rgba(255,71,87,0.2); color: #ff4757;' : 'background: rgba(46,213,115,0.2); color: #2ed573;';
            const btnText = p.enabled ? '<i class="fas fa-power-off"></i> Disable' : '<i class="fas fa-play"></i> Enable';
            
            let pluginIcon = 'fa-puzzle-piece';
            if (p.name.includes('shell')) pluginIcon = 'fa-terminal';
            if (p.name.includes('file')) pluginIcon = 'fa-folder-open';
            
            html += `
                <tr style="border-bottom: 1px solid rgba(255,255,255,0.05);">
                    <td style="padding: 15px;">
                        <div style="display: flex; align-items: center; gap: 10px;">
                            <i class="fas ${pluginIcon}" style="color: var(--accent-cpu);"></i>
                            <span style="font-weight: 500;">${p.name}</span>
                        </div>
                    </td>
                    <td style="padding: 15px;">
                        <span style="display: inline-flex; align-items: center; gap: 6px; color: ${statusColor};">
                            <i class="fas ${statusIcon}"></i> ${statusText}
                        </span>
                    </td>
                    <td style="padding: 15px;">
                        <span style="display: inline-flex; align-items: center; gap: 6px; color: ${runtimeColor};">
                            <i class="fas ${runtimeIcon}"></i> ${runtimeText}
                        </span>
                    </td>
                    ${isAdmin ? `
                    <td style="padding: 15px;">
                        <button onclick="handlePluginToggle('${p.name}', ${!p.enabled})" 
                                style="padding: 8px 16px; border: none; border-radius: 6px; cursor: pointer; font-size: 0.85rem; ${btnClass}">
                            ${btnText}
                        </button>
                    </td>
                    ` : ''}
                </tr>
            `;
        });
    }
    
    html += `
                    </tbody>
                </table>
            </div>
        </div>
    </div>
    `;
    container.innerHTML = html;
}

async function handlePluginToggle(name, enabled) {
    try {
        await togglePlugin(name, enabled);
    } catch (e) {
        alert('Failed to toggle plugin: ' + e.message);
    } finally {
        // Refresh list + submenu without a full page reload.
        await loadPluginsPage();
        await initPlugins();
    }
}
