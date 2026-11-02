// A deterministic flight sampler keeps motion testable without real-time waits.
export const LANDING_HOME={x:46,y:112},DEFAULT_MOON={x:319,y:33},ORBIT_RADIUS=34;
const clamp=value=>Math.max(0,Math.min(1,value));
const smooth=value=>{const t=clamp(value);return t*t*(3-2*t);};
const mix=(a,b,t)=>a+(b-a)*t;
export function perlin1D(value,seed=0){
  const index=Math.floor(value),x=value-index;
  const gradient=i=>{let h=Math.imul(i+seed*1013,0x45d9f3b);h=Math.imul(h^(h>>>16),0x45d9f3b);return (h&1)?1:-1;};
  const fade=x*x*x*(x*(x*6-15)+10);
  return mix(gradient(index)*x,gradient(index+1)*(x-1),fade)*2;
}
export function flightPlan(cycle=0){
  const orbits=2+(Math.abs(cycle)%4),stages=[['rest',2500],['ignite',1000],['lift',1600],['outbound',3400],['orbit',orbits*2700],['return',4000],['descend',2300],['smoke',1400],['pause',1200]];
  let at=0;return {orbits,duration:stages.reduce((sum,stage)=>sum+stage[1],0),stages:stages.map(([name,duration])=>{const stage={name,start:at,duration};at+=duration;return stage;})};
}
function cubic(a,b,c,d,t){const u=1-t;return {x:u*u*u*a.x+3*u*u*t*b.x+3*u*t*t*c.x+t*t*t*d.x,y:u*u*u*a.y+3*u*u*t*b.y+3*u*t*t*c.y+t*t*t*d.y};}
function routePoint(stage,t,moon,cycle){
  const home=LANDING_HOME,top={x:home.x,y:home.y-48},entry={x:moon.x-ORBIT_RADIUS,y:moon.y};
  const point=stage==='outbound'?cubic(top,{x:top.x+35,y:top.y-65},{x:entry.x,y:entry.y+65},entry,t):cubic(entry,{x:entry.x,y:entry.y-70},{x:top.x,y:top.y-70},top,t);
  const envelope=Math.sin(Math.PI*t),noise=perlin1D(t*3.7+cycle*.371+(stage==='return'?5:0),cycle+19);
  point.x+=noise*13*envelope;point.y+=perlin1D(t*4.1+9+cycle*.217,cycle+47)*10*envelope;
  return point;
}
function routeAngle(stage,t,moon,cycle){const a=routePoint(stage,Math.max(0,t-.001),moon,cycle),b=routePoint(stage,Math.min(1,t+.001),moon,cycle);return Math.atan2(b.y-a.y,b.x-a.x)*180/Math.PI;}
function angleMix(angle,target,t){const delta=((target-angle+540)%360)-180;return angle+delta*t;}
export function sampleFlight(milliseconds,cycle=0,moon=DEFAULT_MOON){
  const plan=flightPlan(cycle),time=Math.max(0,Math.min(plan.duration-.0001,milliseconds)),stage=plan.stages.find(stage=>time<stage.start+stage.duration)||plan.stages.at(-1),t=clamp((time-stage.start)/stage.duration);
  const frame={...LANDING_HOME,angle:-90,engine:0,gimbal:0,arms:1,smoke:0,phase:stage.name,orbits:plan.orbits,orbitTurns:0};
  switch(stage.name){
    case 'ignite':{const wobble=Math.sin(t*Math.PI*12)*Math.sin(Math.PI*t);frame.x+=wobble*.55;frame.angle+=wobble*2;frame.engine=smooth(t*2);frame.arms=1-smooth((t-.45)/.5);break;}
    case 'lift':frame.y-=48*smooth(t);frame.engine=1;frame.arms=0;break;
    case 'outbound':Object.assign(frame,routePoint('outbound',t,moon,cycle));frame.angle=routeAngle('outbound',t,moon,cycle);frame.engine=1;frame.arms=0;break;
    case 'orbit':{const angle=Math.PI+Math.PI*2*plan.orbits*t;frame.x=moon.x+Math.cos(angle)*ORBIT_RADIUS;frame.y=moon.y+Math.sin(angle)*ORBIT_RADIUS;frame.angle=angle*180/Math.PI+90;frame.engine=1-smooth((time-stage.start)/420);frame.arms=0;frame.orbitTurns=plan.orbits*t;break;}
    case 'return':Object.assign(frame,routePoint('return',t,moon,cycle));frame.angle=angleMix(routeAngle('return',t,moon,cycle),-90,smooth((t-.55)/.4));frame.engine=smooth(t*10);frame.gimbal=Math.sin(t*11)*7*Math.sin(Math.PI*t);frame.arms=smooth((t-.65)/.35)*.2;break;
    case 'descend':{const settle=1-smooth(t);frame.x+=Math.sin(t*Math.PI*3)*4*settle;frame.y-=48*settle;frame.angle+=Math.sin(t*Math.PI*3)*4*settle;frame.engine=.7+.3*settle;frame.gimbal=-Math.sin(t*Math.PI*4)*10*settle;frame.arms=smooth((t-.25)/.75);break;}
    case 'smoke':frame.smoke=t;break;
  }
  return frame;
}
