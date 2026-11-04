/* Synergia Pocket — linia aktualnego czasu na trwającej lekcji.
   Zegar szkoły: Europe/Warsaw, niezależnie od strefy telefonu. */
(() => {
  'use strict';
  const zoneFormatter = new Intl.DateTimeFormat('en-GB', {
    timeZone: 'Europe/Warsaw', year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit', second: '2-digit', hourCycle: 'h23'
  });

  function schoolClock(instant) {
    const parts = Object.fromEntries(zoneFormatter.formatToParts(instant)
      .filter(p => p.type !== 'literal').map(p => [p.type, p.value]));
    return {
      date: `${parts.year}-${parts.month}-${parts.day}`,
      minuteOfDay: Number(parts.hour) * 60 + Number(parts.minute) + Number(parts.second) / 60,
      time: `${parts.hour}:${parts.minute}`
    };
  }

  function minutes(value) {
    const match = /^(\d{1,2}):(\d{2})$/.exec(String(value ?? '').trim());
    if (!match) return null;
    const hour = Number(match[1]);
    const minute = Number(match[2]);
    return hour >= 0 && hour <= 23 && minute >= 0 && minute <= 59 ? hour * 60 + minute : null;
  }

  function progressFor(date, start, end, canceled, instant = new Date()) {
    if (canceled === true || canceled === '1' || canceled === 1) return null;
    if (!/^\d{4}-\d{2}-\d{2}$/.test(String(date ?? ''))) return null;
    const startMinute = minutes(start);
    const endMinute = minutes(end);
    if (startMinute === null || endMinute === null || endMinute <= startMinute) return null;
    const clock = schoolClock(instant);
    if (date !== clock.date) return null;
    if (clock.minuteOfDay < startMinute || clock.minuteOfDay >= endMinute) return null;
    const duration = endMinute - startMinute;
    const elapsed = clock.minuteOfDay - startMinute;
    return {
      percent: Math.max(0, Math.min(100, elapsed / duration * 100)),
      elapsed: Math.floor(elapsed), duration,
      time: clock.time
    };
  }

  function update(instant = new Date()) {
    for (const card of document.querySelectorAll('.lesson-card[data-lesson-date]')) {
      const info = card.querySelector('.lesson-live');
      if (!info) continue;
      const progress = progressFor(
        card.dataset.lessonDate, card.dataset.lessonStart,
        card.dataset.lessonEnd, card.dataset.lessonCanceled, instant
      );
      card.classList.toggle('lesson-ongoing', Boolean(progress));
      info.classList.toggle('hidden', !progress);
      if (!progress) continue;
      const pct = progress.percent.toFixed(2) + '%';
      info.querySelector('.lesson-live-time').textContent =
        `${progress.time} · ${progress.elapsed}/${progress.duration} min`;
      info.querySelector('.lesson-live-fill').style.width = pct;
      info.querySelector('.lesson-live-marker').style.left = pct;
      const bar = info.querySelector('[role="progressbar"]');
      bar.setAttribute('aria-valuenow', String(Math.round(progress.percent)));
      bar.setAttribute('aria-valuetext',
        `Godzina ${progress.time}, minęło ${progress.elapsed} z ${progress.duration} minut lekcji`);
    }
  }

  window.SynergiaLessonClock = { progressFor, update };
  setInterval(update, 15000);
  document.addEventListener('visibilitychange', () => { if (!document.hidden) update(); });
  window.addEventListener('focus', () => update());
})();
