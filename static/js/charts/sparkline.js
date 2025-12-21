// Helper to resolve CSS variable to actual color
function resolveCssColor(color) {
    if (color.startsWith('var(')) {
        const varName = color.match(/var\(([^)]+)\)/)?.[1];
        if (varName) {
            return getComputedStyle(document.documentElement).getPropertyValue(varName).trim() || '#4dabf7';
        }
    }
    return color;
}

function drawChart(containerId, data, options = {}) {
    const container = document.getElementById(containerId);
    if (!container) return;

    const width = container.clientWidth;
    const height = container.clientHeight;
    const padding = 5;

    if (!data || data.length === 0) {
        container.innerHTML = '<div style="color: var(--text-dim); font-size: 0.8rem; text-align: center; width: 100%;">No Data</div>';
        return;
    }

    // Reuse or create canvas
    let canvas = container.querySelector('canvas');
    if (!canvas) {
        container.innerHTML = '';
        canvas = document.createElement('canvas');
        container.appendChild(canvas);
    }

    // Handle High DPI displays
    const dpr = window.devicePixelRatio || 1;
    if (canvas.width !== width * dpr || canvas.height !== height * dpr) {
        canvas.width = width * dpr;
        canvas.height = height * dpr;
        canvas.style.width = width + 'px';
        canvas.style.height = height + 'px';
    }

    const ctx = canvas.getContext('2d');
    ctx.scale(dpr, dpr);
    ctx.clearRect(0, 0, width, height);

    const maxVal = options.max !== undefined ? options.max : Math.max(...data) * 1.1;
    const minVal = options.min !== undefined ? options.min : Math.min(...data) * 0.9;
    const range = maxVal - minVal || 1;
    const color = options.color || '#ff4757';

    const stepX = (width - 2 * padding) / (data.length - 1);
    
    const points = data.map((val, i) => ({
        x: padding + i * stepX,
        y: height - padding - ((val - minVal) / range) * (height - 2 * padding)
    }));

    // Resolve CSS variable to actual color value
    const resolvedColor = resolveCssColor(color);
    
    // 1. Draw Area Gradient
    const gradient = ctx.createLinearGradient(0, 0, 0, height);
    gradient.addColorStop(0, resolvedColor + '80'); // 0.5 opacity
    gradient.addColorStop(1, resolvedColor + '0D'); // 0.05 opacity

    ctx.beginPath();
    ctx.moveTo(points[0].x, points[0].y);
    points.forEach(p => ctx.lineTo(p.x, p.y));
    ctx.lineTo(points[points.length - 1].x, height - padding);
    ctx.lineTo(points[0].x, height - padding);
    ctx.closePath();
    ctx.fillStyle = gradient;
    ctx.fill();

    // 2. Draw Line
    ctx.beginPath();
    ctx.moveTo(points[0].x, points[0].y);
    points.forEach(p => ctx.lineTo(p.x, p.y));
    ctx.strokeStyle = resolvedColor;
    ctx.lineWidth = 2;
    ctx.lineJoin = 'round';
    ctx.stroke();

    // 3. Draw Text (Min/Max)
    const fontSize = 10;
    ctx.font = `${fontSize}px monospace`;
    ctx.fillStyle = getComputedStyle(document.documentElement).getPropertyValue('--text-dim').trim() || '#888';
    
    ctx.fillText(maxVal.toFixed(0), padding, padding + fontSize);
    ctx.fillText(minVal.toFixed(0), padding, height - padding);
    
    // Reset scale for next call if reused
    ctx.setTransform(1, 0, 0, 1, 0, 0);
}
