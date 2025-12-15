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
    checkRole();

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

function switchPage(pageId) {
    closeMobileMenu();

    document.querySelectorAll('.page').forEach((el) => el.classList.remove('active'));
    document.getElementById('page-' + pageId).classList.add('active');

    document.querySelectorAll('.nav-item').forEach((el) => el.classList.remove('active'));

    const navItem = document.querySelector(`.nav-item[onclick*="'${pageId}'"]`);
    if (navItem) {
        navItem.classList.add('active');
        if (['net-traffic', 'ssh'].includes(pageId)) {
            document.getElementById('network-submenu').style.display = 'flex';
            document.getElementById('net-submenu-icon').classList.remove('fa-chevron-down');
            document.getElementById('net-submenu-icon').classList.add('fa-chevron-up');
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
        users: 'User Management',
        logs: 'Operation Logs',
        profile: 'My Profile',
    };
    if (titles[pageId]) {
        document.getElementById('page-title').innerText = titles[pageId];
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
}
