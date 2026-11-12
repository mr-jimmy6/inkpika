(function () {
  'use strict';
  var match = /(?:^#|&)token=([a-f0-9]{64})(?:&|$)/.exec(window.location.hash), endpoints = [];
  function el(id) { return document.getElementById(id); }
  function show() {
    var entry = endpoints[Number(el('network').value) || 0];
    if (!entry) { return; }
    el('ip').textContent = entry.ip; el('endpoint').textContent = entry.url; el('qr').src = entry.qr;
  }
  function refresh() {
    if (!match) { el('message').textContent = I18n.t("Open this page from KUAL."); return; }
    el('refresh').disabled = true;
    var xhr = new XMLHttpRequest(); xhr.open('GET', '/api/connection', true); xhr.timeout = 15000;
    xhr.setRequestHeader('X-Transfer-Token', match[1]);
    function failed() { el('refresh').disabled = false; el('connection').hidden = true; el('message').textContent = I18n.t("Service disconnected; reopen this page from KUAL."); }
    xhr.onreadystatechange = function () {
      if (xhr.readyState !== 4) { return; }
      if (xhr.status !== 200) { failed(); return; }
      var result; try { result = JSON.parse(xhr.responseText); } catch (ignore) { failed(); return; }
      el('refresh').disabled = false; endpoints = result.endpoints || [];
      el('title').textContent = result.title; document.title = result.title + " · InkPika"; el('qr').alt = result.title + I18n.t("QR code");
      el('description').textContent = result.description; el('stop-label').textContent = result.stop_label;
      el('wifi').textContent = result.wifi || I18n.t("Not connected or unavailable");
      el('connection').hidden = !endpoints.length;
      el('message').textContent = endpoints.length ? I18n.t("Service started; waiting for a phone or computer.") : I18n.t("No LAN IP detected; connect to Wi-Fi and refresh.");
      el('network').innerHTML = '';
      for (var i = 0; i < endpoints.length; i++) { var option = document.createElement('option'); option.value = String(i); option.textContent = endpoints[i].ip; el('network').appendChild(option); }
      el('network-choice').hidden = endpoints.length < 2;
      el('code').textContent = result.code; show();
    };
    xhr.onerror = failed; xhr.ontimeout = failed; xhr.send(null);
  }
  el('refresh').onclick = refresh; el('network').onchange = show; refresh();
}());
