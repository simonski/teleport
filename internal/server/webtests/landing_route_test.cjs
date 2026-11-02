const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const route=import('data:text/javascript;base64,'+fs.readFileSync(path.join(__dirname,'../web/landing_route.js')).toString('base64'));
const at=(plan,name,fraction=.5)=>{const stage=plan.stages.find(stage=>stage.name===name);return stage.start+stage.duration*fraction;};

test('launch wobbles on the support, lifts vertically, and lands under gimballed thrust',async()=>{
  const {flightPlan,sampleFlight,LANDING_HOME}=await route,plan=flightPlan();
  const rest=sampleFlight(0),ignite=sampleFlight(at(plan,'ignite',.375)),lift=sampleFlight(at(plan,'lift'));
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
      assert.ok(Math.abs(Math.hypot(point.x-moon.x,point.y-moon.y)-ORBIT_RADIUS)<1e-8);
      assert.equal(point.engine,0);assert.ok(Math.abs(point.orbitTurns-plan.orbits*fraction)<1e-8);
    }
    assert.equal(sampleFlight(at(plan,'return'),cycle,moon).engine,1);
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
