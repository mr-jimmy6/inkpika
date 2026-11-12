/* ES5 localization shared by Kindle pages and the static website preview. */
(function (root) {
  'use strict';
  var catalog = root.InkPikaMessages || {}, reverse = {}, staticCatalog = {}, staticReverse = {}, key;
  function trim(value) { return value.replace(/^\s+|\s+$/g, ''); }
  for (key in catalog) { if (Object.prototype.hasOwnProperty.call(catalog, key)) { reverse[catalog[key]] = key;
    if (trim(key)) { staticCatalog[trim(key)] = trim(catalog[key]); staticReverse[trim(catalog[key])] = trim(key); } } }
  var match = /(?:^|[?&])lang=(en|zh)(?:&|$)/.exec(root.location ? root.location.search : '');
  var initialized = false;
  var language = root.InkPikaLocale || (match ? match[1] : 'en');
  function translate(value, selected) {
    if (selected === 'zh') { return Object.prototype.hasOwnProperty.call(catalog, value) ? catalog[value] : value; }
    return Object.prototype.hasOwnProperty.call(reverse, value) ? reverse[value] : value;
  }
  function text(value) { return translate(value, language); }
  function staticText(value) {
    var canonical = Object.prototype.hasOwnProperty.call(staticReverse, value) ? staticReverse[value] : value;
    return language === 'zh' && Object.prototype.hasOwnProperty.call(staticCatalog, canonical) ? staticCatalog[canonical] : canonical;
  }
  function localize(node) {
    if (!node) { return; }
    if (node.nodeType === 3) {
      if (!node._i18nSource) {
        if (initialized) { return; }
        node._i18nSource = node.nodeValue;
      }
      if (node._i18nLast && node.nodeValue !== node._i18nLast) { return; }
      var source = node._i18nSource, trimmed = source.replace(/^\s+|\s+$/g, '');
      if (trimmed) { node.nodeValue = source.replace(trimmed, staticText(trimmed)); node._i18nLast = node.nodeValue; }
      return;
    }
    if (node.nodeType === 1) {
      if (/^(SCRIPT|STYLE|CODE|PRE)$/.test(node.tagName) || node.getAttribute('data-i18n-skip') !== null) { return; }
      ['title', 'aria-label', 'placeholder', 'alt'].forEach(function (attr) {
        var source = node.getAttribute('data-i18n-' + attr);
        if (!source && !initialized) { source = node.getAttribute(attr); }
        if (source && reverse[source]) { source = reverse[source]; }
        if (source) { node.setAttribute('data-i18n-' + attr, source); node.setAttribute(attr, text(source)); }
      });
    }
    for (var child = node.firstChild; child; child = child.nextSibling) { localize(child); }
  }
  function setLanguage(value) {
    language = value === 'zh' ? 'zh' : 'en';
    if (root.document) { root.document.documentElement.lang = language === 'zh' ? 'zh-CN' : 'en'; localize(root.document); initialized = true; }
  }
  root.I18n = {t: text, forLanguage: function (selected) { return {t: function (value) { return translate(value, selected); }}; }, setLanguage: setLanguage, language: function () { return language; }, localize: localize};
  if (root.document) {
    // Scripts are placed after static markup and before application rendering.
    setLanguage(language);
  }
}(typeof window !== 'undefined' ? window : this));
