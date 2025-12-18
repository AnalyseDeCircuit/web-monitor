function drawMemoryPieChart(memoryData) {
    const canvas = document.getElementById('mem-pie-chart');
    if (!canvas) return;

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

    const ctx = canvas.getContext('2d');
    const dpr = window.devicePixelRatio || 1;
    const width = canvas.clientWidth;
    const height = canvas.clientHeight;

    if (canvas.width !== width * dpr || canvas.height !== height * dpr) {
        canvas.width = width * dpr;
        canvas.height = height * dpr;
        canvas.style.width = width + 'px';
        canvas.style.height = height + 'px';
    }

    ctx.scale(dpr, dpr);
    ctx.clearRect(0, 0, width, height);

    let currentAngle = -Math.PI / 2;
    const radius = 70;
    const centerX = width / 2;
    const centerY = height / 2;

    segments.forEach((segment) => {
        const percentage = totalBytes > 0 ? segment.value / totalBytes : 0;
        const sliceAngle = percentage * 2 * Math.PI;

        ctx.beginPath();
        ctx.moveTo(centerX, centerY);
        ctx.arc(centerX, centerY, radius, currentAngle, currentAngle + sliceAngle);
        ctx.closePath();

        ctx.fillStyle = segment.color;
        ctx.fill();
        
        ctx.strokeStyle = '#121212';
        ctx.lineWidth = 2;
        ctx.stroke();

        currentAngle += sliceAngle;
    });

    // Reset scale for next call
    ctx.setTransform(1, 0, 0, 1, 0, 0);

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
