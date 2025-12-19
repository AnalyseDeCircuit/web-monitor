function formatSize(bytes) {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

// 主题切换功能
function initTheme() {
    // 从localStorage读取保存的主题,默认为dark
    const savedTheme = localStorage.getItem('theme') || 'dark';
    applyTheme(savedTheme);
}

// 主题循环顺序: dark -> light -> warm -> dark
const themeOrder = ['dark', 'light', 'warm'];

function toggleTheme() {
    const root = document.documentElement;
    const currentTheme = root.getAttribute('data-theme') || 'dark';
    const currentIndex = themeOrder.indexOf(currentTheme);
    // 如果当前主题不在列表中，默认从 dark 开始
    const nextIndex = currentIndex === -1 ? 1 : (currentIndex + 1) % themeOrder.length;
    const newTheme = themeOrder[nextIndex];
    
    applyTheme(newTheme);
    localStorage.setItem('theme', newTheme);
}

function applyTheme(theme) {
    const root = document.documentElement;
    const themeIcon = document.getElementById('theme-icon');
    
    // 确保 theme 是有效值
    if (!themeOrder.includes(theme)) {
        theme = 'dark';
    }
    
    root.setAttribute('data-theme', theme);
    
    // 更新图标
    if (themeIcon) {
        switch (theme) {
            case 'light':
                themeIcon.className = 'fas fa-sun';
                break;
            case 'warm':
                themeIcon.className = 'fas fa-mug-hot';
                break;
            default: // dark
                themeIcon.className = 'fas fa-moon';
        }
    }
    
    // 更新meta theme-color
    const metaThemeColor = document.querySelector('meta[name="theme-color"]');
    if (metaThemeColor) {
        const colors = { dark: '#0d0d0d', light: '#f5f5f5', warm: '#1a1410' };
        metaThemeColor.setAttribute('content', colors[theme] || '#0d0d0d');
    }
}

// 页面加载时初始化主题
document.addEventListener('DOMContentLoaded', initTheme);

/**
 * Efficiently update a list of DOM elements by reusing existing ones.
 * @param {string} containerId - ID of the container element
 * @param {Array} items - Data items to render
 * @param {Function} createFn - Function to create a new element: (item, index) => HTMLElement
 * @param {Function} updateFn - Function to update an existing element: (el, item, index) => void
 */
function updateList(containerId, items, createFn, updateFn) {
    const container = document.getElementById(containerId);
    if (!container) return;

    // Remove excess
    while (container.children.length > items.length) {
        container.removeChild(container.lastChild);
    }

    items.forEach((item, index) => {
        let el = container.children[index];
        if (!el) {
            el = createFn(item, index);
            container.appendChild(el);
        } else {
            updateFn(el, item, index);
        }
    });
}
