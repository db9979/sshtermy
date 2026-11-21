// Kopier-Knöpfe an den Befehlen.
document.querySelectorAll('.cmd').forEach(function (box) {
  var b = document.createElement('button');
  b.className = 'copy'; b.type = 'button'; b.textContent = box.dataset.copy || 'Copy';
  b.addEventListener('click', function () {
    navigator.clipboard.writeText(box.querySelector('pre').innerText.trim()).then(function () {
      var t = b.textContent; b.textContent = '✓'; setTimeout(function () { b.textContent = t; }, 1200);
    });
  });
  box.appendChild(b);
});

// Installation im Kopf: Reiter je System, das eigene System ist vorgewählt.
document.querySelectorAll('.itabs').forEach(function (box) {
  var tabs = box.querySelectorAll('.ibar button');
  function show(os) {
    tabs.forEach(function (t) { t.classList.toggle('on', t.dataset.os === os); t.setAttribute('aria-selected', t.dataset.os === os); });
    box.querySelectorAll('.cmd').forEach(function (c) { c.hidden = c.dataset.os !== os; });
  }
  tabs.forEach(function (t) { t.addEventListener('click', function () { show(t.dataset.os); }); });
  var ua = navigator.userAgent;
  show(/Windows/i.test(ua) ? 'win' : /Mac/i.test(ua) ? 'mac' : /Linux|X11/i.test(ua) ? 'linux' : 'win');
});

// Rundgang: Reiter links schalten Bild und Text rechts um.
document.querySelectorAll('.tour').forEach(function (tour) {
  var btns = tour.querySelectorAll('ol button');
  btns.forEach(function (b) {
    b.addEventListener('click', function () {
      btns.forEach(function (x) { x.classList.toggle('on', x === b); });
      tour.querySelectorAll('.stage figure').forEach(function (f) { f.hidden = f.dataset.i !== b.dataset.i; });
    });
  });
});

// „Mehr“ und Links auf #all-… klappen die vollständige Liste auf.
function openFor(hash) {
  var t = hash && document.getElementById(hash.slice(1));
  var d = t && t.closest('details');
  if (d) { d.open = true; t.scrollIntoView(); }
}
window.addEventListener('hashchange', function () { openFor(location.hash); });
openFor(location.hash);
