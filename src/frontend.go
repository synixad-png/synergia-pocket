package main

// HTML/JS is embedded inside the executable: no external CDN, browser extension,
// WebView runtime, internet-hosted assets or local files are required for the UI.
const htmlPage = `<!doctype html>
<html lang="pl">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="color-scheme" content="dark">
<title>SYNERGIA API • Niezależny klient</title>
<style>
:root{font:14px/1.5 system-ui,-apple-system,'Segoe UI',Arial,sans-serif;color:#e5edfb;background:#0b1525}
*{box-sizing:border-box}body{margin:0;min-height:100vh;background:radial-gradient(ellipse at 18% 0%,#193454 0,transparent 43%),#0b1525}
header{padding:23px max(22px,calc((100vw - 1120px)/2));border-bottom:1px solid #29415a;background:#0e1c2dcc}
.brand{display:flex;gap:17px;align-items:center;flex-wrap:wrap}.logo{width:52px;height:52px;display:grid;place-items:center;border-radius:15px;background:#438be3;color:white;font-weight:950;font-size:24px;box-shadow:0 8px 24px #2a72d044}.brand h1{margin:0;font-size:23px;letter-spacing:.03em}.subtitle{color:#b2c6dc;margin-top:2px;font-size:12px}.chip{margin-left:auto;border:1px solid #3d7f7b;background:#143d3c;color:#8be3d3;padding:7px 11px;border-radius:40px;font-size:11px;font-weight:800;white-space:nowrap}
main{max-width:1160px;margin:auto;padding:27px 20px 36px}.section-title{display:flex;align-items:center;justify-content:space-between;gap:15px;flex-wrap:wrap;margin-bottom:16px}.section-title h2{font-size:21px;margin:0}.small{color:#aebfd3;font-size:12px}
.panel{background:#12243a;border:1px solid #2a425c;border-radius:17px;box-shadow:0 14px 40px #02080e35;margin-bottom:18px;overflow:hidden}
.panel-inner{padding:22px}.profilebar{display:flex;gap:10px;align-items:center;flex-wrap:wrap;margin-bottom:16px}
.profile{cursor:pointer;border:1px solid #36536f;border-radius:10px;background:#172d45;color:#b8cee4;padding:9px 16px;font-weight:800;min-height:39px}.profile.active{background:#347bd0;color:white;border-color:#5797de}.profile .dot{display:inline-block;width:7px;height:7px;border-radius:50%;background:#7b8598;margin:0 7px 0 0}.profile.online .dot{background:#75dda8}
.formgrid{display:grid;grid-template-columns:1fr 1fr auto;gap:14px;align-items:end}.field label{display:block;color:#acc5db;font-size:12px;font-weight:700;margin-bottom:6px}.field input{outline:none;width:100%;padding:13px 14px;border:1px solid #3c5a7a;border-radius:10px;background:#0f2034;color:#f4f9ff;font-size:14px}.field input:focus{border-color:#74b3ff;box-shadow:0 0 0 2px #74b3ff23}
button{cursor:pointer;font:inherit}button:disabled{opacity:.5;cursor:wait}.primary{background:#3e90ec;border:1px solid #59a4ff;color:white;border-radius:10px;font-weight:800;padding:12px 19px;white-space:nowrap;min-height:46px}.primary:hover{background:#307ad1}.secondary{border:1px solid #3c5976;color:#d1e5f9;border-radius:10px;background:#1b3450;padding:10px 15px;font-weight:650}.secondary:hover{background:#254668}.nav{display:flex;flex-wrap:wrap;gap:4px;padding:16px 12px;border-bottom:1px solid #28415d;background:#112137}
.nav button{padding:9px 4px;font-size:11.5px;white-space:nowrap}
.nav button.active{color:white;border-color:#68aaff;background:#275a92}.nav button.demo{color:#f7d89d;border-color:#876838;background:#403019}.nav button.exit{margin-left:auto;color:#f7bbc0;border-color:#85464a;background:#3b272c}
.status{margin:15px 21px;padding:12px 14px;background:#1a3855;border:1px solid #315574;color:#cde4fc;border-radius:10px;min-height:44px;overflow-wrap:anywhere}.status.error{border-color:#ba6262;background:#452733;color:#ffd8da}.status.success{border-color:#3c866c;background:#1b463b;color:#beffe0}
.content{padding:9px 21px 23px}.content h3{font-size:18px;margin:0 0 8px}.intro{color:#afc3d8;margin:0 0 16px}.tablewrap{overflow:auto;border:1px solid #314966;border-radius:11px}.datatable{width:100%;border-collapse:collapse;text-align:left;table-layout:auto}.datatable th{font-size:11px;text-transform:uppercase;letter-spacing:.055em;color:#a8c6e2;background:#1b3856;padding:13px 14px;white-space:nowrap}.datatable td{padding:12px 14px;border-top:1px solid #2b4560;color:#e0ecf9;max-width:460px;overflow-wrap:anywhere}.datatable tbody tr:nth-child(even){background:#152b42}.datatable tbody tr:hover{background:#1c3d5b}.blank{border:1px dashed #49617c;text-align:center;color:#b9c9dc;padding:36px;border-radius:12px;background:#102035}
.footer{color:#93b0ca;font-size:12px;display:flex;justify-content:space-between;gap:13px;flex-wrap:wrap;margin-top:18px}.notice{background:#362e21;border:1px solid #715e3b;border-radius:11px;color:#f2d8a4;padding:11px 13px;font-size:12px;margin:0 0 18px}
.demoflag{display:inline-block;font-size:10px;padding:3px 9px;border-radius:7px;color:#1f1608;background:#f9c56f;font-weight:900;letter-spacing:.07em;margin-left:10px;vertical-align:middle}
.attachment-actions{display:flex;flex-direction:column;gap:7px;align-items:flex-start;min-width:154px}
.attachment-action{font-size:12px;padding:7px 10px;min-height:34px;border:1px solid #4582b7;border-radius:8px;background:#244b70;color:#eef6ff;font-weight:700}
.attachment-action:hover{background:#34699a}
.attachment-action:disabled{opacity:.65}
.attachment-list{display:flex;flex-direction:column;gap:6px;min-width:150px}
.attachment-list .small{overflow-wrap:anywhere;max-width:260px}
.datatable td.notice-text{white-space:pre-wrap;min-width:280px;max-width:580px}
/* v0.11: native rows, plain text only, no HTML injection */
/* v0.9 independent diagnostics panel */
.diag-intro{padding:15px 16px;border:1px solid #365572;border-radius:12px;background:#102b42;margin-bottom:16px;color:#cfe5f9}
.diag-actions{display:flex;gap:9px;flex-wrap:wrap;align-items:center;margin:12px 0 18px}
.diag-actions button{min-height:40px}
.diag-tbl{width:100%;border-collapse:collapse;min-width:570px}
.diag-tbl th,.diag-tbl td{padding:13px 12px;border-bottom:1px solid #28435e;text-align:left;vertical-align:middle}
.diag-tbl th{font-size:11px;letter-spacing:.045em;text-transform:uppercase;color:#add0ec;background:#18344e}
.diag-tbl td{font-size:13px;overflow-wrap:anywhere}
.diag-tbl tr:nth-child(even){background:#102940}
.diag-badge{display:inline-block;padding:5px 9px;border-radius:8px;background:#2e4057;color:#e0ebf9;font-size:11px;font-weight:800;white-space:nowrap}
.diag-badge.ok{background:#155740;color:#baffdf}.diag-badge.empty{background:#334c66;color:#dcefff}
.diag-badge.partial,.diag-badge.unverified{background:#58401f;color:#ffe0a5}
.diag-badge.blocked,.diag-badge.failed{background:#673139;color:#ffd5db}
.diag-detail{color:#b9cce0;font-size:12px}.diag-time{color:#8ca9c4;font-size:11px;white-space:nowrap}
.diag-summary{margin:13px 0;color:#bcd4e9;font-size:12px;min-height:18px}
.diag-info{padding:13px 15px;border:1px solid #665537;border-radius:11px;background:#2a291f;color:#f6dea9;font-size:12px;margin:14px 0}


/* v0.11: balanced two-row desktop navigation; responsive on smaller screens. */
@media(min-width:1180px){.nav{display:grid;grid-template-columns:repeat(8,minmax(0,1fr));gap:6px}.nav button{font-size:11.2px;padding:9px 6px}.nav button.exit{margin-left:0}}
@media(min-width:701px) and (max-width:1179px){.nav{display:grid;grid-template-columns:repeat(4,minmax(0,1fr))}.nav button.exit{margin-left:0}}
@media(max-width:700px){header{padding:18px 17px}.chip{margin-left:0}main{padding:18px 13px}.panel-inner{padding:16px}.formgrid{grid-template-columns:1fr}.nav{padding:13px;gap:7px}.nav button{flex:1}.nav button.exit{margin-left:0}.content{padding:8px 13px 18px}.status{margin:13px}.datatable td,.datatable th{padding:10px}}
</style>
</head>
<body>
<header><div class="brand"><span class="logo">S</span><div><h1>SYNERGIA API</h1><div class="subtitle">Niezależny klient • Windows x64 • v0.12 — wiadomości i załączniki</div></div><span class="chip">LOKALNIE NA TWOIM KOMPUTERZE</span></div></header>
<main>
  <div class="section-title"><h2>Panel klienta Librus</h2><span class="small">Osobne sesje dwóch kont • bez integracji z Synergia Pocket</span></div>
  <div class="notice"><strong>Wersja eksperymentalna.</strong> Samodzielny klient HTTP bez oficjalnego wsparcia Librusa. Logowanie, oceny, plan, Wiadomości i Kontakty działają w potwierdzonej wersji v0.5. Moduły v0.6: Frekwencja, Terminarz i Uwagi. Moduł v0.7: Zadania domowe (działanie potwierdzone przez użytkownika). v0.8: oddzielne pobieranie załączników na żądanie — wymaga sprawdzenia na rzeczywistym koncie. v0.9: ręczna diagnostyka każdego modułu osobno, tylko odczyt, bez automatycznych żądań i bez zmian na koncie. v0.10 — Dokumenty szkoły. v0.10.1: naprawiono nazwę i rozszerzenie pobieranego dokumentu, bez zmian w pozostałych modułach. Wersję programu widać w nagłówku. Wszystkie dane logowania pozostają w pamięci uruchomionego programu.</div>
  <section class="panel"><div class="panel-inner">
    <div class="profilebar" role="tablist" aria-label="Konta"><button type="button" class="profile active" data-profile="0" aria-selected="true"><span class="dot"></span>Profil 1</button><button type="button" class="profile" data-profile="1" aria-selected="false"><span class="dot"></span>Profil 2</button><span id="account-status" class="small">Brak zalogowanego konta</span></div>
    <form id="login-form" class="formgrid" autocomplete="off">
      <div class="field"><label for="login">Login Synergia</label><input id="login" autocomplete="off" spellcheck="false" placeholder="Login konta" maxlength="128" required></div>
      <div class="field"><label for="password">Hasło</label><input id="password" type="password" autocomplete="off" placeholder="Nie jest zapisywane" maxlength="300" required></div>
      <button id="login-button" type="submit" class="primary">Zaloguj</button>
    </form><p class="small">Dane wpisujesz tylko w tej lokalnej aplikacji. Klient łączy się bezpośrednio z domenami Librusa, bez serwera Render.</p>
  </div></section>
  <section class="panel">
    <nav class="nav" aria-label="Menu danych">
      <button class="secondary" data-section="messages">Wiadomości</button>
      <button class="secondary" data-section="grades">Oceny</button>
      <button class="secondary" data-section="timetable">Plan lekcji</button>
      <button class="secondary" data-section="contacts">Kontakty</button>
      <button class="secondary" data-section="attendance">Frekwencja</button>
      <button class="secondary" data-section="agenda">Terminarz</button>
      <button class="secondary" data-section="homework">Zadania domowe</button>
      <button class="secondary" data-section="notes">Uwagi</button>
      <button class="secondary" data-section="me">Dane konta</button>
      <button class="secondary" data-section="notices">Ogłoszenia</button>
      <button class="secondary" data-section="freedays">Dni wolne</button>
      <button class="secondary" data-section="schoolfiles">Dokumenty szkoły</button>
      <button class="secondary" data-section="diagnostics">Diagnostyka</button>
      <button class="secondary demo" data-section="demo">Zobacz DEMO</button>
      <button class="secondary exit" id="exit-button" type="button">Zamknij program</button>
    </nav>
    <div id="status" class="status" role="status" aria-live="polite">Uruchomiono program. Wybierz „Zobacz DEMO”, aby obejrzeć ekran bez logowania.</div>
    <div class="content">
      <h3 id="view-title">Witaj w SYNERGIA API</h3>
      <p class="intro" id="view-desc">Nowy klient został uruchomiony. To nie jest obecna Synergia Pocket — tamta aplikacja działa niezależnie.</p>
      <div id="view-body" class="blank">Kliknij „Zobacz DEMO” lub zaloguj profil powyżej.</div>
    </div>
  </section>
  <div class="footer"><span>Połączenie lokalne: tylko 127.0.0.1 • brak publicznego serwera</span><span>Brak automatycznej wysyłki wiadomości • brak przechowywania haseł</span></div>
</main>
<script>
(() => {
 'use strict';
 let profile=0, busy=false;
 const logged=[false,false];
 const accountNames=['',''];
 let diagnosticRun=0;
 const diagModules=[
  ['me','Logowanie / dane konta'],['messages','Wiadomości'],['contacts','Kontakty'],
  ['grades','Oceny'],['timetable','Plan lekcji'],['attendance','Frekwencja'],
  ['agenda','Terminarz'],['notes','Uwagi'],['homework','Zadania domowe'],
  ['attachments','Załączniki do zadań']
 ];

 const $=id=>document.getElementById(id);
 const titles={messages:'Wiadomości',grades:'Oceny',timetable:'Plan lekcji',contacts:'Kontakty',attendance:'Frekwencja',agenda:'Terminarz',homework:'Zadania domowe',notes:'Uwagi',notices:'Ogłoszenia szkolne',freedays:'Dni wolne',me:'Dane konta'};
 const columns={
  messages:[['date','Data'],['sender','Nadawca'],['subject','Temat'],['preview','Podgląd']],
  grades:[['date','Data'],['value','Ocena'],['subject','Przedmiot'],['category','Kategoria']],
  timetable:[['date','Dzień'],['time','Godziny'],['subjectLabel','Przedmiot / zastępstwo'],['teacher','Nauczyciel']],
  contacts:[['name','Pracownik'],['role','Grupa'],['id','ID konta']],
  attendance:[['date','Data'],['lesson','Lekcja'],['type','Rodzaj'],['subject','Przedmiot'],['present','Obecność?']],
  agenda:[['date','Data'],['time','Godzina'],['category','Kategoria'],['subject','Przedmiot'],['content','Opis']],
  homework:[['date','Dodano'],['dueDate','Termin'],['topic','Temat'],['content','Treść'],['teacher','Nauczyciel'],['category','Kategoria'],['attachmentNames','Załączniki (nazwy)'],['requirement','Oddanie pliku']],
  notes:[['date','Data'],['sentiment','Charakter'],['category','Kategoria'],['teacher','Nauczyciel'],['content','Treść']],
  notices:[['created','Dodano'],['start','Od'],['end','Do'],['read','W Librusie'],['subject','Temat'],['content','Pełna treść']],
  freedays:[['from','Od'],['to','Do'],['scope','Zakres'],['name','Nazwa'],['lessons','Lekcje']],
  me:[['key','Pole'],['value','Wartość']]
 };
 function status(text,kind=''){$('status').textContent=text;$('status').className='status '+kind;}
 function header(title,desc){$('view-title').textContent=title;$('view-desc').textContent=desc;}
 function setBusy(on){busy=on;document.querySelectorAll('button').forEach(b=>b.disabled=on);}
 async function jsonRequest(url,options={}) {
  const res=await fetch(url,{credentials:'same-origin',cache:'no-store',...options});
  const data=await res.json().catch(()=>({error:'Nieprawidłowa odpowiedź lokalnego serwera.'}));
  if(!res.ok)throw Error(data.error||'Błąd operacji');return data;
 }
 function drawTable(section,records,metadata={}){
  if(window.PocketGUI?.render(section,records,metadata))return;
  const specs=columns[section];const wrap=document.createElement('div');wrap.className='tablewrap';
  const table=document.createElement('table');table.className='datatable';
  const thead=document.createElement('thead'),trh=document.createElement('tr');
  for(const [key,name] of specs){const th=document.createElement('th');th.textContent=name;trh.append(th)}
  if(section==='homework'){const th=document.createElement('th');th.textContent='Pobierz pliki';trh.append(th)}
  thead.append(trh);table.append(thead);
  const tbody=document.createElement('tbody');
  for(const rec of records){const tr=document.createElement('tr');for(const [key] of specs){const td=document.createElement('td');td.textContent=String(rec[key]??'');if(section==='notices'&&key==='content')td.classList.add('notice-text');tr.append(td)}
    if(section==='homework'){
      const td=document.createElement('td');td.className='attachment-actions';
      if(/^\d+$/.test(String(rec.id||''))&&String(rec.attachmentNames||'')!=='—'){
        const btn=document.createElement('button');btn.type='button';btn.className='attachment-action';btn.textContent='Pokaż pliki';
        btn.addEventListener('click',()=>showHomeworkFiles(rec,td,btn));td.append(btn);
      }else{td.textContent='—'}
      tr.append(td);
    }
    tbody.append(tr)
  }
  table.append(tbody);wrap.append(table);$('view-body').replaceChildren(wrap);
 }
 async function showHomeworkFiles(record,cell,button){
  if(button.disabled)return;
  const chosen=profile;button.disabled=true;
  const notify=(msg,kind)=>{if(document.querySelector('[data-section="homework"].active'))status(msg,kind)};
  notify('Sprawdzanie załączników dla wybranego zadania…');
  try{
    const query=new URLSearchParams({profile:String(chosen),assignment:String(record.id)});
    const result=await jsonRequest('/api/homework/files?'+query.toString());
    const files=Array.isArray(result.files)?result.files:[];
    const list=document.createElement('div');list.className='attachment-list';
    if(!files.length){list.textContent='Brak plików do pobrania.'}
    for(const file of files){
      if(!/^\d+$/.test(String(file.id||'')))continue;
      const down=document.createElement('button');down.type='button';down.className='attachment-action';
      down.textContent='Pobierz: '+String(file.name||('plik-'+file.id));
      down.addEventListener('click',()=>downloadHomeworkFile(chosen,record.id,file,down));
      list.append(down);
    }
    cell.replaceChildren(list);
    notify('Załączniki są gotowe do pobrania. Kliknij wybrany plik.','success');
  }catch(err){notify('Załączniki: '+err.message,'error')}
  finally{button.disabled=false}
 }
 async function downloadHomeworkFile(chosen,assignment,file,button){
  if(button.disabled)return;
  button.disabled=true;
  const notify=(msg,kind)=>{if(document.querySelector('[data-section="homework"].active'))status(msg,kind)};
  notify('Przygotowywanie pliku: '+String(file.name||'załącznik')+'. W niektórych przypadkach Librus może potrzebować do 150 sekund.');
  try{
    const q=new URLSearchParams({profile:String(chosen),assignment:String(assignment),file:String(file.id)});
    const controller=new AbortController();
    const timer=setTimeout(()=>controller.abort(),160000);
    let response;
    try{response=await fetch('/api/homework/download?'+q.toString(),{credentials:'same-origin',cache:'no-store',signal:controller.signal});
    }finally{clearTimeout(timer)}
    if(!response.ok){const body=await response.json().catch(()=>({}));throw Error(body.error||('Serwer zwrócił HTTP '+response.status))}
    const fileBlob=await response.blob();
    const fileURL=URL.createObjectURL(fileBlob);
    const a=document.createElement('a');a.href=fileURL;a.download=String(file.name||'zalacznik.bin');document.body.append(a);a.click();a.remove();
    setTimeout(()=>URL.revokeObjectURL(fileURL),20000);
    notify('Pobrano plik: '+String(file.name||'załącznik')+'. Sprawdź folder Pobrane.','success');
  }catch(err){notify('Pobieranie załącznika nieudane: '+(err.name==='AbortError'?'Przekroczono czas oczekiwania.':err.message),'error')}
  finally{button.disabled=false}
 }

 // v0.10: completely separate SchoolFiles view. Nothing is downloaded
 // until the user clicks a file. This view never calls other feature modules.
 function schoolViewActive(){return !!document.querySelector('[data-section="schoolfiles"].active')}
 async function renderSchoolFiles(){
   const chosen=profile;
   header('Dokumenty szkoły','Pliki udostępnione przez szkołę. Pobieranie tylko po kliknięciu, maksymalnie 20 MiB na dokument.');
   $('view-body').classList.remove('blank');
   $('view-body').textContent='';
   if(!logged[chosen]){
     $('view-body').textContent='Najpierw zaloguj wybrany profil.';
     status('Dokumenty szkoły: wymagane logowanie.','error');return;
   }
   const container=document.createElement('div');
   const explain=document.createElement('p');explain.className='diag-intro';
   explain.textContent='Lista dokumentów pochodzi z modułu SchoolFiles. Wyświetlenie listy nie pobiera zawartości plików. Kliknięcie „Pobierz” rozpoczyna pobieranie wyłącznie wybranego dokumentu.';
   container.append(explain);
   const info=document.createElement('p');info.className='diag-summary';info.textContent='Pobieranie listy dokumentów…';container.append(info);
   $('view-body').replaceChildren(container);
   status('Odczyt listy dokumentów szkoły…');
   try{
     const q=new URLSearchParams({profile:String(chosen)});
     const response=await jsonRequest('/api/school/files?'+q.toString());
     if(!schoolViewActive()||chosen!==profile)return;
     const documents=Array.isArray(response.items)?response.items:[];
     if(!documents.length){info.textContent='Szkoła nie udostępniła dokumentów dla tego profilu.';status('Dokumenty szkoły: brak wpisów.','success');return}
     info.textContent='Znaleziono '+documents.length+' dokumentów. Pliki są pobierane wyłącznie po kliknięciu.';
     const wrap=document.createElement('div');wrap.className='tablewrap';
     const table=document.createElement('table');table.className='datatable';
     const thead=document.createElement('thead');const h=document.createElement('tr');
     for(const label of ['Data dodania','Nazwa dokumentu','Status','Pobierz']){
       const th=document.createElement('th');th.textContent=label;h.append(th)
     }
     thead.append(h);table.append(thead);
     const tbody=document.createElement('tbody');
     documents.sort((a,b)=>String(b.date||'').localeCompare(String(a.date||'')));
     for(const doc of documents){
       const tr=document.createElement('tr');
       for(const value of [String(doc.date||'—'),String(doc.name||'Dokument'),String(doc.status||'—')]){
         const td=document.createElement('td');td.textContent=value;tr.append(td)
       }
       const cell=document.createElement('td');
       if(doc.downloadable && /^[A-Za-z0-9_-]{1,90}$/.test(String(doc.id||''))){
         const btn=document.createElement('button');btn.className='attachment-action';btn.type='button';btn.textContent='Pobierz';
         btn.addEventListener('click',()=>downloadSchoolFile(chosen,doc,btn));cell.append(btn)
       }else{cell.textContent='Plik niedostępny'}
       tr.append(cell);tbody.append(tr)
     }
     table.append(tbody);wrap.append(table);container.append(wrap);
     status('Pobrano '+documents.length+' pozycji z Dokumentów szkoły.','success');
   }catch(e){
     if(!schoolViewActive()||chosen!==profile)return;
     info.textContent='Nie udało się pobrać listy dokumentów. Pozostałe zakładki działają niezależnie.';
     status('Dokumenty szkoły: '+String(e.message||'Błąd połączenia'),'error');
   }
 }
 // Fetch + Blob do not automatically use the HTTP Content-Disposition filename.
 // Go sends either filename= or RFC 5987 filename*= (UTF-8 for Polish names).
 function schoolFilenameFromResponse(response,fallback){
   const hdr=(response.headers&&response.headers.get&&response.headers.get('Content-Disposition'))||'';
   let name='';
   const encoded=hdr.match(/(?:^|;)\s*filename\*\s*=\s*(?:"([^"]+)"|([^;]+))/i);
   if(encoded){
     const field=(encoded[1]||encoded[2]||'').trim();
     const part=field.match(/^([^']*)'[^']*'(.*)$/);
     if(part&&/^utf-8$/i.test(part[1])){
       try{name=decodeURIComponent(part[2]);}catch(_){}
     }
   }
   if(!name){
     const quoted=hdr.match(/(?:^|;)\s*filename\s*=\s*"((?:\\.|[^"\\])*)"/i);
     if(quoted)name=quoted[1].replace(/\\(.)/g,'$1');
     else{const plain=hdr.match(/(?:^|;)\s*filename\s*=\s*([^;]+)/i);if(plain)name=plain[1].trim();}
   }
   name=String(name||fallback||'dokument.bin').replace(/\\/g,'/').split('/').pop().replace(/[\x00-\x1f\x7f<>:"|?*]/g,'').trim().replace(/[. ]+$/g,'');
   return name||'dokument.bin';
 }
 async function downloadSchoolFile(chosen,doc,btn){
   if(btn.disabled)return;
   btn.disabled=true;
   const notify=(message,kind)=>{if(schoolViewActive()&&chosen===profile)status(message,kind)};
   notify('Pobieranie dokumentu „'+String(doc.name||'Dokument')+'”… Librus może przygotowywać plik przez dłuższy czas.');
   try{
     const q=new URLSearchParams({profile:String(chosen),file:String(doc.id)});
     const controller=new AbortController();const timer=setTimeout(()=>controller.abort(),160000);
     let response;
     try{response=await fetch('/api/school/download?'+q.toString(),{credentials:'same-origin',cache:'no-store',signal:controller.signal});}
     finally{clearTimeout(timer)}
     if(!response.ok){const body=await response.json().catch(()=>({}));throw Error(body.error||('Serwer zwrócił HTTP '+response.status))}
     const file=await response.blob();
     const link=URL.createObjectURL(file);
     const a=document.createElement('a');a.href=link;a.download=schoolFilenameFromResponse(response,doc.name);document.body.append(a);a.click();a.remove();
     setTimeout(()=>URL.revokeObjectURL(link),20000);
     notify('Pobrano dokument „'+String(doc.name||'dokument')+'”. Sprawdź folder Pobrane.','success');
   }catch(e){notify('Nie udało się pobrać dokumentu: '+(e.name==='AbortError'?'Przekroczono czas oczekiwania.':String(e.message)),'error')}
   finally{btn.disabled=false}
 }

 // v0.9: this code is intentionally separate from the data tab renderer.
 // No upstream requests take place until the user explicitly presses Sprawdź.
 function diagStateText(v){return ({ok:'Działa',empty:'Brak wpisów',partial:'Częściowo',blocked:'Sesja wygasła',failed:'Błąd',unverified:'Niezweryfikowane',running:'Sprawdzanie',demo:'DEMO',waiting:'Nie sprawdzono'})[v]||'Nieznany'}
 function diagViewActive(){return !!document.querySelector('[data-section="diagnostics"].active')}
 function renderDiagnostics(){
  diagnosticRun++;
  const chosen=profile;
  header('Diagnostyka modułów','Sprawdzanie na żądanie. Awaria jednej funkcji nie zatrzymuje pozostałych.');
  const container=document.createElement('div');
  const info=document.createElement('div');info.className='diag-intro';
  info.textContent='Test na żądanie odczytuje dane wybranego modułu, ale w diagnostyce pokazuje tylko status i liczbę wpisów. Nie wyświetla treści wiadomości ani nie zmienia danych Librusa.';
  container.append(info);
  const actions=document.createElement('div');actions.className='diag-actions';
  const all=document.createElement('button');all.type='button';all.className='primary';all.textContent='Sprawdź wszystkie moduły';
  const demo=document.createElement('button');demo.type='button';demo.className='secondary';demo.textContent='Przykład — DEMO';
  const sample=document.createElement('button');sample.type='button';sample.className='secondary';sample.textContent='Pobierz plik testowy';
  const report=document.createElement('button');report.type='button';report.className='secondary';report.textContent='Zapisz raport TXT';
  actions.append(all,demo,sample,report);container.append(actions);
  const caution=document.createElement('div');caution.className='diag-info';
  caution.textContent='Test pobrania lokalnego pliku nie sprawdza rzeczywistych załączników Librusa. Jeśli zadania nie mają plików, status załączników pozostaje NIEZWERYFIKOWANY.';
  container.append(caution);
  const wrap=document.createElement('div');wrap.className='tablewrap';
  const table=document.createElement('table');table.className='diag-tbl';
  const head=document.createElement('thead');const heading=document.createElement('tr');
  for(const title of ['Moduł','Stan','Wynik','Czas','Test']){const th=document.createElement('th');th.textContent=title;heading.append(th)}
  head.append(heading);table.append(head);
  const tbody=document.createElement('tbody');const rows=new Map();const reportStates=new Map();
  for(const [id,name] of diagModules){
   const tr=document.createElement('tr');
   const title=document.createElement('td');title.textContent=name;
   const statusCell=document.createElement('td');const badge=document.createElement('span');badge.className='diag-badge';badge.textContent='Nie sprawdzono';statusCell.append(badge);
   const detail=document.createElement('td');detail.className='diag-detail';detail.textContent='Uruchom test na tym profilu.';
   const timing=document.createElement('td');timing.className='diag-time';timing.textContent='—';
   const action=document.createElement('td');const btn=document.createElement('button');btn.className='attachment-action';btn.textContent='Sprawdź';btn.type='button';action.append(btn);
   tr.append(title,statusCell,detail,timing,action);tbody.append(tr);
   rows.set(id,{badge,detail,timing,btn});
  }
  table.append(tbody);wrap.append(table);container.append(wrap);
  const summary=document.createElement('div');summary.className='diag-summary';summary.setAttribute('aria-live','polite');
  summary.textContent='Nie uruchomiono jeszcze diagnostyki.';container.append(summary);
  $('view-body').classList.remove('blank');
  $('view-body').replaceChildren(container);
  const update=(id,res)=>{
   if(!diagViewActive()||profile!==chosen)return;
   const row=rows.get(id);if(!row)return;
   row.badge.textContent=diagStateText(res.state);row.badge.className='diag-badge '+String(res.state||'');
   row.detail.textContent=String(res.detail||'');
   row.timing.textContent=typeof res.elapsedMs==='number'?String(res.elapsedMs)+' ms':'—';
   reportStates.set(id,{state:String(res.state||''),detail:String(res.detail||''),time:typeof res.elapsedMs==='number'?res.elapsedMs:null});
  };
  const diagFetch=async id=>{
   update(id,{state:'running',detail:'Odczyt wybranego modułu…'});
   const q=new URLSearchParams({profile:String(chosen),module:id});
   try{
    const res=await jsonRequest('/api/diagnostics/check?'+q.toString());
    update(id,{state:res.state,detail:String(res.detail||'')+(typeof res.count==='number'?' • wpisów: '+res.count:''),elapsedMs:res.elapsedMs});
    return String(res.state||'failed');
   }catch(e){
    update(id,{state:'failed',detail:'Błąd lokalnego testu: '+String(e.message||'niedostępny')});
    return 'failed';
   }
  };
  let working=false;
  all.addEventListener('click',async()=>{
   if(working)return;
   if(!logged[chosen]){summary.textContent='Najpierw zaloguj wybrany profil albo uruchom DEMO.';return}
   working=true;all.disabled=true;for(const row of rows.values())row.btn.disabled=true;
   const generation=diagnosticRun;
   const counts={ok:0,empty:0,partial:0,unverified:0,blocked:0,failed:0};
   for(let i=0;i<diagModules.length;i++){
    if(generation!==diagnosticRun||!diagViewActive()||profile!==chosen)break;
    const [id,name]=diagModules[i];summary.textContent='Sprawdzanie '+(i+1)+'/'+diagModules.length+': '+name;
    const state=await diagFetch(id);if(state in counts)counts[state]++;
    if(i+1<diagModules.length)await new Promise(resolve=>setTimeout(resolve,250));
   }
   if(generation===diagnosticRun&&diagViewActive()&&profile===chosen){
    summary.textContent='Zakończono. Działa: '+counts.ok+', bez wpisów: '+counts.empty+', częściowo: '+counts.partial+', niezweryfikowane: '+counts.unverified+', błędy: '+(counts.failed+counts.blocked)+'.';
    all.disabled=false;for(const row of rows.values())row.btn.disabled=false;
   }
   working=false;
  });
  for(const [id] of diagModules){rows.get(id).btn.addEventListener('click',async()=>{
    if(working)return;
    if(!logged[chosen]){summary.textContent='Najpierw zaloguj wybrany profil.';return}
    working=true;rows.get(id).btn.disabled=true;
    const result=await diagFetch(id);
    if(diagViewActive()&&profile===chosen){summary.textContent='Przetestowano '+rows.get(id).badge.textContent+'. Każdy moduł możesz sprawdzić niezależnie.';rows.get(id).btn.disabled=false}
    working=false;
  })}
  demo.addEventListener('click',()=>{
   if(!diagViewActive()||profile!==chosen)return;
   for(let i=0;i<diagModules.length;i++){
    const [id]=diagModules[i];update(id,{state:'demo',detail:id==='attachments'?'DEMO: brak prawdziwego pliku; nie testowano pobrania.':'Przykładowy wynik — bez połączenia z Librusem.'});
   }
   summary.textContent='DEMO — wyłącznie przykładowe wyniki. Nie wykonano żadnych żądań do Librusa.';
  });
  report.addEventListener('click',()=>{
   const lines=['SYNERGIA API v0.9 — DIAGNOSTYKA MODUŁÓW',
      'Profil: '+(chosen+1)+' (bez loginu i danych osobowych)',
      'Data: '+new Date().toLocaleString('pl-PL'),
      'Tryb: ręczny test, bez automatycznego wysyłania',
      'Nie testowano rzeczywistego pobierania plików z Librusa.', ''];
   for(const [id,name] of diagModules){
    const data=reportStates.get(id)||{state:'waiting',detail:'Nie uruchomiono testu.',time:null};
    lines.push(name+' ['+id+']: '+diagStateText(data.state)+' — '+data.detail+
     (data.time===null?'':' ('+data.time+' ms)'));
   }
   const blob=new Blob([lines.join('\n')+'\n'],{type:'text/plain;charset=utf-8'});
   const address=URL.createObjectURL(blob);
   const a=document.createElement('a');a.href=address;a.download='SYNERGIA_API_DIAGNOSTYKA_v0_9.txt';document.body.append(a);a.click();a.remove();
   setTimeout(()=>URL.revokeObjectURL(address),20000);
   summary.textContent='Raport diagnostyczny przygotowany bez loginów, haseł i treści szkolnych wiadomości.';
  });
  sample.addEventListener('click',()=>{

   const a=document.createElement('a');a.href='/api/diagnostics/sample-file';
   a.download='SYNERGIA_API_PLIK_TESTOWY.txt';document.body.append(a);a.click();a.remove();
   summary.textContent='Rozpoczęto pobieranie testowego pliku z lokalnego serwera. Realnego pobierania załączników Librusa nie sprawdzono.';
  });
  status('Diagnostyka jest gotowa. Wybierz test pojedynczego modułu albo wszystkich modułów.');
 }
 function selectProfile(id){diagnosticRun++;profile=id;
  window.PocketGUI?.profileChanged();
  window.SynergiaMessagesV12?.leave();
  window.SynergiaUpdatesV13?.leave();
  document.querySelectorAll('[data-profile]').forEach(b=>{const yes=Number(b.dataset.profile)===id;b.classList.toggle('active',yes);b.setAttribute('aria-selected',String(yes));b.classList.toggle('online',logged[Number(b.dataset.profile)])});
  $('login').value='';$('password').value='';$('account-status').textContent=logged[id]?'Zalogowano • '+(accountNames[id]||('Profil '+(id+1))):'Brak zalogowanego konta';
  status('Wybrano Profil '+(id+1)+(logged[id]?'. Możesz pobierać dane.':'. Zaloguj się lub otwórz demo.'));
 }
 $('login-form').addEventListener('submit',async event=>{
  event.preventDefault();if(busy)return;
  const login=$('login').value.trim();const password=$('password').value; $('password').value='';
  if(!login||!password){status('Podaj login i hasło.','error');return;}
  setBusy(true);status('Nawiązywanie bezpośredniego połączenia z Librusem...');
  try{await jsonRequest('/api/login',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({profile,login,password})});
    logged[profile]=true;accountNames[profile]=login;selectProfile(profile);header('Połączono z kontem','Możesz wybrać moduł do odczytu.');
    $('view-body').textContent='Sesja istnieje tylko w pamięci tego programu.';status('Logowanie zakończone.','success');
  }catch(err){status(err.message,'error')}finally{setBusy(false)}
 });
 async function openSection(section){if(busy)return;
  if(window.PocketGUI?.intercept(section))return;
  diagnosticRun++;
  if(section!=='updates'&&section!=='health')window.SynergiaUpdatesV13?.leave();
  if((section==='updates'||section==='health')&&window.SynergiaUpdatesV13){
    document.querySelectorAll('[data-section]').forEach(b=>b.classList.toggle('active',b.dataset.section===section));
    window.SynergiaMessagesV12?.leave();
    await window.SynergiaUpdatesV13.open(section);return;
  }
  if(section!=='messages')window.SynergiaMessagesV12?.leave();
  if(section==='messages'&&window.SynergiaMessagesV12){
    document.querySelectorAll('[data-section]').forEach(b=>b.classList.toggle('active',b.dataset.section===section));
    await window.SynergiaMessagesV12.open();return;
  }
  if(section==='schoolfiles'){
    document.querySelectorAll('[data-section]').forEach(b=>b.classList.toggle('active',b.dataset.section===section));
    await renderSchoolFiles();return;
  }
  if(section==='diagnostics'){
    document.querySelectorAll('[data-section]').forEach(b=>b.classList.toggle('active',b.dataset.section===section));
    renderDiagnostics();return;
  }
  $('view-body').classList.add('blank');
  document.querySelectorAll('[data-section]').forEach(b=>b.classList.toggle('active',b.dataset.section===section));
  if(section==='demo'){
    if(window.PocketGUI){window.PocketGUI.demo();return;}
    header('DEMO — wyłącznie fikcyjne dane','To nie są informacje z konta szkolnego. Bez logowania i dostępu do internetu.');
    drawTable('messages',[
      {date:'2026-11-04',sender:'Nauczyciel (PRZYKŁAD)',subject:'Zajęcia — PRZYKŁAD',preview:'Testowy tekst wiadomości, bez powiązania z Librusem.'},
      {date:'2026-11-03',sender:'Szkoła (PRZYKŁAD)',subject:'Ogłoszenie — PRZYKŁAD',preview:'Tak wyglądałaby lista wiadomości po prawdziwym pobraniu.'}
    ]);status('DEMO — dane całkowicie przykładowe.','success');return;
  }
  if(!logged[profile]){status('Najpierw zaloguj Profil '+(profile+1)+' lub użyj DEMO.','error');return}
  setBusy(true);header(titles[section],'Pobieranie danych bezpośrednio z Synergii…');$('view-body').textContent='Łączenie z Librusem…';status('Pobieranie: '+titles[section]+'...');
  try{const body=await jsonRequest('/api/data?profile='+profile+'&section='+encodeURIComponent(section));
    let rows=Array.isArray(body.items)?body.items:[];
    if(section==='me'){const source=body.items?.Me||body.items||{};rows=Object.entries(source).filter(([key])=>!/(token|pass|cookie|secret)/i.test(key)).map(([key,value])=>({key,value:typeof value==='object'?JSON.stringify(value):String(value)}))}
    if(section==='timetable'){rows=rows.map(x=>({...x,time:String(x.start||'?')+'–'+String(x.end||'?'),subjectLabel:String(x.subject||'')+(x.canceled?' [ODWOŁANA]':x.substitution?' [ZASTĘPSTWO]':'')}));rows.sort((a,b)=>String(a.date||'').localeCompare(String(b.date||''))||String(a.start||'').localeCompare(String(b.start||'')))}
    if(['attendance','agenda','notes'].includes(section)){rows.sort((a,b)=>String(b.date||'').localeCompare(String(a.date||'')))}
    if(section==='notices'){rows.sort((a,b)=>String(b.created||'').localeCompare(String(a.created||'')))}
    if(section==='freedays'){rows.sort((a,b)=>String(a.from||'').localeCompare(String(b.from||'')))}
    header(titles[section],section==='messages'?'Tylko lista i podglądy. Pełna wiadomość nie jest otwierana automatycznie.':section==='homework'?'Osobna lista HomeWorkAssignments. Załączniki pobierane wyłącznie po kliknięciu, limit 20 MB.':section==='notices'?'Tablica szkolna; tylko odczyt. Otwieranie nie oznacza ogłoszeń jako przeczytanych.':section==='freedays'?'Dni wolne szkoły i tylko klasy aktualnie wybranego ucznia; częściowe zwolnienia dotyczą wybranych lekcji.':'Dane pobrane bezpośrednio z Librus Synergia.');
    if(rows.length||section==='grades')drawTable(section,rows,body);else $('view-body').innerHTML='<div class="blank">Nie ma danych do wyświetlenia. To może oznaczać pustą listę albo brak danych dla tego konta.</div>';
    const warnings=Array.isArray(body.warnings)?body.warnings.filter(x=>typeof x==='string'&&x.length<250):[];
    status('Pobrano '+rows.length+' pozycji z modułu: '+titles[section]+'.'+(warnings.length?' Dane uzupełniające: '+warnings.join(' '):''),warnings.length?'':'success');
  }catch(err){$('view-body').textContent='Nie udało się pobrać danych. Sprawdź komunikat powyżej.';status(err.message,'error')}
  finally{setBusy(false)}
 }
 document.querySelectorAll('[data-section]').forEach(b=>b.addEventListener('click',()=>openSection(b.dataset.section)));
 document.querySelectorAll('[data-profile]').forEach(b=>b.addEventListener('click',()=>selectProfile(Number(b.dataset.profile))));
 $('exit-button').addEventListener('click',async()=>{if(!confirm('Zamknąć lokalny program SYNERGIA API?'))return;try{await jsonRequest('/api/exit',{method:'POST'});}catch(_){};document.body.textContent='Program został zamknięty. Możesz zamknąć tę kartę.'});
 window.SynergiaMessageHostV12={profile:()=>profile,logged:()=>logged[profile],active:()=>Boolean(document.querySelector('[data-section="messages"].active')),status,header,busy:setBusy};
 window.PocketGUIHost={profile:()=>profile,logged:()=>logged[profile],openSection,selectProfile,header,status,files:showHomeworkFiles};
 window.SynergiaUpdatesHostV13={profile:()=>profile,logged:()=>logged[profile],status,header,openSection,restore:states=>{
   let changed=false;for(let i=0;i<2;i++)if(states[i]===true&&!logged[i]){logged[i]=true;changed=true;}
   if(changed)selectProfile(logged[profile]?profile:(logged[0]?0:1));
 }};
})();
</script>
</body></html>`
