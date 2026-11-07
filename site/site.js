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
