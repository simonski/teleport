"use strict";
let connectionInterrupted=false,connectionScene=null,connectionProbeBusy=false,connectionCloseTimer=0,connectionInitialRetry=false;

function showConnectionInterrupted(){
  connectionInterrupted=true;
  if(connectionScene){clearTimeout(connectionCloseTimer);connectionScene.classList.add('visible');return;}
  const previousFocus=document.activeElement,background=[...document.body.children].map(element=>({element,inert:element.inert}));
  const scene=document.createElement('section');scene.className='connection-scene';scene.setAttribute('role','status');scene.setAttribute('aria-live','assertive');scene.tabIndex=-1;
  scene.innerHTML=landingBrand()+'<p class="connection-status">connection interrupted</p>';
  scene.querySelector('.login-moon').removeAttribute('tabindex');scene.querySelector('.login-moon').removeAttribute('role');
  document.body.append(scene);connectionScene=scene;
  scene.connectionBackground=background;scene.connectionFocus=previousFocus;
  background.forEach(({element})=>{element.inert=true;});
  scene.focus({preventScroll:true});
  requestAnimationFrame(()=>{if(connectionScene===scene&&connectionInterrupted)scene.classList.add('visible');});
  startLandingFlight(scene.querySelector('.login-scene'),true).catch(()=>{});
}
function restoreConnection(){
  if(!connectionInterrupted)return;
  connectionInterrupted=false;const scene=connectionScene;if(!scene)return;
  scene.classList.remove('visible');
  const finish=()=>{
    if(connectionInterrupted||connectionScene!==scene)return;
    stopLandingFlight();scene.remove();connectionScene=null;
    scene.connectionBackground.forEach(({element,inert})=>{element.inert=inert;});
    if(scene.connectionFocus?.isConnected)scene.connectionFocus.focus({preventScroll:true});
    const oldScene=document.querySelector('.login-scene');if(oldScene)startLandingFlight(oldScene).catch(()=>{});
    if(connectionInitialRetry){connectionInitialRetry=false;start();}
  };
  if(matchMedia('(prefers-reduced-motion: reduce)').matches)finish();else connectionCloseTimer=setTimeout(finish,450);
}
async function probeConnection(){
  if(connectionProbeBusy)return;connectionProbeBusy=true;
  const controller=new AbortController(),timeout=setTimeout(()=>controller.abort(),3000);
  try{
    const response=await fetch('/healthz',{cache:'no-store',signal:controller.signal});
    if(!response.ok||(await response.json()).status!=='ok')throw new Error('Server unavailable');
    restoreConnection();
  }catch{showConnectionInterrupted();}
  finally{clearTimeout(timeout);connectionProbeBusy=false;}
}
window.addEventListener('online',probeConnection);
window.addEventListener('offline',showConnectionInterrupted);
setInterval(probeConnection,2000);
