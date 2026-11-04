/* Publiczny jadłospis SP3: daty i dania w aplikacji, bez przekierowań. */
(() => {
  'use strict';
  const esc = v => String(v == null ? '' : v).replace(/[&<>"']/g, c => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
  }[c]));
  const zone = 'Europe/Warsaw';
  const today = () => new Intl.DateTimeFormat('sv-SE', {timeZone:zone}).format(new Date());
  const day = s => new Date(s + 'T12:00:00Z');
  const iso = d => d.toISOString().slice(0,10);
  const add = (s,n) => {const d=day(s); d.setUTCDate(d.getUTCDate()+n); return iso(d);};
  const label = (s,opts) => new Intl.DateTimeFormat('pl-PL', {...opts,timeZone:'UTC'}).format(day(s));
  const monday = s => add(s,-((day(s).getUTCDay()+6)%7));
  const valid = s => /^\d{4}-\d{2}-\d{2}$/.test(s) && !Number.isNaN(day(s).getTime()) && iso(day(s))===s;
  const stamp = value => {
    const d=new Date(value);
    return value && !Number.isNaN(d.getTime()) ? new Intl.DateTimeFormat('pl-PL', {
      day:'numeric',month:'long',year:'numeric',hour:'2-digit',minute:'2-digit',timeZone:zone
    }).format(d) : 'nieznane';
  };
  let selected=today(), mode='week', data={}, byDate=new Map();

  function renderCalendar() {
    const now=today();
    const monthStart=selected.slice(0,8)+'01';
    const start=mode==='month'?monday(monthStart):monday(selected);
    const count=mode==='month'?42:7;
    const range=mode==='month'?label(selected,{month:'long',year:'numeric'}):
      label(start,{day:'numeric',month:'short'})+' – '+label(add(start,6),{day:'numeric',month:'short',year:'numeric'});
    const buttons=Array.from({length:count},(_,i)=>{
      const date=add(start,i), entry=byDate.get(date), has=entry?.status==='published' && entry.items?.length;
      const cls=['canteen-date',date===selected?'selected':'',date===now?'is-today':'',
        has?'has-menu':'',date.slice(0,7)!==selected.slice(0,7)?'other-month':''].filter(Boolean).join(' ');
      return '<button type="button" class="'+cls+'" data-canteen-day="'+date+'" aria-pressed="'+(date===selected)+
        '"'+(date===now?' aria-current="date"':'')+' aria-label="'+esc(label(date,{weekday:'long',day:'numeric',month:'long',year:'numeric'})+
        (date===now?', dzisiaj':'')+(has?', jadłospis dostępny':entry?.status==='no_meals'?', dzień bez obiadów':', brak jadłospisu'))+'">'+
        '<span>'+day(date).getUTCDate()+'</span><i aria-hidden="true">'+(has?'●':entry?.status==='no_meals'?'–':'')+'</i></button>';
    }).join('');
    const entry=byDate.get(selected);
    const title=selected===now?'Dzisiaj na obiad':selected===add(now,1)?'Jutro na obiad':'Obiad w wybranym dniu';
    const items=entry?.status==='published' && Array.isArray(entry.items)?entry.items:[];
    let body;
    if(items.length) {
      body='<ul class="canteen-dishes">'+items.map((name,i)=>'<li><span class="canteen-dish-number" aria-hidden="true">'+(i+1)+
        '</span><span>'+esc(name)+'</span></li>').join('')+'</ul><p class="canteen-hint">Jadłospis standardowy opublikowany przez szkołę.</p>';
    } else if(entry?.status==='no_meals') {
      body='<div class="canteen-empty"><span aria-hidden="true">☀</span><strong>Dzień bez obiadów</strong><p>Tak oznaczono ten dzień w jadłospisie szkoły.</p></div>';
    } else {
      body='<div class="canteen-empty"><span aria-hidden="true">▦</span><strong>Brak opublikowanego jadłospisu</strong>'+
        '<p>Na '+esc(label(selected,{day:'numeric',month:'long'}))+' nie mamy potwierdzonej listy dań. Po publikacji pojawi się tutaj automatycznie.</p></div>';
    }
    const archive=selected<now && items.length?'<span class="pill">Jadłospis archiwalny</span>':'';
    let warning='';
    const status=data.calendar?.status;
    if(status==='saved'||status==='stale') {
      warning='<div class="canteen-warning" role="status">Pokazujemy ostatnio odczytany jadłospis. Nie udało się teraz potwierdzić zmian w źródle. Odczyt: '+esc(stamp(data.calendar.checked_at))+'.</div>';
    } else if(status==='partial') {
      warning='<div class="canteen-warning" role="status">Część jadłospisów jest chwilowo niedostępna. Pokazujemy odczytane dni.</div>';
    } else if(data.status!=='ok'||status==='unavailable') {
      warning='<div class="canteen-warning" role="status">Nie udało się teraz odczytać jadłospisu. Spróbuj ponownie przyciskiem odświeżania aplikacji.</div>';
    }
    return warning+'<div class="canteen-calendar-layout"><section class="canteen-picker" aria-label="Kalendarz obiadów">'+
      '<div class="canteen-switch" role="group" aria-label="Widok kalendarza">'+
      ['week','month'].map(m=>'<button type="button" data-canteen-mode="'+m+'" aria-pressed="'+(mode===m)+'">'+(m==='week'?'Tydzień':'Miesiąc')+'</button>').join('')+'</div>'+
      '<div class="canteen-month-nav"><button type="button" data-canteen-shift="-1" aria-label="Poprzedni '+(mode==='week'?'tydzień':'miesiąc')+'">‹</button>'+
      '<strong>'+esc(range)+'</strong><button type="button" data-canteen-shift="1" aria-label="Następny '+(mode==='week'?'tydzień':'miesiąc')+'">›</button></div>'+
      '<div class="canteen-weekdays" aria-hidden="true">'+['Pn','Wt','Śr','Cz','Pt','So','Nd'].map(s=>'<span>'+s+'</span>').join('')+'</div>'+
      '<div class="canteen-dates">'+buttons+'</div><p class="canteen-legend"><span aria-hidden="true">●</span> Dostępny jadłospis</p>'+
      '<div class="canteen-shortcuts"><button type="button" class="button button-outline button-sm" data-canteen-go="today">Dzisiaj</button>'+
      '<button type="button" class="button button-outline button-sm" data-canteen-go="tomorrow">Jutro</button></div></section>'+
      '<section class="canteen-daily" aria-live="polite" aria-atomic="true"><div class="canteen-daily-header"><span class="eyebrow">'+esc(title)+'</span>'+
      '<h2>'+esc(label(selected,{weekday:'long'}))+'</h2><p>'+esc(label(selected,{day:'numeric',month:'long',year:'numeric'}))+'</p>'+archive+'</div>'+body+'</section></div>';
  }

  function renderPayments(payment) {
    const announcements=Array.isArray(payment.announcements)?payment.announcements:[];
    const current=announcements.filter(n=>n.period_status==='current');
    const upcoming=announcements.filter(n=>n.period_status==='upcoming');
    const archived=announcements.filter(n=>n.period_status==='archive');
    const dateText=value=>valid(value)?label(value,{day:'numeric',month:'long',year:'numeric'}):'Nie podano';
    const card=notice=>{
      const archive=notice.period_status==='archive';
      const rows=[];
      if(notice.amount) rows.push(['Kwota z ogłoszenia',notice.amount]);
      if(notice.meal_count && notice.unit_price) rows.push(['Liczba i cena obiadów',notice.meal_count+' × '+notice.unit_price]);
      else if(notice.unit_price) rows.push(['Cena jednego obiadu',notice.unit_price]);
      if(notice.deadline) rows.push(['Termin wpłaty',notice.window_start?dateText(notice.window_start)+' – '+dateText(notice.deadline):dateText(notice.deadline)]);
      if(notice.bank_account) rows.push(['Konto szkoły',notice.bank_account]);
      if(notice.transfer_title) rows.push(['Tytuł przelewu',notice.transfer_title]);
      return '<article class="canteen-payment-card'+(archive?' is-archive':'')+'">'+
        '<div class="canteen-payment-heading"><h3>'+esc(notice.period_label)+'</h3><span class="pill">'+
        (archive?'Archiwum':notice.period_status==='upcoming'?'Kolejny miesiąc':'Bieżący miesiąc')+'</span></div>'+
        '<p class="canteen-payment-scope">'+esc(notice.scope)+'</p>'+
        (!notice.complete?'<p class="canteen-warning">Dane niepełne — nie udało się potwierdzić kwoty lub terminu.</p>':'')+
        '<dl class="canteen-payment-fields">'+rows.map(([k,v])=>'<div><dt>'+esc(k)+'</dt><dd>'+esc(v)+'</dd></div>').join('')+'</dl>'+
        (!archive && notice.deadline_passed?'<p class="canteen-hint">Termin podany w ogłoszeniu już minął.</p>':'')+
        (notice.deductions?'<p class="canteen-hint">'+esc(notice.deductions)+'</p>':'')+
        '<p class="canteen-payment-source">'+esc(notice.title)+' · '+
        (notice.source_status==='live'?'Odczyt':'Zapis z ostatniego odczytu')+': '+esc(stamp(notice.checked_at))+'</p></article>';
    };
    const statusWarning=payment.status && payment.status!=='ok'?
      '<p class="canteen-warning" role="status">'+(payment.status==='partial'?
        'Nie udało się sprawdzić części komunikatów. Pokazujemy dostępne informacje.':
        'Nie udało się teraz sprawdzić aktualności szkoły. Zachowane komunikaty mają datę ostatniego odczytu.')+'</p>':'';
    return '<section class="panel canteen-panel canteen-payments"><h2 class="section-title">Płatności za obiady</h2>'+statusWarning+
      (payment.conflict?'<p class="canteen-warning">Znaleziono różne komunikaty dla tego samego miesiąca i zakresu klas. Poniżej pokazujemy je osobno.</p>':'')+
      (current.length?current.map(card).join(''):
        '<p class="canteen-important">Aktualna kwota do zapłaty i termin nie zostały potwierdzone.</p><p>'+esc(payment.message||'Brak aktualnego komunikatu o płatnościach.')+'</p>')+
      upcoming.map(card).join('')+
      (archived.length?'<details class="canteen-payment-archive"><summary>Archiwum opłat ('+archived.length+')</summary>'+archived.map(card).join('')+'</details>':'')+
      '<p class="canteen-hint">Kwoty z ogłoszeń szkoły nie uwzględniają indywidualnych odpisów. Aplikacja nie ma dostępu do salda ani potwierdzenia wpłaty.</p></section>';
  }

  async function view(requestFn) {
    data=await requestFn('/api/canteen/sp3');
    byDate=new Map((Array.isArray(data.calendar?.days)?data.calendar.days:[])
      .filter(d=>d && valid(d.date)).map(d=>[d.date,d]));
    const payment=data.payment||{};
    const notes=Array.isArray(data.notices)?data.notices:[];
    return '<div class="content-head"><div><span class="eyebrow">SP3 WROCŁAW</span><h1>Stołówka</h1>'+
      '<p>Wybierz dzień i zobacz, co będzie na obiad.</p></div></div>'+
      '<section class="panel canteen-panel canteen-calendar-panel"><div id="canteen-calendar">'+renderCalendar()+'</div></section>'+
      renderPayments(payment)+
      '<section class="panel canteen-panel"><h2 class="section-title">Informacje organizacyjne</h2>'+
      (notes.length?notes.map(n=>'<p>'+esc(n)+'</p>').join(''):'<p>Brak nowych informacji organizacyjnych.</p>')+'</section>'+
      '<p class="canteen-footnote">Źródło: jadłospis podlinkowany przez SP3 Wrocław · Sprawdzenie: '+esc(stamp(data.calendar?.checked_at||data.checked_at))+'.</p>';
  }

  document.addEventListener('click', event=>{
    const root=document.getElementById('canteen-calendar');
    if(!root || !root.contains(event.target)) return;
    const button=event.target.closest('button');
    if(!button) return;
    if(button.dataset.canteenDay && valid(button.dataset.canteenDay)) selected=button.dataset.canteenDay;
    else if(['week','month'].includes(button.dataset.canteenMode)) mode=button.dataset.canteenMode;
    else if(button.dataset.canteenGo) selected=add(today(),button.dataset.canteenGo==='tomorrow'?1:0);
    else if(button.dataset.canteenShift) {
      const n=Number(button.dataset.canteenShift);
      if(mode==='week') selected=add(selected,n*7);
      else {const d=day(selected.slice(0,8)+'01');d.setUTCMonth(d.getUTCMonth()+n);selected=iso(d);}
    } else return;
    const focusKey=button.dataset.canteenDay?'day':button.dataset.canteenMode?'mode':button.dataset.canteenGo?'go':'shift';
    const focusValue=button.getAttribute('data-canteen-'+focusKey);
    root.innerHTML=renderCalendar();
    const next=Array.from(root.querySelectorAll('button')).find(b=>b.getAttribute('data-canteen-'+focusKey)===focusValue);
    next?.focus({preventScroll:true});
  });
  window.SynergiaCanteenSP3={view};
})();

