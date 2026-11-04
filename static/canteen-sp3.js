/* SP3 Stołówka — publiczny, niezależny moduł (bez danych i haseł Librusa). */
(() => {
  'use strict';
  const esc = v => String(v == null ? '' : v).replace(/[&<>"']/g, c => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
  }[c]));

  const knownHost = value => {
    try {
      const u = new URL(String(value));
      return u.protocol === 'https:' && u.hostname.toLowerCase() === 'sp3wroc.edupage.org';
    } catch (_) { return false; }
  };
  const external = (value, label, cls = 'button button-outline button-sm') =>
    knownHost(value) ? '<a class="' + cls + '" href="' + esc(value) +
      '" target="_blank" rel="noopener noreferrer">' + esc(label) + '</a>' : '';

  const formatDate = value => {
    if (!value) return '';
    const d = new Date(value);
    return Number.isNaN(d.getTime()) ? '' :
      new Intl.DateTimeFormat('pl-PL', {day:'numeric',month:'short',year:'numeric',
        hour:'2-digit',minute:'2-digit',timeZone:'Europe/Warsaw'}).format(d);
  };

  async function view(requestFn) {
    const data = await requestFn('/api/canteen/sp3');
    const schoolUrl = knownHost(data.source_url) ?
      data.source_url : 'https://sp3wroc.edupage.org/a/stolowka';
    const newsUrl = knownHost(data.news_url) ?
      data.news_url : 'https://sp3wroc.edupage.org/news/';
    const notes = Array.isArray(data.notices) ? data.notices : [];
    const menu = data.menu && typeof data.menu === 'object' ? data.menu : null;
    const payment = data.payment && typeof data.payment === 'object' ? data.payment : {};
    const sections = [];

    sections.push('<div class="content-head"><div>' +
      '<span class="eyebrow">SP3 WROCŁAW · ŹRÓDŁO SZKOLNE</span>' +
      '<h1>Stołówka</h1>' +
      '<p>Jadłospis i informacje o opłatach ze strony Szkoły Podstawowej nr 3.</p>' +
      '</div></div>');

    if (data.status !== 'ok') {
      sections.push('<div class="canteen-warning" role="status">Nie udało się teraz odczytać strony szkoły. ' +
        'Poniższe odnośniki prowadzą bezpośrednio do oficjalnego źródła — nie pokazujemy niezweryfikowanego jadłospisu ani opłat.</div>');
    }

    let menuBody = '';
    if (menu) {
      const current = Boolean(menu.is_current);
      const period = menu.period ? esc(menu.period) : 'Termin niepodany';
      menuBody += '<div class="canteen-menu-top">' +
        '<span class="pill">' + (current ? 'Bieżący termin' : 'Archiwalny termin') + '</span>' +
        '<strong>' + period + '</strong></div>';
      if (menu.image_id && /^[0-9a-f]{24}$/.test(menu.image_id)) {
        menuBody += '<div class="canteen-menu-picture"><img loading="lazy" ' +
          'src="/api/canteen/sp3/menu-image/' + menu.image_id +
          '" alt="Jadłospis SP3 z okresu ' + period +
          ' — oryginalny obraz ze strony szkoły" /></div>';
        menuBody += '<p class="canteen-hint">Jadłospis jest obrazem opublikowanym przez szkołę. ' +
          'W razie problemu z podglądem otwórz oficjalną stronę.</p>';
      } else {
        menuBody += '<p class="canteen-hint">Nie udało się odczytać obrazu jadłospisu. ' +
          'Może znajdować się bezpośrednio na stronie szkoły.</p>';
      }
      if (!current) {
        menuBody += '<p class="canteen-warning">Ta data już minęła. ' +
          'Nie traktuj tego jadłospisu jako aktualnego.</p>';
      }
    } else {
      menuBody = '<div class="empty">Brak możliwego do potwierdzenia jadłospisu na bieżący tydzień. ' +
        'Szkoła może publikować go jako obraz na swojej stronie.</div>';
    }
    menuBody += '<div class="canteen-actions">' +
      external(schoolUrl,'Zobacz jadłospis na stronie SP3') + '</div>';
    sections.push('<section class="panel canteen-panel">' +
      '<h2 class="section-title">Jadłospis</h2>' + menuBody + '</section>');

    let paymentBody = '';
    if (payment.verified && payment.current_amount && payment.current_deadline) {
      paymentBody = '<div class="canteen-payment-amount"><strong>' +
        esc(payment.current_amount) + '</strong><span>Termin: ' +
        esc(payment.current_deadline) + '</span></div>';
    } else {
      paymentBody = '<p class="canteen-important">Aktualna kwota do zapłaty i termin nie zostały potwierdzone.</p>';
    }
    paymentBody += '<p>' + esc(payment.message ||
      'Informacje o kwotach i płatnościach należy zweryfikować w komunikacie szkoły.') +
      '</p>';
    paymentBody += '<p class="canteen-hint">Nie wyświetlamy statusu „opłacono”, ' +
      'ponieważ SP3 nie udostępnia tu indywidualnego salda ucznia.</p>';
    paymentBody += '<div class="canteen-actions">' +
      external(newsUrl,'Komunikaty o opłatach') +
      external(schoolUrl,'Zasady stołówki') + '</div>';
    sections.push('<section class="panel canteen-panel"><h2 class="section-title">Płatności za obiady</h2>' +
      paymentBody + '</section>');

    let notesBody = notes.length ?
      '<div class="canteen-notices">' + notes.map(n => '<p>• ' + esc(n) + '</p>').join('') + '</div>' :
      '<p class="canteen-hint">Aktualne zasady rejestracji i wydawania obiadów znajdziesz na stronie SP3.</p>';
    const regUrl = data.registration_url;
    notesBody += '<div class="canteen-actions">' +
      (regUrl ? external(regUrl, 'Karta zapisu na obiady') : '') +
      external(schoolUrl,'Dokumenty i informacje ze stołówki') + '</div>';
    sections.push('<section class="panel canteen-panel"><h2 class="section-title">Informacje organizacyjne</h2>' +
      notesBody + '</section>');
    sections.push('<p class="canteen-footnote">Źródło: oficjalna strona SP3 Wrocław. ' +
      'Sprawdzenie: ' + esc(formatDate(data.checked_at) || 'nieznane') +
      '. Informacje publiczne, wspólne dla obu profili w Synergia Pocket; nie są danymi z Librusa.</p>');
    return sections.join('');
  }
  window.SynergiaCanteenSP3 = {view};
})();
