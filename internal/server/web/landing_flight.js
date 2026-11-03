"use strict";
let landingFlight=null,landingFlightGeneration=0;
function stopLandingFlight(){landingFlightGeneration++;landingFlight?.dispose();landingFlight=null;}
async function startLandingFlight(){
  stopLandingFlight();const generation=landingFlightGeneration,svg=document.querySelector('.login-scene');if(!svg)return;
  const route=await import('/landing_route.js');if(generation!==landingFlightGeneration||!svg.isConnected)return;
  const ship=svg.querySelector('.flight-rocket'),engine=svg.querySelector('.rocket-engine'),flame=svg.querySelector('.rocket-flame'),moonElement=svg.querySelector('.login-moon'),arms=svg.querySelectorAll('.catch-arm'),smoke=svg.querySelectorAll('.landing-smoke circle'),media=window.matchMedia('(prefers-reduced-motion: reduce)');
  const variants=[...svg.querySelectorAll('.rocket-variant')],fins=svg.querySelectorAll('.rocket-fins'),panels=svg.querySelectorAll('.rocket-panel'),vehicle=svg.querySelector('.ground-vehicle'),vehicles=svg.querySelectorAll('[data-vehicle]'),wheels=svg.querySelectorAll('.ground-wheel'),hose=svg.querySelector('.fuel-hose'),steam=svg.querySelectorAll('.fuel-steam circle'),debris=svg.querySelector('.rocket-debris'),level=svg.querySelector('.tank-level');
  const destinations=[{name:'moon',point:()=>moon},...[...svg.querySelectorAll('[data-planet]')].map(node=>({name:node.dataset.planet,point:()=>({x:Number(node.dataset.x),y:Number(node.dataset.y)})}))];
  let moon={...route.DEFAULT_MOON},destination=destinations[Math.floor(Math.random()*destinations.length)],elapsed=0,cycle=0,previous=0,request=0,disposed=false,dragging=null,manual=false,smoothed=null,currentVariant=-1,currentVehicle='';
  const fleet=[],colors=['#c96951','#698fa3','#a884b0','#b99865','#6e9f8b','#c27e91','#929b63','#8390be','#bd8b70'];
  let padOwner=null,fleetClock=0,nextLaunch=0,fleetSerial=0;
  function stageStart(plan,name){return plan.stages.find(stage=>stage.name===name).start;}
  function clearFleet(){for(const craft of fleet){craft.node.remove();craft.carrier.remove();}fleet.length=0;padOwner=null;fleetClock=0;nextLaunch=0;fleetSerial=0;}
  function launchFleet(){
    const number=++fleetSerial,node=ship.cloneNode(true);node.classList.remove('flight-rocket','login-ship');node.classList.add('fleet-rocket');node.style.color=colors[(number-1)%colors.length];node.style.opacity='1';
    const carrier=vehicle.cloneNode(true);carrier.classList.add('fleet-carrier');carrier.dataset.rocket=String(number);node.dataset.rocket=String(number);node.setAttribute('transform','translate(-78 108) rotate(-90)');carrier.setAttribute('transform','translate(-78 137)');carrier.style.opacity='0';carrier.querySelectorAll('[data-vehicle]').forEach(item=>{item.style.display=item.dataset.vehicle==='carrier'?'':'none';});
    svg.insertBefore(carrier,moonElement);svg.insertBefore(node,moonElement);const trip=number,plan=route.flightPlan(trip),target=destinations[Math.floor(Math.random()*destinations.length)];
    const craft={node,carrier,trip,plan,target,time:stageStart(plan,'lift'),position:{x:-78,y:108},holding:false,finished:false,serial:number,ground:'delivery',groundTime:0};
    node.querySelectorAll('.rocket-variant').forEach((variant,index)=>{variant.style.display=index===trip%route.ROCKETS.length?'':'none';});
    fleet.push(craft);padOwner=craft;nextLaunch=fleetClock+6500;
  }
  function advanceFleet(dt,primaryPhase){
    fleetClock+=dt;
    const primaryBusy=!['outbound','orbit'].includes(primaryPhase);
    for(const craft of fleet){
      if(craft.finished)continue;
      if(craft.ground){
        const previousGroundTime=craft.groundTime;craft.groundTime+=dt;const t=craft.groundTime,delivery=craft.ground==='delivery';let truck,load,rocketX=route.LANDING_HOME.x,rocketY=route.LANDING_HOME.y;
        const ease=value=>{value=Math.max(0,Math.min(1,value));return value*value*(3-2*value);};
        if(t<2400){truck=-78+124*ease(t/2400);load=delivery?1:0;if(delivery){rocketX=truck;rocketY-=4;}}
        else if(t<3500){truck=46;load=delivery?1-ease((t-2400)/1100):ease((t-2400)/1100);rocketY-=4*load;}
        else{truck=46-124*ease((t-3500)/1400);load=delivery?0:1;if(!delivery){rocketX=truck;rocketY-=4;}}
        const candidate={x:rocketX,y:rocketY},others=fleet.filter(other=>other!==craft&&!other.finished).map(other=>other.position);if(smoothed)others.push(smoothed);
        if(others.some(other=>{const before=Math.hypot(craft.position.x-other.x,craft.position.y-other.y),after=Math.hypot(candidate.x-other.x,candidate.y-other.y);return before<route.ROCKET_CLEARANCE?after<before:after<route.ROCKET_CLEARANCE;})){craft.groundTime=previousGroundTime;continue;}
        craft.position=candidate;craft.node.setAttribute('transform',`translate(${rocketX} ${rocketY}) rotate(-90)`);craft.node.querySelector('.rocket-engine').style.opacity='0';
        craft.node.querySelectorAll('.rocket-fins').forEach(fin=>fin.setAttribute('transform',`scale(1 ${.28+.72*Math.sin(Math.PI*Math.min(1,t/4900))**2})`));
        craft.carrier.style.opacity=String(t<3500?Math.min(1,t/400):Math.max(0,Math.min(1,(4900-t)/420)));craft.carrier.setAttribute('transform',`translate(${truck} 137)`);
        craft.carrier.querySelectorAll('.ground-wheel').forEach(wheel=>wheel.setAttribute('transform',`rotate(${truck*12} ${wheel.dataset.wheelX} 3)`));craft.carrier.querySelector('.carrier-support').setAttribute('transform',`rotate(${(1-load)*65} -10 -7)`);
        craft.node.dataset.phase=delivery?'delivery':'pickup';
        if(t>=4900){craft.carrier.style.opacity='0';if(delivery)craft.ground='';else{craft.finished=true;craft.node.style.display='none';if(padOwner===craft)padOwner=null;}}
        continue;
      }
      const next=craft.time+dt,returnAt=stageStart(craft.plan,'return'),smokeAt=stageStart(craft.plan,'smoke');
      if(craft.time<returnAt&&next>=returnAt){
        if(primaryBusy||padOwner&&padOwner!==craft){craft.time=returnAt-.01;craft.holding=true;}
        else{padOwner=craft;craft.holding=false;craft.time=next;}
      }else craft.time=next;
      if(craft.time>=smokeAt){if(Math.hypot(craft.position.x-route.LANDING_HOME.x,craft.position.y-route.LANDING_HOME.y)>2)craft.time=smokeAt-.01;else{craft.ground='pickup';craft.groundTime=0;craft.position={...route.LANDING_HOME};continue;}}
      const frame=route.sampleFlight(craft.time,craft.trip,craft.target.point());
      if(frame.phase==='outbound'&&padOwner===craft)padOwner=null;
      const wanted=craft.holding?route.holdingPoint(craft.serial,fleetClock):frame;
      const others=fleet.filter(other=>other!==craft&&!other.finished).map(other=>other.position);if(smoothed)others.push(smoothed);
      craft.position=route.separatedStep(craft.position,wanted,others,dt*.09);
      const angle=craft.holding?-90+Math.sin(fleetClock*.006+craft.trip)*4:frame.angle;
      craft.node.setAttribute('transform',`translate(${craft.position.x} ${craft.position.y}) rotate(${angle})`);
      const engine=craft.node.querySelector('.rocket-engine'),thrust=craft.holding?.8:frame.engine;
      engine.setAttribute('transform',`translate(${route.ROCKETS[frame.variant].tail} 0) rotate(${craft.holding?Math.sin(fleetClock*.008+craft.trip)*12:frame.gimbal})`);engine.style.opacity=String(thrust);
      craft.node.querySelector('.rocket-flame').setAttribute('transform',`scale(${Math.max(.01,thrust)} ${.65+thrust*.35})`);
      craft.node.dataset.phase=craft.holding?'holding':frame.phase;craft.node.dataset.destination=craft.target.name;
    }
    for(let index=fleet.length-1;index>=0;index--)if(fleet[index].finished){fleet[index].node.remove();fleet[index].carrier.remove();fleet.splice(index,1);}
    if(!primaryBusy&&!padOwner&&fleet.length<route.FLEET_LIMIT-1&&fleetClock>=nextLaunch&&smoothed&&Math.hypot(smoothed.x-route.LANDING_HOME.x,smoothed.y-route.LANDING_HOME.y)>=route.ROCKET_CLEARANCE)launchFleet();
  }
  function paint(frame,instant=false,dt=1000/60){
    const factor=!instant&&['outbound','orbit','return'].includes(frame.phase)?.22:1;
    if(!smoothed)smoothed={x:frame.x,y:frame.y};const dx=frame.x-smoothed.x,dy=frame.y-smoothed.y,distance=Math.hypot(dx,dy),step=instant?1:Math.min(factor,Math.max(.01,dt)*.09/(distance||1));
    const target={x:smoothed.x+dx*step,y:smoothed.y+dy*step};
    if(!instant&&['outbound','orbit','return','lift','descend'].includes(frame.phase))smoothed=route.separatedStep(smoothed,target,fleet.filter(craft=>!craft.finished).map(craft=>craft.position),Math.max(.01,dt)*.09);
    else smoothed=target;
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
  function sample(){return route.sampleFlight(media.matches?route.flightPlan(cycle).stages.find(stage=>stage.name==='rest').start:elapsed,cycle,manual?moon:destination.point());}
  function advance(dt,now){
    const plan=route.flightPlan(cycle),landing=stageStart(plan,'descend'),returnAt=stageStart(plan,'return'),next=elapsed+dt,holding=elapsed<returnAt&&next>=returnAt&&!!padOwner;
    const touchdown=stageStart(plan,'smoke'),stillApproaching=elapsed<touchdown&&next>=touchdown&&smoothed&&Math.hypot(smoothed.x-route.LANDING_HOME.x,smoothed.y-route.LANDING_HOME.y)>2;
    if(holding)elapsed=returnAt-.01;else if(stillApproaching)elapsed=touchdown-.01;else elapsed=next;
    while(elapsed>=route.flightPlan(cycle).duration){elapsed-=route.flightPlan(cycle).duration;cycle++;destination=destinations[Math.floor(Math.random()*destinations.length)];smoothed=null;}
    const frame=sample();if(holding){Object.assign(frame,route.holdingPoint(0,fleetClock));frame.angle=-90+Math.sin(now*.006)*4;frame.gimbal=Math.sin(now*.008)*12;frame.engine=.8;}
    advanceFleet(dt,frame.phase);paint(frame,false,dt);svg.dataset.destination=destination.name;svg.dataset.holding=String(holding);
  }
  function tick(now){
    request=0;if(disposed||!svg.isConnected)return;
    if(!document.hidden&&!media.matches&&!manual)advance(previous?Math.min(now-previous,60):0,now);
    previous=now;if(!media.matches&&!document.hidden&&!manual)request=requestAnimationFrame(tick);
  }
  function resume(){if(disposed)return;if(request)cancelAnimationFrame(request);request=0;previous=0;svg.dataset.paused=String(document.hidden);if(media.matches){clearFleet();paint(sample(),true);}else if(!document.hidden&&!manual)request=requestAnimationFrame(tick);}
  function moonPoint(event){const point=new DOMPoint(event.clientX,event.clientY);return point.matrixTransform(svg.getScreenCTM().inverse());}
  function moveMoon(x,y){
    const bounds=svg.getScreenCTM(),inverse=bounds.inverse(),margin=48;
    const low=new DOMPoint(margin,margin).matrixTransform(inverse),high=new DOMPoint(window.innerWidth-margin,window.innerHeight-margin).matrixTransform(inverse);
    moon={x:Math.max(low.x,Math.min(high.x,x)),y:Math.max(low.y,Math.min(high.y,y))};moonElement.setAttribute('transform',`translate(${moon.x-route.DEFAULT_MOON.x} ${moon.y-route.DEFAULT_MOON.y})`);
  }
  function down(event){if(event.button!==0)return;event.preventDefault();const point=moonPoint(event);dragging={id:event.pointerId,x:point.x-moon.x,y:point.y-moon.y};moonElement.setPointerCapture(event.pointerId);}
  function move(event){if(dragging?.id!==event.pointerId)return;event.preventDefault();const point=moonPoint(event);moveMoon(point.x-dragging.x,point.y-dragging.y);}
  function up(event){if(dragging?.id===event.pointerId){if(moonElement.hasPointerCapture(event.pointerId))moonElement.releasePointerCapture(event.pointerId);dragging=null;}}
  function key(event){const delta=event.shiftKey?20:5,offset={ArrowLeft:[-delta,0],ArrowRight:[delta,0],ArrowUp:[0,-delta],ArrowDown:[0,delta]}[event.key];if(offset){event.preventDefault();moveMoon(moon.x+offset[0],moon.y+offset[1]);}else if(event.key==='Escape'){event.preventDefault();moveMoon(route.DEFAULT_MOON.x,route.DEFAULT_MOON.y);}}
  function resize(){moveMoon(moon.x,moon.y);}
  moonElement.addEventListener('pointerdown',down);moonElement.addEventListener('pointermove',move);moonElement.addEventListener('pointerup',up);moonElement.addEventListener('pointercancel',up);moonElement.addEventListener('keydown',key);
  document.addEventListener('visibilitychange',resume);media.addEventListener('change',resume);
  window.addEventListener('resize',resize);
  paint(sample(),true);resume();
  landingFlight={
    seek(milliseconds,nextCycle=cycle){manual=true;clearFleet();elapsed=milliseconds;cycle=nextCycle;smoothed=null;paint(sample(),true);resume();return {...sample(),moon:{...moon}};},
    seekPhase(name,fraction=.5,nextCycle=cycle){const stage=route.flightPlan(nextCycle).stages.find(stage=>stage.name===name);if(!stage)throw new Error('Unknown landing phase');return this.seek(stage.start+stage.duration*Math.max(0,Math.min(.999999,fraction)),nextCycle);},
    simulate(milliseconds){manual=true;if(request)cancelAnimationFrame(request);request=0;let minimum=Infinity;for(let time=0;time<milliseconds;time+=50){advance(Math.min(50,milliseconds-time),time);const positions=[smoothed,...fleet.filter(craft=>!craft.finished).map(craft=>craft.position)];for(let a=0;a<positions.length;a++)for(let b=a+1;b<positions.length;b++)minimum=Math.min(minimum,Math.hypot(positions[a].x-positions[b].x,positions[a].y-positions[b].y));}return {phase:sample().phase,holding:svg.dataset.holding==='true',position:{...smoothed},minimum:Number.isFinite(minimum)?minimum:null,fleet:fleet.map(craft=>({serial:craft.serial,phase:craft.node.dataset.phase,destination:craft.target.name,position:{...craft.position},groundTime:craft.groundTime})),pad:padOwner?fleet.indexOf(padOwner)+1:0};},
    play(){manual=false;resume();},snapshot:()=>({...sample(),x:smoothed?.x??sample().x,y:smoothed?.y??sample().y,moon:{...moon},elapsed,cycle,destination:destination.name}),
    dispose(){disposed=true;clearFleet();if(request)cancelAnimationFrame(request);moonElement.removeEventListener('pointerdown',down);moonElement.removeEventListener('pointermove',move);moonElement.removeEventListener('pointerup',up);moonElement.removeEventListener('pointercancel',up);moonElement.removeEventListener('keydown',key);document.removeEventListener('visibilitychange',resume);media.removeEventListener('change',resume);window.removeEventListener('resize',resize);},
  };
}
