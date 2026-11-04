"""GUI flow tests with fictional school data. Never contacts Librus."""
import os
from pathlib import Path
from playwright.sync_api import sync_playwright,expect
html=Path(os.environ.get('POCKET_GUI_HTML','/tmp/pocket-gui.html')).read_text()
fake=r"""
window.__calls=[];window.__sent=[];
window.fetch=async(url,init={})=>{
 const u=new URL(String(url),'https://pocket.test'),profile=Number(u.searchParams.get('profile')||0);
 window.__calls.push({path:u.pathname,method:init.method||'GET',profile});
 const response=(data,status=200)=>new Response(JSON.stringify(data),{status,headers:{'Content-Type':'application/json'}});
 if(u.pathname==='/api/updates/profiles')return response({logged:[false,false]});
 if(u.pathname==='/api/login')return response({ok:true});
 if(u.pathname==='/api/updates/state')return response({events:[],enabled:false,unread:0,through:0,statuses:[]});
 if(u.pathname==='/api/data'){
  const section=u.searchParams.get('section');
  const data={grades:[{subject:'Matematyka '+profile,value:'5',category:'Sprawdzian <img src=x>',date:'2026-11-04'}],timetable:[{date:'2026-11-04',start:'08:00',end:'08:45',number:1,subject:'Język polski',room:'12',teacher:'Przykładowy nauczyciel'},{date:'2026-11-04',start:'08:55',end:'09:40',number:2,subject:'Matematyka',substitution:true,oldSubject:'Historia',note:'Zastępstwo <img src=x>'}],homework:[{id:'1',topic:'Przykładowe zadanie',content:'Treść <img src=x>',attachmentNames:'plik.pdf',dueDate:'2026-11-07'}],notes:[{content:'Przykładowa uwaga',date:'2026-11-04'}],notices:[{subject:'Przykładowe ogłoszenie',content:'Informacja',created:'2026-11-04'}],contacts:[{name:'Anna Przykładowa',role:'Nauczyciel'}],freedays:[{from:'2026-11-11',to:'2026-11-11',name:'Dzień wolny'}],agenda:[{date:'2026-11-06',subject:'Matematyka',content:'Sprawdzian'}],attendance:[{date:'2026-11-04',subject:'Język polski',type:'Nieobecność'}],me:{Me:{Name:'Dane przykładowe'}}};
  return response({items:data[section]||[],warnings:[]});
 }
 if(u.pathname==='/api/homework/files')return response({files:[{id:'9',name:'plik.pdf'}]});
 if(u.pathname==='/api/homework/download')return new Response('%PDF-1.7 TEST', {headers:{'Content-Type':'application/pdf'}});
 if(u.pathname==='/api/school/files')return response({items:[{id:'12',name:'Regulamin.pdf',date:'2026-11-04',status:'active',downloadable:true}]});
 if(u.pathname==='/api/school/download')return new Response('%PDF-1.7 TEST',{headers:{'Content-Type':'application/pdf','Content-Disposition':'attachment; filename="Regulamin.pdf"'}});
 if(u.pathname==='/api/messages/list')return response({items:[{id:'42',subject:'Wiadomość testowa '+profile,sender:'Nauczyciel',date:'2026-11-04',preview:'Podgląd',hasAttachments:true}]});
 if(u.pathname==='/api/messages/open')return response({items:{id:'42',sender:'Nauczyciel',senderId:'70',subject:'Wiadomość testowa',date:'2026-11-04',content:'Treść <img src=x>',noReply:false,attachments:[{id:'9',name:'plik.pdf'}]}});
 if(u.pathname==='/api/messages/recipients')return response({items:[{id:'70',name:'Nauczyciel',group:'Szkoła'}]});
 if(u.pathname==='/api/messages/send'){window.__sent.push(JSON.parse(init.body));return response({state:'sent',message:'Librus przyjął wiadomość.'});}
 if(u.pathname==='/api/messages/download')return new Response('%PDF-1.7 TEST',{headers:{'Content-Type':'application/pdf','Content-Disposition':'attachment; filename="plik.pdf"'}});
 return response({error:'Nieobsługiwany testowy endpoint '+u.pathname},400);
};void 0;
"""
with sync_playwright() as p:
 browser=p.chromium.launch(headless=True,args=['--no-sandbox'])
 for width in [390,768,1440]:
  ctx=browser.new_context(viewport={'width':width,'height':1000},accept_downloads=True)
  page=ctx.new_page();errors=[];page.on('pageerror',lambda e:errors.append(str(e)))
  page.set_content(html.replace('<script>','<script>'+fake+'</script><script>',1))
  def nav(section):
   if width<=800:
    page.locator('#menu-toggle').click()
    expect(page.locator('#menu-toggle')).to_have_attribute('aria-expanded','true')
   page.locator('.nav [data-section="'+section+'"]').click()
   if width<=800:page.wait_for_function("document.getElementById('app-sidebar').getBoundingClientRect().right<=0")
  def overflow():assert page.evaluate('document.documentElement.scrollWidth<=innerWidth')
  expect(page.locator('#view-title')).to_have_text('Biurko');expect(page.locator('.module-card')).to_have_count(8)
  overflow()
  nav('demo');expect(page.locator('#demo-banner')).to_be_visible()
  nav('grades');expect(page.locator('.grade-subject')).to_have_count(5)
  page.locator('.grade-chip').first.click();expect(page.locator('#grade-dialog')).to_be_visible();page.locator('#grade-close').click()
  nav('timetable');expect(page.locator('.day-tab')).to_have_count(5);expect(page.locator('.lesson-card')).to_have_count(5)
  overflow();page.screenshot(path='/tmp/pocket-gui-plan-'+str(width)+'.png',full_page=True)
  nav('messages');expect(page.locator('#view-body')).to_contain_text('Informacja o zebraniu')
  assert page.evaluate('window.__sent.length')==0
  page.locator('#leave-demo').click();expect(page.locator('#demo-banner')).to_be_hidden()
  page.locator('#account-toggle').click();expect(page.locator('#account-overlay')).to_be_visible()
  page.locator('#login').fill('synthetic');page.locator('#password').fill('synthetic');page.locator('#login-button').click()
  expect(page.locator('#account-overlay')).to_be_hidden();expect(page.locator('#top-account')).to_contain_text('Połączono')
  nav('grades');expect(page.locator('.grade-subject')).to_contain_text('Matematyka 0')
  page.locator('.grade-chip').first.click();expect(page.locator('#grade-detail')).to_contain_text('Sprawdzian <img src=x>');assert page.locator('#grade-dialog img').count()==0;page.locator('#grade-close').click()
  nav('timetable');expect(page.locator('.lesson-card')).to_have_count(2);expect(page.locator('#view-body')).to_contain_text('ZASTĘPSTWO');assert page.locator('.lesson-info img').count()==0
  nav('homework');expect(page.locator('.record-card')).to_contain_text('Przykładowe zadanie');page.get_by_role('button',name='Pokaż pliki',exact=True).click()
  with page.expect_download() as download:page.get_by_role('button',name='Pobierz: plik.pdf',exact=True).click()
  assert download.value.suggested_filename=='plik.pdf'
  nav('schoolfiles');expect(page.locator('#view-body')).to_contain_text('Regulamin.pdf')
  nav('messages');expect(page.locator('#msg12')).to_contain_text('Wiadomość testowa 0');page.get_by_role('button',name='Otwórz',exact=True).click()
  expect(page.locator('.msg12-body')).to_have_text('Treść <img src=x>');assert page.locator('.msg12-body img').count()==0
  page.get_by_role('button',name='Odpowiedz',exact=True).click();page.locator('.msg12-form textarea').fill('Odpowiedź testowa')
  page.get_by_role('button',name='Podgląd wysyłki',exact=True).click();page.get_by_role('button',name='Wyślij wiadomość',exact=True).click()
  expect(page.locator('#status')).to_contain_text('Librus przyjął')
  assert page.evaluate('window.__sent[0].replyTo')=='42'
  # New-message actions still use the existing real recipient picker and preview.
  nav('messages');page.get_by_role('button',name='Nowa wiadomość',exact=True).click()
  page.locator('.msg12-form select').select_option('70');page.locator('.msg12-form input').fill('Temat testowy');page.locator('.msg12-form textarea').fill('Treść testowa')
  page.get_by_role('button',name='Podgląd wysyłki',exact=True).click();page.get_by_role('button',name='Wyślij wiadomość',exact=True).click()
  expect(page.locator('#status')).to_contain_text('Librus przyjął');assert page.evaluate('window.__sent.length')==2
  nav('notes');expect(page.locator('#view-body')).to_contain_text('Przykładowa uwaga')
  page.locator('#theme-toggle').click();assert page.evaluate('document.documentElement.dataset.theme')=='light'
  overflow();page.screenshot(path='/tmp/pocket-gui-light-'+str(width)+'.png',full_page=True)
  page.locator('#account-toggle').click();page.locator('[data-profile="1"]').click();expect(page.locator('#account-overlay')).to_be_visible()
  expect(page.locator('#view-body')).not_to_contain_text('Przykładowa uwaga');page.locator('#login').fill('synthetic-two');page.locator('#password').fill('synthetic');page.locator('#login-button').click();expect(page.locator('#account-overlay')).to_be_hidden()
  nav('grades');expect(page.locator('#view-body')).to_contain_text('Matematyka 1');expect(page.locator('#view-body')).not_to_contain_text('Matematyka 0')
  page.locator('#account-toggle').click();page.keyboard.press('Escape');expect(page.locator('#account-overlay')).to_be_hidden()
  overflow();assert not errors,errors
  print('PASS GUI',width,'px: demo, grouped grades, plan, messages/reply/send, homework download, profiles, themes, no XSS/overflow/JS errors',flush=True)
  ctx.close()
 browser.close()
