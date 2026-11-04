"use strict";
const storageChoiceKey='teleport-storage-choice-v1';
let storageChoiceMemory=null;
function storageChoice(){if(storageChoiceMemory!==null)return storageChoiceMemory;try{return localStorage.getItem(storageChoiceKey);}catch{return null;}}
function browserLocationAllowed(){return storageChoice()==='allow';}
function clearBrowserLocations(){try{for(let index=localStorage.length-1;index>=0;index--){const key=localStorage.key(index);if(key?.startsWith('teleport-location:'))localStorage.removeItem(key);}}catch{}}
function saveStorageChoice(value){
  storageChoiceMemory=value;try{localStorage.setItem(storageChoiceKey,value);}catch{}
  if(value!=='allow')clearBrowserLocations();
  document.querySelector('#cookie-notice')?.remove();
}
function openCookieNotice(){
  if(document.querySelector('#cookie-notice'))return;
  const box=document.createElement('section');box.id='cookie-notice';box.className='cookie-notice';box.setAttribute('aria-labelledby','cookie-notice-title');
  box.innerHTML='<div><strong id="cookie-notice-title">Your browser storage</strong><p>The sign-in cookie keeps your session secure. Optional storage remembers your last workspace on this device. Your selection is saved on this device. <a href="/privacy.html#cookies">Storage details</a></p></div><div class="cookie-actions"><button type="button" class="secondary" data-storage="reject">Reject optional</button><button type="button" data-storage="allow">Allow preferences</button></div>';
  box.querySelectorAll('[data-storage]').forEach(button=>button.addEventListener('click',()=>saveStorageChoice(button.dataset.storage)));
  document.body.append(box);
}
document.querySelector('#cookie-settings')?.addEventListener('click',openCookieNotice);
if(!storageChoice())openCookieNotice();
