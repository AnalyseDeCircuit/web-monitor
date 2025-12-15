function drawMemoryPieChart(memoryData) {
    const svg = document.getElementById('mem-pie-chart');
    if (!svg) return;

    function parseMemory(str) {
        const match = String(str || '').match(/(\d+\.?\d*)\s*([GMK]iB)/);
        if (!match) return 0;

        const value = parseFloat(match[1]);
        const unit = match[2];

        const multipliers = {
            GiB: 1024 * 1024 * 1024,
            MiB: 1024 * 1024,
            KiB: 1024,
        };

        return value * (multipliers[unit] || 1);
    }

    const totalBytes = parseMemory(memoryData.total);
    const usedBytes = parseMemory(memoryData.used);
    const cachedBytes = parseMemory(memoryData.cached);
    const buffersBytes = parseMemory(memoryData.buffers);

    const cacheBufferBytes = cachedBytes + buffersBytes;
    const freeBytes = totalBytes - usedBytes - cacheBufferBytes;

    const segments = [
        { label: 'Used', value: usedBytes, color: '#ff4757' },
        { label: 'Buffer/Cache', value: cacheBufferBytes, color: '#2ed573' },
        { label: 'Free', value: Math.max(0, freeBytes), color: '#888' },
    ].filter((s) => s.value > 0);

    let currentAngle = -Math.PI / 2;
    const radius = 70;
    const centerX = 90;
    const centerY = 90;

    svg.innerHTML = '';

    segments.forEach((segment) => {
        const percentage = totalBytes > 0 ? segment.value / totalBytes : 0;
        const sliceAngle = percentage * 2 * Math.PI;

        const x1 = centerX + radius * Math.cos(currentAngle);
        const y1 = centerY + radius * Math.sin(currentAngle);

        currentAngle += sliceAngle;

        const x2 = centerX + radius * Math.cos(currentAngle);
        const y2 = centerY + radius * Math.sin(currentAngle);

        const largeArc = sliceAngle > Math.PI ? 1 : 0;

        const path = document.createElementNS('http://www.w3.org/2000/svg', 'path');
        path.setAttribute('d', `M ${centerX} ${centerY} L ${x1} ${y1} A ${radius} ${radius} 0 ${largeArc} 1 ${x2} ${y2} Z`);
        path.setAttribute('fill', segment.color);
        path.setAttribute('stroke', '#121212');
        path.setAttribute('stroke-width', '2');
        svg.appendChild(path);
    });

    const legend = document.getElementById('mem-pie-legend');
    if (legend) {
        legend.innerHTML = segments
            .map(
                (s) => `
                    <div style="display: flex; align-items: center; gap: 8px;">
                        <div style="width: 12px; height: 12px; background-color: ${s.color}; border-radius: 2px;"></div>
                        <div>
                            <div style="font-weight: bold;">${s.label}</div>
                            <div style="color: var(--text-dim); font-size: 0.75rem;">${totalBytes > 0 ? ((s.value / totalBytes) * 100).toFixed(1) : '0.0'}%</div>
                        </div>
                    </div>
                `
            )
            .join('');
    }
}
