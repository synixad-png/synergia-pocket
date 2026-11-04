"""Chromium: Stołówka SP3 na iPhone i desktop, tylko syntetyczne publiczne dane."""
from pathlib import Path
import shutil
from playwright.sync_api import sync_playwright, expect

ROOT = Path(__file__).resolve().parents[1] / "static"
html = (ROOT / "index.html").read_text()
fake = r"""
window.__canteenBroken = false;
window.fetch = async (url, init={}) => {
  const path=String(url).split('?')[0];
  const respond=(data,status=200)=>new Response(JSON.stringify(data),{
    status,headers:{'Content-Type':'application/json'}});
  const profile={
    authenticated:true,mode:'demo',name:'Uczeń testowy',
    active_profile_id:'demo',profiles:[{id:'demo',name:'Uczeń testowy',mode:'demo'}]
  };
  if(path==='/api/session') return respond({authenticated:false});
  if(path==='/api/demo') return respond(profile);
  if(path==='/api/canteen/sp3') {
    if(window.__canteenBroken) return respond({
      status:'unavailable',source_url:'https://sp3wroc.edupage.org/a/stolowka',
      news_url:'https://sp3wroc.edupage.org/news/', checked_at:'2026-11-04T11:00:00Z',
      menu:null,notices:[],payment:{verified:false,current_amount:null,
        message:'Źródło jest niedostępne.'}
    });
    return respond({
      status:'ok',school:'SP3',source_url:'https://sp3wroc.edupage.org/a/stolowka',
      news_url:'https://sp3wroc.edupage.org/news/',
      checked_at:'2026-11-04T11:00:00Z',
      menu:{period:'02-06.11.2026',period_end:'2026-11-06',
        is_current:true,image_id:null,image_available:false},
      notices:['Karta zapisu jest wymagana. <script>alert(1)</script>'],
      payment:{verified:false,current_amount:null,current_deadline:null,
        account_balance:null,message:'Brak kwoty <img src=x onerror=alert(1)>'},
      registration_url:'https://sp3wroc.edupage.org/a/stolowka'
    });
  }
  if(path.startsWith('/api/section/')) return respond({
    items:[],subjects:[],count:0,days:[],stats:{total:0}
  });
  return respond({detail:'Brak fikcyjnej odpowiedzi'},404);
};
"""

with sync_playwright() as playwright:
    executable = shutil.which("chromium") or shutil.which("chromium-browser")
    opts = {"executable_path": executable} if executable else {}
    browser=playwright.chromium.launch(headless=True,args=['--no-sandbox','--disable-dev-shm-usage'],**opts)
    for width,height in [(390,844),(1280,900)]:
        page=browser.new_page(viewport={"width":width,"height":height})
        errors=[]
        page.on("pageerror",lambda error:errors.append(str(error)))
        page.set_content(html)
        page.add_style_tag(content=(ROOT/"style.css").read_text())
        page.add_style_tag(content=(ROOT/"canteen-sp3.css").read_text())
        page.evaluate(fake)
        page.add_script_tag(content=(ROOT/"canteen-sp3.js").read_text())
        page.add_script_tag(content=(ROOT/"app.js").read_text().replace(
            "window.location.origin", '"https://testing.invalid"'
        ))
        page.locator("#demo-btn").click()
        page.locator("#workspace:not(.hidden)").wait_for()
        page.evaluate("navigate('canteen-sp3')")
        expect(page.locator("#content h1")).to_have_text("Stołówka")
        expect(page.locator(".canteen-panel")).to_have_count(3)
        expect(page.locator(".canteen-menu-top")).to_contain_text("02-06.11.2026")
        expect(page.locator(".canteen-important")).to_contain_text("nie zostały potwierdzone")
        assert page.locator("#content script").count() == 0
        assert page.locator("#content img").count() == 0
        assert page.locator('.canteen-actions a[href^="https://sp3wroc.edupage.org/"]').count() >= 3
        page.evaluate("window.__canteenBroken=true; navigate('canteen-sp3')")
        expect(page.locator(".canteen-warning")).to_contain_text("Nie udało się teraz odczytać")
        expect(page.locator("#content h1")).to_have_text("Stołówka")
        page.evaluate("navigate('grades')")
        expect(page.locator("#content h1")).to_have_text("Oceny")
        assert not errors, errors
        page.close()
    browser.close()
print("PASS Chromium Stołówka SP3: mobile/desktop, sourced dates, safe HTML, network failure, grades")
