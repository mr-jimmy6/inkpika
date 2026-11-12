/* Only deliberate site language links affect the public page language. */
(function () {
  'use strict';
  var language = I18n.language(), links = document.querySelectorAll('a'), i;
  for (i = 0; i < links.length; i += 1) {
    var link = links[i], chosen = link.getAttribute('data-language'), href = link.getAttribute('href') || '';
    if (chosen) {
      link.href = '?lang=' + chosen + window.location.hash;
      if (chosen === language) { link.setAttribute('aria-current', 'true'); }
    } else if (href && href.charAt(0) !== '#' && !/^(?:[a-z]+:|\/\/)/i.test(href) && /(?:\.html|\/$)/.test(href.split('#')[0])) {
      var parts = href.split('#');
      link.href = parts[0].split('?')[0] + '?lang=' + language + (parts[1] ? '#' + parts[1] : '');
    }
  }
  if (language === 'zh') {
    for (i = 0; i < links.length; i += 1) {
      var readme = links[i].getAttribute('href') || '';
      if (readme.indexOf('/README.md') >= 0) { links[i].href = readme.replace('/README.md', '/README_ZH.md').replace('#best-practices', '#最佳实践').replace('#connect-to-object-storage', '#连接对象存储').replace('#troubleshooting', '#故障排查'); }
    }
  }
  var preview = document.querySelector('[data-preview-image]');
  if (preview) { preview.src = language === 'zh' ? 'assets/previews/cloud-library.svg' : 'assets/previews/cloud-library-en.svg'; }
  var locale = document.querySelector('meta[property="og:locale"]');
  if (locale) { locale.content = language === 'zh' ? 'zh_CN' : 'en_US'; }
  var descriptions = document.querySelectorAll('meta[name="description"], meta[property="og:title"], meta[property="og:description"]');
  for (i = 0; i < descriptions.length; i += 1) { descriptions[i].content = I18n.t(descriptions[i].content); }
}());
