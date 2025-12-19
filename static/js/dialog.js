/**
 * Custom dialog system to replace native confirm() and alert()
 * Lightweight, no external dependencies
 */

(function () {
    'use strict';

    let dialogResolve = null;
    let isAlert = false;

    const overlay = document.getElementById('app-dialog-overlay');
    const dialog = document.getElementById('app-dialog');
    const iconEl = document.getElementById('app-dialog-icon');
    const titleEl = document.getElementById('app-dialog-title');
    const bodyEl = document.getElementById('app-dialog-body');
    const cancelBtn = document.getElementById('app-dialog-cancel');
    const okBtn = document.getElementById('app-dialog-ok');

    // Icon mappings
    const icons = {
        confirm: 'fa-question-circle',
        alert: 'fa-exclamation-circle',
        error: 'fa-times-circle',
        success: 'fa-check-circle',
        info: 'fa-info-circle'
    };

    function showDialog(options) {
        const {
            type = 'confirm',
            title = type === 'confirm' ? 'Confirm' : 'Notice',
            message = '',
            okText = 'OK',
            cancelText = 'Cancel',
            okClass = 'btn-primary'
        } = options;

        isAlert = type !== 'confirm';

        // Set icon
        iconEl.className = 'fas ' + (icons[type] || icons.confirm);
        iconEl.classList.add('icon-' + type);

        // Set content
        titleEl.textContent = title;
        bodyEl.textContent = message;
        okBtn.textContent = okText;
        okBtn.className = 'btn ' + okClass;
        cancelBtn.textContent = cancelText;

        // Show/hide cancel button
        cancelBtn.style.display = isAlert ? 'none' : '';

        // Show dialog
        overlay.classList.add('show');
        okBtn.focus();

        return new Promise((resolve) => {
            dialogResolve = resolve;
        });
    }

    function closeDialog(result) {
        overlay.classList.remove('show');
        if (dialogResolve) {
            dialogResolve(result);
            dialogResolve = null;
        }
    }

    // Event handlers
    okBtn.addEventListener('click', () => closeDialog(true));
    cancelBtn.addEventListener('click', () => closeDialog(false));

    // Close on overlay click
    overlay.addEventListener('click', (e) => {
        if (e.target === overlay) {
            closeDialog(isAlert ? true : false);
        }
    });

    // Keyboard support
    document.addEventListener('keydown', (e) => {
        if (!overlay.classList.contains('show')) return;

        if (e.key === 'Escape') {
            closeDialog(isAlert ? true : false);
        } else if (e.key === 'Enter') {
            closeDialog(true);
        }
    });

    // Public API
    window.appConfirm = function (message, options = {}) {
        return showDialog({
            type: 'confirm',
            message,
            ...options
        });
    };

    window.appAlert = function (message, options = {}) {
        const type = options.type || 'alert';
        return showDialog({
            type,
            title: options.title || (type === 'error' ? 'Error' : type === 'success' ? 'Success' : 'Notice'),
            message,
            okText: options.okText || 'OK',
            okClass: type === 'error' ? 'btn-danger' : type === 'success' ? 'btn-success' : 'btn-primary',
            ...options
        });
    };

    // Shorthand for common alerts
    window.appError = (msg) => window.appAlert(msg, { type: 'error' });
    window.appSuccess = (msg) => window.appAlert(msg, { type: 'success' });
})();
