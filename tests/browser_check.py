"""Render the actual HTML/CSS/JS in Chromium, mock all API calls (no credentials/network)."""
from pathlib import Path
from playwright.sync_api import sync_playwright, expect

root=Path(__file__).resolve().parents[1]/'static'
html=(root/'index.html').read_text().replace('<script src="/static/app.js" defer></script>','').replace('<link rel="stylesheet" href="/static/style.css">','')
js=(root/'app.js').read_text().replace('window.location.origin','"https://testing.invalid"')  # Only in test: about:blank has no origin
mock=r'''
window.__mock={profiles:[],active:''};
window.fetch=async (url,init={})=>{
 const path=String(url).split('?')[0];
 const body=init.body?JSON.parse(init.body):{};
 const mock=window.__mock;
 let status=200,data={};
 const payload=()=>{
  const current=mock.profiles.find(p=>p.id===mock.active);
  return {authenticated:true,mode:current.mode,name:current.name,profiles:mock.profiles.map(x=>({...x})),active_profile_id:mock.active};
 };
 if(path==='/api/session')data={authenticated:false};
 else if(path==='/api/demo') {mock.profiles=[{id:'demo',name:'Test demo',mode:'demo'}];mock.active='demo';data=payload();}
 else if(path==='/api/login'){mock.profiles=[{id:'first',name:body.label||'Pierwsza',mode:'live'}];mock.active='first';data=payload();}
 else if(path==='/api/profiles/add'){
   if(body.password!=='fake-pass-two'){status=401;data={detail:'Nieprawidłowe dane testowe'};}
   else {mock.profiles.push({id:'second',name:body.label||'Druga',mode:'live'});mock.active='second';data=payload();}
 }
 else if(path==='/api/profiles/switch'){mock.active=body.profile_id;data=payload();}
 else if(path==='/api/profiles/remove'){mock.profiles=mock.profiles.filter(p=>p.id!==body.profile_id);mock.active=mock.profiles[0].id;data=payload();}
 else if(path==='/api/profiles/rename'){mock.profiles.find(p=>p.id===body.profile_id).name=body.label;data=payload();}
 else if(path==='/api/logout'){mock.profiles=[];data={ok:true};}
 else if(path.startsWith('/api/section/')){
  const sec=path.split('/').at(-1);
  data={grades:{subjects:[],count:5},timetable:{days:[]},attendance:{stats:{total:0}},
       agenda:{items:[]},homework:{items:[]},messages:{items:[],unread:0},
       notes:{items:[]},announcements:{items:[]}}[sec];
 } else{status=404;data={detail:'Brak testowej odpowiedzi'};}
 return new Response(JSON.stringify(data),{status,headers:{'Content-Type':'application/json'}});
};
'''
with sync_playwright() as p:
    browser=p.chromium.launch(headless=True,executable_path='/usr/bin/chromium',args=['--no-sandbox','--disable-dev-shm-usage'])
    page=browser.new_page(viewport={'width':390,'height':844},device_scale_factor=1)
    errors=[]
    page.on('pageerror',lambda e:errors.append(str(e)))
    page.set_content(html)
    page.add_style_tag(content=(root/'style.css').read_text())
    page.evaluate(mock)
    page.add_script_tag(content=js)
    page.locator('#demo-btn').click()
    page.locator('#workspace:not(.hidden)').wait_for()
    page.locator('#profile-trigger').click()
    assert 'Test demo' in page.locator('#profile-options').inner_text()
    assert page.locator('#add-profile-btn').is_disabled()
    page.locator('#profile-trigger').click()
    page.locator('#logout').evaluate('(button)=>button.click()')
    page.locator('#auth:not(.hidden)').wait_for()
    print('PASS Chromium: tryb demo, profil i wylogowanie')
    page.locator('#login-label').fill('Hania')
    page.locator('#login').fill('fikcyjne-konto-1')
    page.locator('#password').fill('fake-pass-one')
    page.locator('#login-btn').click()
    page.locator('#workspace:not(.hidden)').wait_for()
    page.locator('#profile-trigger').click()
    page.locator('#add-profile-btn').click()
    page.locator('#profile-label').fill('Martyna')
    page.locator('#profile-login').fill('fikcyjne-konto-2')
    page.locator('#profile-password').fill('wrong-password')
    page.locator('#profile-submit').click()
    expect(page.locator('#profile-error')).to_have_text('Nieprawidłowe dane testowe')
    assert page.locator('#workspace').is_visible()
    page.locator('#profile-password').fill('fake-pass-two')
    page.locator('#profile-submit').click()
    page.locator('#profile-dialog').wait_for(state='hidden')
    expect(page.locator('#active-profile-name')).to_have_text('Martyna')
    assert page.locator('#password').input_value()==''
    assert page.locator('#profile-password').input_value()==''
    page.locator('#profile-trigger').click()
    page.locator('button[data-profile-switch="first"]').click()
    expect(page.locator('#active-profile-name')).to_have_text('Hania')
    page.locator('#profile-trigger').click()
    assert page.locator('#add-profile-btn').is_disabled()
    page.locator('#rename-profile-btn').click()
    page.locator('#profile-label').fill('Pierwsza')
    page.locator('#profile-submit').click()
    expect(page.locator('#active-profile-name')).to_have_text('Pierwsza')
    page.on('dialog',lambda dialog:dialog.accept())
    page.locator('#profile-trigger').click()
    page.locator('button[data-profile-remove="second"]').click()
    page.wait_for_function('window.__mock.profiles.length === 1')
    assert page.locator('#profile-options .profile-option').count()==1
    assert not errors,errors
    print('PASS Chromium: mobilne dodawanie, błąd hasła, przełączanie, zmiana nazwy, usuwanie')
    # Optional screenshot for review, not user-shared personal data.
    page.screenshot(path=str(root.parent/'tests'/'mobile_preview_v4.png'))
    browser.close()
