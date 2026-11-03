const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const route=import('data:text/javascript;base64,'+fs.readFileSync(path.join(__dirname,'../web/landing_route.js')).toString('base64'));
const at=(plan,name,fraction=.5)=>{const stage=plan.stages.find(stage=>stage.name===name);return stage.start+stage.duration*fraction;};

test('attitude turns gradually across angle wrap and uses smooth, seeded variation',async()=>{
  const {turnAttitude}=await route;
  let angle=179;
  for(let time=0;time<3000;time+=50){const next=turnAttitude(angle,-179,50,time,0,0);assert.ok(Math.abs(next-angle)<=7.5);angle=next;}
  assert.ok(Math.abs(angle-181)<.01);
  let noisy=0,plain=0,variation=0;
  for(let time=0;time<5000;time+=50){
    const next=turnAttitude(noisy,90,50,time,2),without=turnAttitude(plain,90,50,time,2,0);
    assert.ok(Math.abs(next-noisy)<=7.5);variation=Math.max(variation,Math.abs(next-without));noisy=next;plain=without;
  }
  assert.ok(variation>.1&&variation<3);
  assert.equal(turnAttitude(20,180,0,0),20);
});

test('orbit attitude catches up smoothly and stays tangent across full revolutions',async()=>{
  const {orbitAttitude}=await route,centre={x:10,y:20};
  let angle=-90;
  for(let time=0;time<8100;time+=50){
    const orbit=Math.PI+time/2700*2*Math.PI,position={x:centre.x+34*Math.cos(orbit),y:centre.y+34*Math.sin(orbit)};
    const next=orbitAttitude(angle,position,centre,50),target=orbit*180/Math.PI+90;
    assert.ok(Math.abs(next-angle)<=13.5+1e-9);
    if(time>1500)assert.ok(Math.abs(((target-next)%360+540)%360-180)<1e-6);
    angle=next;
  }
  assert.equal(orbitAttitude(20,centre,centre,50),20);
});

test('outbound travel is slower and holding points drift away from the pad',async()=>{
  const {flightPlan,holdingPoint,LANDING_HOME,FLEET_LIMIT}=await route;
  assert.equal(FLEET_LIMIT,2);
  assert.equal(flightPlan().stages.find(stage=>stage.name==='outbound').duration,6800);
  const a=holdingPoint(1,0),b=holdingPoint(1,5000),c=holdingPoint(2,0);
  assert.ok(Math.hypot(a.x-LANDING_HOME.x,a.y-LANDING_HOME.y)>60);
  assert.ok(Math.hypot(a.x-b.x,a.y-b.y)>1);
  assert.ok(Math.hypot(a.x-c.x,a.y-c.y)>30);
});

test('opposing rockets steer without crossing the clearance boundary',async()=>{
  const {separatedStep,ROCKET_CLEARANCE}=await route;
  let a={x:0,y:0},b={x:100,y:0};
  for(let i=0;i<80;i++){
    a=separatedStep(a,{x:100,y:0},[b],2);
    b=separatedStep(b,{x:0,y:0},[a],2);
    assert.ok(Math.hypot(a.x-b.x,a.y-b.y)>=ROCKET_CLEARANCE-1e-9);
  }
});

test('launch wobbles on the support, lifts vertically, and lands under gimballed thrust',async()=>{
  const {flightPlan,sampleFlight,LANDING_HOME}=await route,plan=flightPlan();
  const rest=sampleFlight(at(plan,'rest')),ignite=sampleFlight(at(plan,'ignite',.375)),lift=sampleFlight(at(plan,'lift'));
  assert.equal(rest.engine,0);assert.equal(rest.arms,1);
  assert.equal(ignite.y,LANDING_HOME.y);assert.ok(Math.abs(ignite.angle+90)>0&&Math.abs(ignite.angle+90)<3);assert.ok(ignite.engine>0);
  assert.equal(lift.x,LANDING_HOME.x);assert.ok(lift.y<LANDING_HOME.y);assert.ok(lift.engine>0);
  const landing=sampleFlight(at(plan,'descend',.3)),touchdown=sampleFlight(at(plan,'smoke',0));
  assert.ok(landing.engine>0);assert.ok(Math.abs(landing.gimbal)>0);
  assert.equal(touchdown.x,LANDING_HOME.x);assert.equal(touchdown.y,LANDING_HOME.y);assert.equal(touchdown.angle,-90);assert.equal(touchdown.engine,0);assert.equal(touchdown.arms,1);
});

test('coasts for exactly two through five orbits around the current moon and relights to return',async()=>{
  const {flightPlan,sampleFlight,ORBIT_RADIUS}=await route,moon={x:-150,y:220};
  for(let cycle=0;cycle<4;cycle++){
    const plan=flightPlan(cycle);assert.equal(plan.orbits,cycle+2);
    for(const fraction of [.2,.4,.6,.8]){
      const point=sampleFlight(at(plan,'orbit',fraction),cycle,moon);
      assert.ok(Math.abs(Math.hypot(point.x-moon.x,point.y-moon.y)-ORBIT_RADIUS)<=2);
      assert.equal(point.engine,0);assert.ok(Math.abs(point.orbitTurns-plan.orbits*fraction)<1e-8);
    }
    assert.equal(sampleFlight(at(plan,'return'),cycle,moon).engine,1);
  }
});

test('outbound and return follow opposite, non-looping elliptical arcs into and out of orbit',async()=>{
  const {flightPlan,sampleFlight,LANDING_HOME,ORBIT_RADIUS}=await route,plan=flightPlan();
  for(const moon of [{x:110,y:24},{x:319,y:33}])for(const stage of ['outbound','return']){
    const top={x:LANDING_HOME.x,y:LANDING_HOME.y-48},entry={x:moon.x-ORBIT_RADIUS,y:moon.y};
    const start=stage==='outbound'?top:entry,end=stage==='outbound'?entry:top,dx=end.x-start.x,dy=end.y-start.y,length=dx*dx+dy*dy;
    let previous=-1;
    for(let i=0;i<=100;i++){
      const point=sampleFlight(at(plan,stage,i===100?.999999:i/100),0,moon);
      const progress=((point.x-start.x)*dx+(point.y-start.y)*dy)/length;
      assert.ok(progress>=previous-1e-8&&progress<=1+1e-8);previous=progress;
      if(i>0&&i<100)assert.ok(dx*(point.y-start.y)-dy*(point.x-start.x)>0);
    }
    assert.ok(previous>.9999);
  }
});

test('Perlin route variation stays continuous and returns to the same landing support',async()=>{
  const {flightPlan,sampleFlight,perlin1D,LANDING_HOME}=await route,plan=flightPlan();
  const one=sampleFlight(at(plan,'outbound'),0),two=sampleFlight(at(plan,'outbound'),1);
  assert.ok(Math.hypot(one.x-two.x,one.y-two.y)>.01);
  assert.ok(Math.abs(perlin1D(1-1e-6)-perlin1D(1+1e-6))<1e-4);
  for(let cycle=0;cycle<4;cycle++)for(let t=0;t<flightPlan(cycle).duration;t+=75){const frame=sampleFlight(t,cycle);assert.ok(['x','y','angle','engine','gimbal','arms'].every(key=>Number.isFinite(frame[key])));}
  assert.equal(sampleFlight(at(plan,'pause')).y,LANDING_HOME.y);
});

test('carrier delivers and releases a supported rocket before leaving the pad',async()=>{
  const {flightPlan,sampleFlight,LANDING_HOME}=await route,plan=flightPlan();
  const arriving=sampleFlight(at(plan,'delivery',.5)),unloaded=sampleFlight(at(plan,'carrier-exit',0)),ready=sampleFlight(at(plan,'rest'));
  assert.equal(arriving.vehicle.kind,'carrier');assert.equal(arriving.x,arriving.vehicle.x);assert.equal(arriving.engine,0);
  assert.ok(arriving.x<LANDING_HOME.x);assert.equal(unloaded.x,LANDING_HOME.x);assert.equal(unloaded.y,LANDING_HOME.y);assert.equal(unloaded.arms,1);
  assert.equal(ready.vehicle,null);assert.ok(ready.fins>.9);assert.equal(sampleFlight(at(plan,'ignite',0)).fins,0);
});

test('refuelling connects, fills and vents steam, disconnects, then backs away before relaunch',async()=>{
  const {flightPlan,sampleFlight}=await route,plan=flightPlan(1),frame=(name,t=.5)=>sampleFlight(at(plan,name,t),1);
  assert.equal(plan.damaged,false);assert.equal(plan.replacement,true);
  assert.equal(frame('tanker-arrive').vehicle.kind,'tanker');
  assert.ok(frame('connect').hose>0&&frame('connect').hose<1);
  assert.equal(frame('refuel').hose,1);assert.ok(frame('refuel').steam>0);assert.equal(frame('refuel').engine,0);
  assert.ok(frame('refuel',.9).fuel>frame('refuel',.1).fuel);
  assert.ok(frame('disconnect',.9).hose<frame('disconnect',.1).hose);
  assert.equal(frame('tanker-back').hose,0);assert.ok(frame('tanker-back',.9).vehicle.x<frame('tanker-back',.1).vehicle.x);
  assert.equal(frame('tanker-exit').steam,0);assert.ok(frame('ignite').exhaust>0);assert.ok(frame('lift').exhaust>0);
});

test('occasional broken panels leave on a recovery truck and replacements cycle through six distinct rockets',async()=>{
  const {flightPlan,sampleFlight,ROCKETS,LANDING_HOME}=await route,seen=new Set();
  assert.equal(new Set(ROCKETS.map(rocket=>rocket.id)).size,6);assert.equal(ROCKETS.at(-1).id,'starship');
  for(let cycle=0;cycle<18;cycle++){
    const plan=flightPlan(cycle),frame=(name,t=.5)=>sampleFlight(at(plan,name,t),cycle);
    seen.add(frame('rest').variant);
    const taken=frame('recovery-exit'),fresh=frame('replacement-unload',.999999),next=sampleFlight(0,cycle+1);
    if(plan.damaged){const old=frame('fault');assert.ok(old.broken&&old.debris.y>LANDING_HOME.y);}
    assert.equal(taken.vehicle.kind,'recovery');assert.equal(taken.x,taken.vehicle.x);assert.equal(taken.broken,plan.damaged);
    assert.notEqual(taken.variant,fresh.variant);assert.equal(next.variant,fresh.variant);assert.equal(fresh.engine,0);assert.ok(!fresh.broken);assert.equal(fresh.x,next.x);assert.ok(Math.abs(fresh.y-next.y)<.0001);
  }
  assert.equal(seen.size,6);
});
