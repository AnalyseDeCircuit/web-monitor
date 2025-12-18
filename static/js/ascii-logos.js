// ASCII Logos Loader  
// Logos from: https://github.com/fastfetch-cli/fastfetch (正常大小版本)
// License: MIT

// Distribution mapping (使用完整大小的 logos)
const ASCII_LOGO_MAP = {
    'linux': 'linux.txt',
    'ubuntu': 'ubuntu.txt',
    'debian': 'debian.txt',
    'arch': 'arch.txt',
    'fedora': 'fedora.txt',
    'centos': 'centos.txt',
    // fastfetch uses redhat.txt
    'rhel': 'redhat.txt',
    'redhat': 'redhat.txt',
    'opensuse': 'opensuse.txt',
    'manjaro': 'manjaro.txt',
    'alpine': 'alpine.txt',
    'rocky': 'rocky.txt',
    'kali': 'kali.txt',
    'gentoo': 'gentoo.txt',
    'nixos': 'nixos.txt',
    'pop': 'pop.txt',
    'elementary': 'elementary.txt',
    'mint': 'linuxmint.txt',
    'linuxmint': 'linuxmint.txt',
    'almalinux': 'almalinux.txt',
    'zorin': 'zorin.txt'
};

// Distribution-specific color palettes
const COLOR_PALETTES = {
    'ubuntu': {
        '1': '#E95420',  // Ubuntu orange
        '2': '#772953'   // Ubuntu aubergine
    },
    'fedora': {
        '1': '#294172',  // Fedora dark blue
        '2': '#3C6EB4'   // Fedora light blue
    },
    'debian': {
        '1': '#A80030',  // Debian red
        '2': '#D70751'   // Debian magenta
    },
    'arch': {
        '1': '#1793D1',  // Arch blue
        '2': '#0088CC'   // Arch light blue
    },
    'manjaro': {
        '1': '#35BF5C',  // Manjaro green
        '2': '#00D9A3'   // Manjaro teal
    },
    'mint': {
        '1': '#87CF3E',  // Mint green
        '2': '#5EAC24'   // Mint dark green
    },
    'default': {
        '1': '#FCC624',  // Yellow
        '2': '#97D700',  // Green
        '3': '#FF6600',  // Orange
        '4': '#00B2EE',  // Blue
        '5': '#A347BA',  // Purple
        '6': '#E63462',  // Pink
        '7': '#CCCCCC',  // Light gray
        '8': '#FFFFFF'   // White
    }
};

// Cache for loaded ASCII art
const asciiCache = {};

// Detect OS and return appropriate logo key
function getOSLogo() {
    const osName = (window.systemInfo && window.systemInfo.os) || '';
    const osLower = osName.toLowerCase();
    
    if (osLower.includes('pop')) return 'pop';
    if (osLower.includes('elementary')) return 'elementary';
    if (osLower.includes('mint')) return 'mint';
    if (osLower.includes('ubuntu')) return 'ubuntu';
    if (osLower.includes('debian')) return 'debian';
    if (osLower.includes('manjaro')) return 'manjaro';
    if (osLower.includes('arch')) return 'arch';
    if (osLower.includes('fedora')) return 'fedora';
    if (osLower.includes('centos')) return 'centos';
    if (osLower.includes('red hat') || osLower.includes('rhel')) return 'rhel';
    if (osLower.includes('opensuse') || osLower.includes('suse')) return 'opensuse';
    if (osLower.includes('alpine')) return 'alpine';
    if (osLower.includes('rocky')) return 'rocky';
    if (osLower.includes('alma')) return 'almalinux';
    if (osLower.includes('kali')) return 'kali';
    if (osLower.includes('gentoo')) return 'gentoo';
    if (osLower.includes('nixos')) return 'nixos';
    if (osLower.includes('zorin')) return 'zorin';
    
    return 'linux';
}

// Get color palette for a distribution
function getColorPalette(logoKey) {
    return COLOR_PALETTES[logoKey] || COLOR_PALETTES['default'];
}

// Load ASCII art from file
async function loadASCIIArt(logoKey) {
    if (asciiCache[logoKey]) {
        return asciiCache[logoKey];
    }
    
    const filename = ASCII_LOGO_MAP[logoKey] || ASCII_LOGO_MAP['linux'];
    // Some deployments only expose "/static-hashed/" (templates use hashed assets).
    // Our router does NOT validate the hash segment, so we can safely use a dummy one.
    const urlCandidates = [
        `/static/ascii-logos/${filename}`,
        `/static-hashed/00000000/ascii-logos/${filename}`
    ];
    
    try {
        let response = null;

        for (const url of urlCandidates) {
            // eslint-disable-next-line no-await-in-loop
            const res = await fetch(url);
            if (res.ok) {
                response = res;
                break;
            }
        }

        if (!response) {
            throw new Error(`Failed to load ${filename}`);
        }

        const text = await response.text();
        asciiCache[logoKey] = text;
        return text;
    } catch (error) {
        console.warn('Failed to load ASCII logo:', error);
        return 'Linux';
    }
}

// Format ASCII art with color support
function formatASCIIArt(art, logoKey) {
    const colors = getColorPalette(logoKey);
    const lines = art.trim().split('\n');
    
    return lines.map(line => {
        let html = '';
        let currentColor = null;
        let i = 0;
        
        while (i < line.length) {
            if (line[i] === '$' && i + 1 < line.length && /[1-8]/.test(line[i + 1])) {
                if (currentColor !== null) {
                    html += '</span>';
                }
                
                const colorNum = line[i + 1];
                currentColor = colors[colorNum] || colors['1'];
                html += `<span style="color: ${currentColor}">`;
                i += 2;
            } else {
                const char = line[i];
                if (char === '&') html += '&amp;';
                else if (char === '<') html += '&lt;';
                else if (char === '>') html += '&gt;';
                else if (char === ' ') html += '&nbsp;';
                else html += char;
                i++;
            }
        }
        
        if (currentColor !== null) {
            html += '</span>';
        }
        
        return html || '&nbsp;';
    }).join('\n');
}

// Main export function
async function getFormattedASCIILogo() {
    const logoKey = getOSLogo();
    const ascii = await loadASCIIArt(logoKey);
    return formatASCIIArt(ascii, logoKey);
}

// Ensure global access even if bundled/strict contexts change
window.getFormattedASCIILogo = getFormattedASCIILogo;
