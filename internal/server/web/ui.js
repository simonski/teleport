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
  return [...dialog.querySelectorAll('button,input,textarea,select,summary,a[href],[tabindex]')].filter(e => !e.matches(':disabled') && !e.hidden && !e.closest('[inert]') && e.tabIndex >= 0 && e.getClientRects().length);
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
    function focusChoice() {
      const option=available[active];if(!option)return;
      // Keep focus scrolling inside the popup. Scrolling its owning dialog
      // dismisses the popup through the outside-scroll handler.
      option.focus({preventScroll:true});
      const box=option.getBoundingClientRect(),bounds=list.getBoundingClientRect();
      if(box.top<bounds.top)list.scrollTop+=box.top-bounds.top;
      else if(box.bottom>bounds.bottom)list.scrollTop+=box.bottom-bounds.bottom;
    }
    function navigateChoice(key) {
      if (!available.length) return;
      active = key === 'Home' ? 0 : key === 'End' ? available.length - 1 : (active + (key === 'ArrowDown' ? 1 : -1) + available.length) % available.length;
      focusChoice();
    }
    openChoice = {button, list, anchor:{left:rect.left,top:rect.top}, navigate:navigateChoice}; focusChoice();
    let search = '', lastKey = 0;
    list.onkeydown = e => {
      if (e.key === 'Escape') {e.preventDefault();e.stopPropagation();dismissChoice(true);return;}
      if (e.key === 'Tab') {dismissChoice(true);return;}
      if (['ArrowDown','ArrowUp','Home','End'].includes(e.key)) {e.preventDefault();navigateChoice(e.key);}
      else if (e.key.length === 1 && !e.ctrlKey && !e.metaKey && e.key !== ' ') {e.preventDefault();search = performance.now() - lastKey < 700 ? search + e.key : e.key;lastKey = performance.now();const found = available.findIndex(o => o.textContent.toLowerCase().startsWith(search.toLowerCase()));if (found >= 0) {active = found;focusChoice();}}
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
  if(typeof enhanceDate==='function')document.querySelectorAll('input[type=date]').forEach(enhanceDate);
  if (openChoice && !openChoice.button.isConnected) dismissChoice();
}
new MutationObserver(enhanceControls).observe(document.documentElement, {childList:true,subtree:true});
document.addEventListener('submit', e => { if (e.target.closest('tk-dialog') && e.target.getAttribute('method') === 'dialog') e.preventDefault(); }, true);
document.addEventListener('pointerdown', e => {if (openChoice && !openChoice.list.contains(e.target) && !openChoice.button.contains(e.target)) dismissChoice();}, true);
window.addEventListener('resize', () => dismissChoice());
window.addEventListener('scroll', e => {
  if(!openChoice||openChoice.list.contains(e.target))return;
  const rect=openChoice.button.getBoundingClientRect(),anchor=openChoice.anchor;
  // A scroll event queued before opening cannot invalidate the new position.
  if(!anchor||Math.abs(rect.left-anchor.left)>.5||Math.abs(rect.top-anchor.top)>.5)dismissChoice();
}, true);
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
function landingRocketFleet(){
  const shapes=[
    ['dart','M-10-4H2Q10-4 15 0Q10 4 2 4H-10Z','M-9-4-3-9 0-4M-9 4-3 9 0 4','M5-2a2 2 0 1 0 0 4 2 2 0 0 0 0-4'],
    ['capsule','M-10-5H1Q9-7 12 0Q9 7 1 5H-10Z','','M2-3h4v6H2ZM-7-5V5'],
    ['booster','M-12-4H3L14 0 3 4H-12ZM-10-7H-2L2-5H-10ZM-10 7H-2L2 5H-10Z','M-12-4-7-10-4-4M-12 4-7 10-4 4','M5-2v4M-5-4v8'],
    ['needle','M-13-3H5L20 0 5 3H-13Z','M-12-3-5-8-3-3M-12 3-5 8-3 3','M8-1v2M-8-3v6'],
    ['shuttle','M-11-5H3Q9-5 15 0Q9 5 3 5H-11Z','M-10-4-7-13 6-4M-10 4-7 13 6 4','M7-2h3v4H7ZM-8-5V5'],
    ['starship','M-15-6H7Q17-6 23 0Q17 6 7 6H-15Z','M-13-6-9-12-5-6M-13 6-9 12-5 6M8-6 12-10 16-4M8 6 12 10 16 4','M9-3h4M9 0h5M9 3h4M-5-6V6M2-6V6'],
  ];
  return shapes.map(([id,body,fins,details],index)=>`<g class="rocket-variant" data-rocket="${id}"${index?' style="display:none"':''}><path class="rocket-hull" d="${body}"/><g class="rocket-fins"><path d="${fins}"/></g><path class="rocket-details" d="${details}"/><path class="rocket-panel" d="M-5 2h5l-2 3h-3Z"/></g>`).join('');
}
function landingGroundVehicles(){
  const wheels=[-14,-6,13,23].map(x=>`<g class="ground-wheel" data-wheel-x="${x}"><circle cx="${x}" cy="3" r="2.7"/><path d="M${x-1.6} 3h3.2"/></g>`).join('');
  return `<g class="ground-vehicle" aria-hidden="true"><g data-vehicle="carrier"><path class="vehicle-chassis" d="M-23-4h40v-7h11l7 7v5h-58Z"/><path d="M20-9h6l5 5H20ZM-19-7h30M-15-8v-6M8-8v-6"/>${wheels}<path class="carrier-support" d="M-10-7v-18h7v18M7-7v-18h-3"/></g><g data-vehicle="tanker"><path class="vehicle-chassis" d="M-22-3h37v-9h12l7 8v5h-56Z"/><path class="vehicle-tank" d="M-18-4v-6q0-5 5-5H6q5 0 5 5v6Z"/><path d="M19-10h6l5 6H19ZM-19-7h29"/><path class="tank-level" d="M-12-9H4"/>${wheels}</g><g data-vehicle="recovery"><path class="vehicle-chassis" d="M-23-3h38v-10h12l7 9v5h-57Z"/><path d="M19-11h6l5 7H19ZM-17-4v-18h6l14 17M-14-19l16 14M-20-5h28"/>${wheels}<path class="recovery-hook" d="M-14-19H1v8q0 4 4 2"/></g></g>`;
}
function landingBrand() {
  const puffs=Array.from({length:6},()=>'<circle cx="46" cy="130" r="2"/>').join('');
  const stars=Array.from({length:22},(_,i)=>`<circle class="landing-star" cx="${12+(i*71)%340}" cy="${7+(i*31)%145}" r="${i%4===0?1.1:.6}" style="--star-delay:${(i%7)*-.7}s"/>`).join('');
  const planets=[['mercury',82,21,3,'#a9a5a0'],['venus',127,34,5,'#d9b278'],['earth',172,16,5,'#74a9ba'],['mars',219,34,4,'#c98167'],['jupiter',263,14,8,'#c8a88c'],['saturn',296,87,7,'#d5c398'],['uranus',349,85,5,'#9cc8c8'],['neptune',348,135,5,'#829cc7']];
  const worlds=planets.map(([name,x,y,r,color])=>`<g class="landing-planet" data-planet="${name}" data-x="${x}" data-y="${y}" aria-label="${name}"><circle cx="${x}" cy="${y}" r="${r}" fill="${color}"/>${name==='saturn'?`<ellipse cx="${x}" cy="${y}" rx="11" ry="3" fill="none" stroke="${color}"/>`:''}</g>`).join('');
  return `<div class="login-brand"><h1 class="sr-only">teleport</h1><svg class="login-scene" viewBox="0 0 360 170" role="group" aria-label="Rocket flight"><g aria-hidden="true">${stars}${worlds}</g><g class="landing-scaffold" aria-hidden="true"><path d="M18 132h51M25 132V74h5v58M25 78l5 12-5 12 5 12-5 12M25 76h15M25 97h14"/><path class="catch-arm" d="M25 108h16l4 2"/><path class="catch-arm" d="M25 117h16l4-2"/></g><text class="landing-word" x="190" y="106" text-anchor="middle" aria-hidden="true">teleport</text><g class="landing-smoke" aria-hidden="true">${puffs}</g><g class="fuel-steam" aria-hidden="true">${puffs}</g><path class="fuel-hose" aria-hidden="true"/><g class="rocket-debris" aria-hidden="true"><path d="M-5 2h5l-2 3h-3Z"/></g>${landingGroundVehicles()}<g class="login-ship flight-rocket" transform="translate(46 112) rotate(-90)" aria-hidden="true"><g class="rocket-engine" transform="translate(-10 0)"><path class="rocket-flame" d="M0-3Q-23-6-29 0Q-22 6 0 3Z"/><path class="rocket-flame-core" d="M0-1.5Q-12-3-18 0Q-12 3 0 1.5Z"/></g><g class="rocket-body">${landingRocketFleet()}</g></g><g class="login-moon" tabindex="0" role="button" aria-label="Moon. Arrow keys move; Escape resets."><circle cx="319" cy="33" r="14"/><circle class="moon-crater" cx="314" cy="29" r="3"/><circle class="moon-crater" cx="323" cy="37" r="4"/></g></svg></div>`;
}
