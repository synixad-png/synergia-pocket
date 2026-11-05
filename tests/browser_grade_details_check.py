"""Real grade detail UI with fictional data, no requests to school accounts."""
from pathlib import Path
import re
from playwright.sync_api import sync_playwright,expect
ROOT=Path(__file__).resolve().parents[1]/'static'
html=(ROOT/'index.html').read_text()
html=re.sub(r'<script[^>]+src="[^"]+"[^>]*></script>','',html)
html=re.sub(r'<link[^>]+href="/static/[^"]+"[^>]*>','',html)
long=('Długi opis osiągnięć ucznia.\n'*100)+'KONIEC PEŁNEGO OPISU <img src=x onerror=alert(1)>'
with sync_playwright() as pw:
 b=pw.chromium.launch(headless=True)
 for width in [390,768,1440]:
  page=b.new_page(viewport={'width':width,'height':844})
  page.route('https://pocket.test/**',lambda route:route.fulfill(content_type='text/html',body=html))
  page.goto('https://pocket.test/')
  page.add_style_tag(content=(ROOT/'style.css').read_text())
  page.evaluate('''text=>{
   window.longGrade=text;
   window.fetch=async(path)=>{
    const user={authenticated:true,mode:'live',name:'Test',active_profile_id:'one',profiles:[{id:'one',name:'Test',mode:'live'}]};
    let data={};
    if(path==='/api/session')data=user;
    else if(String(path).startsWith('/api/section/grades'))data={count:3,subjects:[{name:'Język polski',grades:[{value:text,category:'Ocena opisowa',source:'descriptive',source_label:'Opisowe',date:'2026-11-05',semester:1,comments:['Długi komentarz\\nKONIEC KOMENTARZA'],requirements:['Wymaganie testowe']},{value:'5',category:'Sprawdzian',date:'2026-11-04',weight:2},{value:'7',source:'point',points:7,max_points:10}]}]};
    else if(String(path).startsWith('/api/section/attendance'))data={stats:{total:0}};
    else if(String(path).startsWith('/api/section/timetable'))data={days:[]};
    else data={items:[]};
    return new Response(JSON.stringify(data),{headers:{'Content-Type':'application/json'}});
   };
  }''',long)
  page.add_script_tag(content=(ROOT/'app.js').read_text())
  expect(page.locator('#workspace')).to_be_visible()
  if page.locator('#menu-btn').is_visible():page.locator('#menu-btn').click()
  page.locator('#main-nav [data-page="grades"]').click()
  expect(page.locator('button.grade-chip')).to_have_count(3)
  for theme in ['light','dark']:
   page.evaluate('(theme)=>document.documentElement.dataset.theme=theme',theme)
   trigger=page.locator('button.grade-chip').first
   trigger.click()
   expect(page.locator('#grade-dialog')).to_be_visible()
   assert page.locator('.grade-detail-value').inner_text()==long
   expect(page.locator('#grade-detail-body')).to_contain_text('KONIEC KOMENTARZA')
   expect(page.locator('#grade-detail-body')).to_contain_text('Wymaganie testowe')
   assert page.locator('#grade-detail-body img').count()==0
   assert page.evaluate('document.body.style.overflow')=='hidden'
   assert page.evaluate('document.documentElement.scrollWidth<=innerWidth')
   assert page.locator('.grade-dialog-box').evaluate('(n)=>n.scrollWidth<=n.clientWidth')
   page.locator('#grade-detail-bottom').click()
   expect(page.locator('#grade-dialog')).to_be_hidden()
   assert page.locator('#grade-detail-body').inner_text()==''
   assert trigger.evaluate('(n)=>n===document.activeElement')
  page.locator('button.grade-chip').nth(1).focus();page.keyboard.press('Enter')
  expect(page.locator('.grade-detail-value')).to_have_text('5')
  expect(page.locator('#grade-detail-body')).to_contain_text('Sprawdzian')
  expect(page.locator('#grade-detail-body')).to_contain_text('Waga')
  page.keyboard.press('Shift+Tab')
  assert page.locator('#grade-detail-bottom').evaluate('(n)=>n===document.activeElement')
  page.keyboard.press('Escape');expect(page.locator('#grade-dialog')).to_be_hidden()
  page.locator('button.grade-chip').nth(2).click();expect(page.locator('.grade-detail-value')).to_have_text('7 / 10 pkt')
  page.locator('#grade-detail-close').click();expect(page.locator('#grade-dialog')).to_be_hidden()
  page.close();print('PASS',width,'px: full descriptive text, comments, requirements, numeric/point grades, return, keyboard, no overflow/XSS',flush=True)
 b.close()
