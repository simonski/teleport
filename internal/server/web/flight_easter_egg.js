"use strict";
let flightEasterEgg=null;

// Retain only a possible code prefix, never ordinary typed text.
function flightCodeMatcher(){
  const codes=['idkfa','iddqd'];let prefix='',previous=0;
  const match=(key,now)=>{
    if(now-previous>1800)prefix='';
    previous=now;
    const candidate=prefix+key.toLowerCase();
    if(codes.includes(candidate)){prefix='';return true;}
    prefix='';
    for(let size=Math.min(4,candidate.length);size>0;size--){
      const suffix=candidate.slice(-size);
      if(codes.some(code=>code.startsWith(suffix))){prefix=suffix;break;}
    }
    return prefix?null:false;
  };
  match.reset=()=>{prefix='';previous=0;};
  return match;
}

function resetFlightEasterEgg(){flightEasterEgg?.finish();}
function showFlightEasterEgg(){
  if(!session||flightEasterEgg||foregroundOpen())return;
  const scroll={left:window.scrollX,top:window.scrollY};
  const dialog=document.createElement('tk-dialog');
  dialog.className='flight-easter-egg';
  dialog.innerHTML=landingBrand()+'<p class="sr-only">Press Escape to return to the workspace.</p>';
  document.body.append(dialog);dialog.showModal();
  dialog.removeAttribute('aria-labelledby');dialog.setAttribute('aria-label','Rocket flight');
  const shade=dialog.shade,close=dialog.close.bind(dialog),state={closing:false,timer:0,finish};
  flightEasterEgg=state;
  shade.classList.add('flight-easter-egg-shade');
  // Establish the initial opacity before entering; transitions remain interruptible.
  shade.getBoundingClientRect();shade.classList.add('flight-easter-egg-visible');
  function finish(){
    if(flightEasterEgg!==state)return;
    clearTimeout(state.timer);shade.removeEventListener('transitionend',ended);
    stopLandingFlight();close();dialog.remove();flightEasterEgg=null;
    window.scrollTo({...scroll,behavior:'instant'});
  }
  function ended(event){if(state.closing&&event.target===shade&&event.propertyName==='opacity')finish();}
  shade.addEventListener('transitionend',ended);
  dialog.close=()=>{
    if(state.closing)return;
    state.closing=true;shade.classList.remove('flight-easter-egg-visible');
    if(window.matchMedia('(prefers-reduced-motion: reduce)').matches)finish();
    else state.timer=setTimeout(finish,350);
  };
  startLandingFlight().catch(()=>{if(flightEasterEgg===state)finish();});
}

const matchFlightCode=flightCodeMatcher();
let backlogImportTimer=0;
function resetFlightCode(){matchFlightCode.reset();if(backlogImportTimer)clearTimeout(backlogImportTimer);backlogImportTimer=0;}
document.addEventListener('keydown',event=>{
  if(flightEasterEgg)return; // The dialog owns keys until its exit fade finishes.
  if(event.key==='Shift')return;
  if(backlogImportTimer){clearTimeout(backlogImportTimer);backlogImportTimer=0;}
  const editing=event.composedPath().some(node=>node.matches?.('input,textarea,select,[contenteditable]:not([contenteditable="false"]),[role="textbox"],.xterm'));
  if(!session||event.defaultPrevented||event.ctrlKey||event.metaKey||event.altKey||event.isComposing||event.repeat||editing||foregroundOpen()||!/^[a-z]$/i.test(event.key)){
    resetFlightCode();return;
  }
  const result=matchFlightCode(event.key,performance.now());
  if(result===null&&event.key.toLowerCase()==='i'&&typeof view!=='undefined'&&view==='Backlog'){
    const actor=session.account.id;backlogImportTimer=setTimeout(()=>{backlogImportTimer=0;matchFlightCode.reset();if(session?.account.id===actor&&view==='Backlog'&&!foregroundOpen())backlogImportForm();},1800);
  }
  if(result!==false){event.preventDefault();event.stopImmediatePropagation();}
  if(result===true)showFlightEasterEgg();
},true);
document.addEventListener('focusin',resetFlightCode);
document.addEventListener('pointerdown',resetFlightCode,true);
document.addEventListener('visibilitychange',resetFlightCode);
window.addEventListener('blur',resetFlightCode);
