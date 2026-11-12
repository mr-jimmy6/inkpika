(function () {
  'use strict';
  var model = window.inkpikaDemo, view = 'kual', trail = [], menuPage = 0;
  var frame = document.getElementById('native');
  var token = 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa';
  var names = {kual: 'InkPika', cloud: I18n.t("Cloud library"), 'wireless-device': I18n.t("Wireless transfer · connection page"), wireless: I18n.t("Wireless transfer · phone / computer"), 'settings-device': I18n.t("Settings · connection page"), settings: I18n.t("Settings · phone / computer")};
  var deviceTasks = {
    kual: I18n.t("Open KUAL on Kindle and select InkPika."),
    cloud: I18n.t("Browse books and downloads in the Kindle library window."),
    'wireless-device': I18n.t("View connection details on Kindle, then switch to a phone or computer to pair for transfer."),
    'settings-device': I18n.t("View saved configuration and test connectivity on Kindle. Open Phone / computer to pair and edit."),
    wireless: I18n.t("Pair, upload or export ebooks in a phone or computer browser."),
    settings: I18n.t("Edit and save configuration in a phone or computer browser.")
  };
  var tips = {
    kual: I18n.t("Select Start sync, then inspect books and status. You can also open the web interfaces or manage background services."),
    cloud: I18n.t("Browse folders and pages, download a book, and pause or resume in Downloads."),
    'wireless-device': I18n.t("This is the connection page shown on Kindle. Switch to a phone or computer and enter the sample code 123456."),
    wireless: I18n.t("Enter sample code 123456 first. After pairing, upload the built-in test books or search, select and export the sample library."),
    'settings-device': I18n.t("Review masked credentials and try Start test. Select Phone / computer for pairing with sample code 123456."),
    settings: I18n.t("Switch platforms, edit folders and formats, select Save and apply, then return to the cloud library. Use demo values only.")
  };
  function el(id) { return document.getElementById(id); }
  function status(message) { if (message) { el('status').textContent = message; } }
  function button(label, action) { var node = document.createElement('button'); node.type = 'button'; node.textContent = label; node.onclick = action; return node; }
  function updateServices() {
    el('services').innerHTML = '';
    ['cloud', 'wireless', 'settings'].forEach(function (key) {
      var li = document.createElement('li'), state = document.createElement('span');
      li.textContent = {cloud: I18n.t("Cloud library"), wireless: I18n.t("Wireless transfer"), settings: I18n.t("Settings")}[key];
      state.textContent = model.state.services[key] ? I18n.t("Running") : I18n.t("Stopped"); li.appendChild(state); el('services').appendChild(li);
    });
  }
  function act(action) {
    var parts = action.split('-');
    if (parts[0] === 'open') { openView(parts[1] === 'cloud' ? 'cloud' : parts[1] + '-device'); }
    else if (parts[0] === 'sync') { model.sync(parts[1]); }
    else { model.service(parts[1], parts[0] === 'start'); }
  }
  function renderMenu() {
    var focusedIndex = document.activeElement && document.activeElement.getAttribute('data-menu-index');
    var items = model.menu(), titles = ['InkPika'];
    for (var i = 0; i < trail.length; i += 1) {
      if (!items[trail[i]] || !items[trail[i]].items) { trail = trail.slice(0, i); break; }
      titles.push(items[trail[i]].name); items = items[trail[i]].items;
    }
    el('menu-path').textContent = titles.join(' / '); el('back').disabled = trail.length === 0;
    menuPage = Math.min(menuPage, Math.max(0, Math.ceil(items.length / 10) - 1));
    el('menu').innerHTML = '';
    items.slice(menuPage * 10, menuPage * 10 + 10).forEach(function (item, index) {
      var node;
      if (item.items || item.preview) {
        node = button(item.name + (item.items ? ' ▽' : ''), function () {
          if (item.items) {
            trail.push(menuPage * 10 + index); menuPage = 0; renderMenu(); el('back').focus();
          } else {
            var prior = index;
            act(item.preview);
            // Rendering replaces the clicked node; keep keyboard navigation inside the menu.
            var rows = el('menu').querySelectorAll('button');
            if (view === 'kual' && rows.length) { rows[Math.min(prior, rows.length - 1)].focus(); }
          }
        });
      } else { node = document.createElement('div'); node.textContent = item.name; }
      node.className = 'menu-row' + (item.items || item.preview ? '' : ' menu-info');
      node.setAttribute('data-menu-index', String(index)); el('menu').appendChild(node);
    });
    el('menu-pages').hidden = items.length <= 10;
    el('menu-page').textContent = (menuPage + 1) + ' / ' + Math.max(1, Math.ceil(items.length / 10));
    el('menu-prev').disabled = menuPage === 0; el('menu-next').disabled = (menuPage + 1) * 10 >= items.length;
    if (focusedIndex !== null) {
      var focused = el('menu').querySelector('button[data-menu-index="' + focusedIndex + '"]');
      if (focused) { focused.focus(); }
    }
  }
  function openView(name, autoPair, fromHash) {
    if (!Object.prototype.hasOwnProperty.call(names, name)) { name = 'kual'; }
    view = name;
    var onKindle = name !== 'wireless' && name !== 'settings';
    el('view-title').textContent = names[name]; el('tip').textContent = tips[name];
    el('device-location').textContent = onKindle ? I18n.t("You are using Kindle") : I18n.t("You are using a phone / computer");
    el('device-task').textContent = deviceTasks[name];
    el('device-frame').className = 'device-frame device-frame--' + (onKindle ? 'kindle' : 'companion');
    el('device-frame').setAttribute('aria-label', onKindle ? I18n.t("Kindle screen preview") : I18n.t("Phone / computer browser preview"));
    ['kindle', 'companion'].forEach(function (device) {
      var active = device === (onKindle ? 'kindle' : 'companion'), route = el('route-' + device);
      route.className = 'route-device' + (active ? ' is-current' : '');
      if (active) { route.setAttribute('aria-current', 'step'); } else { route.removeAttribute('aria-current'); }
    });
    el('view-context').textContent = name === 'kual' ? I18n.t("KUAL extension · all seven entries") : name.indexOf('-device') >= 0 || name === 'cloud' ? I18n.t("Kindle web interface · local sample") : I18n.t("Phone / computer web interface · local sample");
    el('kual-view').hidden = name !== 'kual'; el('web-view').hidden = name === 'kual'; el('web-controls').hidden = name === 'kual'; el('home').hidden = name === 'kual';
    var nav = document.querySelectorAll('[data-view]');
    for (var i = 0; i < nav.length; i += 1) {
      var active = nav[i].getAttribute('data-view').split('-')[0] === name.split('-')[0];
      if (active) { nav[i].setAttribute('aria-current', 'page'); } else { nav[i].removeAttribute('aria-current'); }
    }
    if (!fromHash && window.history.replaceState) { window.history.replaceState(null, '', '#' + name); }
    if (name === 'kual') { frame.src = 'about:blank'; renderMenu(); updateServices(); return; }
    var service = name.split('-')[0];
    model.service(service, true);
    var tools = el('web-tools'); tools.innerHTML = '';
    if (name.indexOf('-device') >= 0) {
      tools.appendChild(button(I18n.t("Switch to phone / computer: enter code"), function () { openView(service); }));
      tools.appendChild(button(I18n.t("Switch to phone / computer: simulate QR scan"), function () { openView(service, true); }));
      el('web-hint').textContent = I18n.t("Kindle connection page. Addresses, Wi-Fi and codes are samples; the QR code contains only demo information. Use the buttons above to continue pairing.");
    } else if (service !== 'cloud') {
      tools.appendChild(button(I18n.t("Back to Kindle: view connection page"), function () { openView(service + '-device'); }));
      if (service === 'wireless') {
        tools.appendChild(button(I18n.t("Choose three built-in test books"), function () {
          var doc = frame.contentDocument;
          if (!doc || !doc.getElementById('upload') || doc.getElementById('upload').hidden) { status(I18n.t("Pair first using sample code 123456.")); return; }
          if (frame.contentWindow.inkpikaSampleFiles()) {
            doc.getElementById('upload-tab').click(); status(I18n.t("Three public test books selected. Select Start upload to continue."));
          } else { status(I18n.t("Wait for the current upload to finish, or choose local test files using the file picker.")); }
        }));
      }
      el('web-hint').textContent = service === 'wireless' ? I18n.t("Sample code: 123456. Files stay in this browser; nothing is uploaded to a server or Kindle. Export lets you save real public test books.") : I18n.t("Sample code: 123456. Use demo values; configuration lasts only for this preview and is cleared on refresh or reset.");
    } else {
      tools.appendChild(button(I18n.t("Open settings"), function () { openView('settings-device'); }));
      el('web-hint').textContent = I18n.t("Try the cloud library and download tabs. Open Settings to review configuration and run a sample connection test.");
    }
    frame.title = (onKindle ? I18n.t("Kindle: ") : I18n.t("Phone / computer: ")) + names[name] + I18n.t(" web preview");
    frame.src = 'native/' + name + '.html?lang=' + model.language() + (name === 'cloud' || name.indexOf('-device') >= 0 ? '#token=' + token : autoPair ? '#code=123456' : '');
    updateServices();
  }
  model.onchange = function (message) { status(message); updateServices(); if (view === 'kual') { renderMenu(); } };
  el('back').onclick = function () { trail.pop(); menuPage = 0; renderMenu(); };
  el('menu-prev').onclick = function () { menuPage -= 1; renderMenu(); };
  el('menu-next').onclick = function () { menuPage += 1; renderMenu(); };
  el('home').onclick = function () { trail = []; menuPage = 0; openView('kual'); };
  el('reset').onclick = function () { model.reset(); trail = []; menuPage = 0; openView('kual'); };
  var nav = document.querySelectorAll('[data-view]');
  for (var i = 0; i < nav.length; i += 1) {
    nav[i].onclick = function () { if (this.getAttribute('data-view') === 'kual') { trail = []; menuPage = 0; } openView(this.getAttribute('data-view')); };
  }
  window.addEventListener('hashchange', function () { trail = []; menuPage = 0; openView(window.location.hash.slice(1), false, true); });
  openView(window.location.hash.slice(1) || 'kual', false, true);
}());
