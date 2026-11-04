(function timetableExtras() {
  'use strict';
  const formatter = new Intl.DateTimeFormat('en-GB', {
    timeZone:'Europe/Warsaw',year:'numeric',month:'2-digit',day:'2-digit',
    hour:'2-digit',minute:'2-digit',second:'2-digit',hourCycle:'h23'
  });
  function schoolNow(instant) {
    const parts=Object.fromEntries(formatter.formatToParts(instant).filter(x=>x.type!=='literal').map(x=>[x.type,x.value]));
    return {date:`${parts.year}-${parts.month}-${parts.day}`,minute:+parts.hour*60+(+parts.minute)+(+parts.second)/60};
  }
  function hm(value) {
    const match=/^(\d{1,2}):(\d{2})$/.exec(String(value||''));
    if(!match)return null;
    const h=+match[1],m=+match[2];
    return h>=0&&h<=23&&m>=0&&m<=59?h*60+m:null;
  }
  function finishedByClock(day,start,end,canceled,instant=new Date()) {
    if(canceled===true||canceled===1||canceled==='1')return false;
    if(!/^\d{4}-\d{2}-\d{2}$/.test(String(day||'')))return false;
    const from=hm(start),to=hm(end);
    if(from===null||to===null||to<=from)return false;
    const now=schoolNow(instant);
    return day<now.date || (day===now.date && now.minute>=to);
  }
  function add(tag,cls,text) {
    const node=document.createElement(tag);
    if(cls)node.className=cls;
    if(text!==undefined)node.textContent=text;
    return node;
  }
  function subjectKey(v) {
    return String(v||'').trim().normalize('NFKC').toLocaleLowerCase('pl-PL').replace(/\s+/g,' ');
  }
  function gradeClass(v) {
    const n=parseInt(v,10);
    return n<=2?'low':n===3?'mid':n>=5?'good':'';
  }
  function updateFinished(instant=new Date()) {
    if(!document.querySelectorAll)return;
    for(const card of document.querySelectorAll('.lesson-card[data-lesson-date]')) {
      const finished=finishedByClock(card.dataset.lessonDate,card.dataset.lessonStart,card.dataset.lessonEnd,card.dataset.lessonCanceled,instant);
      card.classList.toggle('lesson-completed',finished);
      let note=card.querySelector('.lesson-finished');
      if(finished&&!note) {
        note=add('div','lesson-finished','✓ Zakończona według planu');
        const live=card.querySelector('.lesson-live');
        if(live)card.insertBefore(note,live);else card.appendChild(note);
      }
      if(note)note.classList.toggle('hidden',!finished);
    }
  }
  function enrichChanges(card,lesson) {
    const change=Boolean(lesson.substitution||lesson.room_changed||lesson.note);
    if(!change)return;
    const panel=add('div','lesson-change');
    panel.appendChild(add('strong','',lesson.substitution?'↔ Zastępstwo':lesson.room_changed?'↔ Zmiana sali':'Informacja o lekcji'));
    const differences=[
      ['Przedmiot',lesson.original_subject,lesson.subject],
      ['Nauczyciel',lesson.original_teacher,lesson.teacher],
      ['Sala',lesson.original_room,lesson.room]
    ];
    let count=0;
    for(const [name,before,after] of differences) {
      if(before&&after&&String(before)!==String(after)) {
        panel.appendChild(add('div','',`${name}: ${before} → ${after}`));count++;
      }
    }
    if(lesson.note) { panel.appendChild(add('div','',String(lesson.note))); count++; }
    if(!count)panel.appendChild(add('div','','Zmiana oznaczona w Synergii. Aktualne dane widoczne powyżej.'));
    // Własny panel zastępuje tylko powielone powiadomienia zastępstwa.
    for(const node of card.querySelectorAll('.lesson-note')) {
      if(lesson.substitution&&node.textContent.startsWith('Zastępstwo'))node.remove();
      else if(lesson.note&&node.textContent===String(lesson.note))node.remove();
    }
    card.appendChild(panel);
  }
  function gradePill(grade, clickable=true) {
    const pill=add(clickable?'button':'span','lesson-grade-pill '+gradeClass(grade.value));
    if(clickable){pill.type='button';pill.dataset.page='grades';pill.title='Zobacz wszystkie oceny';}
    pill.appendChild(add('strong','',grade.value));
    if(clickable)pill.appendChild(add('span','',grade.category||'Ocena'));
    return pill;
  }
  function enrichGrades(column,lessonNodes,lessons,gradesForDay) {
    const used=new Set();
    for(let i=0;i<lessonNodes.length;i++) {
      const card=lessonNodes[i],lesson=lessons[i];
      if(!lesson)continue;
      const found=gradesForDay.filter(g=>!used.has(g.id)&&subjectKey(g.subject)===subjectKey(lesson.subject));
      if(!found.length)continue;
      found.forEach(g=>used.add(g.id));
      const box=add('div','lesson-grades');
      box.appendChild(add('strong','lesson-grades-heading','Oceny wpisane tego dnia'));
      found.forEach(g=>box.appendChild(gradePill(g)));
      const live=card.querySelector('.lesson-live');
      if(live)card.insertBefore(box,live);else card.appendChild(box);
    }
    const remaining=gradesForDay.filter(g=>!used.has(g.id));
    if(!remaining.length)return;
    const box=add('div','lesson-day-grades');
    box.appendChild(add('strong','','Oceny wpisane tego dnia'));
    for(const grade of remaining) {
      const row=add('div','lesson-day-grade-row');
      row.appendChild(gradePill(grade,false));
      row.appendChild(add('span','',`${grade.subject} · ${grade.category}`));
      box.appendChild(row);
    }
    box.appendChild(add('small','','Nie przypisano ich do konkretnej godziny lekcji.'));
    column.appendChild(box);
  }
  function enhance(plan,gradeData) {
    if(!plan||!Array.isArray(plan.days))return;
    const columns=[...document.querySelectorAll('.week-grid > .day-col')];
    const gradesByDate=new Map();
    let id=0;
    for(const subject of gradeData?.subjects||[])for(const grade of subject.grades||[]) {
      const day=String(grade.date||'').slice(0,10);
      if(!/^\d{4}-\d{2}-\d{2}$/.test(day))continue;
      if(!gradesByDate.has(day))gradesByDate.set(day,[]);
      gradesByDate.get(day).push({id:id++,subject:String(subject.name||''),value:String(grade.value??''),
        category:String(grade.category||'Ocena')});
    }
    for(let i=0;i<columns.length;i++){
      const day=plan.days[i];
      if(!day)continue;
      const col=columns[i],lessons=day.lessons||[];
      const cards=[...col.querySelectorAll('.lesson-card')];
      let substitutions=0;
      for(let j=0;j<cards.length;j++){
        const lesson=lessons[j];
        if(!lesson)continue;
        if(lesson.substitution)substitutions++;
        enrichChanges(cards[j],lesson);
      }
      if(substitutions) {
        const note=add('div','lesson-day-notice',`↔ Zastępstwa tego dnia: ${substitutions}`);
        const header=col.querySelector('.day-header');
        if(header)header.insertAdjacentElement('afterend',note);else col.prepend(note);
      }
      enrichGrades(col,cards,lessons,gradesByDate.get(day.date)||[]);
      const tab=document.querySelector(`.day-tab[data-day-index="${i}"]`);
      const total=(gradesByDate.get(day.date)||[]).length;
      if(tab&&total){
        const dot=add('span','day-grade-dot','●');
        dot.title=`Oceny wpisane tego dnia: ${total}`;
        dot.setAttribute('aria-label',dot.title);
        tab.appendChild(dot);
      }
    }
    if(gradeData===null){
      const bar=add('div','lesson-grades-warning','Oceny są chwilowo niedostępne. Plan i zastępstwa wyświetlają się niezależnie.');
      document.querySelector('.timetable-wrap')?.insertAdjacentElement('beforebegin',bar);
    }
    updateFinished();
  }
  window.SynergiaTimetable={enhance,finishedByClock,updateFinished};
  setInterval(updateFinished,15000);
  document.addEventListener('visibilitychange',()=>{if(!document.hidden)updateFinished();});
  window.addEventListener('focus',()=>updateFinished());
})();
