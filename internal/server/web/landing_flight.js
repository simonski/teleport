"use strict";
let landingFlight=null,landingFlightGeneration=0;
function stopLandingFlight(){landingFlightGeneration++;landingFlight?.dispose();landingFlight=null;}
async function startLandingFlight(){
  stopLandingFlight();const generation=landingFlightGeneration,svg=document.querySelector('.login-scene');if(!svg)return;
  const route=await import('/landing_route.js');if(generation!==landingFlightGeneration||!svg.isConnected)return;
  const ship=svg.querySelector('.flight-rocket'),engine=svg.querySelector('.rocket-engine'),flame=svg.querySelector('.rocket-flame'),moonElement=svg.querySelector('.login-moon'),arms=svg.querySelectorAll('.catch-arm'),smoke=svg.querySelectorAll('.landing-smoke circle'),media=window.matchMedia('(prefers-reduced-motion: reduce)');
  const variants=[...svg.querySelectorAll('.rocket-variant')],fins=svg.querySelectorAll('.rocket-fins'),panels=svg.querySelectorAll('.rocket-panel'),vehicle=svg.querySelector('.ground-vehicle'),vehicles=svg.querySelectorAll('[data-vehicle]'),wheels=svg.querySelectorAll('.ground-wheel'),hose=svg.querySelector('.fuel-hose'),steam=svg.querySelectorAll('.fuel-steam circle'),debris=svg.querySelector('.rocket-debris'),level=svg.querySelector('.tank-level');
  let moon={...route.DEFAULT_MOON},elapsed=0,cycle=0,previous=0,request=0,disposed=false,dragging=null,manual=false,smoothed=null,currentVariant=-1,currentVehicle='';
  function paint(frame,instant=false){
    const factor=!instant&&['outbound','orbit','return'].includes(frame.phase)?.22:1;
    if(!smoothed)smoothed={x:frame.x,y:frame.y};smoothed.x+=(frame.x-smoothed.x)*factor;smoothed.y+=(frame.y-smoothed.y)*factor;
    ship.setAttribute('transform',`translate(${smoothed.x} ${smoothed.y}) rotate(${frame.angle})`);
    ship.style.opacity=String(frame.opacity);
    engine.setAttribute('transform',`translate(${route.ROCKETS[frame.variant].tail} 0) rotate(${frame.gimbal})`);
    engine.style.opacity=String(frame.engine);
    const flicker=1+Math.sin(elapsed*.091)*.12;
    flame.setAttribute('transform',`scale(${Math.max(.01,frame.engine*flicker)} ${.65+frame.engine*.35})`);flame.style.opacity=String(frame.engine);
    const opening=(1-frame.arms)*22;
    arms[0].setAttribute('transform',`rotate(${-opening} 25 108)`);arms[1].setAttribute('transform',`rotate(${opening} 25 117)`);
    if(currentVariant!==frame.variant){variants.forEach((element,index)=>{element.style.display=index===frame.variant?'':'none';});currentVariant=frame.variant;}
    fins.forEach(element=>element.setAttribute('transform',`scale(1 ${.28+frame.fins*.72})`));
    panels.forEach(element=>{element.style.opacity=frame.broken?'0':'1';});
    vehicle.style.opacity=String(frame.vehicle?.opacity||0);
    if(frame.vehicle){
      const truck=frame.vehicle;
      vehicle.setAttribute('transform',`translate(${truck.x} ${truck.y})`);
      if(currentVehicle!==truck.kind){vehicles.forEach(element=>{element.style.display=element.dataset.vehicle===truck.kind?'':'none';});currentVehicle=truck.kind;}
      wheels.forEach(wheel=>wheel.setAttribute('transform',`rotate(${truck.x*12} ${wheel.dataset.wheelX} 3)`));
      svg.querySelector('.carrier-support').setAttribute('transform',`rotate(${(1-truck.load)*65} -10 -7)`);
      svg.querySelector('.recovery-hook').setAttribute('transform',`translate(0 ${truck.load*6})`);
    }
    level.setAttribute('stroke-dasharray','16');level.setAttribute('stroke-dashoffset',String(16*(1-frame.fuel)));
    const hoseStart={x:20,y:132},hoseEnd={x:20+26*frame.hose,y:132-12*frame.hose};
    hose.setAttribute('d',`M${hoseStart.x} ${hoseStart.y}Q${hoseStart.x+6} ${143+frame.hose*3} ${hoseEnd.x} ${hoseEnd.y}`);hose.style.opacity=frame.hose>0?'1':'0';
    debris.style.opacity=String(frame.debris?.opacity||0);
    if(frame.debris)debris.setAttribute('transform',`translate(${frame.debris.x} ${frame.debris.y}) rotate(${frame.debris.angle})`);
    for(let index=0;index<smoke.length;index++){
      const launch=frame.exhaust>0,t=launch?(elapsed*.00065+index/6)%1:frame.phase==='smoke'?Math.max(0,Math.min(1,frame.smoke-index*.09)):0;
      smoke[index].setAttribute('cx',String(route.LANDING_HOME.x+(index-2.5)*(launch?4+10*t:5)+Math.sin(t*4+index)*6*t));smoke[index].setAttribute('cy',String(130-t*(launch?9:12+index*2)));smoke[index].setAttribute('r',String(2+t*(4+index*.6)));smoke[index].style.opacity=String(Math.sin(Math.PI*t)*(launch?frame.exhaust*.36:frame.phase==='smoke'?.26:0));
      const vapour=(elapsed*.0007+index/6)%1;
      steam[index].setAttribute('cx',String(46+Math.sin(index+vapour*3)*3-vapour*9));steam[index].setAttribute('cy',String(120-vapour*25));steam[index].setAttribute('r',String(1.5+vapour*4));steam[index].style.opacity=String(Math.sin(Math.PI*vapour)*frame.steam*.42);
    }
    moonElement.setAttribute('transform',`translate(${moon.x-route.DEFAULT_MOON.x} ${moon.y-route.DEFAULT_MOON.y})`);
    svg.dataset.flightPhase=frame.phase;svg.dataset.engine=String(frame.engine);svg.dataset.orbits=String(frame.orbits);svg.dataset.rocket=route.ROCKETS[frame.variant].id;svg.dataset.exhaust=String(frame.exhaust);svg.dataset.steam=String(frame.steam);svg.dataset.hose=String(frame.hose);
  }
  function sample(){return route.sampleFlight(media.matches?route.flightPlan(cycle).stages.find(stage=>stage.name==='rest').start:elapsed,cycle,moon);}
  function tick(now){
    request=0;if(disposed||!svg.isConnected)return;
    if(!document.hidden&&!media.matches&&!manual){if(previous)elapsed+=Math.min(now-previous,60);while(elapsed>=route.flightPlan(cycle).duration){elapsed-=route.flightPlan(cycle).duration;cycle++;smoothed=null;}paint(sample());}
    previous=now;if(!media.matches&&!document.hidden&&!manual)request=requestAnimationFrame(tick);
  }
  function resume(){if(disposed)return;if(request)cancelAnimationFrame(request);request=0;previous=0;if(media.matches)paint(sample(),true);else if(!document.hidden&&!manual)request=requestAnimationFrame(tick);}
  function moonPoint(event){const point=new DOMPoint(event.clientX,event.clientY);return point.matrixTransform(svg.getScreenCTM().inverse());}
  function moveMoon(x,y){
    const bounds=svg.getScreenCTM(),inverse=bounds.inverse(),margin=48;
    const low=new DOMPoint(margin,margin).matrixTransform(inverse),high=new DOMPoint(window.innerWidth-margin,window.innerHeight-margin).matrixTransform(inverse);
    moon={x:Math.max(low.x,Math.min(high.x,x)),y:Math.max(low.y,Math.min(high.y,y))};paint(sample(),manual||media.matches);
  }
  function down(event){if(event.button!==0)return;event.preventDefault();const point=moonPoint(event);dragging={id:event.pointerId,x:point.x-moon.x,y:point.y-moon.y};moonElement.setPointerCapture(event.pointerId);}
  function move(event){if(dragging?.id!==event.pointerId)return;event.preventDefault();const point=moonPoint(event);moveMoon(point.x-dragging.x,point.y-dragging.y);}
  function up(event){if(dragging?.id===event.pointerId){if(moonElement.hasPointerCapture(event.pointerId))moonElement.releasePointerCapture(event.pointerId);dragging=null;}}
  function key(event){const delta=event.shiftKey?20:5,offset={ArrowLeft:[-delta,0],ArrowRight:[delta,0],ArrowUp:[0,-delta],ArrowDown:[0,delta]}[event.key];if(offset){event.preventDefault();moveMoon(moon.x+offset[0],moon.y+offset[1]);}else if(event.key==='Escape'){event.preventDefault();moon={...route.DEFAULT_MOON};paint(sample(),true);}}
  function resize(){moveMoon(moon.x,moon.y);}
  moonElement.addEventListener('pointerdown',down);moonElement.addEventListener('pointermove',move);moonElement.addEventListener('pointerup',up);moonElement.addEventListener('pointercancel',up);moonElement.addEventListener('keydown',key);
  document.addEventListener('visibilitychange',resume);media.addEventListener('change',resume);
  window.addEventListener('resize',resize);
  paint(sample(),true);resume();
  landingFlight={
    seek(milliseconds,nextCycle=cycle){manual=true;elapsed=milliseconds;cycle=nextCycle;smoothed=null;paint(sample(),true);resume();return {...sample(),moon:{...moon}};},
    seekPhase(name,fraction=.5,nextCycle=cycle){const stage=route.flightPlan(nextCycle).stages.find(stage=>stage.name===name);if(!stage)throw new Error('Unknown landing phase');return this.seek(stage.start+stage.duration*Math.max(0,Math.min(.999999,fraction)),nextCycle);},
    play(){manual=false;resume();},snapshot:()=>({...sample(),moon:{...moon},elapsed,cycle}),
    dispose(){disposed=true;if(request)cancelAnimationFrame(request);moonElement.removeEventListener('pointerdown',down);moonElement.removeEventListener('pointermove',move);moonElement.removeEventListener('pointerup',up);moonElement.removeEventListener('pointercancel',up);moonElement.removeEventListener('keydown',key);document.removeEventListener('visibilitychange',resume);media.removeEventListener('change',resume);window.removeEventListener('resize',resize);},
  };
}
