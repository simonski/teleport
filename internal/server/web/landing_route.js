// A deterministic flight sampler keeps motion testable without real-time waits.
export const LANDING_HOME={x:46,y:112},DEFAULT_MOON={x:319,y:33},ORBIT_RADIUS=34;
export const FLEET_LIMIT=2,ROCKET_CLEARANCE=30;
export const ROCKETS=[
  {id:'dart',name:'Dart',tail:-10,fins:true},
  {id:'capsule',name:'Capsule',tail:-10,fins:false},
  {id:'booster',name:'Twin booster',tail:-12,fins:true},
  {id:'needle',name:'Needle',tail:-13,fins:true},
  {id:'shuttle',name:'Shuttle',tail:-11,fins:true},
  {id:'starship',name:'Starship',tail:-15,fins:true},
];
const clamp=value=>Math.max(0,Math.min(1,value));
const smooth=value=>{const t=clamp(value);return t*t*(3-2*t);};
const mix=(a,b,t)=>a+(b-a)*t;
export function perlin1D(value,seed=0){
  const index=Math.floor(value),x=value-index;
  const gradient=i=>{let h=Math.imul(i+seed*1013,0x45d9f3b);h=Math.imul(h^(h>>>16),0x45d9f3b);return (h&1)?1:-1;};
  const fade=x*x*x*(x*(x*6-15)+10);
  return mix(gradient(index)*x,gradient(index+1)*(x-1),fade)*2;
}
// Follow the shortest turn with a soft response and a strict angular speed limit.
export function turnAttitude(current,target,dt,clock,seed=0,noise=1){
  const desired=target+perlin1D(clock/2200+seed*.37,seed+73)*3*noise;
  const difference=((desired-current)%360+540)%360-180;
  const step=difference*(1-Math.exp(-Math.max(0,dt)/450));
  const limit=Math.max(0,dt)*.15;
  return current+Math.max(-limit,Math.min(limit,step));
}
// Catch up to the actual orbital tangent, then track it without trailing it.
export function orbitAttitude(current,position,centre,dt){
  const dx=position.x-centre.x,dy=position.y-centre.y;
  if(Math.hypot(dx,dy)<.001)return current;
  const target=Math.atan2(dy,dx)*180/Math.PI+90;
  const difference=((target-current)%360+540)%360-180,limit=Math.max(0,dt)*.27;
  return current+Math.max(-limit,Math.min(limit,difference));
}
export function holdingPoint(serial,clock){
  const lane=serial%2,drift=clock/9000;
  return {x:130+lane*55+perlin1D(drift+serial*.37,serial+7)*12,y:-30+lane*35+perlin1D(drift+.4,serial+13)*9};
}
// Choose a bounded step that maintains clearance from every other rocket.
export function separatedStep(position,target,others,maxDistance,clearance=ROCKET_CLEARANCE){
  const dx=target.x-position.x,dy=target.y-position.y,distance=Math.hypot(dx,dy);
  if(!distance||maxDistance<=0)return {...position};
  const ux=dx/distance,uy=dy/distance,step=Math.min(distance,maxDistance);
  const directions=[[ux,uy],[-uy,ux],[uy,-ux],[-ux,-uy]];
  for(const [x,y] of directions){
    const next={x:position.x+x*step,y:position.y+y*step};
    if(others.every(other=>{
      const before=Math.hypot(position.x-other.x,position.y-other.y),after=Math.hypot(next.x-other.x,next.y-other.y);
      return before<clearance?after>before:after>=clearance;
    }))return next;
  }
  return {...position};
}
export function flightPlan(cycle=0){
  const orbits=2+(Math.abs(cycle)%4),replacement=true,damaged=cycle%3===2;
  const stages=[...(cycle===0?[['delivery',2400],['unload',1100],['carrier-exit',1400]]:[]),['rest',1800],['ignite',1000],['lift',1600],['outbound',6800],['orbit',orbits*2700],['return',4000],['descend',2300],['smoke',1400],['pause',1000],
    ...(damaged?[['fault',1200]]:[['tanker-arrive',1700],['connect',900],['refuel',2400],['disconnect',900],['tanker-back',1000],['tanker-exit',1300]]),
    ['recovery-arrive',1700],['recovery-load',1400],['recovery-exit',1800],['replacement-arrive',2400],['replacement-unload',1100],['replacement-exit',1400]];
  let at=0;return {orbits,replacement,damaged,duration:stages.reduce((sum,stage)=>sum+stage[1],0),stages:stages.map(([name,duration])=>{const stage={name,start:at,duration};at+=duration;return stage;})};
}
function routePoint(stage,t,moon,cycle){
  const home=LANDING_HOME,top={x:home.x,y:home.y-48},entry={x:moon.x-ORBIT_RADIUS,y:moon.y};
  const start=stage==='outbound'?top:entry,end=stage==='outbound'?entry:top;
  const dx=end.x-start.x,dy=end.y-start.y,distance=Math.hypot(dx,dy)||1;
  const progress=(1-Math.cos(Math.PI*t))/2,envelope=Math.sin(Math.PI*t);
  const arc=Math.min(45,Math.max(12,distance*.26)),noise=perlin1D(t*3.7+cycle*.371+(stage==='return'?5:0),cycle+19)*4;
  const offset=envelope*(arc+noise);
  return {x:start.x+dx*progress-dy/distance*offset,y:start.y+dy*progress+dx/distance*offset};
}
function routeAngle(stage,t,moon,cycle){const a=routePoint(stage,Math.max(0,t-.001),moon,cycle),b=routePoint(stage,Math.min(1,t+.001),moon,cycle);return Math.atan2(b.y-a.y,b.x-a.x)*180/Math.PI;}
function angleMix(angle,target,t){const delta=((target-angle+540)%360)-180;return angle+delta*t;}
export function sampleFlight(milliseconds,cycle=0,moon=DEFAULT_MOON){
  const plan=flightPlan(cycle),time=Math.max(0,Math.min(plan.duration-.0001,milliseconds)),stage=plan.stages.find(stage=>time<stage.start+stage.duration)||plan.stages.at(-1),t=clamp((time-stage.start)/stage.duration);
  const variant=cycle%ROCKETS.length;
  const frame={...LANDING_HOME,angle:-90,engine:0,gimbal:0,arms:1,smoke:0,exhaust:0,steam:0,fins:0,fuel:1,hose:0,vehicle:null,debris:null,broken:false,opacity:1,variant,phase:stage.name,orbits:plan.orbits,orbitTurns:0};
  const truck=(kind,x,load=0,opacity=1)=>({kind,x,y:137,load,opacity});
  const delivered=stage.name.startsWith('replacement-');
  if(delivered)frame.variant=(variant+1)%ROCKETS.length;
  if(['delivery','replacement-arrive'].includes(stage.name)){
    frame.vehicle=truck('carrier',mix(-78,LANDING_HOME.x,smooth(t)),1,smooth(t*5));frame.x=frame.vehicle.x;frame.y-=4;frame.arms=0;frame.opacity=frame.vehicle.opacity;
  }else if(['unload','replacement-unload'].includes(stage.name)){
    frame.vehicle=truck('carrier',LANDING_HOME.x,1-smooth(t));frame.y-=4*(1-smooth(t));frame.arms=smooth(t);
  }else if(['carrier-exit','replacement-exit'].includes(stage.name)){
    frame.vehicle=truck('carrier',mix(LANDING_HOME.x,-78,smooth(t)),0,1-smooth((t-.7)/.3));frame.fins=Math.sin(Math.PI*t)*.65;
  }
  switch(stage.name){
    case 'rest':frame.fins=Math.sin(Math.PI*t)**2;break;
    case 'ignite':{const wobble=Math.sin(t*Math.PI*12)*Math.sin(Math.PI*t);frame.x+=wobble*.55;frame.angle+=wobble*2;frame.engine=smooth(t*2);frame.exhaust=smooth(t);frame.arms=1-smooth((t-.45)/.5);break;}
    case 'lift':frame.y-=48*smooth(t);frame.engine=1;frame.arms=0;frame.exhaust=1-smooth(t);break;
    case 'outbound':Object.assign(frame,routePoint('outbound',t,moon,cycle));frame.angle=routeAngle('outbound',t,moon,cycle);frame.engine=1;frame.arms=0;break;
    case 'orbit':{const angle=Math.PI+Math.PI*2*plan.orbits*t,radius=ORBIT_RADIUS+perlin1D(plan.orbits*t*1.3+cycle*.37,cycle+83)*2*Math.sin(Math.PI*t)**2;frame.x=moon.x+Math.cos(angle)*radius;frame.y=moon.y+Math.sin(angle)*radius;frame.angle=angle*180/Math.PI+90;frame.engine=1-smooth((time-stage.start)/420);frame.arms=0;frame.orbitTurns=plan.orbits*t;break;}
    case 'return':Object.assign(frame,routePoint('return',t,moon,cycle));frame.angle=angleMix(routeAngle('return',t,moon,cycle),-90,smooth((t-.55)/.4));frame.engine=smooth(t*10);frame.gimbal=Math.sin(t*11)*7*Math.sin(Math.PI*t);frame.arms=smooth((t-.65)/.35)*.2;break;
    case 'descend':{const settle=1-smooth(t);frame.x+=Math.sin(t*Math.PI*3)*4*settle;frame.y-=48*settle;frame.angle+=Math.sin(t*Math.PI*3)*4*settle;frame.engine=.7+.3*settle;frame.gimbal=-Math.sin(t*Math.PI*4)*10*settle;frame.arms=smooth((t-.25)/.75);break;}
    case 'smoke':frame.smoke=t;break;
    case 'tanker-arrive':frame.vehicle=truck('tanker',mix(-78,2,smooth(t)),0,smooth(t*5));frame.fuel=.12;break;
    case 'connect':frame.vehicle=truck('tanker',2);frame.hose=smooth(t);frame.fuel=.12;frame.steam=smooth((t-.6)/.4)*.35;break;
    case 'refuel':frame.vehicle=truck('tanker',2);frame.hose=1;frame.fuel=mix(.12,1,smooth(t));frame.steam=Math.sin(Math.PI*t)*.65+.25;break;
    case 'disconnect':frame.vehicle=truck('tanker',2);frame.hose=1-smooth(t);frame.steam=(1-smooth(t))*.45;break;
    case 'tanker-back':frame.vehicle=truck('tanker',mix(2,-18,smooth(t)));break;
    case 'tanker-exit':frame.vehicle=truck('tanker',mix(-18,-78,smooth(t)),0,1-smooth((t-.7)/.3));break;
    case 'fault':frame.broken=t>.12;frame.debris={x:LANDING_HOME.x+7+10*smooth(t),y:LANDING_HOME.y+3+17*smooth(t),angle:110*smooth(t),opacity:smooth(t*8)};break;
    case 'recovery-arrive':frame.broken=plan.damaged;if(plan.damaged)frame.debris={x:63,y:132,angle:110,opacity:1};frame.vehicle=truck('recovery',mix(-78,LANDING_HOME.x,smooth(t)),0,smooth(t*5));break;
    case 'recovery-load':frame.broken=plan.damaged;frame.vehicle=truck('recovery',LANDING_HOME.x,smooth(t));frame.angle=mix(-90,-25,smooth(t));frame.y=mix(LANDING_HOME.y,123,smooth(t));frame.arms=1-smooth(t);if(plan.damaged)frame.debris={x:mix(63,34,smooth(t)),y:mix(132,129,smooth(t)),angle:110,opacity:1};break;
    case 'recovery-exit':frame.broken=plan.damaged;frame.vehicle=truck('recovery',mix(LANDING_HOME.x,-78,smooth(t)),1,1-smooth((t-.7)/.3));frame.x=frame.vehicle.x;frame.y=123;frame.angle=-25;frame.arms=0;frame.opacity=frame.vehicle.opacity;if(plan.damaged)frame.debris={x:frame.x-12,y:129,angle:110,opacity:frame.opacity};break;
  }
  return frame;
}
