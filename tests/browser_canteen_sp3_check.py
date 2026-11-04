"""Real Chromium: calendar navigation, meals, empty days, mobile and theme."""
from datetime import datetime, timezone
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
      status:'unavailable',checked_at:'2026-11-04T11:00:00Z',
      calendar:{status:'unavailable',days:[]},
      menu:null,notices:[],payment:{verified:false}
    });
    return respond({
      status:'ok',checked_at:'2026-11-04T11:00:00Z',
      calendar:{status:'ok',checked_at:'2026-11-04T11:00:00Z',days:[
        {date:'2026-11-04',status:'published',items:['Zupa testowa','Naleśniki testowe']},
        {date:'2026-11-05',status:'published',items:['Spaghetti testowe','Kompot <img src=x onerror=alert(1)>']},
        {date:'2026-11-06',status:'no_meals',items:[]},
        {date:'2026-10-30',status:'published',items:['Archiwalny obiad']}
      ]},
      notices:['Karta zapisu jest wymagana. <script>alert(1)</script>'],
      payment:{verified:false,message:'Brak aktualnej kwoty.'}
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
    for width,height in [(320,740),(390,844),(1280,1000)]:
      for theme in ['light','dark']:
        page=browser.new_page(viewport={"width":width,"height":height},timezone_id='America/Los_Angeles')
        # Warsaw is already 4 November; browser timezone is deliberately different.
        page.clock.install(time=datetime(2026,11,3,23,30,tzinfo=timezone.utc))
        errors=[]
        page.on("pageerror",lambda error:errors.append(str(error)))
        page.set_content(html)
        page.add_style_tag(content=(ROOT/"style.css").read_text())
        page.add_style_tag(content=(ROOT/"canteen-sp3.css").read_text())
        page.evaluate("(theme)=>document.documentElement.setAttribute('data-theme',theme)",theme)
        page.evaluate(fake)
        page.add_script_tag(content=(ROOT/"canteen-sp3.js").read_text())
        page.add_script_tag(content=(ROOT/"app.js").read_text().replace(
            "window.location.origin", '"https://testing.invalid"'
        ))
        page.locator("#demo-btn").click()
        page.locator("#workspace:not(.hidden)").wait_for()
        page.evaluate("navigate('canteen-sp3')")
        expect(page.locator("#content h1")).to_have_text("Stołówka")
        expect(page.locator(".canteen-date")).to_have_count(7)
        expect(page.locator(".canteen-daily")).to_contain_text("Zupa testowa")
        expect(page.locator('.canteen-date[aria-current="date"]')).to_have_attribute('data-canteen-day','2026-11-04')
        page.get_by_role('button',name='Jutro',exact=True).click()
        expect(page.locator(".canteen-daily")).to_contain_text("Spaghetti testowe")
        expect(page.locator(".canteen-daily")).not_to_contain_text("Zupa testowa")
        expect(page.locator(".canteen-daily")).to_contain_text("<img src=x onerror=alert(1)>")
        page.locator('[data-canteen-day="2026-11-06"]').click()
        expect(page.locator(".canteen-daily")).to_contain_text("Dzień bez obiadów")
        page.locator('[data-canteen-day="2026-11-07"]').click()
        expect(page.locator(".canteen-daily")).to_contain_text("Brak opublikowanego jadłospisu")
        page.get_by_role('button',name='Miesiąc',exact=True).click()
        expect(page.locator(".canteen-date")).to_have_count(42)
        page.get_by_role('button',name='Poprzedni miesiąc',exact=True).click()
        expect(page.locator(".canteen-month-nav")).to_contain_text("październik")
        page.locator('[data-canteen-day="2026-10-30"]').click()
        expect(page.locator(".canteen-daily")).to_contain_text("Archiwalny obiad")
        expect(page.locator(".canteen-daily")).to_contain_text("Jadłospis archiwalny")
        page.get_by_role('button',name='Następny miesiąc',exact=True).click()
        page.get_by_role('button',name='Następny miesiąc',exact=True).click()
        expect(page.locator(".canteen-month-nav")).to_contain_text("grudzień")
        expect(page.locator(".canteen-daily")).to_contain_text("Brak opublikowanego jadłospisu")
        page.get_by_role('button',name='Dzisiaj',exact=True).click()
        expect(page.locator(".canteen-daily")).to_contain_text("Naleśniki testowe")
        expect(page.locator(".canteen-important")).to_contain_text("nie zostały potwierdzone")
        assert page.locator("#content script, #content img, #content a").count() == 0
        contrast=page.locator('.canteen-date.selected').evaluate('''el=>{
          const rgb=s=>s.match(/[\\d.]+/g).slice(0,3).map(Number).map(v=>{
            v/=255;return v<=.04045?v/12.92:((v+.055)/1.055)**2.4;});
          const lum=s=>{const [r,g,b]=rgb(s);return .2126*r+.7152*g+.0722*b;};
          const css=getComputedStyle(el),a=lum(css.color),b=lum(css.backgroundColor);
          return (Math.max(a,b)+.05)/(Math.min(a,b)+.05);
        }''')
        assert contrast>=4.5,(theme,contrast)
        assert page.evaluate("document.documentElement.scrollWidth<=innerWidth"),width
        # Every date stays inside the viewport, including a narrow iPhone.
        assert page.locator(".canteen-dates").bounding_box()['width']<=width
        page.screenshot(path=f"/tmp/canteen-{width}-{theme}.png",full_page=True)
        page.evaluate("window.__canteenBroken=true; navigate('canteen-sp3')")
        expect(page.locator(".canteen-warning")).to_contain_text("Nie udało się teraz odczytać")
        expect(page.locator(".canteen-daily")).to_contain_text("Brak opublikowanego jadłospisu")
        page.evaluate("navigate('grades')")
        expect(page.locator("#content h1")).to_have_text("Oceny")
        assert not errors, errors
        page.close()
    browser.close()
print("PASS calendar Chromium: 320/390/1280, light/dark, Warsaw date, week/month, meals, empty/archive/free days, safe text, isolation")

