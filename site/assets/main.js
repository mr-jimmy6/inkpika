/* Progressive enhancement only: the complete guide is readable without JS. */
(function () {
  "use strict";

  var tabs = document.querySelectorAll("[data-guide-tab]");
  var panels = document.querySelectorAll("[data-guide-panel]");
  var links = document.querySelectorAll("[data-guide-link]");
  var tabList = document.querySelector(".guide-tabs");
  var i;

  function activateGuide(name, moveFocus) {
    var selected = document.getElementById("tab-" + name);
    if (!selected) { return; }
    for (var n = 0; n < tabs.length; n += 1) {
      var active = tabs[n] === selected;
      tabs[n].setAttribute("aria-selected", active ? "true" : "false");
      tabs[n].setAttribute("tabindex", active ? "0" : "-1");
      panels[n].hidden = !active;
    }
    if (moveFocus) { selected.focus(); }
  }

  function guideFromHash() {
    var match = /^#guide-(wireless|cloud|sync)$/.exec(window.location.hash);
    if (match) { activateGuide(match[1], false); }
    return !!match;
  }

  function selectTab(event) {
    event.preventDefault();
    activateGuide(this.getAttribute("data-guide-tab"), false);
    // Keep reload/back links meaningful without moving the page on every tab click.
    if (window.history && window.history.replaceState) {
      window.history.replaceState(null, "", this.getAttribute("href"));
    }
  }

  function navigateTabs(event) {
    var key = event.key || event.keyCode;
    var index = 0;
    var target;
    for (var n = 0; n < tabs.length; n += 1) {
      if (tabs[n] === this) { index = n; }
    }
    if (key === "ArrowRight" || key === 39) { target = (index + 1) % tabs.length; }
    if (key === "ArrowLeft" || key === 37) { target = (index + tabs.length - 1) % tabs.length; }
    if (key === "Home" || key === 36) { target = 0; }
    if (key === "End" || key === 35) { target = tabs.length - 1; }
    if (target === undefined) { return; }
    event.preventDefault();
    tabs[target].focus();
    tabs[target].click();
  }

  tabList.setAttribute("role", "tablist");
  for (i = 0; i < tabs.length; i += 1) {
    tabs[i].setAttribute("role", "tab");
    tabs[i].setAttribute("aria-controls", panels[i].id);
    panels[i].setAttribute("role", "tabpanel");
    panels[i].setAttribute("tabindex", "0");
    tabs[i].addEventListener("click", selectTab);
    tabs[i].addEventListener("keydown", navigateTabs);
  }
  if (!guideFromHash()) { activateGuide("wireless", false); }
  window.addEventListener("hashchange", guideFromHash);
  for (i = 0; i < links.length; i += 1) {
    links[i].addEventListener("click", function () {
      activateGuide(this.getAttribute("data-guide-link"), false);
    });
  }

  var copyButton = document.getElementById("copy-agent-prompt");
  var promptText = document.getElementById("agent-install-prompt");
  var copyStatus = document.getElementById("agent-copy-status");

  function copiedPrompt() {
    copyStatus.textContent = I18n.t("Copied. Paste it into your agent.");
  }

  function copyWithSelection() {
    promptText.focus();
    promptText.select();
    promptText.setSelectionRange(0, promptText.value.length);
    var copied = false;
    try { copied = document.execCommand("copy"); } catch (error) { /* Keep text selected for manual copy. */ }
    if (copied) {
      copiedPrompt();
      copyButton.focus();
    } else {
      copyStatus.textContent = I18n.t("Prompt selected. Use the copy shortcut or long-press to copy.");
    }
  }

  if (copyButton && promptText && copyStatus) {
    copyButton.hidden = false;
    copyButton.addEventListener("click", function () {
      if (navigator.clipboard && navigator.clipboard.writeText) {
        try {
          navigator.clipboard.writeText(promptText.value).then(copiedPrompt, copyWithSelection);
        } catch (error) { copyWithSelection(); }
      } else {
        copyWithSelection();
      }
    });
  }

}());
