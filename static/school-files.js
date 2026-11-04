/* Niezależny interfejs Dokumentów szkoły. Nie modyfikuje pozostałych zakładek. */
'use strict';
(function () {
  const toText = value => String(value == null ? '' : value);
  const esc = value => toText(value).replace(/[&<>"']/g, c => ({
    '&':'&amp;', '<':'&lt;', '>':'&gt;', '"':'&quot;', "'":'&#39;'
  }[c]));
  function dateLabel(value) {
    if (!value) return 'Data niepodana';
    const d = new Date(toText(value).slice(0, 10) + 'T12:00:00');
    return Number.isNaN(d.getTime()) ? 'Data niepodana' :
      new Intl.DateTimeFormat('pl-PL',{day:'numeric',month:'long',year:'numeric'}).format(d);
  }
  function notice(message) {
    if (typeof window.statusToast === 'function') { window.statusToast(message); return; }
    const node = document.getElementById('school-download-status') || document.getElementById('toast');
    if (node) { node.textContent = message; node.classList.remove('hidden'); }
  }
  function filename(response, fallback) {
    const cd = response.headers.get('Content-Disposition') || '';
    const encoded = /filename\*\s*=\s*UTF-8''([^;]+)/i.exec(cd);
    let name = '';
    if (encoded) {
      try { name = decodeURIComponent(encoded[1].trim().replace(/^"|"$/g,'')); } catch (_) {}
    }
    if (!name) {
      const quoted = /filename\s*=\s*"([^"]*)"/i.exec(cd);
      name = quoted ? quoted[1] : fallback;
    }
    return toText(name || fallback || 'dokument_szkolny.bin').replace(/\\/g,'/').split('/').pop()
      .replace(/[\x00-\x1f\x7f<>:"/\\|?*]/g,'_').trim() || 'dokument_szkolny.bin';
  }
  function card(doc, i) {
    const id = toText(doc.id);
    const name = toText(doc.name || 'Dokument szkolny');
    return '<article class="school-file-card">' +
      '<div class="school-file-symbol" aria-hidden="true">▤</div>' +
      '<div class="school-file-meta"><h3>' + esc(name) + '</h3>' +
      '<p>Dodano: ' + esc(dateLabel(doc.added)) + '</p></div>' +
      '<div class="school-file-actions">' +
      (doc.can_download
        ? '<button type="button" class="button button-primary button-sm" data-school-download="' +
          esc(id) + '" data-school-filename="' + esc(name) +
          '" aria-label="Pobierz dokument: ' + esc(name) + '">↓ Pobierz</button>'
        : '<span class="school-file-disabled">Brak pliku do pobrania</span>') +
      '</div></article>';
  }
  async function view(requestFn) {
    const data = await requestFn('/api/school-files');
    const items = Array.isArray(data.items) ? data.items : [];
    const cards = items.map(card).join('');
    const heading = '<div class="content-head"><div><span class="eyebrow">SYNERGIA POCKET</span>' +
      '<h1>Dokumenty szkoły</h1><p>Pliki udostępnione przez szkołę dla wybranego profilu dziecka.</p></div></div>';
    const banner = data.demo ?
      '<div class="school-file-demo">Tryb demonstracyjny — poniższy dokument jest fikcyjny.</div>' : '';
    const truncated = data.truncated ?
      '<p class="school-file-info">Wyświetlono pierwsze 500 dokumentów.</p>' : '';
    const body = cards || '<div class="empty">Szkoła nie udostępnia obecnie dokumentów na tym koncie.</div>';
    return heading + banner + '<section class="panel school-files-panel">' +
      '<div class="section-row"><h2 class="section-title">Dokumenty (' + items.length + ')</h2></div>' +
      '<p id="school-download-status" class="school-file-info" role="status" aria-live="polite">' +
      'Pobieranie uruchamia się dopiero po kliknięciu „Pobierz”.</p>' +
      '<div class="school-files-list">' + body + '</div>' + truncated + '</section>';
  }
  async function download(button) {
    if (button.disabled) return;
    const id = button.dataset.schoolDownload || '';
    const fallback = button.dataset.schoolFilename || 'dokument_szkolny.bin';
    button.disabled = true;
    const previous = button.textContent;
    button.textContent = 'Pobieranie…';
    try {
      const response = await fetch('/api/school-files/' + encodeURIComponent(id) + '/download', {
        credentials: 'same-origin', cache: 'no-store'
      });
      if (!response.ok) {
        let message = 'Nie udało się pobrać dokumentu (HTTP ' + response.status + ').';
        try {
          const data = await response.json();
          if (data && typeof data.detail === 'string') message = data.detail;
        } catch (_) {}
        throw new Error(message);
      }
      const blob = await response.blob();
      if (blob.size > 20 * 1024 * 1024) throw new Error('Dokument przekracza limit 20 MiB.');
      const name = filename(response, fallback);
      const url = URL.createObjectURL(blob);
      try {
        const link = document.createElement('a');
        link.href = url;
        link.download = name;
        link.style.display = 'none';
        document.body.appendChild(link);
        link.click();
        link.remove();
      } finally {
        setTimeout(() => URL.revokeObjectURL(url), 30000);
      }
      notice('Pobieranie dokumentu: ' + name);
    } catch (error) {
      notice(error && error.message || 'Nie udało się pobrać dokumentu.');
    } finally {
      button.disabled = false;
      button.textContent = previous;
    }
  }
  document.addEventListener('click', event => {
    const button = event.target.closest('button[data-school-download]');
    if (button) { event.preventDefault(); download(button); }
  });
  window.SynergiaSchoolFiles = { view };
})();
