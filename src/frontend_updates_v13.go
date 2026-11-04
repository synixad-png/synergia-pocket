package main

import "strings"

func runtimeHTMLV13() string {
	page := strings.ReplaceAll(runtimeHTMLV12(), "v0.12", "v0.13")
	page = strings.Replace(page, "v0.13 — wiadomości i załączniki", "v0.13 — powiadomienia i kontrola API", 1)
	if start := strings.Index(page, `<div class="notice">`); start >= 0 {
		if end := strings.Index(page[start:], "</div>"); end >= 0 {
			page = page[:start] + `<div class="notice"><strong>Niezależny klient Synergii.</strong> Dwa oddzielne profile. Hasła i historia powiadomień pozostają tylko w pamięci programu. Obserwowanie zmian możesz włączyć osobno dla każdego profilu.</div>` + page[start+end+len("</div>"):]
		}
	}
	nav := `<button class="secondary" data-section="updates" id="updates-nav">Powiadomienia</button><button class="secondary" data-section="health">Kontrola API</button>`
	page = strings.Replace(page, `<button class="secondary" data-section="diagnostics">`, nav+`<button class="secondary" data-section="diagnostics">`, 1)
	page = strings.Replace(page, "</head>", "<style>"+updatesCSSV13+"</style></head>", 1)
	return strings.Replace(page, "</body>", "<script>"+updatesJSV13+"</script></body>", 1)
}

const updatesCSSV13 = `
.watch13-tools{display:flex;flex-wrap:wrap;gap:8px;margin:16px 0}.watch13-tools button{min-height:44px}
.watch13-note{line-height:1.7;color:#d0e0f1}.watch13-card{background:#102137;border:1px solid #3c5976;border-radius:12px;padding:15px;margin:10px 0;overflow-wrap:anywhere}
.watch13-card h4{margin:0 0 8px}.watch13-card p{margin:7px 0;line-height:1.6}.watch13-warning{color:#ffd68d}.watch13-table{overflow:auto}.watch13-table table{width:100%;border-collapse:collapse}.watch13-table td,.watch13-table th{padding:12px;text-align:left;border-bottom:1px solid #3c5976;vertical-align:top}.watch13-count{font-size:22px;color:#95c9ff}
@media(max-width:600px){.watch13-tools button{flex:1 1 100%;white-space:normal}.watch13-table table{min-width:520px}}
`
const updatesJSV13 = `
(()=>{'use strict';
 const host=window.SynergiaUpdatesHostV13;let generation=0,chosen=-1,mode='',health=[],checking=false,abort=null;
 const names={messages:'Wiadomości odebrane',grades:'Oceny',timetable:'Plan lekcji i zastępstwa',attendance:'Frekwencja',agenda:'Terminarz',homework:'Zadania domowe',notes:'Uwagi',notices:'Ogłoszenia',schoolfiles:'Dokumenty szkoły',outbox:'Wiadomości wysłane',recipients:'Odbiorcy wiadomości',freedays:'Dni wolne'};
 const labels={ok:'Poprawnie',empty:'Brak wpisów',failed:'Błąd',partial:'Częściowo',unverified:'Niesprawdzone'};
 const el=(tag,text,cls)=>{const n=document.createElement(tag);if(text!=null)n.textContent=text;if(cls)n.className=cls;return n;};
 const button=(text,fn)=>{const b=el('button',text,'secondary');b.type='button';b.addEventListener('click',fn);return b;};
 const active=seq=>seq===generation&&chosen===host.profile()&&Boolean(document.querySelector('[data-section="'+mode+'"].active'));
 const localTime=value=>value?new Date(value).toLocaleString('pl-PL'):'—';
 async function request(path,profile,post=false,query={}){const res=await fetch(path+'?'+new URLSearchParams({profile,...query}),{method:post?'POST':'GET',credentials:'same-origin',cache:'no-store',signal:abort?.signal});const data=await res.json().catch(()=>({error:'Niepoprawna odpowiedź programu.'}));if(!res.ok)throw Error(data.error||'Operacja nie powiodła się.');return data;}
 function root(){const n=el('div');n.id='watch13';const body=document.getElementById('view-body');body.classList.remove('blank');body.replaceChildren(n);return n;}
 function table(where,rows){const scroll=el('div',null,'watch13-table'),table=el('table'),head=el('tr');for(const text of ['Moduł','Stan','Wynik','Sprawdzenie'])head.append(el('th',text));const thead=el('thead');thead.append(head);table.append(thead);const body=el('tbody');for(const row of rows){const tr=el('tr');tr.append(el('td',names[row.module]||row.name||row.module),el('td',labels[row.state]||row.state),el('td',(row.detail||'')+(row.errorKind?' ['+row.errorKind+']':'')),el('td',localTime(row.checked)));body.append(tr);}table.append(body);scroll.append(table);where.append(scroll);}
 async function mutate(route,query={}){const seq=generation;try{await request('/api/updates/'+route,chosen,true,query);if(active(seq))await refresh();}catch(e){if(active(seq))host.status(e.message,'error');}}
 function render(data){const wrap=root();wrap.append(el('p','Profil '+(chosen+1)+' · '+(data.enabled?'Obserwowanie włączone':'Obserwowanie wyłączone')+(data.running?' · Trwa sprawdzanie…':''),'watch13-count'));
  wrap.append(el('p','Sprawdzanie co 5 minut, gdy ten program jest uruchomiony. Pierwszy poprawny odczyt każdej sekcji ustala stan początkowy. Błąd sekcji zachowuje jej poprzedni stan.','watch13-note'));
  wrap.append(el('p','To powiadomienia wewnątrz programu Windows. Po zamknięciu programu historia znika. Nie są to powiadomienia push na iPhone. Wiadomości obejmują 30 ostatnich odebranych wpisów; przerwy w pracy mogą powodować pominięcie starszych zmian.','watch13-note'));
  const tools=el('div',null,'watch13-tools');tools.append(button(data.enabled?'Wyłącz obserwowanie':'Włącz obserwowanie',()=>mutate('enable',{enabled:String(!data.enabled)})));
  if(data.enabled){const check=button('Sprawdź teraz',()=>mutate('check'));check.disabled=data.running;tools.append(check);}
  if(data.unread)tools.append(button('Oznacz powiadomienia jako obejrzane',()=>mutate('ack',{through:String(data.through)})));wrap.append(tools);
  wrap.append(el('p','Nieobejrzane: '+data.unread+(data.next?' · Następne sprawdzenie: '+localTime(data.next):''),'watch13-note'));
  if(!data.events?.length)wrap.append(el('p','Brak wykrytych nowych lub zmienionych wpisów. Szczegóły odczytu są poniżej.'));
  for(const event of [...(data.events||[])].reverse()){const card=el('article',null,'watch13-card');card.append(el('h4',event.name),el('p','Nowe wpisy: '+event.added+' · Zmienione wpisy: '+event.changed+(event.module==='timetable'?' (liczba dni planu)':'')),el('p',localTime(event.at)),button('Otwórz: '+event.name,()=>host.openSection(event.module)));wrap.append(card);}
  wrap.append(el('h4','Stan poszczególnych sekcji'));table(wrap,data.statuses||[]);
 }
 async function refresh(){if(mode!=='updates')return;const seq=generation;try{const data=await request('/api/updates/state',chosen);if(active(seq)){render(data);badge(data.unread);}}catch(e){if(active(seq))host.status(e.message,'error');}}
 function badge(count){const nav=document.getElementById('updates-nav');nav.textContent='Powiadomienia'+(count?' ('+count+')':'');}
 function exportReport(){const lines=['SYNERGIA API v0.13 — kontrola odczytu','Profil '+(chosen+1),'Raport bez treści szkolnych i danych logowania.','Odczyt nie potwierdza wysyłania ani pobierania plików.',''];for(const row of health)lines.push([names[row.module],labels[row.state],row.errorKind||'',row.count,row.detail,row.checked].join(' | '));const blob=new Blob([lines.join('\n')],{type:'text/plain;charset=utf-8'}),url=URL.createObjectURL(blob),a=el('a');a.href=url;a.download='SYNERGIA_API_KONTROLA_v0_13.txt';a.click();setTimeout(()=>URL.revokeObjectURL(url),2000);}
 function renderHealth(){const wrap=root();wrap.append(el('p','Odrębne sprawdzanie 12 sekcji odczytu, w tym nowych modułów i formatu odpowiedzi API. Kontrola nie wysyła wiadomości, nie otwiera ich pełnej treści i nie pobiera plików. Starsza Diagnostyka pozostaje dostępna.','watch13-note'));
  const bar=el('div',null,'watch13-tools');const all=button(checking?'Sprawdzanie…':'Sprawdź wszystkie',()=>runHealth(Object.keys(names)));all.disabled=checking;bar.append(all);if(health.length)bar.append(button('Pobierz raport TXT',exportReport));wrap.append(bar);
  const choices=el('div',null,'watch13-tools');for(const [id,name] of Object.entries(names)){const b=button(name,()=>runHealth([id]));b.disabled=checking;choices.append(b);}wrap.append(choices);table(wrap,health);
  wrap.append(el('p','Wysyłanie, odpowiedzi i rzeczywiste pobieranie załączników wymagają osobnego testu użytkownika. Dane konta i kontakty można sprawdzić w starszej Diagnostyce.','watch13-note'));
 }
 async function runHealth(ids){if(checking)return;checking=true;const seq=generation;renderHealth();try{for(const id of ids){if(!active(seq))break;let row;try{row=await request('/api/health/check',chosen,false,{module:id});}catch(e){row={module:id,state:'failed',detail:e.message,checked:new Date().toISOString()};}if(!active(seq))break;health=health.filter(r=>r.module!==id);health.push(row);renderHealth();}}finally{if(active(seq)){checking=false;renderHealth();}}}
 function leave(){generation++;abort?.abort();abort=null;checking=false;health=[];chosen=-1;badge(0);if(document.querySelector('[data-section="updates"].active,[data-section="health"].active'))document.getElementById('view-body').textContent='Wybierz zakładkę dla bieżącego profilu.';mode='';}
 async function open(section){leave();chosen=host.profile();mode=section;abort=new AbortController();host.header(section==='updates'?'Powiadomienia':'Kontrola API','Profil '+(chosen+1)+' — niezależna kontrola danych.');const wrap=root();if(!host.logged()){wrap.append(el('p','Najpierw zaloguj wybrany profil.'));return;}if(section==='updates')await refresh();else renderHealth();}
 setInterval(async()=>{if(!host.logged())return;if(mode==='updates'){await refresh();return;}const profile=host.profile();try{const data=await request('/api/updates/state',profile);if(profile===host.profile())badge(data.unread);}catch(_){}},10000);
 window.SynergiaUpdatesV13={open,leave};
 fetch('/api/updates/profiles',{credentials:'same-origin',cache:'no-store'}).then(r=>r.ok?r.json():null).then(data=>{if(Array.isArray(data?.logged)&&data.logged.length===2)host.restore(data.logged);}).catch(()=>{});
})();
`
