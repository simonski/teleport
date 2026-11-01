"use strict";
// CSS-rendered controls retain native form values without opening OS pickers.
const modalStack = [];
let openChoice = null;
function dismissChoice(restore = false) {
  if (!openChoice) return;
  const {button, list} = openChoice;
  list.remove(); button.setAttribute('aria-expanded', 'false'); openChoice = null;
  if (restore && button.isConnected) button.focus();
}
function modalFocusables(dialog) {
  return [...dialog.querySelectorAll('button,input,textarea,select,a[href],[tabindex]')].filter(e => !e.matches(':disabled') && !e.hidden && !e.closest('[inert]') && e.tabIndex >= 0 && e.getClientRects().length);
}
class TeleportDialog extends HTMLElement {
  constructor() {
    super();
    // Let target controls handle keys before containing application shortcuts.
    for (const type of ['keydown', 'keyup', 'keypress']) this.addEventListener(type, e => e.stopPropagation());
  }
  get open() { return this.hasAttribute('open'); }
  showModal() {
    const heading = this.querySelector('h2,h1');
    if (heading) { heading.id ||= `modal-title-${crypto.randomUUID()}`; this.setAttribute('aria-labelledby', heading.id); }
    if (this.open) {
      enhanceControls();
      if (!this.contains(document.activeElement)) (modalFocusables(this)[0] || this).focus();
      return;
    }
    dismissChoice();
    this.opener = document.activeElement;
    this.setAttribute('role', 'dialog'); this.setAttribute('aria-modal', 'true');
    this.tabIndex = -1;
    this.shade = document.createElement('div'); this.shade.className = 'modal-shade';
    // Track both ends: dragging from an editor into the backdrop must not dismiss.
    let outside = false;
    this.shade.addEventListener('pointerdown', e => outside = e.target === this.shade && e.button === 0);
    this.shade.addEventListener('pointerup', e => { if (outside && e.target === this.shade && e.button === 0) this.close(); outside = false; });
    this.shade.addEventListener('pointercancel', () => outside = false);
    this.before = document.createComment('dialog'); this.beforeParent = this.parentNode;
    this.beforeParent.insertBefore(this.before, this); document.body.append(this.shade); this.shade.append(this);
    modalStack.push(this); this.setAttribute('open', ''); syncModalInert();
    if (typeof resetShift === 'function') resetShift();
    (modalFocusables(this)[0] || this).focus();
  }
  close(value = '') {
    if (!this.open) return;
    dismissChoice(); this.returnValue = value; this.removeAttribute('open');
    const index = modalStack.indexOf(this); if (index >= 0) modalStack.splice(index, 1);
    if (this.before?.isConnected) this.before.replaceWith(this); else this.remove();
    this.shade?.remove(); this.shade = null; syncModalInert();
    this.dispatchEvent(new Event('close'));
    const target = this.opener;
    if (target?.isConnected && !target.closest('[inert]')) target.focus();
  }
}
customElements.define('tk-dialog', TeleportDialog);
function dismissOverlays() { dismissChoice(); for (const modal of [...modalStack].reverse()) modal.close(); }
function syncModalInert() {
  const top = modalStack.at(-1);
  for (const child of document.body.children) {
    if (top) {
      if (!child.hasAttribute('data-modal-inert')) child.dataset.modalInert = child.inert ? 'keep' : 'restore';
      child.inert = child !== top.shade;
    } else if (child.hasAttribute('data-modal-inert')) {
      child.inert = child.dataset.modalInert === 'keep'; delete child.dataset.modalInert;
    }
  }
}
function enhanceSelect(select) {
  if (select.dataset.customChoice) return;
  select.dataset.customChoice = 'true'; select.classList.add('choice-source'); select.tabIndex = -1; select.setAttribute('aria-hidden', 'true');
  const button = document.createElement('button'); button.type = 'button'; button.className = 'choice-button secondary';
  button.setAttribute('role', 'combobox'); button.setAttribute('aria-haspopup', 'listbox'); button.setAttribute('aria-expanded', 'false');
  select.insertAdjacentElement('afterend', button);
  const labels = [...(select.labels || [])];
  const name = select.getAttribute('aria-label') || labels.map(label => [...label.childNodes].filter(n => n !== select && n !== button && !n.contains?.(select)).map(n => n.textContent).join(' ').trim()).join(' ') || select.name || select.id || 'Choose';
  button.setAttribute('aria-label', name);
  function sync() { button.textContent = select.selectedOptions[0]?.textContent || 'Choose'; button.disabled = select.disabled || !!select.closest('fieldset:disabled'); button.setAttribute('aria-required', String(select.required)); }
  const descriptor = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value');
  Object.defineProperty(select, 'value', {get() {return descriptor.get.call(this);}, set(value) {descriptor.set.call(this, value); sync();}, configurable:true});
  select.addEventListener('change', sync); select.addEventListener('invalid', () => { button.focus(); button.setAttribute('aria-invalid', 'true'); });
  for (const label of labels) label.addEventListener('click', e => { if (e.target === label) {e.preventDefault();button.focus();} });
  button.addEventListener('click', () => {
    if (openChoice?.button === button) {dismissChoice(true);return;}
    dismissChoice(); sync(); if (button.disabled) return;
    const list = document.createElement('div'); list.className = 'choice-list'; list.id = `choice-${crypto.randomUUID()}`; list.setAttribute('role', 'listbox'); list.setAttribute('aria-label', name);
    button.setAttribute('aria-controls', list.id); button.setAttribute('aria-expanded', 'true');
    // A modal owns its popup; never place it behind the modal's inert boundary.
    (select.closest('tk-dialog[open]') || document.body).append(list);
    const rect = button.getBoundingClientRect(), width = Math.min(Math.max(rect.width, 160), innerWidth - 16);
    list.style.width = `${width}px`; list.style.left = `${Math.max(8, Math.min(rect.left, innerWidth - width - 8))}px`;
    let active = Math.max(0, select.selectedIndex);
    const options = [...select.options];
    const groups=new Map();
    options.forEach((option, index) => {
      if (option.hidden) return;
      const row = document.createElement('button'); row.type = 'button'; row.setAttribute('role', 'option'); row.dataset.value = option.value;
      row.textContent = option.textContent; row.disabled = option.disabled || !!option.closest('optgroup:disabled'); row.setAttribute('aria-selected', String(option.selected)); row.tabIndex = -1;
      row.onclick = () => {select.value = option.value; select.dispatchEvent(new Event('input', {bubbles:true}));select.dispatchEvent(new Event('change', {bubbles:true}));dismissChoice(true);};
      const group=option.parentElement;
      if(group?.tagName==='OPTGROUP'){
        if(!groups.has(group)){const container=document.createElement('div');container.className='choice-group';container.setAttribute('role','group');container.setAttribute('aria-label',group.label);const title=document.createElement('div');title.className='choice-group-title';title.textContent=group.label;title.setAttribute('aria-hidden','true');container.append(title);list.append(container);groups.set(group,container);}
        groups.get(group).append(row);
      }else list.append(row);
    });
    const available = [...list.querySelectorAll('button:not(:disabled)')];
    const selected = list.querySelector('[aria-selected="true"]'); active = Math.max(0, available.indexOf(selected));
    const maxHeight = Math.min(280, innerHeight - 24); list.style.maxHeight = `${maxHeight}px`;
    const height = Math.min(list.scrollHeight, maxHeight);
    list.style.top = `${rect.bottom + height + 8 < innerHeight ? rect.bottom + 4 : Math.max(8, rect.top - height - 4)}px`;
    function navigateChoice(key) {
      if (!available.length) return;
      active = key === 'Home' ? 0 : key === 'End' ? available.length - 1 : (active + (key === 'ArrowDown' ? 1 : -1) + available.length) % available.length;
      available[active].focus();
    }
    openChoice = {button, list, navigate:navigateChoice}; available[active]?.focus();
    let search = '', lastKey = 0;
    list.onkeydown = e => {
      if (e.key === 'Escape') {e.preventDefault();e.stopPropagation();dismissChoice(true);return;}
      if (e.key === 'Tab') {dismissChoice(true);return;}
      if (['ArrowDown','ArrowUp','Home','End'].includes(e.key)) {e.preventDefault();navigateChoice(e.key);}
      else if (e.key.length === 1 && !e.ctrlKey && !e.metaKey && e.key !== ' ') {e.preventDefault();search = performance.now() - lastKey < 700 ? search + e.key : e.key;lastKey = performance.now();const found = available.findIndex(o => o.textContent.toLowerCase().startsWith(search.toLowerCase()));if (found >= 0) {active = found;available[active].focus();}}
    };
  });
  button.addEventListener('keydown', e => {
    if (!['ArrowDown','ArrowUp','Home','End'].includes(e.key)) return;
    e.preventDefault();
    if (openChoice?.button === button) openChoice.navigate(e.key);
    else {button.click();if (e.key === 'Home' || e.key === 'End') openChoice?.navigate(e.key);}
  });
  new MutationObserver(sync).observe(select, {attributes:true,childList:true,subtree:true});
  select.addEventListener('focus', () => button.focus()); sync();
}
function enhanceControls() {
  document.querySelectorAll('select:not([data-custom-choice])').forEach(enhanceSelect);
  document.querySelectorAll('input[type=date]').forEach(input => {input.type='text';input.inputMode='numeric';input.pattern='[0-9]{4}-[0-9]{2}-[0-9]{2}';input.placeholder='YYYY-MM-DD';});
  if (openChoice && !openChoice.button.isConnected) dismissChoice();
}
new MutationObserver(enhanceControls).observe(document.documentElement, {childList:true,subtree:true});
document.addEventListener('submit', e => { if (e.target.closest('tk-dialog') && e.target.getAttribute('method') === 'dialog') e.preventDefault(); }, true);
document.addEventListener('pointerdown', e => {if (openChoice && !openChoice.list.contains(e.target) && !openChoice.button.contains(e.target)) dismissChoice();}, true);
window.addEventListener('resize', () => dismissChoice());
window.addEventListener('scroll', e => {if (openChoice && !openChoice.list.contains(e.target)) dismissChoice();}, true);
document.addEventListener('focusin', e => {
  const top = modalStack.at(-1);
  if (top && !top.contains(e.target)) (modalFocusables(top)[0] || top).focus();
}, true);
for (const type of ['keydown', 'keyup', 'keypress']) document.addEventListener(type, e => {
  const top = modalStack.at(-1); if (!top) return;
  if (type === 'keydown' && e.key === 'Escape') {e.preventDefault();e.stopImmediatePropagation();if (openChoice) dismissChoice(true); else top.close();return;}
  if (!top.contains(e.target)) {e.preventDefault();e.stopImmediatePropagation();(modalFocusables(top)[0] || top).focus();return;}
  if (type !== 'keydown') return;
  if (e.key === 'Tab') {
    if (openChoice) dismissChoice(true);
    const items = modalFocusables(top), first = items[0], last = items.at(-1);
    if (!items.length) {e.preventDefault();top.focus();}
    else if (e.shiftKey && (document.activeElement === first || document.activeElement === top)) {e.preventDefault();last.focus();}
    else if (!e.shiftKey && (document.activeElement === last || document.activeElement === top)) {e.preventDefault();first.focus();}
  }
}, true);
function rocketDrawing() { return '<path d="M13 21 8 24l1-7C12 9 19 5 27 5c0 8-4 15-12 18l-7 1 3-5M18 9a3 3 0 1 0 4 4 3 3 0 0 0-4-4ZM10 24l-5 3 3-5" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/>'; }
function rocketMark() { return '<svg class="rocket" viewBox="0 0 32 32" aria-hidden="true">'+rocketDrawing()+'</svg>'; }
function landingBrand() {
  const ship='<use href="#landing-rocket" x="-14" y="-14" width="28" height="28" transform="rotate(45)"/>';
  return `<div class="login-brand"><h1 class="sr-only">teleport</h1><svg class="login-scene" viewBox="0 0 360 170" aria-hidden="true"><defs><symbol id="landing-rocket" viewBox="0 0 32 32">${rocketDrawing()}</symbol></defs><g class="login-moon"><circle cx="319" cy="33" r="14"/><circle class="moon-crater" cx="314" cy="29" r="3"/><circle class="moon-crater" cx="323" cy="37" r="4"/></g><g class="login-ship ship-back">${ship}</g><text class="landing-word" x="180" y="106" text-anchor="middle">teleport</text><g class="login-ship ship-front">${ship}</g></svg></div>`;
}
