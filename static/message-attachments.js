/* Załączniki wiadomości — izolowany interfejs pobierania na iPhone/desktop. */
(() => {
  'use strict';

  const active = new Set();
  let generation = 0;

  function clear() {
    ++generation;
    for (const controller of active) controller.abort();
    active.clear();
  }

  function safeFilename(response, fallback) {
    const disposition = response.headers.get('Content-Disposition') || '';
    const extended = /filename\*\s*=\s*UTF-8''([^;]+)/i.exec(disposition);
    let name = '';
    if (extended) {
      try { name = decodeURIComponent(extended[1].trim().replace(/^"|"$/g, '')); } catch (_) {}
    }
    if (!name) {
      const quoted = /filename\s*=\s*"([^"]*)"/i.exec(disposition);
      name = quoted ? quoted[1] : fallback;
    }
    return String(name || fallback || 'zalacznik.bin').replace(/\\/g, '/')
      .split('/').pop().replace(/[\x00-\x1f\x7f<>:"/\\|?*]/g, '_').trim() || 'zalacznik.bin';
  }

  function element(tag, cls, text) {
    const el = document.createElement(tag);
    if (cls) el.className = cls;
    if (text !== undefined) el.textContent = String(text);
    return el;
  }

  async function download(btn, file, ctx, status, epoch) {
    if (btn.disabled || epoch !== generation) return;
    btn.disabled = true;
    const original = btn.textContent;
    btn.textContent = 'Pobieranie…';
    status.textContent = 'Przygotowywanie załącznika. Możesz korzystać z pozostałych modułów.';
    const controller = new AbortController();
    active.add(controller);
    try {
      const params = new URLSearchParams({profile_id: ctx.profileId, mailbox: ctx.mailbox});
      const path = '/api/message-attachments/' +
        encodeURIComponent(ctx.messageId) + '/' +
        encodeURIComponent(file.id) + '/download?' + params.toString();
      const response = await fetch(path, {credentials: 'same-origin', cache: 'no-store',
                                         signal: controller.signal});
      if (!response.ok) {
        let message = 'Nie udało się pobrać załącznika (HTTP ' + response.status + ').';
        try {
          const body = await response.json();
          if (body && typeof body.detail === 'string') message = body.detail;
        } catch (_) {}
        throw new Error(message);
      }
      const blob = await response.blob();
      if (blob.size > 20 * 1024 * 1024) throw new Error('Załącznik przekracza limit 20 MiB.');
      if (controller.signal.aborted || epoch !== generation) return;
      const filename = safeFilename(response, file.filename || 'zalacznik.bin');
      const objectUrl = URL.createObjectURL(blob);
      try {
        const link = document.createElement('a');
        link.href = objectUrl;
        link.download = filename;
        link.style.display = 'none';
        document.body.appendChild(link);
        link.click();
        link.remove();
      } finally {
        setTimeout(() => URL.revokeObjectURL(objectUrl), 30000);
      }
      status.textContent = 'Rozpoczęto pobieranie: ' + filename;
    } catch (error) {
      if (!controller.signal.aborted && epoch === generation) {
        status.textContent = error?.message || 'Błąd pobierania załącznika.';
      }
    } finally {
      active.delete(controller);
      btn.disabled = false;
      btn.textContent = original;
    }
  }

  function enhance(host, message, context) {
    if (!host || !message || !context?.profileId || !context?.messageId) return;
    clear();
    const files = Array.isArray(message.attachment_files) ? message.attachment_files : [];
    if (!files.length) return;
    const epoch = generation;
    const section = element('section', 'message-files-panel');
    section.setAttribute('aria-label', 'Załączniki wiadomości');
    section.appendChild(element('h3', 'message-files-title', 'Załączniki (' + files.length + ')'));
    const list = element('div', 'message-files-list');
    const status = element('p', 'message-files-status',
      'Pobieranie rozpocznie się po kliknięciu wybranego pliku.');
    status.setAttribute('role', 'status');
    status.setAttribute('aria-live', 'polite');
    files.forEach((file) => {
      if (!file || !file.id) return;
      const row = element('div', 'message-files-row');
      const icon = element('span', 'message-files-icon', '▤');
      icon.setAttribute('aria-hidden', 'true');
      const info = element('span', 'message-files-info',
        file.filename || ('Załącznik ' + file.id));
      const btn = element('button', 'button button-outline button-sm', '↓ Pobierz');
      btn.type = 'button';
      btn.setAttribute('aria-label', 'Pobierz: ' + (file.filename || 'Załącznik'));
      btn.addEventListener('click', () => download(btn, file, context, status, epoch));
      row.append(icon, info, btn);
      list.append(row);
    });
    section.append(list, status);
    host.appendChild(section);
  }

  window.SynergiaMessageAttachments = {enhance, clear};
})();
