// CPU Cores expand/collapse toggle
let cpuCoresExpanded = false;

function toggleCpuCores() {
    cpuCoresExpanded = !cpuCoresExpanded;
    const compactView = document.getElementById('cpu-cores-compact');
    const detailedView = document.getElementById('cpu-detailed-cores');
    const toggleText = document.getElementById('cpu-cores-toggle');
    
    if (cpuCoresExpanded) {
        if (compactView) compactView.style.display = 'none';
        if (detailedView) {
            detailedView.style.display = 'grid';
            detailedView.style.gridTemplateColumns = 'repeat(auto-fill, minmax(100px, 1fr))';
            detailedView.style.gap = '10px';
        }
        if (toggleText) toggleText.innerText = '▲ Collapse';
    } else {
        if (compactView) compactView.style.display = 'flex';
        if (detailedView) detailedView.style.display = 'none';
        if (toggleText) toggleText.innerText = '▼ Expand';
    }
}

function getSensorItemKey(item) {
            if (!item) return 'unknown';
            if (item.type === 'fan') return `fan:${item.label || 'unknown'}`;
            return `temp:${item.chip || 'unknown'}:${item.label || item.chip || 'unknown'}`;
        }

        function loadSensorOrder() {
            try {
                const raw = localStorage.getItem(SENSOR_ORDER_KEY);
                const parsed = raw ? JSON.parse(raw) : null;
                return Array.isArray(parsed) ? parsed : [];
            } catch {
                return [];
            }
        }

        function saveSensorOrder(order) {
            try {
                localStorage.setItem(SENSOR_ORDER_KEY, JSON.stringify(order));
            } catch {
                // ignore
            }
        }

        function orderItemsByStoredOrder(items, keyFn) {
            const order = loadSensorOrder();
            if (!order.length) return items;

            const rank = new Map();
            order.forEach((k, i) => rank.set(k, i));
            return items.slice().sort((a, b) => {
                const ka = keyFn(a);
                const kb = keyFn(b);
                const ra = rank.has(ka) ? rank.get(ka) : Number.POSITIVE_INFINITY;
                const rb = rank.has(kb) ? rank.get(kb) : Number.POSITIVE_INFINITY;
                if (ra !== rb) return ra - rb;
                return ka.localeCompare(kb);
            });
        }

        function applySensorDragAndDrop(container) {
            if (!container) return;

            container.querySelectorAll('[data-sensor-key]').forEach(el => {
				// Avoid attaching duplicate listeners on every render.
				if (el.dataset && el.dataset.dndBound === '1') {
					return;
				}
				if (el.dataset) {
					el.dataset.dndBound = '1';
				}

                el.addEventListener('dragstart', (e) => {
                    const key = el.getAttribute('data-sensor-key');
                    if (!key) return;
                    el.classList.add('sensor-dragging');
                    try {
                        e.dataTransfer.setData('text/plain', key);
                        e.dataTransfer.effectAllowed = 'move';
                    } catch {
                        // ignore
                    }
                });

                el.addEventListener('dragend', () => {
                    el.classList.remove('sensor-dragging');
                    container.querySelectorAll('.sensor-drop-target').forEach(t => t.classList.remove('sensor-drop-target'));
                });

                el.addEventListener('dragover', (e) => {
                    e.preventDefault();
                    el.classList.add('sensor-drop-target');
                    try { e.dataTransfer.dropEffect = 'move'; } catch {}
                });

                el.addEventListener('dragleave', () => {
                    el.classList.remove('sensor-drop-target');
                });

                el.addEventListener('drop', (e) => {
                    e.preventDefault();
                    el.classList.remove('sensor-drop-target');

                    let draggedKey = '';
                    try { draggedKey = e.dataTransfer.getData('text/plain'); } catch {}
                    if (!draggedKey) return;

                    const targetKey = el.getAttribute('data-sensor-key');
                    if (!targetKey || draggedKey === targetKey) return;

                    const keys = lastSensorItemKeys.slice();
                    const from = keys.indexOf(draggedKey);
                    const to = keys.indexOf(targetKey);
                    if (from === -1 || to === -1) return;

                    // Remove dragged
                    keys.splice(from, 1);

                    // Insert before/after target based on pointer position
                    const rect = el.getBoundingClientRect();
                    const before = (e.clientY - rect.top) < (rect.height / 2);
                    const insertAt = before ? (from < to ? to - 1 : to) : (from < to ? to : to + 1);
                    const clamped = Math.max(0, Math.min(keys.length, insertAt));
                    keys.splice(clamped, 0, draggedKey);

                    saveSensorOrder(keys);
                    if (lastData) {
                        _renderStatsInternal(lastData);
                    }
                });
            });
        }

        function setSort(column) {
            if (currentSort.column === column) {
                currentSort.direction = currentSort.direction === 'desc' ? 'asc' : 'desc';
            } else {
                currentSort.column = column;
                currentSort.direction = 'desc';
            }
            if (lastData) {
                _renderStatsInternal(lastData);
            }
        }

        function onNetworkInterfaceChange() {
            const select = document.getElementById('net-interface-select');
            if (!select) return;
            selectedInterface = select.value || '__all__';
            localStorage.setItem('netInterface', selectedInterface);
            // Reset baselines so next tick computes fresh speeds for chosen interface
            lastInterfaceStats = {};
            lastNetwork = null;
            lastTime = null;
        }

        function syncNetworkInterfaceOptions(ifaces) {
            const select = document.getElementById('net-interface-select');
            if (!select) return;

            const names = ['__all__', ...Object.keys(ifaces || {})];
            const currentOptions = Array.from(select.options).map((o) => o.value);
            const sameLength = currentOptions.length === names.length;
            const sameItems = sameLength && names.every((n, idx) => currentOptions[idx] === n);
            if (!sameItems) {
                select.innerHTML = '';
                names.forEach((n) => {
                    const opt = document.createElement('option');
                    opt.value = n;
                    opt.textContent = n === '__all__' ? 'All interfaces' : n;
                    select.appendChild(opt);
                });
            }

            if (!names.includes(selectedInterface)) {
                selectedInterface = '__all__';
                localStorage.setItem('netInterface', selectedInterface);
            }
            select.value = selectedInterface;
        }

        function renderSelectedInterfaceMeta(name, iface) {
            const ipEl = document.getElementById('net-interface-ip');
            const statusEl = document.getElementById('net-interface-status');
            if (!ipEl || !statusEl) return;

            if (!iface || name === '__all__') {
                ipEl.innerText = 'Aggregated across all interfaces';
                statusEl.innerText = 'MULTI';
                statusEl.className = 'net-status-badge multi';
                return;
            }

            ipEl.innerText = iface.ip || 'No IP assigned';
            statusEl.innerText = iface.is_up ? 'UP' : 'DOWN';
            statusEl.className = 'net-status-badge ' + (iface.is_up ? 'up' : 'down');
        }

        function renderStats(data) {
            lastData = data;
            requestAnimationFrame(() => {
                _renderStatsInternal(data);
            });
        }

function _renderStatsInternal(data) {
            // Boot Time
            document.getElementById('boot-time').innerText = 'Up since: ' + data.boot_time;

            // Check active page to optimize rendering
            const isCpuPage = document.getElementById('page-cpu').classList.contains('active');
            const isMemPage = document.getElementById('page-memory').classList.contains('active');
            const isGeneralPage = document.getElementById('page-general').classList.contains('active');

            // General Page CPU Widget
            if (isGeneralPage) {
                document.getElementById('cpu-total-text').innerText = data.cpu.percent + '%';
                document.getElementById('cpu-total-bar').style.width = data.cpu.percent + '%';
                if (data.cpu.freq && data.cpu.freq.avg !== undefined) {
                    document.getElementById('cpu-freq').innerText = data.cpu.freq.avg.toFixed(0) + ' MHz';
                }
                
                // Change container style to column for list view
                const cpuCoresContainer = document.getElementById('cpu-cores');
                if (cpuCoresContainer) {
                    cpuCoresContainer.style.display = 'flex';
                    cpuCoresContainer.style.flexDirection = 'column';
                    cpuCoresContainer.style.gap = '10px';
                    cpuCoresContainer.style.marginTop = '15px';
                    cpuCoresContainer.style.maxHeight = '300px';
                    cpuCoresContainer.style.overflowY = 'auto';
                }

                updateList('cpu-cores', data.cpu.per_core, 
                    (core, index) => {
                        const div = document.createElement('div');
                        div.className = 'stat-group';
                        div.style.marginBottom = '0';
                        div.innerHTML = `
                            <div class="stat-label">
                                <span>Core ${index} <span class="core-freq" style="color: var(--text-dim); font-size: 0.8em; margin-left: 5px;"></span></span>
                                <span class="core-val"></span>
                            </div>
                            <div class="progress-bg">
                                <div class="progress-fill cpu-fill core-fill" style="width: 0%;"></div>
                            </div>
                        `;
                        return div;
                    },
                    (el, core, index) => {
                        const freq = data.cpu.freq.per_core[index] ? data.cpu.freq.per_core[index].toFixed(0) : '-';
                        el.querySelector('.core-freq').innerText = `@ ${freq} MHz`;
                        el.querySelector('.core-val').innerText = `${core}%`;
                        el.querySelector('.core-fill').style.width = `${core}%`;
                    }
                );
            }

            // CPU Details Page
            if (isCpuPage && data.cpu) {
                const coreCount = data.cpu.per_core ? data.cpu.per_core.length : 1;
                
                // CPU Overview
                const overviewPercent = document.getElementById('cpu-overview-percent');
                const overviewFreq = document.getElementById('cpu-overview-freq');
                const overviewTemp = document.getElementById('cpu-overview-temp');
                
                if (overviewPercent) overviewPercent.innerText = data.cpu.percent + '%';
                if (overviewFreq && data.cpu.freq && data.cpu.freq.avg !== undefined) {
                    const freqGHz = (data.cpu.freq.avg / 1000).toFixed(2);
                    overviewFreq.innerText = freqGHz + ' GHz';
                }
                // Temperature from history
                if (overviewTemp && data.cpu.temp_history && data.cpu.temp_history.length > 0) {
                    const latestTemp = data.cpu.temp_history[data.cpu.temp_history.length - 1] || 0;
                    overviewTemp.innerText = latestTemp.toFixed(0) + '°C';
                    // Color based on temperature
                    if (latestTemp >= 80) {
                        overviewTemp.style.color = '#ff4757';
                    } else if (latestTemp >= 60) {
                        overviewTemp.style.color = '#ffa502';
                    } else {
                        overviewTemp.style.color = '#2ed573';
                    }
                }

                // Load Average with progress bars
                if (data.cpu.load_avg) {
                    const loads = [
                        { id: '1', value: data.cpu.load_avg[0] },
                        { id: '5', value: data.cpu.load_avg[1] },
                        { id: '15', value: data.cpu.load_avg[2] }
                    ];
                    
                    loads.forEach(load => {
                        const textEl = document.getElementById(`cpu-load-${load.id}`);
                        const barEl = document.getElementById(`cpu-load-${load.id}-bar`);
                        if (!textEl || !barEl) return;
                        
                        textEl.innerText = load.value.toFixed(2);
                        
                        // Progress relative to core count (100% = cores fully loaded)
                        const percent = Math.min((load.value / coreCount) * 100, 100);
                        barEl.style.width = percent + '%';
                        
                        // Color: green < 70%, yellow 70-100%, red > 100%
                        if (load.value > coreCount) {
                            barEl.style.background = '#ff4757';
                            textEl.style.color = '#ff4757';
                        } else if (load.value > coreCount * 0.7) {
                            barEl.style.background = '#ffa502';
                            textEl.style.color = '#ffa502';
                        } else {
                            barEl.style.background = 'var(--accent-cpu)';
                            textEl.style.color = 'var(--accent-cpu)';
                        }
                    });
                    
                    const coreCountEl = document.getElementById('cpu-core-count');
                    if (coreCountEl) coreCountEl.innerText = coreCount;
                }

                // CPU Information
                if (data.cpu.info) {
                    const info = data.cpu.info;
                    const modelEl = document.getElementById('cpu-model');
                    const archEl = document.getElementById('cpu-arch');
                    const coresEl = document.getElementById('cpu-info-cores');
                    const threadsEl = document.getElementById('cpu-threads');
                    const freqRangeEl = document.getElementById('cpu-freq-range');
                    
                    if (modelEl) modelEl.innerText = info.model || '-';
                    if (archEl) archEl.innerText = info.architecture || '-';
                    if (coresEl) coresEl.innerText = info.cores || '-';
                    if (threadsEl) threadsEl.innerText = info.threads || '-';
                    if (freqRangeEl) {
                        const freqRange = info.min_freq && info.max_freq 
                            ? `${info.min_freq.toFixed(0)} - ${info.max_freq.toFixed(0)} MHz`
                            : '-';
                        freqRangeEl.innerText = freqRange;
                    }
                }

                // CPU Usage History (using existing percent history or creating from current)
                const trendContainer = document.getElementById('cpu-usage-trend');
                if (trendContainer && data.cpu.percent_history && data.cpu.percent_history.length > 0) {
                    drawChart('cpu-usage-trend', data.cpu.percent_history, { color: 'var(--accent-cpu)', min: 0, max: 100 });
                }

                // Compact core view (small boxes with progress bar)
                const compactContainer = document.getElementById('cpu-cores-compact');
                if (compactContainer && data.cpu.per_core) {
                    updateList('cpu-cores-compact', data.cpu.per_core,
                        (core, index) => {
                            const div = document.createElement('div');
                            div.className = 'cpu-core-mini';
                            div.style.cssText = `
                                width: 80px;
                                padding: 8px;
                                border-radius: 6px;
                                display: flex;
                                flex-direction: column;
                                align-items: center;
                                background: rgba(0,0,0,0.2);
                                transition: background 0.3s;
                            `;
                            div.innerHTML = `
                                <div style="display: flex; justify-content: space-between; width: 100%; margin-bottom: 5px;">
                                    <span class="core-num" style="font-size: 0.7rem; color: var(--text-dim);">C${index}</span>
                                    <span class="core-pct" style="font-size: 0.8rem; font-weight: bold;"></span>
                                </div>
                                <div class="progress-bg" style="width: 100%; height: 5px; margin-bottom: 4px; border-radius: 2px;">
                                    <div class="core-fill" style="height: 100%; width: 0%; transition: width 0.3s; border-radius: 2px;"></div>
                                </div>
                                <div class="core-freq" style="font-size: 0.65rem; color: var(--text-dim);"></div>
                            `;
                            return div;
                        },
                        (el, core, index) => {
                            const pctEl = el.querySelector('.core-pct');
                            const fillEl = el.querySelector('.core-fill');
                            const freqEl = el.querySelector('.core-freq');
                            
                            pctEl.innerText = core + '%';
                            fillEl.style.width = core + '%';
                            
                            // Frequency
                            const freq = data.cpu.freq && data.cpu.freq.per_core && data.cpu.freq.per_core[index] 
                                ? (data.cpu.freq.per_core[index] / 1000).toFixed(1) : '-';
                            freqEl.innerText = freq + ' GHz';
                            
                            // Color based on usage
                            if (core >= 80) {
                                pctEl.style.color = '#ff4757';
                                fillEl.style.background = '#ff4757';
                                el.style.background = 'rgba(255, 71, 87, 0.15)';
                            } else if (core >= 50) {
                                pctEl.style.color = '#ffa502';
                                fillEl.style.background = '#ffa502';
                                el.style.background = 'rgba(255, 165, 2, 0.1)';
                            } else {
                                pctEl.style.color = 'var(--accent-cpu)';
                                fillEl.style.background = 'var(--accent-cpu)';
                                el.style.background = 'rgba(0,0,0,0.2)';
                            }
                        }
                    );
                }

                // Detailed Cores (expanded view)
                const detailedCoresContainer = document.getElementById('cpu-detailed-cores');
                if (detailedCoresContainer && detailedCoresContainer.style.display !== 'none') {
                    detailedCoresContainer.style.display = 'grid';
                    detailedCoresContainer.style.gridTemplateColumns = 'repeat(auto-fill, minmax(100px, 1fr))';
                    detailedCoresContainer.style.gap = '10px';

                    updateList('cpu-detailed-cores', data.cpu.per_core,
                        (core, index) => {
                            const div = document.createElement('div');
                            div.className = 'card cpu-core-card';
                            div.style.padding = '10px';
                            div.style.marginBottom = '0';
                            div.style.textAlign = 'center';
                            div.innerHTML = `
                                <div style="font-size: 0.8rem; color: var(--text-dim); margin-bottom: 5px;">Core ${index}</div>
                                <div class="core-val" style="font-size: 1.2rem; font-weight: bold; color: var(--accent-cpu); margin-bottom: 5px;"></div>
                                <div class="progress-bg" style="height: 4px;">
                                    <div class="progress-fill core-fill" style="width: 0%; background: var(--accent-cpu);"></div>
                                </div>
                                <div class="core-freq" style="font-size: 0.7rem; color: var(--text-dim); margin-top: 5px;"></div>
                            `;
                            return div;
                        },
                        (el, core, index) => {
                            el.querySelector('.core-val').innerText = core + '%';
                            el.querySelector('.core-fill').style.width = core + '%';
                            const freq = data.cpu.freq && data.cpu.freq.per_core && data.cpu.freq.per_core[index] 
                                ? data.cpu.freq.per_core[index].toFixed(0) : '-';
                            el.querySelector('.core-freq').innerText = `${freq} MHz`;
                        }
                    );
                }
            }

            // Memory
            document.getElementById('mem-text').innerText = `${data.memory.used} / ${data.memory.total}`;
            document.getElementById('mem-percent').innerText = data.memory.percent + '%';
            document.getElementById('mem-bar').style.width = data.memory.percent + '%';

            // Memory Details
            const memDetails = [
                { label: 'Buffers', val: data.memory.buffers },
                { label: 'Cached', val: data.memory.cached },
                { label: 'Shared', val: data.memory.shared },
                { label: 'Slab', val: data.memory.slab },
                { label: 'Active', val: data.memory.active },
                { label: 'Inactive', val: data.memory.inactive },
            ];
            
            updateList('mem-details', memDetails,
                (item) => {
                    const div = document.createElement('div');
                    div.innerHTML = `<span style="color: var(--text-dim);">${item.label}:</span> <span class="mem-val" style="float: right;">${item.val}</span>`;
                    return div;
                },
                (el, item) => {
                    el.querySelector('.mem-val').innerText = item.val;
                }
            );

            // Memory Page
            if (isMemPage) {
                // Memory Overview
                const memOverviewUsed = document.getElementById('mem-overview-used');
                const memOverviewPercent = document.getElementById('mem-overview-percent');
                const memOverviewAvailable = document.getElementById('mem-overview-available');
                const swapOverviewUsed = document.getElementById('swap-overview-used');
                const swapOverviewPercent = document.getElementById('swap-overview-percent');
                
                if (memOverviewUsed) memOverviewUsed.innerText = `${data.memory.used} / ${data.memory.total}`;
                if (memOverviewPercent) memOverviewPercent.innerText = data.memory.percent + '%';
                if (memOverviewAvailable) memOverviewAvailable.innerText = data.memory.available || '0';
                
                if (data.swap) {
                    if (swapOverviewUsed) swapOverviewUsed.innerText = `${data.swap.used} / ${data.swap.total}`;
                    if (swapOverviewPercent) swapOverviewPercent.innerText = data.swap.percent + '%';
                }

                // Memory Usage Trends
                if (data.memory.history && data.memory.history.length > 0) {
                    const history = data.memory.history;
                    const latest = history[history.length - 1] || 0;
                    
                    // Determine color based on pressure
                    let color = '#2ed573'; // Green
                    if (latest > 80) color = '#ff4757'; // Red
                    else if (latest > 60) color = '#ffa502'; // Yellow
                    
                    drawChart('mem-trend', history, { color: color, min: 0, max: 100 });
                    const trendCurrent = document.getElementById('mem-trend-current');
                    if (trendCurrent) trendCurrent.innerText = `${latest.toFixed(1)}%`;
                }

                // Memory Details (7 items in 4-column grid)
                const memDetails = [
                    { label: 'Available', val: data.memory.available, color: '#2ed573' },
                    { label: 'Buffers', val: data.memory.buffers, color: 'var(--accent-mem)' },
                    { label: 'Cached', val: data.memory.cached, color: '#ffa502' },
                    { label: 'Shared', val: data.memory.shared, color: '#ff6b81' },
                    { label: 'Slab', val: data.memory.slab, color: '#70a1ff' },
                    { label: 'Active', val: data.memory.active, color: '#7bed9f' },
                    { label: 'Inactive', val: data.memory.inactive, color: '#a4b0be' }
                ];
                
                updateList('mem-page-details', memDetails,
                    (item) => {
                        const div = document.createElement('div');
                        div.style.cssText = 'text-align: center; padding: 10px; background: rgba(0,0,0,0.15); border-radius: 6px;';
                        div.innerHTML = `
                            <div style="font-size: 0.7rem; color: var(--text-dim); margin-bottom: 5px;">${item.label}</div>
                            <div class="mem-val" style="font-size: 1.1rem; font-weight: bold; color: ${item.color};">${item.val}</div>
                        `;
                        return div;
                    },
                    (el, item) => {
                        el.querySelector('.mem-val').innerText = item.val;
                    }
                );

                // Memory Pie Chart
                if (data.memory) {
                    drawMemoryPieChart(data.memory);
                }

                // Process Table
                if (data.processes) {
                    const procCount = document.getElementById('proc-count');
                    if (procCount) procCount.innerText = `${data.processes.length} processes`;
                    
                    let sortedProcs = [...data.processes];
                    sortedProcs.sort((a, b) => {
                        let valA = a[currentSort.column];
                        let valB = b[currentSort.column];
                        
                        if (typeof valA === 'string') valA = valA.toLowerCase();
                        if (typeof valB === 'string') valB = valB.toLowerCase();
                        
                        if (valA < valB) return currentSort.direction === 'asc' ? -1 : 1;
                        if (valA > valB) return currentSort.direction === 'asc' ? 1 : -1;
                        return 0;
                    });

                    updateList('process-table-body', sortedProcs,
                        (proc) => {
                            const tr = document.createElement('tr');
                            tr.style.borderBottom = '1px solid rgba(255,255,255,0.05)';
                            tr.innerHTML = `
                                <td style="padding: 8px; color: var(--accent-cpu);" class="mono-text"></td>
                                <td style="padding: 8px; max-width: 200px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;"></td>
                                <td style="padding: 8px; color: var(--text-dim);"></td>
                                <td style="padding: 8px; text-align: right;" class="mono-text"></td>
                                <td style="padding: 8px; text-align: right; color: var(--accent-net);" class="mono-text"></td>
                                <td style="padding: 8px; text-align: right; color: var(--accent-mem);" class="mono-text"></td>
                            `;
                            return tr;
                        },
                        (el, proc) => {
                            el.children[0].innerText = proc.pid;
                            el.children[1].innerText = proc.name;
                            el.children[1].title = proc.name;
                            el.children[2].innerText = proc.username || '-';
                            el.children[3].innerText = (proc.num_threads ?? 0).toString();
                            el.children[4].innerText = (proc.cpu_percent ?? 0).toFixed(1) + '%';
                            el.children[5].innerText = (proc.memory_percent ?? 0).toFixed(1) + '%';
                        }
                    );
                }
            }

            // Top Processes (General Page Widget)
            if (isGeneralPage && data.processes) {
                // Limit to top 10 processes
                const topProcesses = data.processes.slice(0, 10);
                updateList('processes-container', topProcesses,
                    (proc) => {
                        const div = document.createElement('div');
                        div.style.display = 'flex';
                        div.style.justifyContent = 'space-between';
                        div.style.borderBottom = '1px solid rgba(255,255,255,0.05)';
                        div.style.padding = '2px 0';
                        div.innerHTML = `
                            <span style="overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 70%;" class="proc-name"></span>
                            <span>
                                <span class="proc-mem" style="color: var(--accent-mem); font-weight: bold;"></span>
                            </span>
                        `;
                        return div;
                    },
                    (el, proc) => {
                        el.querySelector('.proc-name').innerText = `[${proc.pid}] ${proc.name}`;
                        el.querySelector('.proc-mem').innerText = proc.memory_percent.toFixed(1) + '%';
                    }
                );
            }

            // Processes Page
            const isProcessesPage = document.getElementById('page-processes').classList.contains('active');
            if (isProcessesPage && data.processes) {
                // Update allProcesses with uptime_seconds for sorting
                allProcesses = data.processes.map(p => {
                    let uptimeSeconds = 0;
                    if (p.uptime) {
                        const match = p.uptime.match(/(\d+)([smhd])/);
                        if (match) {
                            const num = parseInt(match[1]);
                            const unit = match[2];
                            switch(unit) {
                                case 's': uptimeSeconds = num; break;
                                case 'm': uptimeSeconds = num * 60; break;
                                case 'h': uptimeSeconds = num * 3600; break;
                                case 'd': uptimeSeconds = num * 86400; break;
                            }
                        }
                    }
                    return { ...p, uptime_seconds: uptimeSeconds };
                });
                
                // Render the processes
                filterAndRenderProcesses();
            }

            // Sensors
            const sensorsContainer = document.getElementById('sensors-container');
            let sensorsList = [];
            
            if (data.fans) {
                data.fans.forEach(fan => sensorsList.push({type: 'fan', ...fan}));
            }
            if (data.sensors) {
                for (const [chip, sensors] of Object.entries(data.sensors)) {
                    sensors.forEach(sensor => sensorsList.push({type: 'temp', chip: chip, ...sensor}));
                }
            }

            if (sensorsList.length === 0) {
                sensorsContainer.innerHTML = '<div class="no-data" style="color: var(--text-dim); text-align: center; padding: 20px;">No sensors detected</div>';
            } else {
                if (sensorsContainer.querySelector('.no-data')) {
                    sensorsContainer.innerHTML = '';
                }
                sensorsList = orderItemsByStoredOrder(sensorsList, getSensorItemKey);
                lastSensorItemKeys = sensorsList.map(getSensorItemKey);

                updateList('sensors-container', sensorsList,
                    (item) => {
                        const div = document.createElement('div');
                        const key = getSensorItemKey(item);
                        div.className = 'sensor-item';
                        div.style.display = 'flex';
                        div.style.justifyContent = 'space-between';
                        div.style.borderBottom = '1px solid rgba(255,255,255,0.05)';
                        div.style.padding = '2px 0';
                        div.classList.add('sensor-draggable');
                        div.setAttribute('draggable', 'true');
                        div.setAttribute('data-sensor-key', key);
                        div.innerHTML = `
                            <span class="sensor-label" style="overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 75%;"></span>
                            <span class="sensor-value" style="font-weight: bold;"></span>
                        `;
                        return div;
                    },
                    (el, item) => {
                        const labelSpan = el.querySelector('.sensor-label');
                        const valSpan = el.querySelector('.sensor-value');
                        
                        if (item.type === 'fan') {
                            labelSpan.innerText = item.label;
                            valSpan.style.color = 'var(--accent-net)';
                            valSpan.innerText = `${item.current} RPM`;
                        } else {
                            labelSpan.innerText = item.label || item.chip;
                            valSpan.style.color = item.current > 70 ? '#ff4757' : '#2ed573';
                            valSpan.innerText = `${item.current}°C`;
                        }
                    }
                );

                applySensorDragAndDrop(sensorsContainer);
            }

            // Power
            const powerContainer = document.getElementById('power-container');
            let powerList = [];
            
            if (data.power) {
                if (data.power.percent !== undefined) {
                    powerList.push({type: 'battery', ...data.power});
                }
                if (data.power.consumption_watts !== undefined) {
                    powerList.push({type: 'total', watts: data.power.consumption_watts});
                }
                if (data.power.rapl) {
                    for (const [domain, watts] of Object.entries(data.power.rapl)) {
                        if (domain.includes('Package') && data.power.consumption_watts == watts) continue;
                        powerList.push({type: 'rapl', domain: domain, watts: watts});
                    }
                }
            }

            if (powerList.length === 0) {
                powerContainer.innerHTML = '<div class="no-data" style="color: var(--text-dim); text-align: center; padding: 10px;">No power data</div>';
            } else {
                if (powerContainer.querySelector('.no-data')) {
                    powerContainer.innerHTML = '';
                }
                updateList('power-container', powerList,
                    (item) => {
                        const div = document.createElement('div');
                        div.className = 'sensor-item';
                        div.innerHTML = `<span></span><span class="sensor-value"></span>`;
                        return div;
                    },
                    (el, item) => {
                        const labelSpan = el.firstElementChild;
                        const valSpan = el.lastElementChild;
                        
                        if (item.type === 'battery') {
                            labelSpan.innerText = 'Battery';
                            valSpan.innerText = `${item.percent}% ${item.power_plugged ? '(Charging)' : ''}`;
                            valSpan.style.color = 'inherit';
                        } else if (item.type === 'total') {
                            labelSpan.innerText = 'Power Draw';
                            valSpan.innerText = `${item.watts} W`;
                            valSpan.style.color = '#ffa502';
                        } else {
                            labelSpan.innerText = item.domain;
                            labelSpan.style.color = 'var(--text-dim)';
                            labelSpan.style.fontSize = '0.85em';
                            labelSpan.style.paddingLeft = '10px';
                            valSpan.innerText = `${item.watts} W`;
                            valSpan.style.fontSize = '0.85em';
                            valSpan.style.color = 'inherit';
                        }
                    }
                );
            }

            // Network
            if (data.network) {
                // General Page
                if (document.getElementById('net-sent')) {
                    document.getElementById('net-sent').innerText = data.network.bytes_sent;
                    document.getElementById('net-recv').innerText = data.network.bytes_recv;
                }
                
                // Network Page
                if (document.getElementById('net-page-sent')) {
                    document.getElementById('net-page-sent').innerText = data.network.bytes_sent;
                    document.getElementById('net-page-recv').innerText = data.network.bytes_recv;
                }
                const ifaceMap = data.network.interfaces || {};
                syncNetworkInterfaceOptions(ifaceMap);

                const nowTs = Date.now();
                let sentSpeedStr = '0 B/s';
                let recvSpeedStr = '0 B/s';

                const iface = selectedInterface !== '__all__' ? ifaceMap[selectedInterface] : null;
                const selectedName = iface ? selectedInterface : '__all__';
                renderSelectedInterfaceMeta(selectedName, iface);

                if (selectedName === '__all__') {
                    if (lastNetwork && lastTime) {
                        const timeDiff = (nowTs - lastTime) / 1000;
                        if (timeDiff > 0) {
                            const sentDiff = data.network.raw_sent - lastNetwork.raw_sent;
                            const recvDiff = data.network.raw_recv - lastNetwork.raw_recv;
                            sentSpeedStr = formatSize(sentDiff / timeDiff) + '/s';
                            recvSpeedStr = formatSize(recvDiff / timeDiff) + '/s';
                        }
                    }
                } else if (iface) {
                    const prev = lastInterfaceStats[selectedName];
                    if (prev) {
                        const timeDiff = (nowTs - prev.ts) / 1000;
                        if (timeDiff > 0) {
                            const sentDiff = iface.raw_sent - prev.rawSent;
                            const recvDiff = iface.raw_recv - prev.rawRecv;
                            sentSpeedStr = formatSize(sentDiff / timeDiff) + '/s';
                            recvSpeedStr = formatSize(recvDiff / timeDiff) + '/s';
                        }
                    }
                }

                if (document.getElementById('net-speed-sent')) {
                    document.getElementById('net-speed-sent').innerText = sentSpeedStr;
                    document.getElementById('net-speed-recv').innerText = recvSpeedStr;
                }
                if (document.getElementById('net-page-speed-sent')) {
                    document.getElementById('net-page-speed-sent').innerText = sentSpeedStr;
                    document.getElementById('net-page-speed-recv').innerText = recvSpeedStr;
                }

                Object.entries(ifaceMap).forEach(([name, stats]) => {
                    lastInterfaceStats[name] = {
                        rawSent: stats.raw_sent || 0,
                        rawRecv: stats.raw_recv || 0,
                        ts: nowTs,
                    };
                });
                lastNetwork = data.network;
                lastTime = nowTs;

                // Network Page Interfaces
                const isNetPage = document.getElementById('page-net-traffic').classList.contains('active');
                if (isNetPage) {
                    // TCP Connection States
                    if (data.network.connection_states) {
                        const states = data.network.connection_states;
                        document.getElementById('tcp-established').innerText = states.ESTABLISHED || 0;
                        document.getElementById('tcp-time-wait').innerText = states.TIME_WAIT || 0;
                        document.getElementById('tcp-close-wait').innerText = states.CLOSE_WAIT || 0;
                        document.getElementById('tcp-listen').innerText = states.LISTEN || 0;
                        document.getElementById('tcp-syn-sent').innerText = states.SYN_SENT || 0;
                        document.getElementById('tcp-syn-recv').innerText = states.SYN_RECV || 0;
                        document.getElementById('tcp-fin-wait1').innerText = states.FIN_WAIT1 || 0;
                        document.getElementById('tcp-fin-wait2').innerText = states.FIN_WAIT2 || 0;
                        document.getElementById('tcp-last-ack').innerText = states.LAST_ACK || 0;
                    }

                    // Socket Stats
                    if (data.network.sockets) {
                        document.getElementById('net-tcp-count').innerText = data.network.sockets.tcp || 0;
                        document.getElementById('net-udp-count').innerText = data.network.sockets.udp || 0;
                        document.getElementById('net-tcp-tw').innerText = data.network.sockets.tcp_tw || 0;
                    }

                    // Network Errors & Packet Loss
                    if (data.network.errors) {
                        document.getElementById('net-errors-in').innerText = data.network.errors.total_errors_in || 0;
                        document.getElementById('net-errors-out').innerText = data.network.errors.total_errors_out || 0;
                        document.getElementById('net-drops-in').innerText = data.network.errors.total_drops_in || 0;
                        document.getElementById('net-drops-out').innerText = data.network.errors.total_drops_out || 0;
                    }

                    // Per-Interface Error Details
                    if (data.network.interfaces) {
                        const interfaces = Object.entries(data.network.interfaces)
                            .map(([name, stats]) => ({name, ...stats}))
                            .filter(i => (i.errors_in || 0) > 0 || (i.errors_out || 0) > 0 || (i.drops_in || 0) > 0 || (i.drops_out || 0) > 0);
                        
                        const errorContainer = document.getElementById('net-error-details');
                        if (interfaces.length === 0) {
                            errorContainer.innerHTML = '<div style="grid-column: 1 / -1; text-align: center; color: var(--text-dim); padding: 10px;">No errors or drops detected</div>';
                        } else {
                            errorContainer.innerHTML = interfaces.map(iface => `
                                <div style="padding: 10px; background: rgba(255,255,255,0.03); border-radius: 4px; border-left: 3px solid ${(iface.errors_in + iface.errors_out) > 0 ? '#ff6b6b' : '#ffa94d'};">
                                    <div style="font-weight: bold; margin-bottom: 5px;">${iface.name}</div>
                                    <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 5px; font-size: 0.85rem;">
                                        <div style="color: var(--text-dim);">Errors In: <span style="color: #ff6b6b; font-weight: bold;">${iface.errors_in || 0}</span></div>
                                        <div style="color: var(--text-dim);">Errors Out: <span style="color: #ff6b6b; font-weight: bold;">${iface.errors_out || 0}</span></div>
                                        <div style="color: var(--text-dim);">Drops In: <span style="color: #ffa94d; font-weight: bold;">${iface.drops_in || 0}</span></div>
                                        <div style="color: var(--text-dim);">Drops Out: <span style="color: #ffa94d; font-weight: bold;">${iface.drops_out || 0}</span></div>
                                    </div>
                                </div>
                            `).join('');
                        }
                    }

                    // Listening Ports (Network Page)
                    if (data.network && data.network.listening_ports) {
                        const portsContainer = document.getElementById('listening-ports');
                        if (data.network.listening_ports.length > 0) {
                            portsContainer.innerHTML = data.network.listening_ports.map(port => `
                                <div style="display: flex; justify-content: space-between; align-items: center; padding: 8px; background: rgba(255,255,255,0.02); border-radius: 4px; border-left: 3px solid var(--accent-net);">
                                    <div>
                                        <div style="font-weight: bold; color: var(--accent-cpu);">Port ${port.port}</div>
                                        <div style="font-size: 0.8rem; color: var(--text-dim);">${port.protocol}</div>
                                    </div>
                                </div>
                            `).join('');
                        } else {
                            portsContainer.innerHTML = '<div style="color: var(--text-dim); text-align: center; padding: 10px;">No listening ports</div>';
                        }
                    }

                    if (data.network.interfaces) {
                        const interfaces = Object.entries(data.network.interfaces).map(([name, stats]) => ({name, ...stats}));
                        updateList('net-interfaces', interfaces,
                            (iface) => {
                                const div = document.createElement('div');
                                div.className = 'card';
                                div.style.marginBottom = '0';
                                div.style.padding = '15px';
                                div.innerHTML = `
                                    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 10px;">
                                    <span class="iface-name" style="font-weight: bold; color: var(--accent-cpu);"></span>
                                    <span class="iface-status" style="font-size: 0.8rem; padding: 2px 6px; border-radius: 4px;"></span>
                                </div>
                                <div style="font-size: 0.9rem; margin-bottom: 5px;">
                                    <span style="color: var(--text-dim);">IP:</span> <span class="iface-ip"></span>
                                </div>
                                <div style="font-size: 0.9rem; margin-bottom: 5px;">
                                    <span style="color: var(--text-dim);">Speed:</span> <span class="iface-speed"></span>
                                </div>
                                <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 10px; margin-top: 10px; font-size: 0.85rem;">
                                    <div><i class="fas fa-arrow-up" style="color: var(--accent-cpu);"></i> <span class="iface-sent"></span></div>
                                    <div><i class="fas fa-arrow-down" style="color: var(--accent-mem);"></i> <span class="iface-recv"></span></div>
                                </div>
                            `;
                            return div;
                        },
                        (el, iface) => {
                            el.querySelector('.iface-name').innerText = iface.name;
                            const statusEl = el.querySelector('.iface-status');
                            statusEl.innerText = iface.is_up ? 'UP' : 'DOWN';
                            statusEl.style.background = iface.is_up ? 'rgba(46, 213, 115, 0.2)' : 'rgba(255, 71, 87, 0.2)';
                            statusEl.style.color = iface.is_up ? '#2ed573' : '#ff4757';
                            
                            el.querySelector('.iface-ip').innerText = iface.ip;
                            el.querySelector('.iface-speed').innerText = iface.speed > 0 ? iface.speed + ' Mb/s' : 'N/A';
                            el.querySelector('.iface-sent').innerText = iface.bytes_sent;
                            el.querySelector('.iface-recv').innerText = iface.bytes_recv;
                        }
                    );
                }
            }

            // GPU Summary (General Page)
            const gpuListContainer = document.getElementById('general-gpu-list');
            if (gpuListContainer) {
                if (data.gpu && data.gpu.length > 0) {
                    updateList('general-gpu-list', data.gpu,
                        (gpu) => {
                            const div = document.createElement('div');
                            div.style.padding = '10px';
                            div.style.background = 'rgba(255,255,255,0.03)';
                            div.style.borderRadius = '6px';
                            div.style.borderLeft = '3px solid var(--accent-net)';
                            div.innerHTML = `
                                <div class="gpu-summary-name" style="font-size: 0.9rem; font-weight: bold; color: var(--accent-net); margin-bottom: 8px;"></div>
                                <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 8px; font-size: 0.85rem;">
                                    <div>
                                        <div style="color: var(--text-dim);">Temp</div>
                                        <div class="gpu-summary-temp" style="font-weight: bold;">-</div>
                                    </div>
                                    <div>
                                        <div style="color: var(--text-dim);">Load</div>
                                        <div class="gpu-summary-load" style="font-weight: bold;">-</div>
                                    </div>
                                    <div>
                                        <div style="color: var(--text-dim);">Freq</div>
                                        <div class="gpu-summary-freq" style="font-weight: bold;">-</div>
                                    </div>
                                    <div>
                                        <div style="color: var(--text-dim);">VRAM</div>
                                        <div class="gpu-summary-vram" style="font-weight: bold;">-</div>
                                    </div>
                                </div>
                            `;
                            return div;
                        },
                        (el, gpu) => {
                            el.querySelector('.gpu-summary-name').innerText = gpu.name || '-';
                            el.querySelector('.gpu-summary-temp').innerText = gpu.temp_c ? `${gpu.temp_c.toFixed(1)}°C` : '-';
                            el.querySelector('.gpu-summary-load').innerText = gpu.load_percent ? `${gpu.load_percent.toFixed(1)}%` : '-';
                            el.querySelector('.gpu-summary-freq').innerText = gpu.freq_mhz ? `${gpu.freq_mhz.toFixed(0)} MHz` : '-';
                            el.querySelector('.gpu-summary-vram').innerText = gpu.vram_used && gpu.vram_total ? `${gpu.vram_used} / ${gpu.vram_total}` : '-';
                        }
                    );
                } else {
                    gpuListContainer.innerHTML = '<div style="color: var(--text-dim); text-align: center; padding: 20px;">No GPU detected</div>';
                }
            }

            // SSH (General Page)
            if (data.ssh_stats) {
                const statusEl = document.getElementById('general-ssh-status');
                statusEl.innerText = data.ssh_stats.status;
                statusEl.style.color = data.ssh_stats.status === 'Running' ? '#2ed573' : '#ff4757';
                
                document.getElementById('general-ssh-connections').innerText = data.ssh_stats.connections;

                updateList('general-ssh-sessions', data.ssh_stats.sessions,
                    (session) => {
                        const div = document.createElement('div');
                        div.style.display = 'flex';
                        div.style.justifyContent = 'space-between';
                        div.style.borderBottom = '1px solid rgba(255,255,255,0.05)';
                        div.style.padding = '4px 0';
                        div.innerHTML = `
                            <div>
                                <span class="ssh-user" style="color: var(--accent-cpu); font-weight: bold;"></span>
                                <span class="ssh-ip" style="color: var(--text-dim); font-size: 0.8em; margin-left: 5px;"></span>
                            </div>
                            <span class="ssh-time" style="color: var(--text-dim); text-align: right;"></span>
                        `;
                        return div;
                    },
                        (el, session) => {
                            el.querySelector('.ssh-user').innerText = session.user || '-';
                            el.querySelector('.ssh-ip').innerText = session.ip ? `(${session.ip})` : '';
                            
                            let displayTime = session.started;
                            if (session.started && session.started !== '-' && session.started !== 0) {
                                try {
                                    const date = new Date(typeof session.started === 'number' ? session.started : session.started);
                                    displayTime = date.toLocaleString();
                                } catch (e) {
                                    displayTime = session.started;
                                }
                            }
                            el.querySelector('.ssh-time').innerText = displayTime || '-';
                        }
                );
                
                if (data.ssh_stats.sessions.length === 0) {
                    const container = document.getElementById('general-ssh-sessions');
                    if (container.innerHTML === '') {
                         container.innerHTML = '<div style="color: var(--text-dim); text-align: center; padding: 10px;">No active sessions</div>';
                    } else if (container.children.length > 0 && !container.querySelector('.ssh-user')) {
                         // Keep "No active sessions" if it's already there and we have no data
                    } else {
                         container.innerHTML = '<div style="color: var(--text-dim); text-align: center; padding: 10px;">No active sessions</div>';
                    }
                }
            }

            // Disk (General Page)
            if (data.disk) {
                updateList('disk-container', data.disk,
                (disk) => {
                    const div = document.createElement('div');
                    div.className = 'disk-item';
                    div.innerHTML = `
                        <div class="stat-label">
                            <span class="disk-mount"></span>
                            <span class="disk-percent"></span>
                        </div>
                        <div class="progress-bg">
                            <div class="progress-fill disk-fill" style="width: 0%"></div>
                        </div>
                        <div class="disk-info">
                            <span class="disk-device"></span>
                            <span class="disk-usage"></span>
                        </div>
                    `;
                    return div;
                },
                (el, disk) => {
                    el.querySelector('.disk-mount').innerText = disk.mountpoint;
                    el.querySelector('.disk-percent').innerText = disk.percent + '%';
                    el.querySelector('.disk-fill').style.width = disk.percent + '%';
                    el.querySelector('.disk-device').innerText = disk.device;
                    el.querySelector('.disk-usage').innerText = `${disk.used} used of ${disk.total}`;
                }
            );
            }

            // Storage Page
            const isStoragePage = document.getElementById('page-storage').classList.contains('active');
            if (isStoragePage && data.disk) {
                // Disk Overview - Aggregate by device
                const deviceMap = {};
                data.disk.forEach(disk => {
                    if (!deviceMap[disk.device]) {
                        deviceMap[disk.device] = {
                            device: disk.device,
                            total: disk.total,
                            used: disk.used,
                            free: disk.free,
                            percent: disk.percent,
                            mounts: []
                        };
                    }
                    deviceMap[disk.device].mounts.push(disk.mountpoint);
                });
                
                const devices = Object.values(deviceMap);
                updateList('disk-overview', devices,
                    (dev) => {
                        const div = document.createElement('div');
                        div.className = 'card';
                        div.style.padding = '15px';
                        div.style.marginBottom = '0';
                        div.innerHTML = `
                            <div style="font-weight: bold; margin-bottom: 10px; color: var(--accent-disk);" class="dev-name"></div>
                            <div class="progress-bg" style="height: 12px; margin-bottom: 8px;">
                                <div class="progress-fill disk-fill" style="width: 0%"></div>
                            </div>
                            <div style="display: flex; justify-content: space-between; font-size: 0.85rem;">
                                <span class="dev-used"></span>
                                <span class="dev-percent"></span>
                            </div>
                        `;
                        return div;
                    },
                    (el, dev) => {
                        el.querySelector('.dev-name').innerText = dev.device;
                        el.querySelector('.disk-fill').style.width = dev.percent + '%';
                        el.querySelector('.dev-used').innerText = `${dev.used} / ${dev.total}`;
                        el.querySelector('.dev-percent').innerText = dev.percent + '%';
                    }
                );

                // Partitions List
                updateList('storage-partitions', data.disk,
                    (disk) => {
                        const div = document.createElement('div');
                        div.style.background = 'rgba(255,255,255,0.03)';
                        div.style.padding = '15px';
                        div.style.borderRadius = '8px';
                        div.innerHTML = `
                            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 10px;">
                                <div>
                                    <div style="font-weight: bold; color: var(--accent-cpu);" class="part-mount"></div>
                                    <div style="font-size: 0.8rem; color: var(--text-dim); margin-top: 2px;" class="part-device"></div>
                                </div>
                                <div style="font-size: 1.2rem; font-weight: bold;" class="part-percent"></div>
                            </div>
                            <div class="progress-bg" style="height: 8px; margin-bottom: 8px;">
                                <div class="progress-fill disk-fill" style="width: 0%"></div>
                            </div>
                            <div style="display: flex; justify-content: space-between; font-size: 0.85rem; color: var(--text-dim);">
                                <span class="part-usage"></span>
                                <span class="part-free"></span>
                            </div>
                        `;
                        return div;
                    },
                    (el, disk) => {
                        el.querySelector('.part-mount').innerText = disk.mountpoint;
                        el.querySelector('.part-device').innerText = disk.device;
                        el.querySelector('.part-percent').innerText = disk.percent + '%';
                        el.querySelector('.disk-fill').style.width = disk.percent + '%';
                        el.querySelector('.part-usage').innerText = `Used: ${disk.used}`;
                        el.querySelector('.part-free').innerText = `Free: ${disk.free}`;
                    }
                );
            }

            // Disk I/O Stats
            if (isStoragePage && data.disk_io) {
                const ioList = Object.entries(data.disk_io).map(([name, stats]) => ({name, ...stats}));
                
                const container = document.getElementById('disk-io-stats');
                if (ioList.length === 0) {
                    container.innerHTML = '<div style="color: var(--text-dim); text-align: center; padding: 20px;">No I/O data available</div>';
                } else {
                    container.innerHTML = '';
                    ioList.forEach(io => {
                        const div = document.createElement('div');
                        div.className = 'process-item level-0';
                        div.style.cursor = 'default';
                        
                        div.innerHTML = `
                            <div class="process-info">
                                <div class="process-name">${io.name}</div>
                                <div class="process-meta">
                                    Read: ${io.read_bytes} (${io.read_count.toLocaleString()} ops) • 
                                    Write: ${io.write_bytes} (${io.write_count.toLocaleString()} ops)
                                </div>
                            </div>
                            <div class="process-bars" style="display: flex; gap: 15px; align-items: center;">
                                <div style="min-width: 80px;">
                                    <div style="font-size: 0.75rem; color: var(--text-dim);">Read Ops</div>
                                    <div style="font-size: 0.9rem; font-weight: bold; color: var(--accent-net);">${(io.read_count / 1000).toFixed(1)}k</div>
                                </div>
                                <div style="min-width: 80px;">
                                    <div style="font-size: 0.75rem; color: var(--text-dim);">Write Ops</div>
                                    <div style="font-size: 0.9rem; font-weight: bold; color: var(--accent-disk);">${(io.write_count / 1000).toFixed(1)}k</div>
                                </div>
                            </div>
                        `;
                        container.appendChild(div);
                    });
                }
            }

            // Inode Usage
            if (isStoragePage && data.inodes) {
                const container = document.getElementById('inode-usage');
                if (data.inodes.length === 0) {
                    container.innerHTML = '<div style="color: var(--text-dim); text-align: center; padding: 20px;">No inode data available</div>';
                } else {
                    container.innerHTML = '';
                    data.inodes.forEach(inode => {
                        const div = document.createElement('div');
                        div.style.padding = '15px';
                        div.style.background = 'rgba(255,255,255,0.05)';
                        div.style.borderRadius = '8px';
                        div.style.borderLeft = '4px solid var(--accent-disk)';
                        div.innerHTML = `
                            <div style="display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 10px;">
                                <div>
                                    <div style="font-weight: bold; margin-bottom: 5px;">${inode.mountpoint}</div>
                                    <div style="font-size: 0.85rem; color: var(--text-dim);">
                                        ${inode.used.toLocaleString()} / ${inode.total.toLocaleString()} inodes
                                    </div>
                                </div>
                                <div style="text-align: right;">
                                    <div style="font-weight: bold; font-size: 1.2rem; color: ${inode.percent > 80 ? '#ff4757' : 'var(--accent-mem)'};">${inode.percent}%</div>
                                    <div style="font-size: 0.8rem; color: var(--text-dim);">${inode.free.toLocaleString()} free</div>
                                </div>
                            </div>
                            <div class="progress-bg">
                                <div class="progress-fill" style="width: ${inode.percent}%; background: ${inode.percent > 80 ? '#ff4757' : 'var(--accent-disk)'}"></div>
                            </div>
                        `;
                        container.appendChild(div);
                    });
                }
            }

            // SSH Page Rendering (always update, not just when page is active)
            if (data.ssh_stats) {
                renderSSHStats(data.ssh_stats);
            }
        }
    }

function renderSSHStats(ssh) {
            if (!ssh) return;

            // SSH Status
            const statusEl = document.getElementById('ssh-status');
            if (statusEl) {
                statusEl.innerText = ssh.status || 'Unknown';
                statusEl.style.color =
                    ssh.status === 'Running' ? 'var(--accent-memory)' : '#ff4757';
            }

            // SSH Connections
            const connEl = document.getElementById('ssh-connections');
            if (connEl) connEl.innerText = ssh.connections || 0;

            // SSH Memory
            const memEl = document.getElementById('ssh-memory');
            if (memEl) {
                if (ssh.ssh_process_rss) {
                    memEl.innerText = ssh.ssh_process_rss;
                } else {
                    memEl.innerText = ssh.ssh_process_memory ? ssh.ssh_process_memory.toFixed(2) + '%' : '-';
                }
            }

            // Failed Logins
            const failedEl = document.getElementById('ssh-failed-logins');
            if (failedEl) failedEl.innerText = ssh.failed_logins || 0;

            // Active Sessions
            const sessContainer = document.getElementById('ssh-sessions');
            if (sessContainer) {
                if (!ssh.sessions || ssh.sessions.length === 0) {
                    sessContainer.innerHTML = '<div style="color: var(--text-dim); text-align: center; padding: 20px;">No active sessions</div>';
                } else {
                    sessContainer.innerHTML = '';
                    ssh.sessions.forEach(session => {
                        const div = document.createElement('div');
                        div.className = 'process-item level-0';
                        div.style.padding = '12px';
                        div.style.background = 'rgba(255,255,255,0.05)';
                        div.style.borderRadius = '6px';

                        // Convert UTC time to local time (same as General page)
                        let displayTime = session.started;
                        if (session.started && session.started !== '-' && session.started !== 0) {
                            try {
                                const date = new Date(typeof session.started === 'number' ? session.started : session.started);
                                displayTime = date.toLocaleString();
                            } catch (e) {
                                displayTime = session.started;
                            }
                        }

                        div.innerHTML = `
                            <div class="process-name">${session.user}@${session.ip}</div>
                            <div class="process-meta">Connected: ${displayTime}</div>
                        `;
                        sessContainer.appendChild(div);
                    });
                }
            }

            // Auth Methods
            const authContainer = document.getElementById('ssh-auth-methods');
            if (authContainer && ssh.auth_methods) {
                authContainer.innerHTML = `
                    <div style="display: flex; justify-content: space-between; padding: 8px 0; border-bottom: 1px solid rgba(255,255,255,0.1);">
                        <span style="color: var(--text-dim);">Public Key</span>
                        <span style="font-weight: bold; color: var(--accent-network);">${ssh.auth_methods.publickey || 0}</span>
                    </div>
                    <div style="display: flex; justify-content: space-between; padding: 8px 0; border-bottom: 1px solid rgba(255,255,255,0.1);">
                        <span style="color: var(--text-dim);">Password</span>
                        <span style="font-weight: bold; color: var(--accent-disk);">${ssh.auth_methods.password || 0}</span>
                    </div>
                    <div style="display: flex; justify-content: space-between; padding: 8px 0;">
                        <span style="color: var(--text-dim);">Other Methods</span>
                        <span style="font-weight: bold; color: var(--accent-cpu);">${ssh.auth_methods.other || 0}</span>
                    </div>
                `;
            }

            // Host Key Fingerprint
            const hostKeyEl = document.getElementById('ssh-hostkey');
            if (hostKeyEl) hostKeyEl.innerText = ssh.hostkey_fingerprint || '-';

            // Known Hosts
            const historyEl = document.getElementById('ssh-history');
            if (historyEl) {
                if (ssh.history_size !== undefined && ssh.history_size !== null) {
                    historyEl.innerText = `${ssh.history_size} entries`;
                } else {
                    historyEl.innerText = '-';
                }
            }

            // OOM Risk Processes
            const oomContainer = document.getElementById('ssh-oom-processes');
            if (oomContainer) {
                if (!ssh.oom_risk_processes || ssh.oom_risk_processes.length === 0) {
                    oomContainer.innerHTML = '<div style="color: var(--text-dim); text-align: center; padding: 20px;">All processes within safe memory range</div>';
                } else {
                    oomContainer.innerHTML = '';
                    ssh.oom_risk_processes.forEach(proc => {
                        const div = document.createElement('div');
                        div.style.padding = '12px';
                        div.style.background = 'rgba(255,107,107,0.1)';
                        div.style.borderRadius = '6px';
                        div.style.borderLeft = '4px solid #FF6B6B';
                        const memText = proc.memory_rss
                            || (proc.memory_percent !== undefined && proc.memory_percent !== null
                                ? proc.memory_percent.toFixed(1) + '%'
                                : (proc.memory !== undefined && proc.memory !== null ? proc.memory + '%' : '-'));
                        div.innerHTML = `
                            <div style="display: flex; justify-content: space-between; align-items: center;">
                                <div>
                                    <div style="font-weight: bold;">${proc.name}</div>
                                    <div style="font-size: 0.85rem; color: var(--text-dim);">PID: ${proc.pid}</div>
                                </div>
                                <div style="font-size: 1.2rem; font-weight: bold; color: #FF6B6B;">${memText}</div>
                            </div>
                        `;
                        oomContainer.appendChild(div);
                    });
                }
            }
        }

async function refreshSSHStats(force = true) {
            const btn = document.getElementById('ssh-refresh-btn');
            if (btn) btn.disabled = true;
            try {
                const url = force ? '/api/ssh/stats?force=1' : '/api/ssh/stats';
                const res = await fetch(url);
                if (!res.ok) {
                    throw new Error(`HTTP ${res.status}`);
                }
                const payload = await res.json();
                const ssh = payload.ssh_stats || payload;
                renderSSHStats(ssh);
            } catch (e) {
                console.warn('Failed to refresh SSH stats:', e);
            } finally {
                if (btn) btn.disabled = false;
            }
        }

window.addEventListener('DOMContentLoaded', function () {
            const btn = document.getElementById('ssh-refresh-btn');
            if (!btn) return;
            btn.addEventListener('click', function (e) {
                e.preventDefault();
                refreshSSHStats(true);
            });
        });

        // 注：认证/登出/fetch 拦截等逻辑已迁移到 static/js/auth.js 与 static/js/app.js。
