function toggleMobileMenu() {
    const sidebar = document.getElementById('sidebar');
    const mobileBtn = document.getElementById('mobile-menu-btn');
    const overlay = document.getElementById('sidebar-overlay');

    sidebar.classList.toggle('mobile-open');
    mobileBtn.classList.toggle('active');
    
    if (overlay) {
        overlay.classList.toggle('active');
    }
}

function closeMobileMenu() {
    const sidebar = document.getElementById('sidebar');
    const mobileBtn = document.getElementById('mobile-menu-btn');
    const overlay = document.getElementById('sidebar-overlay');

    sidebar.classList.remove('mobile-open');
    mobileBtn.classList.remove('active');
    
    if (overlay) {
        overlay.classList.remove('active');
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
            
            // Determine theme from data-theme attribute (supports dark/light/warm)
            const theme = document.documentElement.getAttribute('data-theme') || 'dark';
            
            // Fetch plugin info to get correct entry URL (proxyUrl)
            // We use a self-invoking async function to handle the fetch
            (async () => {
                let src = `/plugins/${pluginName}/`;
                try {
                    // Try to find plugin info in cached list or fetch it
                    // Assuming loadPlugins is available globally
                    if (typeof loadPlugins === 'function') {
                        const plugins = await loadPlugins();
                        const plugin = plugins.find(p => p.name === pluginName);
                        if (plugin && plugin.proxyUrl) {
                            src = plugin.proxyUrl;
                        }
                    }
                } catch (e) {
                    console.warn("Failed to resolve plugin URL, using default", e);
                }
                
                const separator = src.includes('?') ? '&' : '?';
                iframe.src = `${src}${separator}theme=${theme}`;
                pluginContainer.appendChild(iframe);
            })();
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
    if (pageId === 'alerts' && typeof loadAlertsPage === 'function') loadAlertsPage();
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
    
    // Helper functions
    const getTypeStyle = (type) => {
        if (type === 'privileged') return 'background: rgba(255,71,87,0.15); color: #ff6b7a; border: 1px solid rgba(255,71,87,0.3);';
        return 'background: rgba(46,213,115,0.15); color: #2ed573; border: 1px solid rgba(46,213,115,0.3);';
    };
    
    const getRiskStyle = (risk) => {
        if (risk === 'high') return 'background: rgba(255,71,87,0.2); color: #ff4757;';
        if (risk === 'medium') return 'background: rgba(255,165,2,0.2); color: #ffa502;';
        return 'background: rgba(46,213,115,0.2); color: #2ed573;';
    };
    
    const getPluginIcon = (name) => {
        if (name.includes('shell')) return 'fa-terminal';
        if (name.includes('file')) return 'fa-folder-open';
        return 'fa-puzzle-piece';
    };
    
    const getStateInfo = (state, enabled, running) => {
        const states = {
            'available': { icon: 'fa-download', color: 'var(--text-dim)', text: 'Available' },
            'pending': { icon: 'fa-clock', color: '#ffa502', text: 'Pending Approval' },
            'installed': { icon: 'fa-check', color: 'var(--accent-cpu)', text: 'Installed' },
            'enabled': { icon: 'fa-check-circle', color: 'var(--accent-mem)', text: 'Enabled' },
            'running': { icon: 'fa-play-circle', color: 'var(--accent-mem)', text: 'Running' },
            'disabled': { icon: 'fa-pause-circle', color: 'var(--text-dim)', text: 'Disabled' },
            'stopped': { icon: 'fa-stop-circle', color: 'var(--text-dim)', text: 'Stopped' },
            'error': { icon: 'fa-exclamation-circle', color: '#ff4757', text: 'Error' }
        };
        // Prefer showing running status if actually running
        if (running) return states['running'];
        if (enabled) return states['enabled'];
        return states[state] || states['installed'];
    };
    
    let html = `
    <div class="grid-container">
        <div class="card" style="grid-column: 1 / -1;">
            <div class="card-title" style="margin-bottom: 20px; display: flex; align-items: center; justify-content: space-between;">
                <div>
                    <i class="fas fa-plug" style="margin-right: 8px; color: var(--accent-cpu);"></i>
                    ${isAdmin ? 'Plugin Management' : 'Installed Plugins'}
                </div>
                ${isAdmin ? '<button onclick="handleRefreshPlugins()" title="Refresh plugins (rescan directory)" style="padding: 6px 12px; background: rgba(255,255,255,0.1); border: none; border-radius: 4px; color: var(--text-main); cursor: pointer;"><i class="fas fa-sync-alt"></i></button>' : ''}
            </div>
            ${isAdmin ? `
            <div style="display: flex; gap: 20px; margin-bottom: 20px; padding: 12px; background: rgba(255,255,255,0.03); border-radius: 8px; font-size: 0.85rem;">
                <div style="display: flex; align-items: center; gap: 6px;">
                    <span style="padding: 2px 8px; border-radius: 4px; ${getTypeStyle('normal')}">normal</span>
                    <span style="color: var(--text-dim);">No host access</span>
                </div>
                <div style="display: flex; align-items: center; gap: 6px;">
                    <span style="padding: 2px 8px; border-radius: 4px; ${getTypeStyle('privileged')}">privileged</span>
                    <span style="color: var(--text-dim);">Requires host setup</span>
                </div>
            </div>
            ` : '<div style="color: var(--text-dim); margin-bottom: 12px;">Read-only list (admin can manage).</div>'}
            <div style="overflow-x: auto;">
                <table style="width: 100%; border-collapse: collapse;">
                    <thead>
                        <tr style="border-bottom: 1px solid rgba(255,255,255,0.1);">
                            <th style="padding: 12px; text-align: left; color: var(--text-dim); font-weight: 500;">Plugin</th>
                            <th style="padding: 12px; text-align: left; color: var(--text-dim); font-weight: 500;">Type</th>
                            <th style="padding: 12px; text-align: left; color: var(--text-dim); font-weight: 500;">State</th>
                            ${isAdmin ? '<th style="padding: 12px; text-align: right; color: var(--text-dim); font-weight: 500;">Actions</th>' : ''}
                        </tr>
                    </thead>
                    <tbody>
    `;
    
    if (plugins.length === 0) {
        html += `
            <tr>
                <td colspan="${isAdmin ? 4 : 3}" style="padding: 40px; text-align: center; color: var(--text-dim);">
                    <i class="fas fa-inbox" style="font-size: 2.5rem; margin-bottom: 12px; display: block; opacity: 0.5;"></i>
                    <div>No plugins available</div>
                </td>
            </tr>
        `;
    } else {
        plugins.forEach(p => {
            const stateInfo = getStateInfo(p.state, p.enabled, p.running);
            const pluginIcon = getPluginIcon(p.name);
            const typeStyle = getTypeStyle(p.type);
            const riskStyle = getRiskStyle(p.risk);
            
            // Action buttons
            let actionBtns = '';
            if (isAdmin) {
                const toggleBtnStyle = p.enabled 
                    ? 'background: rgba(255,71,87,0.15); color: #ff4757; border: 1px solid rgba(255,71,87,0.3);'
                    : 'background: rgba(46,213,115,0.15); color: #2ed573; border: 1px solid rgba(46,213,115,0.3);';
                const toggleIcon = p.enabled ? 'fa-pause' : 'fa-play';
                const toggleTitle = p.enabled ? 'Disable' : 'Enable';
                
                actionBtns = `
                    <div style="display: flex; gap: 8px; justify-content: flex-end;">
                        <button onclick="handlePluginToggle('${p.name}', ${!p.enabled})" 
                                title="${toggleTitle}"
                                style="padding: 6px 12px; border: none; border-radius: 6px; cursor: pointer; ${toggleBtnStyle}">
                            <i class="fas ${toggleIcon}"></i>
                        </button>
                        ${p.type === 'privileged' ? `
                        <button onclick="showPluginDetails('${p.name}')" 
                                title="View Details"
                                style="padding: 6px 12px; border: none; border-radius: 6px; cursor: pointer; background: rgba(255,255,255,0.1); color: var(--text-main);">
                            <i class="fas fa-info-circle"></i>
                        </button>
                        <button onclick="handlePluginInstall('${p.name}')" 
                                title="${['installed', 'enabled', 'running', 'stopped'].includes(p.state) ? 'Hooks Already Executed' : 'Run Install Hooks'}"
                                ${['installed', 'enabled', 'running', 'stopped'].includes(p.state) ? 'disabled' : ''}
                                style="padding: 6px 12px; border: none; border-radius: 6px; cursor: ${['installed', 'enabled', 'running', 'stopped'].includes(p.state) ? 'not-allowed' : 'pointer'}; background: ${['installed', 'enabled', 'running', 'stopped'].includes(p.state) ? 'rgba(128,128,128,0.2)' : 'rgba(52,152,219,0.2)'}; color: ${['installed', 'enabled', 'running', 'stopped'].includes(p.state) ? '#888' : '#3498db'}; border: 1px solid ${['installed', 'enabled', 'running', 'stopped'].includes(p.state) ? 'rgba(128,128,128,0.3)' : 'rgba(52,152,219,0.3)'};">
                            <i class="fas ${['installed', 'enabled', 'running', 'stopped'].includes(p.state) ? 'fa-check' : 'fa-cog'}"></i>
                        </button>
                        ` : ''}
                    </div>
                `;
            }
            
            html += `
                <tr style="border-bottom: 1px solid rgba(255,255,255,0.05);">
                    <td style="padding: 15px;">
                        <div style="display: flex; align-items: center; gap: 12px;">
                            <div style="width: 40px; height: 40px; border-radius: 8px; background: rgba(255,255,255,0.05); display: flex; align-items: center; justify-content: center;">
                                <i class="fas ${pluginIcon}" style="color: var(--accent-cpu); font-size: 1.1rem;"></i>
                            </div>
                            <div>
                                <div style="font-weight: 500; margin-bottom: 2px;">${p.name}</div>
                                <div style="font-size: 0.8rem; color: var(--text-dim);">
                                    ${p.description || `v${p.version || '?'}`}
                                    ${p.adminOnly ? '<i class="fas fa-shield-alt" title="Admin Only" style="margin-left: 6px; color: #ffa502;"></i>' : ''}
                                </div>
                            </div>
                        </div>
                    </td>
                    <td style="padding: 15px;">
                        <div style="display: flex; flex-direction: column; gap: 6px;">
                            <span style="padding: 3px 10px; border-radius: 4px; font-size: 0.75rem; display: inline-block; width: fit-content; ${typeStyle}">${p.type || 'normal'}</span>
                            ${p.risk ? `<span style="padding: 3px 10px; border-radius: 4px; font-size: 0.75rem; display: inline-block; width: fit-content; ${riskStyle}">${p.risk} risk</span>` : ''}
                        </div>
                    </td>
                    <td style="padding: 15px;">
                        <span style="display: inline-flex; align-items: center; gap: 6px; color: ${stateInfo.color};">
                            <i class="fas ${stateInfo.icon}"></i> ${stateInfo.text}
                        </span>
                        ${p.error ? `<div style="font-size: 0.75rem; color: #ff4757; margin-top: 4px;"><i class="fas fa-exclamation-triangle"></i> ${p.error}</div>` : ''}
                    </td>
                    ${isAdmin ? `<td style="padding: 15px;">${actionBtns}</td>` : ''}
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

// Handle refresh button - rescan plugins directory and reload list
async function handleRefreshPlugins() {
    try {
        // Trigger backend rescan (passive refresh)
        await refreshPluginRegistry();
    } catch (e) {
        // Ignore refresh errors, still reload the page
        console.warn('Registry refresh warning:', e);
    }
    // Reload plugin list and nav
    await loadPluginsPage();
    await initPlugins();
}

async function handlePluginToggle(name, enabled) {
    try {
        await togglePlugin(name, enabled);
    } catch (e) {
        appError('Failed to toggle plugin: ' + e.message);
    } finally {
        await loadPluginsPage();
        await initPlugins();
    }
}

// In V2, "install" is a no-op (containers are created on enable).
// This function now just shows plugin info and confirms enabling.
async function handlePluginInstall(name) {
    try {
        const plugins = await loadPlugins();
        const plugin = plugins.find(p => p.name === name);
        
        // If already enabled/running, just inform
        if (plugin && (plugin.enabled || plugin.running)) {
            appInfo(`Plugin ${name} is already enabled.`);
            return;
        }
        
        // Get security summary for confirmation
        const summary = await getPluginSecuritySummary(name);
        
        // Build confirmation message
        let message = `<div style="text-align: left; max-width: 500px;">`;
        message += `<p><strong>Plugin:</strong> ${name}</p>`;
        message += `<p><strong>Risk Level:</strong> <span style="color: ${summary.risk === 'high' || summary.risk === 'critical' ? '#ff6b7a' : '#2ed573'};">${summary.risk}</span></p>`;
        
        if (summary.permissions && summary.permissions.length > 0) {
            message += `<p><strong>Permissions:</strong></p><ul style="margin: 8px 0; padding-left: 20px;">`;
            summary.permissions.forEach(perm => {
                message += `<li style="color: var(--text-dim); margin: 4px 0;">${perm}</li>`;
            });
            message += `</ul>`;
        }
        
        if (summary.warnings && summary.warnings.length > 0) {
            message += `<p><strong>Security Warnings:</strong></p>`;
            message += `<div style="background: rgba(255,71,87,0.1); padding: 12px; border-radius: 6px; margin: 8px 0;">`;
            summary.warnings.forEach(w => {
                message += `<div style="color: #ff6b7a; margin: 4px 0;"><i class="fas fa-exclamation-triangle" style="margin-right: 6px;"></i>${w}</div>`;
            });
            message += `</div>`;
        }
        
        if (summary.dockerParams && summary.dockerParams.length > 0) {
            message += `<p><strong>Container Config:</strong></p>`;
            message += `<div style="background: rgba(0,0,0,0.2); padding: 12px; border-radius: 6px; margin: 8px 0; font-size: 0.85rem;">`;
            summary.dockerParams.forEach(p => {
                message += `<div style="color: var(--text-dim); margin: 2px 0;">${p}</div>`;
            });
            message += `</div>`;
        }
        
        message += `<p style="margin-top: 16px; color: var(--text-dim); font-size: 0.85rem;">Enabling will create and start the plugin container.</p>`;
        message += `</div>`;
        
        const confirmed = await appConfirm('Enable Plugin?', message);
        if (!confirmed) return;
        
        // Enable with full confirmation
        const result = await enablePlugin(name, {
            risk: summary.risk,
            permissions: summary.permissions || [],
            dockerParams: summary.dockerParams || []
        });
        
        if (result.success) {
            appSuccess(result.message || `Plugin ${name} enabled successfully`);
        } else if (result.requiresConfirmation) {
            appError('Additional confirmation required');
        } else {
            appError(result.message || 'Failed to enable plugin');
        }
    } catch (e) {
        appError('Failed to enable plugin: ' + e.message);
    } finally {
        await loadPluginsPage();
        await initPlugins();
    }
}

async function showPluginDetails(name) {
    try {
        const manifest = await getPluginManifest(name);
        
        let content = `<div style="text-align: left; max-width: 550px; max-height: 70vh; overflow-y: auto;">`;
        
        // Basic info
        content += `
            <div style="margin-bottom: 16px;">
                <h4 style="margin: 0 0 8px 0; color: var(--accent-cpu);">Basic Information</h4>
                <table style="width: 100%; font-size: 0.9rem;">
                    <tr><td style="color: var(--text-dim); padding: 4px 0;">Name:</td><td>${manifest.name}</td></tr>
                    <tr><td style="color: var(--text-dim); padding: 4px 0;">Version:</td><td>${manifest.version}</td></tr>
                    <tr><td style="color: var(--text-dim); padding: 4px 0;">Type:</td><td><span style="color: ${manifest.type === 'privileged' ? '#ff6b7a' : '#2ed573'};">${manifest.type}</span></td></tr>
                    <tr><td style="color: var(--text-dim); padding: 4px 0;">Risk Level:</td><td>${manifest.risk || 'low'}</td></tr>
                    ${manifest.description ? `<tr><td style="color: var(--text-dim); padding: 4px 0;">Description:</td><td>${manifest.description}</td></tr>` : ''}
                </table>
            </div>
        `;
        
        // Container config
        if (manifest.container) {
            content += `
                <div style="margin-bottom: 16px;">
                    <h4 style="margin: 0 0 8px 0; color: var(--accent-cpu);">Container</h4>
                    <table style="width: 100%; font-size: 0.9rem;">
                        <tr><td style="color: var(--text-dim); padding: 4px 0;">Image:</td><td style="word-break: break-all;">${manifest.container.image}</td></tr>
                        <tr><td style="color: var(--text-dim); padding: 4px 0;">Port:</td><td>${manifest.container.port}</td></tr>
                        ${manifest.container.hostPort ? `<tr><td style="color: var(--text-dim); padding: 4px 0;">Host Port:</td><td>${manifest.container.hostPort}</td></tr>` : ''}
                    </table>
                </div>
            `;
        }
        
        // Permissions
        if (manifest.permissions && manifest.permissions.length > 0) {
            content += `
                <div style="margin-bottom: 16px;">
                    <h4 style="margin: 0 0 8px 0; color: var(--accent-cpu);">Permissions</h4>
                    <div style="display: flex; flex-wrap: wrap; gap: 6px;">
                        ${manifest.permissions.map(p => `<span style="padding: 4px 10px; background: rgba(255,71,87,0.15); color: #ff6b7a; border-radius: 4px; font-size: 0.8rem;">${p}</span>`).join('')}
                    </div>
                </div>
            `;
        }
        
        // Install hooks
        if (manifest.install && manifest.install.hooks && manifest.install.hooks.length > 0) {
            content += `
                <div style="margin-bottom: 16px;">
                    <h4 style="margin: 0 0 8px 0; color: var(--accent-cpu);">Install Hooks</h4>
                    <div style="font-size: 0.85rem;">
                        ${manifest.install.hooks.map(h => `
                            <div style="padding: 8px; margin: 4px 0; background: rgba(255,255,255,0.03); border-radius: 4px; border-left: 2px solid var(--accent-cpu);">
                                <strong>${h.type}</strong>
                                ${h.description ? `<div style="color: var(--text-dim); margin-top: 2px;">${h.description}</div>` : ''}
                            </div>
                        `).join('')}
                    </div>
                </div>
            `;
        }
        
        content += `</div>`;
        
        // Use alert-style dialog (just OK button)
        await appAlert(`Plugin: ${manifest.name}`, content);
        
    } catch (e) {
        appError('Failed to load plugin details: ' + e.message);    }
}