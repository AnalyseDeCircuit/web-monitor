function drawChart(containerId, data, options = {}) {
    const container = document.getElementById(containerId);
    if (!container) return;

    const width = container.clientWidth;
    const height = container.clientHeight;
    const padding = 5;

    container.innerHTML = '';

    if (!data || data.length === 0) {
        container.innerHTML = '<div style="color: var(--text-dim); font-size: 0.8rem; text-align: center; width: 100%;">No Data</div>';
        return;
    }

    const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    svg.setAttribute('width', '100%');
    svg.setAttribute('height', '100%');
    svg.setAttribute('viewBox', `0 0 ${width} ${height}`);
    svg.style.overflow = 'visible';

    const maxVal = options.max !== undefined ? options.max : Math.max(...data) * 1.1;
    const minVal = options.min !== undefined ? options.min : Math.min(...data) * 0.9;
    const range = maxVal - minVal || 1;

    let pathD = '';
    const stepX = (width - 2 * padding) / (data.length - 1);

    data.forEach((val, i) => {
        const x = padding + i * stepX;
        const y = height - padding - ((val - minVal) / range) * (height - 2 * padding);
        if (i === 0) pathD += `M ${x} ${y}`;
        else pathD += ` L ${x} ${y}`;
    });

    const defs = document.createElementNS('http://www.w3.org/2000/svg', 'defs');
    const gradientId = 'grad-' + containerId;
    const gradient = document.createElementNS('http://www.w3.org/2000/svg', 'linearGradient');
    gradient.setAttribute('id', gradientId);
    gradient.setAttribute('x1', '0%');
    gradient.setAttribute('y1', '0%');
    gradient.setAttribute('x2', '0%');
    gradient.setAttribute('y2', '100%');

    const stop1 = document.createElementNS('http://www.w3.org/2000/svg', 'stop');
    stop1.setAttribute('offset', '0%');
    stop1.setAttribute('stop-color', options.color || '#ff4757');
    stop1.setAttribute('stop-opacity', '0.5');

    const stop2 = document.createElementNS('http://www.w3.org/2000/svg', 'stop');
    stop2.setAttribute('offset', '100%');
    stop2.setAttribute('stop-color', options.color || '#ff4757');
    stop2.setAttribute('stop-opacity', '0.05');

    gradient.appendChild(stop1);
    gradient.appendChild(stop2);
    defs.appendChild(gradient);
    svg.appendChild(defs);

    const areaPath = document.createElementNS('http://www.w3.org/2000/svg', 'path');
    areaPath.setAttribute('d', pathD + ` L ${width - padding} ${height - padding} L ${padding} ${height - padding} Z`);
    areaPath.setAttribute('fill', `url(#${gradientId})`);
    svg.appendChild(areaPath);

    const linePath = document.createElementNS('http://www.w3.org/2000/svg', 'path');
    linePath.setAttribute('d', pathD);
    linePath.setAttribute('fill', 'none');
    linePath.setAttribute('stroke', options.color || '#ff4757');
    linePath.setAttribute('stroke-width', '2');
    svg.appendChild(linePath);

    const fontSize = 10;
    const textStyle = `font-size: ${fontSize}px; fill: var(--text-dim); font-family: monospace;`;

    const maxText = document.createElementNS('http://www.w3.org/2000/svg', 'text');
    maxText.setAttribute('x', padding);
    maxText.setAttribute('y', padding + fontSize);
    maxText.setAttribute('style', textStyle);
    maxText.textContent = maxVal.toFixed(0);
    svg.appendChild(maxText);

    const minText = document.createElementNS('http://www.w3.org/2000/svg', 'text');
    minText.setAttribute('x', padding);
    minText.setAttribute('y', height - padding);
    minText.setAttribute('style', textStyle);
    minText.textContent = minVal.toFixed(0);
    svg.appendChild(minText);

    container.appendChild(svg);
}
