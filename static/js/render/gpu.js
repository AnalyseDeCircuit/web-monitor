// GPU Rendering Logic
const gpuCharts = {};

function renderGPUs(gpus) {
    const gpuGrid = document.getElementById('gpu-grid');
    if (!gpuGrid) return;

    if (!gpus || gpus.length === 0) {
        gpuGrid.innerHTML = '<div style="color: #888; text-align: center; padding: 20px;">No GPUs detected or driver not loaded.</div>';
        return;
    }

    gpus.forEach((gpu, index) => {
        let card = document.getElementById(`gpu-card-${index}`);
        if (!card) {
            card = createGPUCard(gpu, index);
            gpuGrid.appendChild(card);
            initGPUCharts(index);
        }
        updateGPUCard(gpu, index);
    });
}

function createGPUCard(gpu, index) {
    const div = document.createElement('div');
    div.className = 'card';
    div.id = `gpu-card-${index}`;
    div.innerHTML = `
        <div class="card-header">
            <div>
                <div class="gpu-name">${gpu.name}</div>
                <div class="gpu-meta">
                    <i class="fas fa-microchip"></i> ${gpu.vendor} &nbsp;|&nbsp;
                    <i class="fas fa-map-marker-alt"></i> ${gpu.pci_address} &nbsp;|&nbsp;
                    <i class="fas fa-terminal"></i> ${gpu.drm_card}
                </div>
            </div>
            <div class="gpu-temp" id="temp-${index}">--°C</div>
        </div>

        <div class="stat-row">
            <div class="stat-item">
                <span class="stat-label">Frequency</span>
                <span class="stat-value" id="freq-${index}">--</span> <span style="font-size:0.8em; color:#888">MHz</span>
            </div>
            <div class="stat-item">
                <span class="stat-label">Power</span>
                <span class="stat-value" id="power-${index}">--</span> <span style="font-size:0.8em; color:#888">W</span>
            </div>
            <div class="stat-item">
                <span class="stat-label">Load</span>
                <span class="stat-value" id="load-${index}">--</span> <span style="font-size:0.8em; color:#888">%</span>
            </div>
        </div>

        <div class="vram-container">
            <div class="vram-details">
                <div style="display:flex; justify-content:space-between; margin-bottom:5px;">
                    <span style="color:var(--text-dim)">VRAM Usage</span>
                    <span id="vram-text-${index}" style="font-weight:bold">-- / --</span>
                </div>
                <div class="vram-bar-bg">
                    <div id="vram-bar-${index}" class="vram-bar-fill"></div>
                </div>
            </div>
        </div>

        <div class="chart-container">
            <canvas id="chart-load-${index}"></canvas>
        </div>
    `;
    return div;
}

function initGPUCharts(index) {
    const ctxLoad = document.getElementById(`chart-load-${index}`).getContext('2d');
    gpuCharts[`load-${index}`] = new Chart(ctxLoad, {
        type: 'line',
        data: {
            labels: Array(30).fill(''),
            datasets: [
                {
                    label: 'Load %',
                    data: Array(30).fill(0),
                    borderColor: '#ff4757',
                    backgroundColor: 'rgba(255, 71, 87, 0.1)',
                    fill: true,
                    tension: 0.4,
                    pointRadius: 0,
                    borderWidth: 2,
                },
                {
                    label: 'Power W',
                    data: Array(30).fill(0),
                    borderColor: '#ffa502',
                    backgroundColor: 'transparent',
                    tension: 0.4,
                    pointRadius: 0,
                    borderWidth: 2,
                    yAxisID: 'y1',
                },
            ],
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            plugins: { legend: { display: true, labels: { color: '#888', boxWidth: 10 } } },
            scales: {
                x: { display: false },
                y: {
                    min: 0,
                    max: 100,
                    grid: { color: 'rgba(255,255,255,0.05)' },
                    ticks: { color: '#888' },
                    title: { display: true, text: 'Load %', color: '#ff4757' },
                },
                y1: {
                    position: 'right',
                    grid: { display: false },
                    ticks: { color: '#ffa502' },
                    title: { display: true, text: 'Power W', color: '#ffa502' },
                },
            },
            animation: false,
            interaction: {
                mode: 'index',
                intersect: false,
            },
        },
    });
}

function updateGPUCard(gpu, index) {
    const tempEl = document.getElementById(`temp-${index}`);
    if (tempEl) tempEl.innerText = gpu.temp_c ? `${gpu.temp_c.toFixed(1)}°C` : 'N/A';

    const freqEl = document.getElementById(`freq-${index}`);
    if (freqEl) freqEl.innerText = gpu.freq_mhz ? gpu.freq_mhz.toFixed(0) : '--';

    const powerEl = document.getElementById(`power-${index}`);
    if (powerEl) powerEl.innerText = gpu.power_w ? gpu.power_w.toFixed(1) : '--';

    const loadEl = document.getElementById(`load-${index}`);
    if (loadEl) loadEl.innerText = gpu.load_percent ? gpu.load_percent.toFixed(1) : '--';

    const vramTextEl = document.getElementById(`vram-text-${index}`);
    if (vramTextEl) vramTextEl.innerText = `${gpu.vram_used} / ${gpu.vram_total}`;

    const vramPercent = gpu.vram_percent || 0;
    const vramBarEl = document.getElementById(`vram-bar-${index}`);
    if (vramBarEl) vramBarEl.style.width = `${vramPercent}%`;

    if (gpuCharts[`load-${index}`]) {
        const chart = gpuCharts[`load-${index}`];
        const load = gpu.load_percent || 0;
        const power = gpu.power_w || 0;

        chart.data.datasets[0].data.push(load);
        chart.data.datasets[0].data.shift();

        chart.data.datasets[1].data.push(power);
        chart.data.datasets[1].data.shift();

        chart.update();
    }
}
