"use strict";
let connectionInterrupted=false,connectionRestoring=false,connectionScene=null,connectionProbeBusy=false,connectionCloseTimer=0,connectionInitialRetry=false;

function showConnectionInterrupted(){
  connectionInterrupted=true;
  if(connectionScene){
    clearTimeout(connectionCloseTimer);connectionScene.classList.add('visible');
    if(connectionRestoring){connectionRestoring=false;connectionScene.connectionSequence++;connectionScene.querySelector('.connection-status').textContent='connection interrupted';connectionScene.connectionFlightReady=startLandingFlight(connectionScene.querySelector('.login-scene'),true).catch(()=>{});}
    return;
  }
  const previousFocus=document.activeElement,background=[...document.body.children].map(element=>({element,inert:element.inert}));
  const scene=document.createElement('section');scene.className='connection-scene';scene.setAttribute('role','status');scene.setAttribute('aria-live','assertive');scene.tabIndex=-1;
  scene.innerHTML=landingBrand()+'<p class="connection-status">connection interrupted</p>';
  scene.querySelector('.login-moon').removeAttribute('tabindex');scene.querySelector('.login-moon').removeAttribute('role');
  document.body.append(scene);connectionScene=scene;
  scene.connectionBackground=background;scene.connectionFocus=previousFocus;
  background.forEach(({element})=>{element.inert=true;});
  scene.focus({preventScroll:true});
  requestAnimationFrame(()=>{if(connectionScene===scene&&connectionInterrupted)scene.classList.add('visible');});
  scene.connectionSequence=0;scene.connectionFlightReady=startLandingFlight(scene.querySelector('.login-scene'),true).catch(()=>{});
}
function restoreConnection(){
  if(!connectionInterrupted||connectionRestoring)return;
  const scene=connectionScene;if(!scene)return;
  connectionRestoring=true;const sequence=++scene.connectionSequence;
  scene.querySelector('.connection-status').textContent='connection restoring…';
  const current=()=>connectionRestoring&&connectionScene===scene&&scene.connectionSequence===sequence;
  const finish=()=>{
    if(!current())return;
    connectionInterrupted=false;connectionRestoring=false;
    stopLandingFlight();scene.remove();connectionScene=null;
    scene.connectionBackground.forEach(({element,inert})=>{element.inert=inert;});
    if(scene.connectionFocus?.isConnected)scene.connectionFocus.focus({preventScroll:true});
    const oldScene=document.querySelector('.login-scene');if(oldScene)startLandingFlight(oldScene).catch(()=>{});
    if(connectionInitialRetry){connectionInitialRetry=false;start();}
  };
  const fade=()=>{if(!current())return;scene.classList.remove('visible');connectionCloseTimer=setTimeout(finish,450);};
  if(matchMedia('(prefers-reduced-motion: reduce)').matches){finish();return;}
  scene.connectionFlightReady.then(()=>{
    if(!current())return;
    if(landingFlight){landingFlight.launch();connectionCloseTimer=setTimeout(fade,4500);}else fade();
  });
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
