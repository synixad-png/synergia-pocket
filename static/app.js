'use strict';

const $ = (id) => document.getElementById(id);
const labels = { dashboard:'Oś czasu', timetable:'Plan lekcji', grades:'Oceny', attendance:'Frekwencja', agenda:'Terminarz', homework:'Zadania domowe', messages:'Wiadomości', notes:'Uwagi', announcements:'Ogłoszenia', 'school-files':'Dokumenty szkoły', 'canteen-sp3':'Stołówka SP3' };
const today = new Date();
const state = { user:null, page:'dashboard', week: monday(today), mailbox:'inbox', loading:0, messageEpoch:0, messages:[], refresh:false, profileFormMode:'add', gradeDetails:[], selectedDayIndex:Math.min(4,Math.max(0,(today.getDay()+6)%7)) };
const plDate = new Intl.DateTimeFormat('pl-PL',{day:'numeric',month:'long',year:'numeric'});
const plShort = new Intl.DateTimeFormat('pl-PL',{day:'numeric',month:'short'});
const weekday = new Intl.DateTimeFormat('pl-PL',{weekday:'short'});
const month = new Intl.DateTimeFormat('pl-PL',{month:'short'});
function esc(v) { return String(v ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c])); }
function dateObj(value) { if(!value) return null; const d = new Date(String(value).slice(0,10)+'T12:00:00'); return Number.isNaN(d.getTime()) ? null : d; }
function dateLabel(value, short=false) { const d=dateObj(value); return d ? (short?plShort:plDate).format(d) : 'Brak daty'; }
function iso(d) { const x=new Date(d.getTime()-d.getTimezoneOffset()*60000); return x.toISOString().slice(0,10); }
function monday(d) { const x=new Date(d); x.setHours(12,0,0,0); x.setDate(x.getDate() - ((x.getDay()+6)%7)); return x; }
function addDays(d, n) { const x=new Date(d); x.setDate(x.getDate()+n); return x; }
function empty(message='Nie ma danych w tej sekcji.') { return `<div class="empty">${esc(message)}</div>`; }
function panel(title, body, action='') { return `<div class="panel"><div class="section-row"><h2 class="section-title">${esc(title)}</h2>${action}</div>${body}</div>`; }
function head(title, detail, actions='') { return `<div class="content-head"><div><span class="eyebrow">SYNERGIA POCKET</span><h1>${esc(title)}</h1><p>${esc(detail)}</p></div>${actions}</div>`; }
function pills(items) { return items.map(s=>`<span class="pill">${esc(s)}</span>`).join(' '); }
function cleanHour(v) { return v?String(v).slice(0,5):'—'; }
function errDetail(err) { return err && err.message || 'Wystąpił błąd.'; }
async function request(path, init={}) {
  const res=await fetch(path,{credentials:'same-origin', cache:'no-store',...init});
  let payload;
  try { payload=await res.json(); } catch { payload={detail:'Nieprawidłowa odpowiedź serwera.'}; }
  if(!res.ok) { if(res.status===401 && !['/api/login','/api/profiles/add'].some(p=>path.startsWith(p))) showAuth(); throw new Error(payload.detail || `Błąd ${res.status}`); }
  return payload;
}
async function section(name, params={}) {
  const url = new URL('/api/section/'+encodeURIComponent(name),window.location.origin);
  if(params.week) url.searchParams.set('week',params.week);
  if(params.mailbox) url.searchParams.set('mailbox',params.mailbox);
  if(state.refresh) url.searchParams.set('refresh','true');
  return request(url.pathname+url.search);
}
function statusToast(message) {
  const toast=$('toast'); toast.textContent=message; toast.classList.remove('hidden');
  clearTimeout(statusToast.timer); statusToast.timer=setTimeout(()=>toast.classList.add('hidden'),3600);
}
function closeProfileMenu() { $('profile-menu').classList.add('hidden'); $('profile-trigger').setAttribute('aria-expanded','false'); }
function closeProfileDialog() {
  $('profile-dialog').classList.add('hidden');
  $('profile-form').reset();
  $('profile-error').textContent='';
}
function closeProfileOverlay() { closeProfileMenu(); closeProfileDialog(); }
function renderProfileMenu() {
  const profiles=state.user?.profiles||[];
  const activeId=state.user?.active_profile_id;
  $('active-profile-name').textContent=state.user?.name||'Profil';
  $('profile-options').innerHTML=profiles.map(p=>`<div class="profile-option-row"><button type="button" class="profile-option ${p.id===activeId?'active':''}" data-profile-switch="${esc(p.id)}" aria-current="${p.id===activeId?'true':'false'}"><span class="profile-avatar">${esc((p.name||'U').slice(0,1).toUpperCase())}</span><span class="profile-option-label">${esc(p.name)}</span>${p.id===activeId?'<small>Aktywny ✓</small>':''}</button>${profiles.length>1?`<button type="button" class="profile-remove" data-profile-remove="${esc(p.id)}" title="Usuń sesję profilu">Usuń</button>`:''}</div>`).join('');
  $('add-profile-btn').disabled=profiles.length>=2||state.user?.mode==='demo';
  $('add-profile-btn').textContent=profiles.length>=2?'✓ Dwa profile dodane':state.user?.mode==='demo'?'Niedostępne w trybie demo':'＋ Dodaj drugie konto';
  $('rename-profile-btn').disabled=!activeId;
}
function showAuth() {
  ++state.loading; ++state.messageEpoch; state.messages=[]; state.gradeDetails=[]; state.user=null;
  closeProfileOverlay(); closeDialog();
  $('auth').classList.remove('hidden'); $('workspace').classList.add('hidden'); $('password').value='';
}
function showApp(user) {
  ++state.loading; ++state.messageEpoch; state.messages=[];
  closeDialog(); closeProfileOverlay(); $('password').value='';
  state.user=user; state.mailbox='inbox'; state.week=monday(new Date());
  $('auth').classList.add('hidden'); $('workspace').classList.remove('hidden');
  $('demo-banner').classList.toggle('hidden', user.mode!=='demo'); $('mode-footer').textContent=user.mode==='demo'?'DEMO':'SYNERGIA ONLINE';
  $('avatar').textContent=(user.name||'U').split(/\s+/).map(n=>n[0]).slice(0,2).join('').toUpperCase();
  $('today-label').textContent=plDate.format(today);
  renderProfileMenu();
  navigate('dashboard');
  document.dispatchEvent(new Event('synergia:authenticated'));
}
function openProfileForm(mode) {
  state.profileFormMode=mode;
  closeProfileMenu();
  $('profile-form').reset(); $('profile-error').textContent='';
  const renaming=mode==='rename';
  $('profile-dialog-title').textContent=renaming?'Zmień nazwę profilu':'Dodaj drugie konto Synergia';
  $('profile-dialog-info').textContent=renaming?'Nazwa służy tylko do wyświetlania profilu w aplikacji.':'Zaloguj drugie konto Synergia. Dane obu córek pozostają oddzielne. Hasła nie zapisujemy na dysku ani w przeglądarce.';
  $('profile-credentials').classList.toggle('hidden',renaming);
  $('profile-login').required=!renaming; $('profile-password').required=!renaming;
  $('profile-label').value=renaming?state.user?.name||'':'';
  $('profile-submit').textContent=renaming?'Zapisz nazwę':'Dodaj i przełącz profil';
  $('profile-dialog').classList.remove('hidden'); $('profile-label').focus();
}
async function switchProfile(id) {
  if(!state.user||!id||id===state.user.active_profile_id){closeProfileMenu();return;}
  closeProfileMenu(); ++state.loading; ++state.messageEpoch; closeDialog();
  $('content').innerHTML='<div class="loading">Przełączanie profilu…</div>';
  try {
    const user=await request('/api/profiles/switch',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({profile_id:id})});
    showApp(user); statusToast('Aktywny profil: '+user.name);
  } catch(e) {statusToast(errDetail(e)); if(state.user)navigate(state.page);}
}
async function removeProfile(id) {
  const profile=state.user?.profiles?.find(p=>p.id===id);
  if(!profile||!window.confirm('Usunąć profil „'+profile.name+'” z bieżącej sesji? Można dodać go ponownie po zalogowaniu.'))return;
  closeProfileMenu(); ++state.loading; ++state.messageEpoch; closeDialog();
  try {
    const user=await request('/api/profiles/remove',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({profile_id:id})});
    showApp(user); statusToast('Profil został usunięty z tej sesji.');
  } catch(e){statusToast(errDetail(e)); if(state.user)navigate(state.page);}
}
async function logout() { try {await request('/api/logout',{method:'POST'});} catch {} showAuth(); }
function closeSidebar() { $('sidebar').classList.remove('open'); $('sidebar-mask').classList.add('hidden'); }
async function navigate(page, opt={}) {
  if(!labels[page]) return;
  closeGradeDetails(); state.gradeDetails=[];
  state.page=page; $('breadcrumb-name').textContent=labels[page]; closeSidebar();
  document.querySelectorAll('[data-page]').forEach(x=>x.classList.toggle('active',x.dataset.page===page));
  $('content').innerHTML='<div class="loading">Pobieranie danych…</div>';
  const seq=++state.loading;
  try {
    let html, gradeData;
    if(page==='dashboard') html=await dashboard();
    else if(page==='grades') {gradeData=await section('grades');html=renderGrades(gradeData);}
    else if(page==='timetable') { const [plan,grades]=await Promise.all([section('timetable',{week:iso(state.week)}),section('grades').catch(()=>null)]); html=renderTimetable(plan); opt.timetableExtras={plan,grades}; }
    else if(page==='attendance') html=renderAttendance(await section('attendance'));
    else if(page==='agenda') html=renderAgenda(await section('agenda'));
    else if(page==='homework') html=renderHomework(await section('homework'));
    else if(page==='messages') html=renderMessages(await section('messages',{mailbox:state.mailbox}));
    else if(page==='notes') html=renderNotes(await section('notes'));
    else if(page==='announcements') html=renderAnnouncements(await section('announcements'));
    else if(page==='school-files') html=await window.SynergiaSchoolFiles.view(request);
    else if(page==='canteen-sp3') html=await window.SynergiaCanteenSP3.view(request);
    if(seq===state.loading) { if(page==='grades')state.gradeDetails=(gradeData.subjects||[]).flatMap(subject=>(subject.grades||[]).map(grade=>({...grade,subject:subject.name}))); $('content').innerHTML=html; document.querySelectorAll('.donut[data-pct]').forEach(el=>el.style.setProperty('--percentage',el.dataset.pct+'%')); if(page==='timetable') { window.SynergiaTimetable?.enhance(opt.timetableExtras?.plan,opt.timetableExtras?.grades); window.SynergiaLessonClock?.update(); } }
    return true;
  } catch(ex) {
    if(seq===state.loading) $('content').innerHTML=head(labels[page],'Nie udało się pobrać danych.')+`<div class="error-box">${esc(errDetail(ex))}</div><p><button class="button button-outline button-sm" data-action="retry">Spróbuj ponownie</button></p>`;
    return false;
  } finally { state.refresh=false; }
}
function stat(label,value,note,icon,cls='') { return `<div class="stat-card"><div class="stat-top"><span class="stat-label">${esc(label)}</span><span class="stat-icon ${cls}">${icon}</span></div><div class="stat-value">${esc(value)}</div><div class="stat-note">${esc(note)}</div></div>`; }
async function dashboard() {
  const [grades,timetable,attendance,agenda,messages,homework]=await Promise.all(['grades','timetable','attendance','agenda','messages','homework'].map((name)=>section(name, name==='timetable'?{week:iso(state.week)}:name==='messages'?{mailbox:'inbox'}:{}).catch(()=>null)));
  const gradeCount=grades?.count;
  const lessons=timetable?.days?.flatMap(x=>x.lessons?.map(y=>({...y,date:x.date}))||[])||[];
  const dayToday=iso(today);
  const nowLessons=lessons.filter(x=>x.date===dayToday&&!x.canceled);
  const upcoming=(agenda?.items||[]).filter(x=>x.date&&x.date.slice(0,10)>=dayToday).slice(0,4);
  const tasks=(homework?.items||[]).filter(x=>!x.due_date||x.due_date.slice(0,10)>=dayToday).slice(0,3);
  const newest=grades?.subjects?.flatMap(s=>(s.grades||[]).map(g=>({...g,subject:s.name})))||[];
  newest.sort((a,b)=>(b.date||'').localeCompare(a.date||''));
  const attended=attendance?.stats;
  const rate=attended&&attended.total?Math.round(100*(attended.present+attended.late)/attended.total):null;
  const cards = [stat('Wszystkie oceny',gradeCount??'—','Wszystkie pobrane oceny','✦'),stat('Lekcje dzisiaj',timetable?nowLessons.length:'—','Według aktualnego planu','▦','purple'),stat('Frekwencja',rate===null?'—':rate+'%','Wskaźnik orientacyjny','◴','green'),stat('Nieprzeczytane',messages?.unread??'—','Wiadomości od szkoły','✉','orange')].join('');
  const rows=newest.slice(0,4).map(x=>`<div class="mini-row"><span class="subject-mark">${esc(x.value)}</span><div class="mini-body"><strong>${esc(x.subject)}</strong><small>${esc(x.category||'Ocena')} · ${dateLabel(x.date,true)}</small></div><span class="mini-trailing">${esc(x.kind||'')}</span></div>`).join('');
  const upcomingRows=upcoming.map(x=>`<div class="mini-row"><span class="subject-mark teal">${esc((x.date||'').slice(8,10))}</span><div class="mini-body"><strong>${esc(x.title||'Wydarzenie')}</strong><small>${esc(x.subject||x.description||'')}</small></div><span class="mini-trailing">${dateLabel(x.date,true)}</span></div>`).join('');
  const taskRows=tasks.map(x=>`<div class="mini-row"><span class="subject-mark gold">✎</span><div class="mini-body"><strong>${esc(x.topic)}</strong><small>${esc(x.text)}</small></div><span class="mini-trailing">${dateLabel(x.due_date,true)}</span></div>`).join('');
  const failed=[grades,timetable,attendance,agenda,messages,homework].some(x=>x===null);
  // Mobilna oś czasu — własny układ inspirowany przejrzystą nawigacją aplikacji szkolnych.
  const feed = [
    ...newest.slice(0,4).map(x=>({date:x.date,type:'Ocena',title:x.subject,description:`${x.category||'Nowa ocena'} · ${x.value}`,icon:esc(x.value),page:'grades',kind:'grade'})),
    ...(messages?.items||[]).slice(0,2).map(x=>({date:x.date,type:'Wiadomość',title:x.subject||'Wiadomość ze szkoły',description:x.sender||'Nauczyciel',icon:'✉',page:'messages',kind:'message'})),
    ...tasks.slice(0,2).map(x=>({date:x.date||x.due_date,type:'Zadanie',title:x.topic||'Zadanie domowe',description:`Termin: ${dateLabel(x.due_date,true)}`,icon:'✎',page:'homework',kind:'task'})),
    ...upcoming.slice(0,2).map(x=>({date:x.date,type:'Terminarz',title:x.title||'Wydarzenie',description:x.subject||'Zobacz szczegóły',icon:'▣',page:'agenda',kind:'event'}))
  ].sort((a,b)=>String(b.date||'').localeCompare(String(a.date||''))).slice(0,7);
  const timeline = `<section class="mobile-timeline" aria-label="Oś czasu"><div class="mobile-timeline-heading"><h2>Oś czasu</h2><span>Najnowsze informacje ze szkoły</span></div>`+
    (feed.length?feed.map(x=>`<button type="button" class="timeline-item timeline-${x.kind}" data-page="${x.page}"><span class="timeline-icon">${x.icon}</span><span class="timeline-copy"><small>${esc(x.type)} · ${dateLabel(x.date,true)}</small><strong>${esc(x.title)}</strong><span>${esc(x.description)}</span></span><span class="timeline-arrow">›</span></button>`).join(''):empty('Nie ma jeszcze wpisów na osi czasu.'))+`</section>`;
  return `<div class="welcome"><div><span class="eyebrow eyebrow-light">TWÓJ SZKOLNY DZIEŃ</span><h2>Dzień dobry, ${esc(state.user?.name?.split(' ')[0]||'Uczniu')}! 👋</h2><p>Przejrzyj najważniejsze informacje. Wszystko w jednym spokojnym miejscu.</p></div><span class="welcome-chip">${esc(weekday.format(today))} · ${esc(plShort.format(today))}</span></div>`+
    (failed?'<div class="error-box dashboard-error">Niektóre sekcje są chwilowo niedostępne. Pozostałe dane wyświetlamy normalnie.</div>':'')+
    `<div class="dashboard-grid">${cards}</div>${timeline}<div class="two-cols"><div>${panel('Ostatnie oceny',rows||empty('Brak ocen.'),'<button class="section-more" data-page="grades">Zobacz wszystkie →</button>')}${panel('Najbliższe wydarzenia',upcomingRows||empty('Brak nadchodzących wydarzeń.'),'<button class="section-more" data-page="agenda">Terminarz →</button>')}</div><div>${panel('Zadania do wykonania',taskRows||empty('Brak aktualnych zadań.'),'<button class="section-more" data-page="homework">Wszystkie →</button>')}${panel('Dzisiaj na planie',nowLessons.length?nowLessons.map(x=>`<div class="mini-row"><div class="mini-body"><strong>${esc(x.subject)}</strong><small>${esc(cleanHour(x.start))}–${esc(cleanHour(x.end))} · Sala ${esc(x.room||'—')}</small></div></div>`).join(''):empty('Dzisiaj nie ma lekcji.'),'<button class="section-more" data-page="timetable">Cały plan →</button>')}</div></div>`;
}
function gradeKindClass(value) { const n=parseInt(value,10); return n<=2?'low':n===3?'mid':n>=5?'good':''; }
function renderGrades(d) {
  let gradeIndex=0;
  const cards=(d.subjects||[]).map(s=>`<article class="subject-card"><h3>${esc(s.name)}</h3><div class="grade-chip-row">${(s.grades||[]).map(g=>{
    const value=g.source==='point'&&g.max_points!=null?String(g.points??g.value)+' / '+g.max_points+' pkt':String(g.value||'Opis');
    const long=value.length>5||g.source==='text';
    return `<button type="button" data-action="grade-detail" data-index="${gradeIndex++}" aria-haspopup="dialog" class="grade-chip ${long?'grade-chip-long':gradeKindClass(g.value)}" title="${esc(g.category)} · ${dateLabel(g.date)}"><span>${esc(value)}</span><small>${esc(g.source_label||'Klasyczne')} · ${esc(g.category||'Ocena')}${g.weight==null?'':' · waga '+esc(g.weight)}</small>${(g.comments||[]).length?`<small>${esc(g.comments.join(' · '))}</small>`:''}<small class="grade-open-hint">Pokaż szczegóły →</small></button>`;
  }).join('')}</div><div class="grade-foot">${esc(s.grades?.length||0)} ocen</div></article>`).join('');
  const sources=(d.sources||[]).map(s=>`${esc(s.label)}: ${s.state==='unavailable'?'niedostępne':s.state==='failed'||s.state==='unsupported'?'błąd':esc(s.count)}`).join(' · ');
  const warning=(d.warnings||[]).length?`<div class="error-box" role="status">Niepełny odczyt: ${(d.warnings||[]).map(esc).join(' ')}</div>`:'';
  return head('Oceny',`Wszystkie pobrane oceny: ${d.count??0}. Bez filtrowania semestru i bez wyliczania niepotwierdzonej średniej.`)+warning+(sources?`<p class="grades-sources">${sources}</p>`:'')+`<div class="subject-grid">${cards||empty('Brak pobranych ocen.')}</div>`;
}
let gradeReturnFocus=null, gradePreviousOverflow='';
function gradeDetailText(value) { return typeof value==='string'?value:JSON.stringify(value,null,2)??''; }
function openGradeDetails(index) {
  const grade=state.gradeDetails[index];
  if(!grade||!state.user||state.page!=='grades')return;
  const dialog=$('grade-dialog');
  gradeReturnFocus=document.activeElement; gradePreviousOverflow=document.body.style.overflow;
  const value=grade.source==='point'&&grade.max_points!=null?String(grade.points??grade.value)+' / '+grade.max_points+' pkt':String(grade.value||'Brak treści oceny.');
  const meta=[['Przedmiot',grade.subject],['Data',dateLabel(grade.date)],['Rodzaj',grade.kind],['Kategoria',grade.category],['Źródło',grade.source_label],['Semestr',grade.semester],['Waga',grade.weight]].filter(([,v])=>v!=null&&v!=='');
  const paragraphs=(title,values)=>Array.isArray(values)&&values.length?'<section><h3>'+esc(title)+'</h3>'+values.map(v=>'<p class="grade-detail-text">'+esc(gradeDetailText(v))+'</p>').join('')+'</section>':'';
  $('grade-detail-body').innerHTML='<h2 id="grade-detail-title">'+esc(grade.subject||'Szczegóły oceny')+'</h2><div class="grade-detail-value">'+esc(value)+'</div><dl class="grade-detail-meta">'+meta.map(([k,v])=>'<div><dt>'+esc(k)+'</dt><dd>'+esc(v)+'</dd></div>').join('')+'</dl>'+paragraphs('Komentarze',grade.comments)+paragraphs('Wymagania',grade.requirements);
  dialog.classList.remove('hidden');document.body.style.overflow='hidden';
  dialog.querySelector('.grade-dialog-box').scrollTop=0;$('grade-detail-back').focus();
}
function closeGradeDetails() {
  const dialog=$('grade-dialog');if(!dialog||dialog.classList.contains('hidden'))return;
  dialog.classList.add('hidden');$('grade-detail-body').replaceChildren();document.body.style.overflow=gradePreviousOverflow;
  if(gradeReturnFocus?.isConnected)gradeReturnFocus.focus();gradeReturnFocus=null;
}
function renderTimetable(d) {
  const end=addDays(state.week,4);
  const controls=`<div class="week-controls"><button class="week-arrow" data-action="week-prev" title="Poprzedni tydzień">‹</button><span class="week-title">${esc(plShort.format(state.week))} – ${esc(plShort.format(end))}</span><button class="week-arrow" data-action="week-next" title="Następny tydzień">›</button><button class="button button-outline button-sm" data-action="week-today">Dziś</button></div>`;
  const dayTabs=`<div class="mobile-daytabs" aria-label="Wybierz dzień tygodnia">${(d.days||[]).map((day,i)=>`<button type="button" class="day-tab ${state.selectedDayIndex===i?'active':''}" data-action="day-select" data-day-index="${i}" aria-pressed="${state.selectedDayIndex===i}"><small>${esc(weekday.format(dateObj(day.date)||today))}</small><strong>${esc((day.date||'').slice(8,10))}</strong></button>`).join('')}</div>`;
  const days=(d.days||[]).map((day,i)=>`<div class="day-col ${state.selectedDayIndex===i?'active':''}"><div class="day-header"><strong>${esc(weekday.format(dateObj(day.date)||today))}</strong><small>${dateLabel(day.date,true)}</small></div>${(day.lessons||[]).map(x=>`<div class="lesson-card ${x.canceled?'canceled':''} ${x.substitution?'substitution':''}" data-lesson-date="${esc(day.date||'')}" data-lesson-start="${esc(cleanHour(x.start))}" data-lesson-end="${esc(cleanHour(x.end))}" data-lesson-canceled="${x.canceled?'1':'0'}"><div class="lesson-meta"><span>${esc(x.num??'—')}. lekcja</span><span>${esc(cleanHour(x.start))}–${esc(cleanHour(x.end))}</span></div><div class="lesson-subject">${esc(x.subject)}</div><div class="lesson-detail">${esc(x.teacher||'Nauczyciel niepodany')} · sala ${esc(x.room||'—')}</div>${x.canceled?'<div class="lesson-note">Odwołana</div>':''}${x.substitution?`<div class="lesson-note">Zastępstwo${x.original_subject?' za '+esc(x.original_subject):''}</div>`:''}${x.note?`<div class="lesson-note">${esc(x.note)}</div>`:''}${!x.canceled?'<div class="lesson-live hidden"><div class="lesson-live-heading"><span class="lesson-live-status">● Trwa lekcja</span><span class="lesson-live-time"></span></div><div class="lesson-live-track" role="progressbar" aria-label="Postęp bieżącej lekcji" aria-valuemin="0" aria-valuemax="100" aria-valuenow="0"><div class="lesson-live-fill"></div><span class="lesson-live-marker"></span></div></div>':''}</div>`).join('')||empty('Bez lekcji')}</div>`).join('');
  return head('Plan lekcji','Bieżący tydzień, odwołane zajęcia i zastępstwa.',controls)+dayTabs+`<div class="panel timetable-wrap"><div class="week-grid">${days||empty('Brak planu lekcji.')}</div></div>`;
}
function renderAttendance(d) {
  const s=d.stats||{total:0,present:0,late:0,excused:0,absent:0};
  const percent=s.total?Math.round(100*(s.present+s.late)/s.total):0;
  const metrics=['Obecności','Spóźnienia','Usprawiedliwienia','Nieobecności'].map((name,i)=>`<div class="metric"><strong>${esc([s.present,s.late,s.excused,s.absent][i]??0)}</strong><span>${esc(name)}</span></div>`).join('');
  const entries=(d.records||[]).map(x=>`<div class="mini-row"><div class="mini-body"><strong><span class="status-dot ${esc(x.status)}"></span>${esc(x.label)}</strong><small>${dateLabel(x.date)} · lekcja ${esc(x.lesson??'—')}</small></div></div>`).join('');
  return head('Frekwencja','Zestawienie obecności i nieobecności z dziennika.')+`<div class="panel summary-split"><div class="donut" data-pct="${percent}"><div class="donut-inner"><strong>${percent}%</strong><span>obecność*</span></div></div><div class="attendance-metrics">${metrics}</div></div><p class="attendance-note">* Orientacyjnie: obecności i spóźnienia / wszystkie zarejestrowane lekcje; nie jest to oficjalny wskaźnik Librusa.</p>${panel('Historia wpisów',entries||empty('Brak wpisów frekwencji.'))}`;
}
function dateBox(v) { const d=dateObj(v); return `<div class="date-box"><strong>${d?d.getDate():'–'}</strong><small>${d?esc(month.format(d)):'brak'}</small></div>`; }
function renderAgenda(d) {
  const items=(d.items||[]).map(x=>`<div class="event-item">${dateBox(x.date)}<div class="event-main"><h3>${esc(x.title||'Wydarzenie')}</h3><p>${esc(x.description||'Brak opisu')}</p></div><div class="event-right">${esc(x.subject||'Szkoła')}</div></div>`).join('');
  return head('Terminarz','Sprawdziany, kartkówki, spotkania i szkolne wydarzenia.')+panel('Kalendarz wydarzeń',items||empty('Brak wydarzeń.'));
}
function renderHomework(d) {
  const cards=(d.items||[]).map(x=>`<article class="task-card"><span class="pill">${esc(dateLabel(x.due_date,true))}</span><h3>${esc(x.topic||'Zadanie domowe')}</h3><p>${esc(x.text||'Nie dodano opisu.')}</p><div class="task-card-footer"><span>✎ ${esc(x.teacher||'Szkoła')}</span><span>${x.attachments?`📎 ${esc(x.attachments)} zał.`:'Bez załączników'}</span></div></article>`).join('');
  return head('Zadania domowe','Lista prac do wykonania i ich terminów.')+`<div class="task-grid">${cards||empty('Brak zadań domowych.')}</div>`;
}
function renderMessages(d) {
  state.messages=d.items||[];
  const tabs=`<div class="tabs"><button data-action="inbox" class="tab ${state.mailbox==='inbox'?'active':''}">Odebrane</button><button data-action="outbox" class="tab ${state.mailbox==='outbox'?'active':''}">Wysłane</button></div>`;
  const items=state.messages.map((x,i)=>`<button class="message-row ${x.unread?'unread':''}" data-action="message" data-index="${i}"><span class="message-avatar">${esc((x.sender||x.recipient||'S').slice(0,1).toUpperCase())}</span><span class="message-row-main"><span class="message-row-top"><strong>${esc(x.sender||x.recipient||'Nadawca')}</strong><time>${dateLabel(x.date,true)}</time></span><h3>${esc(x.subject||'Bez tematu')}</h3><p>${esc(x.preview)}</p></span>${x.unread?'<span class="new-dot"></span>':''}</button>`).join('');
  return head('Wiadomości','Odebrane i wysłane wiadomości szkolne.',tabs)+'<div class="message-toolbar"><button type="button" class="button button-primary" data-action="compose-new">✎ Napisz wiadomość</button><span>Wybierz pracownika szkoły z listy kontaktów</span></div><div class="push-panel"><div><strong>🔔 Powiadomienia na telefonie</strong><p id="push-status" aria-live="polite">Sprawdzanie obsługi powiadomień…</p><small>Nowe, nieprzeczytane wiadomości z obu kont. Bez treści wiadomości na ekranie blokady.</small></div><div class="push-actions"><button type="button" class="button button-outline button-sm" id="push-enable">Włącz powiadomienia</button><button type="button" class="button button-outline button-sm" id="push-test" hidden>Wyślij test</button><button type="button" class="button button-outline button-sm" id="push-disable" hidden>Wyłącz</button></div></div>'+panel(state.mailbox==='inbox'?'Skrzynka odbiorcza':'Wysłane',items||empty('Brak wiadomości.'));
}
function renderNotes(d) {
  const items=(d.items||[]).map(x=>`<div class="event-item">${dateBox(x.date)}<div class="event-main"><h3>${esc(x.category||'Uwaga')} <span class="pill ${x.sentiment==='positive'?'good':x.sentiment==='negative'?'danger':''}">${x.sentiment==='positive'?'Pozytywna':x.sentiment==='negative'?'Negatywna':'Neutralna'}</span></h3><p>${esc(x.text)}</p></div><div class="event-right">${esc(x.teacher)}</div></div>`).join('');
  return head('Uwagi i pochwały','Informacje o zachowaniu wpisane przez nauczycieli.')+panel('Wpisy',items||empty('Brak uwag.'));
}
function renderAnnouncements(d) {
  const items=(d.items||[]).map(x=>`<div class="event-item">${dateBox(x.date)}<div class="event-main"><h3>${esc(x.title||'Ogłoszenie')} ${x.unread?'<span class="pill">Nowe</span>':''}</h3><p>${esc(x.text)}</p></div></div>`).join('');
  return head('Ogłoszenia','Komunikaty i informacje z tablicy szkolnej.')+panel('Tablica ogłoszeń',items||empty('Brak ogłoszeń.'));
}
async function openMessage(i) {
  const msg=state.messages[i]; if(!msg) return;
  const epoch=state.messageEpoch;
  $('message-dialog').classList.remove('hidden'); $('dialog-body').innerHTML='<div class="loading">Otwieranie wiadomości…</div>';
  try {
    const data=await request('/api/message/'+encodeURIComponent(msg.id)+'?mailbox='+encodeURIComponent(state.mailbox));
    if(epoch!==state.messageEpoch)return;
    $('dialog-body').innerHTML=`<h2 id="dialog-title">${esc(data.subject||'Bez tematu')}</h2><small class="message-modal-meta">Od: ${esc(data.sender||'—')} · ${dateLabel(data.date)}</small><div class="body-content"><p class="message-body-text">${esc(data.body||'Brak treści.')}</p></div>${data.attachments?.length&&!data.attachment_files?.length?`<small class="message-modal-attachments">Załączniki: ${data.attachments.map(esc).join(', ')} (brak danych identyfikujących pliki)</small>`:''}`;
    window.SynergiaMessageAttachments?.enhance($('dialog-body'), data, {profileId:state.user?.active_profile_id, messageId:data.id||msg.id, mailbox:state.mailbox});
    if (state.mailbox==='inbox' && !data.no_reply && state.user?.mode!=='demo') {
      const replyBtn=document.createElement('button');replyBtn.type='button';replyBtn.className='button button-primary';replyBtn.textContent='↩ Odpowiedz'; 
      const actions=document.createElement('div');actions.className='message-modal-actions';actions.append(replyBtn);$('dialog-body').append(actions);
      replyBtn.addEventListener('click',()=>{closeDialog();window.SynergiaMessaging.reply({id:state.user.active_profile_id,name:state.user.name},data,()=>navigate('messages'));});
    }
  } catch(e) { if(epoch===state.messageEpoch) $('dialog-body').innerHTML=`<div class="error-box">${esc(errDetail(e))}</div>`; }
}
function closeDialog() { closeGradeDetails(); window.SynergiaMessageAttachments?.clear(); $('message-dialog').classList.add('hidden'); }
function initEvents() {
  // Przełącznik wyglądu jest dostępny również przed logowaniem.
  document.addEventListener('click', (event) => {
    const themeOption = event.target.closest('button[data-theme-choice]');
    if (themeOption) {
      window.SynergiaTheme?.set(themeOption.dataset.themeChoice);
      document.querySelectorAll('.theme-menu').forEach(menu=>menu.classList.add('hidden'));
      document.querySelectorAll('[data-theme-trigger]').forEach(button=>button.setAttribute('aria-expanded','false'));
      return;
    }
    const trigger=event.target.closest('[data-theme-trigger]');
    if (trigger) {
      const picker=trigger.closest('.theme-picker');
      const menu=picker.querySelector('.theme-menu');
      const shouldOpen=menu.classList.contains('hidden');
      document.querySelectorAll('.theme-menu').forEach(node=>node.classList.add('hidden'));
      document.querySelectorAll('[data-theme-trigger]').forEach(button=>button.setAttribute('aria-expanded','false'));
      menu.classList.toggle('hidden',!shouldOpen);
      trigger.setAttribute('aria-expanded',String(shouldOpen));
      return;
    }
    if (!event.target.closest('.theme-picker')) {
      document.querySelectorAll('.theme-menu').forEach(menu=>menu.classList.add('hidden'));
      document.querySelectorAll('[data-theme-trigger]').forEach(button=>button.setAttribute('aria-expanded','false'));
    }
  });
  $('login-form').addEventListener('submit', async (e)=>{
    e.preventDefault(); $('login-error').textContent=''; const btn=$('login-btn'); btn.disabled=true; btn.textContent='Łączenie…';
    try { const user=await request('/api/login',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({login:$('login').value,password:$('password').value,label:$('login-label').value})}); showApp(user); }
    catch(ex) { $('login-error').textContent=errDetail(ex); }
    finally { btn.disabled=false; btn.innerHTML='Zaloguj się <span>→</span>'; }
  });
  $('demo-btn').addEventListener('click',async()=>{ $('demo-btn').disabled=true; try {showApp(await request('/api/demo',{method:'POST'}));} catch(e) { $('login-error').textContent=errDetail(e); } finally{$('demo-btn').disabled=false;} });
  $('profile-trigger').addEventListener('click',()=>{
    if(!state.user)return;
    const open=$('profile-menu').classList.contains('hidden');
    $('profile-menu').classList.toggle('hidden',!open);
    $('profile-trigger').setAttribute('aria-expanded',String(open));
  });
  $('add-profile-btn').addEventListener('click',()=>{if(state.user?.mode!=='demo'&&(state.user?.profiles||[]).length<2)openProfileForm('add');});
  $('rename-profile-btn').addEventListener('click',()=>{if(state.user)openProfileForm('rename');});
  $('profile-dialog-close').addEventListener('click',closeProfileDialog);
  $('profile-dialog').addEventListener('click',e=>{if(e.target===$('profile-dialog'))closeProfileDialog();});
  $('profile-form').addEventListener('submit', async e=>{
    e.preventDefault(); $('profile-error').textContent='';
    const btn=$('profile-submit');btn.disabled=true;
    const renaming=state.profileFormMode==='rename';
    btn.textContent=renaming?'Zapisywanie…':'Łączenie z drugim kontem…';
    try {
      const path=renaming?'/api/profiles/rename':'/api/profiles/add';
      const body=renaming?{profile_id:state.user.active_profile_id,label:$('profile-label').value}:
        {label:$('profile-label').value,login:$('profile-login').value,password:$('profile-password').value};
      const user=await request(path,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});
      showApp(user);statusToast(renaming?'Zmieniono nazwę profilu.':'Dodano drugie konto. Możesz przełączać profile.');
    }catch(ex){$('profile-error').textContent=errDetail(ex);}
    finally{btn.disabled=false;btn.textContent=renaming?'Zapisz nazwę':'Dodaj i przełącz profil';}
  });
  $('password-toggle').addEventListener('click',()=>{const p=$('password'); p.type=p.type==='password'?'text':'password';$('password-toggle').textContent=p.type==='password'?'Pokaż':'Ukryj';});
  $('logout').addEventListener('click',logout); $('leave-demo').addEventListener('click',logout);
  $('refresh').addEventListener('click',async()=>{state.refresh=true; if(await navigate(state.page)) statusToast('Dane zostały odświeżone.');});
  $('menu-btn').addEventListener('click',()=>{$('sidebar').classList.add('open');$('sidebar-mask').classList.remove('hidden');});
  $('sidebar-mask').addEventListener('click',closeSidebar);
  $('grade-detail-close').addEventListener('click',closeGradeDetails);
  $('grade-detail-back').addEventListener('click',closeGradeDetails);
  $('grade-detail-bottom').addEventListener('click',closeGradeDetails);
  $('grade-dialog').addEventListener('click',e=>{if(e.target===$('grade-dialog'))closeGradeDetails();});
  $('grade-dialog').addEventListener('keydown',e=>{if(e.key==='Tab'){const nodes=[$('grade-detail-back'),$('grade-detail-close'),$('grade-detail-bottom')];const first=nodes[0],last=nodes.at(-1);if(e.shiftKey&&document.activeElement===first){e.preventDefault();last.focus();}else if(!e.shiftKey&&document.activeElement===last){e.preventDefault();first.focus();}}});
  $('dialog-close').addEventListener('click',closeDialog);
  $('message-dialog').addEventListener('click',e=>{if(e.target===$('message-dialog'))closeDialog();});
  window.addEventListener('keydown',e=>{if(e.key==='Escape'){closeDialog();closeProfileOverlay();document.querySelectorAll('.theme-menu').forEach(menu=>menu.classList.add('hidden'));}});
  document.addEventListener('click',e=>{
    const profileButton=e.target.closest('button[data-profile-switch],button[data-profile-remove]');
    if(profileButton){if(profileButton.dataset.profileSwitch)switchProfile(profileButton.dataset.profileSwitch);else removeProfile(profileButton.dataset.profileRemove);return;}
    if(!e.target.closest('#profile-control'))closeProfileMenu();
    const btn=e.target.closest('button[data-page],button[data-action]'); if(!btn) return;
    if(btn.dataset.page) { navigate(btn.dataset.page); return; }
    const action=btn.dataset.action;
    if(action==='day-select'){state.selectedDayIndex=Math.min(4,Math.max(0,Number(btn.dataset.dayIndex)||0));document.querySelectorAll('.day-tab,.day-col').forEach(el=>{const active=Number(el.dataset.dayIndex??[...el.parentElement.children].indexOf(el))===state.selectedDayIndex;el.classList.toggle('active',active);if(el.matches('.day-tab'))el.setAttribute('aria-pressed',String(active));});}
    else if(action==='week-prev'||action==='week-next'){state.week=addDays(state.week,action==='week-prev'?-7:7);navigate('timetable');}
    else if(action==='week-today'){state.week=monday(new Date());state.selectedDayIndex=Math.min(4,Math.max(0,(new Date().getDay()+6)%7));navigate('timetable');}
    else if(action==='inbox'||action==='outbox'){state.mailbox=action;navigate('messages');}
    else if(action==='grade-detail')openGradeDetails(Number(btn.dataset.index));
    else if(action==='message')openMessage(Number(btn.dataset.index));
    else if(action==='compose-new') window.SynergiaMessaging.compose({id:state.user.active_profile_id,name:state.user.name},()=>navigate('messages'));
    else if(action==='retry')navigate(state.page);
  });
}
window.SynergiaPushApp = {
  getUser: () => state.user,
  async openMessageInProfile(profileId, messageId) {
    if(!state.user || state.user.mode!=='live')return;
    if((state.user.profiles||[]).some(p=>p.id===profileId) &&
        state.user.active_profile_id!==profileId) {
      const user=await request('/api/profiles/switch',{method:'POST',
        headers:{'Content-Type':'application/json'},
        body:JSON.stringify({profile_id:profileId})});
      showApp(user);
    }
    state.mailbox='inbox';
    state.refresh=true;
    const ok=await navigate('messages');
    if(ok){
      const index=state.messages.findIndex(x=>String(x.id)===String(messageId));
      if(index>=0)await openMessage(index);
    }
  }
};
async function init() { initEvents(); try { const user=await request('/api/session'); if(user.authenticated)showApp(user); else showAuth(); } catch(ex) {showAuth();$('login-error').textContent='Nie można teraz wznowić sesji. Odśwież aplikację, aby spróbować ponownie. '+errDetail(ex);} }
init();

