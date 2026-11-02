import * as THREE from './vendor/three.module.js';
import {CARD_WIDTH,CARD_HEIGHT,graphCurvePoints,graphNeighborhood,graphNodeIcons,layoutGraph2D} from './graph_2d_model.js';

export {layoutGraph2D};

// WebGL draws the curves and flow markers; DOM cards keep text and icons crisp,
// selectable by keyboard and available to assistive technology.
export function createGraphScene(element,graph,callbacks,retained={}) {
  const scene=new THREE.Scene(),camera=new THREE.OrthographicCamera(-1,1,1,-1,.1,100);
  camera.position.z=20;
  const renderer=new THREE.WebGLRenderer({antialias:true,alpha:true});
  renderer.setPixelRatio(Math.min(window.devicePixelRatio,2));
  renderer.domElement.tabIndex=0;
  renderer.domElement.setAttribute('role','img');
  renderer.domElement.setAttribute('aria-label','2D ticket graph. Drag blank space to pan; wheel to zoom. Arrows select, Enter opens, Alt-arrows arrange.');
  element.classList.add('graph2d-surface');
  element.append(renderer.domElement);
  const layer=document.createElement('div');layer.className='graph2d-nodes';element.append(layer);
  const nodes=new Map(graph.nodes.map(node=>[node.id,node])),positions=new Map(graph.nodes.map(node=>[node.id,{...node.position}])),cards=new Map(),edges=[];
  const reduced=window.matchMedia('(prefers-reduced-motion: reduce)');
  let selected=nodes.has(retained.selected)?retained.selected:'',hovered='',focused='',gesture=null,disposed=false,inViewport=true,loop=false,width=1,height=1,phase=0,lastFrame=0,transitionUntil=0;
  let palette,related=null;
  const particleRecords=[],matrix=new THREE.Object3D();
  function readPalette(){const style=getComputedStyle(document.documentElement);return {
    paper:new THREE.Color(style.getPropertyValue('--paper').trim()),
    line:style.getPropertyValue('--graph-connection').trim()||'#aaa1bd',
    doing:style.getPropertyValue('--graph-flow').trim()||'#67a560',
    done:style.getPropertyValue('--graph-done').trim()||'#83a58d',
    dependency:style.getPropertyValue('--graph-dependency').trim()||'#b88751',
  };}
  palette=readPalette();
  function colorFor(edge){return edge.kind==='dependency'?palette.dependency:nodes.get(edge.target)?.status==='Doing'?palette.doing:nodes.get(edge.target)?.status==='Done'?palette.done:palette.line;}
  function curveFor(edge){return new THREE.CubicBezierCurve3(...graphCurvePoints(positions.get(edge.source),positions.get(edge.target)).map(point=>new THREE.Vector3(point.x,point.y,0)));}
  for(const edge of graph.edges){
    if(!nodes.has(edge.source)||!nodes.has(edge.target))continue;
    const dashed=edge.kind==='dependency'||nodes.get(edge.source).context||nodes.get(edge.target).context;
    const material=dashed?new THREE.LineDashedMaterial({color:colorFor(edge),dashSize:6,gapSize:5,transparent:true,opacity:.7}):new THREE.LineBasicMaterial({color:colorFor(edge),transparent:true,opacity:.7});
    const curve=curveFor(edge),line=new THREE.Line(new THREE.BufferGeometry().setFromPoints(curve.getPoints(32)),material);
    line.computeLineDistances();scene.add(line);const record={...edge,curve,line,targetOpacity:.7};edges.push(record);
    if((edge.flowing||nodes.get(edge.target)?.status==='Doing')&&particleRecords.length<300)for(let index=0;index<3;index++)particleRecords.push({edge:record,offset:index/3+(edges.length%7)/21});
    if(edge.directed){const geometry=new THREE.BufferGeometry();geometry.setAttribute('position',new THREE.Float32BufferAttribute([0,0,0,-8,4,0,-8,-4,0],3));const arrow=new THREE.Mesh(geometry,new THREE.MeshBasicMaterial({color:colorFor(edge),transparent:true,opacity:.7,side:THREE.DoubleSide}));scene.add(arrow);record.arrow=arrow;}
  }
  const particles=new THREE.InstancedMesh(new THREE.CircleGeometry(2.6,12),new THREE.MeshBasicMaterial({transparent:true,opacity:.95,depthTest:false}),particleRecords.length);
  particles.frustumCulled=false;particles.renderOrder=2;scene.add(particles);
  function createCard(node){
    const card=document.createElement('button');card.type='button';card.className='graph2d-card';card.dataset.graphNode=node.id;card.dataset.kind=node.kind;card.dataset.status=node.status||'';
    if(node.context)card.classList.add('context');
    const icon=document.createElement('span');icon.className='graph2d-icon';icon.setAttribute('aria-hidden','true');
    const svg=document.createElementNS('http://www.w3.org/2000/svg','svg');svg.setAttribute('viewBox','0 0 24 24');
    const path=document.createElementNS(svg.namespaceURI,'path');path.setAttribute('d',graphNodeIcons[node.kind]||graphNodeIcons.task);svg.append(path);icon.append(svg);
    const copy=document.createElement('span');copy.className='graph2d-copy';
    const title=document.createElement('strong');title.textContent=node.title;title.title=node.title;
    const meta=document.createElement('span');meta.className='graph2d-meta';meta.textContent=node.meta||(node.ticketId?`#${node.ticketId} · ${node.kind} · ${node.status}`:'Project');
    copy.append(title,meta);
    const status=document.createElement('span');status.className='graph2d-status';status.setAttribute('aria-hidden','true');
    if(node.status==='Done')status.textContent='✓';else if(node.status==='Doing')status.textContent='↻';else if(node.status==='Cancelled')status.textContent='−';else status.textContent='·';
    card.append(icon,copy,status);card.setAttribute('aria-label',`${node.title}, ${node.kind}${node.status?', '+node.status:''}`);
    card.addEventListener('click',()=>{if(!gesture?.moved)select(node.id);});
    card.addEventListener('dblclick',()=>callbacks.open(node.id));
    card.addEventListener('pointerenter',()=>{hovered=node.id;emphasize();});
    card.addEventListener('pointerleave',()=>{hovered='';emphasize();});
    card.addEventListener('focus',()=>{focused=card.matches(':focus-visible')?node.id:'';emphasize();});
    card.addEventListener('blur',()=>{focused='';emphasize();});
    card.addEventListener('contextmenu',event=>{event.preventDefault();select(node.id);callbacks.context?.(node.id,card,{x:event.clientX,y:event.clientY});});
    layer.append(card);cards.set(node.id,card);
  }
  for(const node of graph.nodes)createCard(node);
  for(const edge of edges)if(edge.label){const label=document.createElement('span');label.className='graph2d-edge-label';label.textContent=edge.label;layer.append(label);edge.labelElement=label;}
  function positionRoutes(){for(const edge of edges){
    if(edge.arrow){const point=edge.curve.getPoint(.995),tangent=edge.curve.getTangent(.995);edge.arrow.position.set(point.x,point.y,.5);edge.arrow.rotation.z=Math.atan2(tangent.y,tangent.x);}
    if(edge.labelElement){const point=edge.curve.getPoint(.5);edge.labelElement.style.transform=`translate3d(${(point.x-camera.position.x)*camera.zoom+width/2}px,${(camera.position.y-point.y)*camera.zoom+height/2-10}px,0) translate(-50%,-50%)`;}
  }}
  function projectCards(){
    positionRoutes();
    for(const [id,card] of cards){const point=positions.get(id),x=(point.x-camera.position.x)*camera.zoom+width/2,y=(camera.position.y-point.y)*camera.zoom+height/2;
      card.style.transform=`translate3d(${x}px,${y}px,0) scale(${camera.zoom}) translate(-50%,-50%)`;
      card.hidden=x+CARD_WIDTH*camera.zoom/2<0||x-CARD_WIDTH*camera.zoom/2>width||y+CARD_HEIGHT*camera.zoom/2<0||y-CARD_HEIGHT*camera.zoom/2>height;
    }
    let grid=24*camera.zoom;if(grid<12)grid*=2**Math.ceil(Math.log2(12/grid));element.style.backgroundSize=`${grid}px ${grid}px`;
    element.style.backgroundPosition=`${width/2-camera.position.x*camera.zoom}px ${height/2+camera.position.y*camera.zoom}px`;
  }
  function updateParticles(){
    particleRecords.forEach((particle,index)=>{
      const point=particle.edge.curve.getPoint((phase+particle.offset)%1);matrix.position.set(point.x,point.y,1);matrix.updateMatrix();particles.setMatrixAt(index,matrix.matrix);
      const color=new THREE.Color(colorFor(particle.edge));if(related&&!(related.has(particle.edge.source)&&related.has(particle.edge.target)))color.lerp(palette.paper,.82);particles.setColorAt(index,color);
    });
    particles.instanceMatrix.needsUpdate=true;if(particles.instanceColor)particles.instanceColor.needsUpdate=true;
  }
  function draw(){if(disposed)return;updateParticles();renderer.render(scene,camera);}
  function syncLoop(){
    if(disposed)return;
    const active=!document.hidden&&inViewport&&!reduced.matches&&(particleRecords.length>0||performance.now()<transitionUntil);
    if(active===loop)return;loop=active;lastFrame=0;renderer.setAnimationLoop(active?animate:null);
  }
  function animate(now){
    if(disposed)return;const delta=lastFrame?Math.min((now-lastFrame)/1000,.05):0;lastFrame=now;phase=(phase+delta*.32)%1;
    for(const edge of edges){edge.line.material.opacity+=(edge.targetOpacity-edge.line.material.opacity)*.22;if(edge.arrow)edge.arrow.material.opacity=edge.line.material.opacity;}
    draw();syncLoop();
  }
  function emphasize(){
    related=graphNeighborhood(graph,hovered||focused);
    for(const [id,card] of cards){card.classList.toggle('graph2d-muted',!!related&&!related.has(id));card.classList.toggle('graph2d-related',!!related&&related.has(id));card.classList.toggle('graph2d-hovered',id===(hovered||focused));}
    for(const edge of edges){edge.targetOpacity=related?related.has(edge.source)&&related.has(edge.target)?1:.12:.7;if(reduced.matches){edge.line.material.opacity=edge.targetOpacity;if(edge.arrow)edge.arrow.material.opacity=edge.targetOpacity;}if(edge.labelElement)edge.labelElement.classList.toggle('graph2d-muted',edge.targetOpacity<.5);}
    transitionUntil=performance.now()+190;draw();syncLoop();
  }
  function select(id){selected=nodes.has(id)?id:'';for(const [key,card] of cards){card.setAttribute('aria-pressed',String(key===selected));card.classList.toggle('selected',key===selected);}callbacks.select(selected);draw();}
  function redrawEdges(){for(const edge of edges){edge.curve=curveFor(edge);edge.line.geometry.setFromPoints(edge.curve.getPoints(32));edge.line.computeLineDistances();}}
  function move(id,point){
    if(!positions.has(id))return;point=callbacks.constrain?.(point)||point;positions.set(id,{x:point.x,y:point.y,z:0});redrawEdges();projectCards();callbacks.move(id,{x:point.x,y:point.y,z:0});draw();
  }
  function updateView(){camera.updateProjectionMatrix();projectCards();draw();}
  function zoom(factor,anchor){
    const rect=renderer.domElement.getBoundingClientRect(),x=anchor?anchor.x-rect.left-width/2:0,y=anchor?height/2-(anchor.y-rect.top):0;
    const before=camera.zoom,next=Math.max(.002,Math.min(2.5,before/factor));camera.position.x+=x/before-x/next;camera.position.y+=y/before-y/next;camera.zoom=next;updateView();
  }
  function fit(){
    const points=[...positions.values()];let minX=-124,maxX=124,minY=-36,maxY=36;
    if(points.length){minX=Math.min(...points.map(p=>p.x))-CARD_WIDTH/2;maxX=Math.max(...points.map(p=>p.x))+CARD_WIDTH/2;minY=Math.min(...points.map(p=>p.y))-CARD_HEIGHT/2;maxY=Math.max(...points.map(p=>p.y))+CARD_HEIGHT/2;}
    for(const edge of edges)for(const point of edge.curve.getPoints(12)){minX=Math.min(minX,point.x-12);maxX=Math.max(maxX,point.x+12);minY=Math.min(minY,point.y-12);maxY=Math.max(maxY,point.y+12);}
    camera.position.x=(minX+maxX)/2;camera.position.y=(minY+maxY)/2;camera.zoom=Math.max(.002,Math.min(1.15,(width-80)/(maxX-minX+40),(height-80)/(maxY-minY+40)));updateView();
  }
  function world(event){const rect=element.getBoundingClientRect();return {x:camera.position.x+(event.clientX-rect.left-width/2)/camera.zoom,y:camera.position.y-(event.clientY-rect.top-height/2)/camera.zoom,z:0};}
  function down(event){
    if(event.button!==0)return;const card=event.target.closest?.('[data-graph-node]');
    if(card&&!graph.canSave)return;
    const id=card?.dataset.graphNode||'',point=world(event);
    gesture={id,startX:event.clientX,startY:event.clientY,point,origin:id?{...positions.get(id)}:{x:camera.position.x,y:camera.position.y},moved:false,pointer:event.pointerId};
    element.setPointerCapture(event.pointerId);callbacks.interacting(true);
  }
  function drag(event){
    if(!gesture)return;const dx=event.clientX-gesture.startX,dy=event.clientY-gesture.startY;
    if(!gesture.moved&&Math.hypot(dx,dy)<5)return;gesture.moved=true;
    if(gesture.id){const point={x:gesture.origin.x+dx/camera.zoom,y:gesture.origin.y-dy/camera.zoom,z:0};if(Math.abs(point.x)<=100000&&Math.abs(point.y)<=100000)move(gesture.id,point);}
    else{camera.position.x=gesture.origin.x-dx/camera.zoom;camera.position.y=gesture.origin.y+dy/camera.zoom;updateView();}
  }
  function up(event){
    if(!gesture)return;const previous=gesture;gesture=null;callbacks.interacting(false);if(element.hasPointerCapture(previous.pointer))element.releasePointerCapture(previous.pointer);
    if(previous.id&&!previous.moved&&event.type==='pointerup')select(previous.id);
  }
  function cancelGesture(){if(!gesture)return;if(gesture.id&&gesture.moved)move(gesture.id,gesture.origin);else if(gesture.moved){camera.position.x=gesture.origin.x;camera.position.y=gesture.origin.y;updateView();}up({type:'pointercancel'});}
  function wheel(event){event.preventDefault();zoom(Math.exp(Math.max(-100,Math.min(100,event.deltaY))*.002),{x:event.clientX,y:event.clientY});}
  function key(event){
    if(event.ctrlKey||event.metaKey||event.isComposing)return;
    const id=event.target.closest?.('[data-graph-node]')?.dataset.graphNode||selected;
    if(id&&['ArrowLeft','ArrowRight','ArrowUp','ArrowDown','Enter','F10'].includes(event.key)){focused=id;emphasize();}
    if(event.key==='F10'&&event.shiftKey&&id){event.preventDefault();select(id);callbacks.context?.(id,event.target);return;}
    if(event.key==='Escape'){event.preventDefault();cancelGesture();return;}
    if(event.key==='Enter'&&id){event.preventDefault();callbacks.open(id);return;}
    if(!['ArrowLeft','ArrowRight','ArrowUp','ArrowDown'].includes(event.key))return;event.preventDefault();
    if(event.altKey&&graph.canSave&&id){const point={...positions.get(id)},amount=event.shiftKey?50:10;point.x+=event.key==='ArrowRight'?amount:event.key==='ArrowLeft'?-amount:0;point.y+=event.key==='ArrowUp'?amount:event.key==='ArrowDown'?-amount:0;if(Math.abs(point.x)<=100000&&Math.abs(point.y)<=100000)move(id,point);}
    else{const ids=[...nodes.keys()],direction=['ArrowLeft','ArrowUp'].includes(event.key)?-1:1,next=ids[(ids.indexOf(id)+direction+ids.length)%ids.length];select(next);const point=positions.get(next);if(point&&cards.get(next)?.hidden){camera.position.x=point.x;camera.position.y=point.y;updateView();}cards.get(next)?.focus({preventScroll:true});}
  }
  function appearance(){palette=readPalette();for(const edge of edges){edge.line.material.color.set(colorFor(edge));if(edge.arrow)edge.arrow.material.color.set(colorFor(edge));}draw();}
  function visibility(){if(reduced.matches||performance.now()>transitionUntil){for(const edge of edges){edge.line.material.opacity=edge.targetOpacity;if(edge.arrow)edge.arrow.material.opacity=edge.targetOpacity;}draw();}syncLoop();}
  function leavePointer(){hovered='';emphasize();}
  const resize=new ResizeObserver(()=>{width=Math.max(1,element.clientWidth);height=Math.max(1,element.clientHeight);camera.left=-width/2;camera.right=width/2;camera.top=height/2;camera.bottom=-height/2;renderer.setSize(width,height,false);updateView();});
  resize.observe(element);
  const intersection=new IntersectionObserver(entries=>{inViewport=entries[0]?.isIntersecting??true;syncLoop();});intersection.observe(element);
  element.addEventListener('pointerdown',down);element.addEventListener('pointermove',drag);element.addEventListener('pointerup',up);element.addEventListener('pointercancel',cancelGesture);element.addEventListener('wheel',wheel,{passive:false});element.addEventListener('keydown',key);
  element.addEventListener('pointerleave',leavePointer);
  document.addEventListener('teleport-appearance',appearance);document.addEventListener('visibilitychange',visibility);reduced.addEventListener('change',visibility);
  width=Math.max(1,element.clientWidth);height=Math.max(1,element.clientHeight);camera.left=-width/2;camera.right=width/2;camera.top=height/2;camera.bottom=-height/2;renderer.setSize(width,height,false);
  if(retained.projection==='2d'&&retained.camera){[camera.position.x,camera.position.y,camera.zoom]=retained.camera;updateView();}else fit();
  select(selected);syncLoop();
  if(retained.focused)queueMicrotask(()=>{if(!disposed&&document.activeElement===document.body)cards.get(retained.focused)?.focus({preventScroll:true});});
  return {select,move,zoom,fit,snapshot:()=>({projection:'2d',camera:[camera.position.x,camera.position.y,camera.zoom],target:[0,0,0],selected,focused,hovered: hovered||focused,flowingEdges:particleRecords.length/3,animationRunning:loop,phase}),dispose(){
    disposed=true;renderer.setAnimationLoop(null);resize.disconnect();intersection.disconnect();
    element.removeEventListener('pointerdown',down);element.removeEventListener('pointermove',drag);element.removeEventListener('pointerup',up);element.removeEventListener('pointercancel',cancelGesture);element.removeEventListener('wheel',wheel);element.removeEventListener('keydown',key);
    element.removeEventListener('pointerleave',leavePointer);
    document.removeEventListener('teleport-appearance',appearance);document.removeEventListener('visibilitychange',visibility);reduced.removeEventListener('change',visibility);
    scene.traverse(object=>{object.geometry?.dispose();if(object.material)object.material.dispose();});renderer.dispose();renderer.forceContextLoss();renderer.domElement.remove();layer.remove();element.classList.remove('graph2d-surface');
  }};
}
