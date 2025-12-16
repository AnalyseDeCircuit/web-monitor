// Entrypoint: bootstrap only. All feature logic is split into domain files.

document.addEventListener('DOMContentLoaded', function () {
    checkAuthentication().then((ok) => {
        if (!ok) return;

        // Add logout button to sidebar
        const sidebar = document.querySelector('.sidebar');
        if (sidebar) {
            const logoutBtn = document.createElement('button');
            logoutBtn.innerHTML = '<i class="fas fa-sign-out-alt"></i> <span>Log out</span>';
            logoutBtn.className = 'nav-item';
            logoutBtn.style.marginTop = '10px';
            logoutBtn.style.borderTop = '1px solid rgba(255,255,255,0.1)';
            logoutBtn.style.paddingTop = '20px';
            logoutBtn.style.cursor = 'pointer';
            logoutBtn.onclick = (e) => {
                e.preventDefault();
                logout();
            };
            sidebar.appendChild(logoutBtn);
        }

        // Start WebSocket
        connectWebSocket();

        // Service worker registration
        if ('serviceWorker' in navigator) {
            window.addEventListener('load', () => {
                navigator.serviceWorker
                    .register('/sw.js')
                    .then((registration) => {
                        console.log('Service Worker registered:', registration);
                        setInterval(() => {
                            registration.update();
                        }, 60000);
                    })
                    .catch((err) => {
                        console.log('Service Worker registration failed:', err);
                    });
            });

            navigator.serviceWorker.addEventListener('controllerchange', () => {
                console.log('Service Worker controller changed - app updated');
                const msg = document.createElement('div');
                msg.style.cssText = `
                    position: fixed;
                    bottom: 20px;
                    right: 20px;
                    background: #2ed573;
                    color: #1a1a1a;
                    padding: 15px 20px;
                    border-radius: 8px;
                    font-weight: bold;
                    z-index: 10000;
                    animation: slideIn 0.3s ease;
                `;
                msg.textContent = '✓ App updated. Refresh to see changes.';
                document.body.appendChild(msg);
                setTimeout(() => msg.remove(), 5000);
            });
        }

        // PWA install prompt
        let deferredPrompt;

        window.addEventListener('beforeinstallprompt', (e) => {
            e.preventDefault();
            deferredPrompt = e;

            const installBtn = document.createElement('button');
            installBtn.style.cssText = `
                position: fixed;
                bottom: 20px;
                left: 20px;
                background: #00a8ff;
                color: #1a1a1a;
                padding: 10px 20px;
                border-radius: 8px;
                border: none;
                font-weight: bold;
                cursor: pointer;
                z-index: 9999;
                font-size: 14px;
                transition: all 0.3s ease;
            `;
            installBtn.innerHTML = '📱 Install App';
            installBtn.onmouseover = () => {
                installBtn.style.transform = 'scale(1.05)';
                installBtn.style.boxShadow = '0 0 15px rgba(0, 168, 255, 0.5)';
            };
            installBtn.onmouseout = () => {
                installBtn.style.transform = 'scale(1)';
                installBtn.style.boxShadow = 'none';
            };

            installBtn.addEventListener('click', async () => {
                if (deferredPrompt) {
                    deferredPrompt.prompt();
                    const { outcome } = await deferredPrompt.userChoice;
                    console.log(`User response: ${outcome}`);
                    deferredPrompt = null;
                    installBtn.remove();
                }
            });

            if (!window.matchMedia('(display-mode: standalone)').matches && !navigator.standalone) {
                document.body.appendChild(installBtn);
            }
        });

        window.addEventListener('appinstalled', () => {
            console.log('PWA was installed');
            deferredPrompt = null;
        });
    });
});
