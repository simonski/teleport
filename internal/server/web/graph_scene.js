import * as THREE from './vendor/three.module.js';
import { OrbitControls } from './vendor/OrbitControls.js';

// Renderer state belongs to this viewer. Only explicit coordinate saves call the API.
export function createGraphScene(element, graph, callbacks, retained = {}) {
  const scene = new THREE.Scene();
  const camera = new THREE.PerspectiveCamera(45, 1, 0.5, 1000000);
  const renderer = new THREE.WebGLRenderer({ antialias: true, alpha: false });
  renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
  renderer.domElement.tabIndex = 0;
  renderer.domElement.setAttribute('aria-label', '3D ticket graph. Drag to orbit, wheel to zoom, Shift-drag a node to arrange. Arrows select; Enter opens; Alt-arrows move the selection.');
  renderer.domElement.setAttribute('role', 'img');
  element.appendChild(renderer.domElement);
  const controls = new OrbitControls(camera, renderer.domElement);
  controls.enableDamping = false;
  controls.zoomToCursor = true;
  controls.screenSpacePanning = true;
  controls.minDistance = 40;
  controls.maxDistance = 500000;
  const objects = new Map(), labels = [], edges = [];
  const labelIDs = new Set(graph.nodes.slice(0, 80).map(node => node.id));
  let selected = retained.selected || '', dragging = null, down = null, disposed = false;
  const raycaster = new THREE.Raycaster(), pointer = new THREE.Vector2();
  const grid = new THREE.GridHelper(2600, 26, '#c6c8bc', '#dfe1d7');
  grid.position.y = -330;
  scene.add(grid);
  const sizes = { project: 38, goal: 30, epic: 24, task: 16 };
  const colors = { project: '#ca4a2b', goal: '#7b8861', epic: '#ae925b', task: '#708a94' };
  const palette = () => getComputedStyle(document.documentElement);
  function render() { if (!disposed) renderer.render(scene, camera); }
  function label(node) {
    if (!labelIDs.has(node.id) && node.id !== selected) return;
    const detailed = node.kind !== 'task' || node.id === selected;
    const canvas = document.createElement('canvas'); canvas.width = detailed ? 512 : 192; canvas.height = 96;
    const ctx = canvas.getContext('2d');
    ctx.fillStyle = palette().getPropertyValue('--surface').trim(); ctx.fillRect(0, 0, canvas.width, 96);
    ctx.fillStyle = palette().getPropertyValue('--ink').trim(); ctx.font = '500 30px sans-serif';
    const name = detailed ? node.title.length > 25 ? node.title.slice(0, 24) + '…' : node.title : `#${node.ticketId}`;
    ctx.textAlign = 'center'; ctx.fillText(name, canvas.width / 2, 38);
    ctx.fillStyle = palette().getPropertyValue('--muted').trim(); ctx.font = '22px monospace';
    ctx.fillText(detailed ? node.ticketId ? `#${node.ticketId} · ${node.kind}` : 'PROJECT' : node.status, canvas.width / 2, 76);
    const texture = new THREE.CanvasTexture(canvas);
    texture.colorSpace = THREE.SRGBColorSpace;
    const sprite = new THREE.Sprite(new THREE.SpriteMaterial({ map: texture, depthTest: false, sizeAttenuation: false }));
    sprite.scale.set(detailed ? 0.4 : 0.15, 0.075, 1); sprite.position.copy(node.position).add(new THREE.Vector3(0, sizes[node.kind] + 38, 0));
    sprite.renderOrder = node.id === selected ? 10 : 1;
    sprite.userData.nodeId = node.id; scene.add(sprite); labels.push(sprite);
  }
  for (const node of graph.nodes) {
    const geometry = node.kind === 'project' ? new THREE.BoxGeometry(50, 50, 50) : new THREE.SphereGeometry(sizes[node.kind], 18, 12);
    const material = new THREE.MeshBasicMaterial({ color: node.status === 'Done' ? '#4f8058' : colors[node.kind], wireframe: node.context, transparent: node.context, opacity: node.context ? 0.65 : 1 });
    const mesh = new THREE.Mesh(geometry, material); mesh.position.copy(node.position); mesh.userData.nodeId = node.id;
    objects.set(node.id, mesh); scene.add(mesh); label(node);
  }
  for (const edge of graph.edges) {
    const source = objects.get(edge.source), target = objects.get(edge.target); if (!source || !target) continue;
    const context = graph.nodes.find(n => n.id === edge.target)?.context;
    const material = edge.kind === 'dependency' || context ? new THREE.LineDashedMaterial({ color: edge.kind === 'dependency' ? '#ca4a2b' : '#a7ad9c', dashSize: 10, gapSize: 7 }) : new THREE.LineBasicMaterial({ color: '#a7ad9c' });
    const line = new THREE.Line(new THREE.BufferGeometry().setFromPoints([source.position, target.position]), material);
    line.computeLineDistances(); scene.add(line); edges.push({ line, source: edge.source, target: edge.target });
  }
  function updateEdges() { for (const e of edges) { e.line.geometry.setFromPoints([objects.get(e.source).position, objects.get(e.target).position]); e.line.computeLineDistances(); } }
  function select(id) {
    selected = objects.has(id) ? id : '';
    for (const [key, mesh] of objects) mesh.scale.setScalar(key === selected ? 1.35 : 1);
    callbacks.select(selected); appearance();
  }
  function move(id, position) {
    const object = objects.get(id); if (!object) return;
    object.position.copy(position);
    const sprite = labels.find(l => l.userData.nodeId === id), node = graph.nodes.find(n => n.id === id);
    if (sprite) sprite.position.copy(position).add(new THREE.Vector3(0, sizes[node.kind] + 28, 0));
    updateEdges(); callbacks.move(id, { x: position.x, y: position.y, z: position.z }); render();
  }
  function zoom(factor) {
    const anchor = objects.get(selected)?.position || controls.target;
    camera.position.sub(anchor).multiplyScalar(factor).add(anchor);
    controls.target.sub(anchor).multiplyScalar(factor).add(anchor); controls.update(); render();
  }
  function fit() {
    const box = new THREE.Box3(); for (const object of objects.values()) box.expandByPoint(object.position);
    if (box.isEmpty()) { box.expandByPoint(new THREE.Vector3(-200, -200, -200)); box.expandByPoint(new THREE.Vector3(200, 200, 200)); }
    const center = box.getCenter(new THREE.Vector3()), size = box.getSize(new THREE.Vector3());
    const distance = Math.max(400, size.length() * 0.5 / Math.sin(camera.fov * Math.PI / 360)) * Math.max(1, 1 / camera.aspect) * 1.2;
    camera.position.copy(center).add(new THREE.Vector3(0.5, 0.35, 1).normalize().multiplyScalar(distance));
    controls.target.copy(center); controls.update(); render();
  }
  function aim(event) { const rect = renderer.domElement.getBoundingClientRect(); pointer.set((event.clientX - rect.left) / rect.width * 2 - 1, -(event.clientY - rect.top) / rect.height * 2 + 1); raycaster.setFromCamera(pointer, camera); return raycaster.intersectObjects([...objects.values()])[0]?.object; }
  function pointerDown(event) {
    down = { x: event.clientX, y: event.clientY, button:event.button };
    const hit = aim(event);
    if (hit && event.button===0 && event.shiftKey && graph.canSave) {
      controls.enabled = false; select(hit.userData.nodeId);
      dragging = { id: selected, original: hit.position.clone(), plane: new THREE.Plane().setFromNormalAndCoplanarPoint(camera.getWorldDirection(new THREE.Vector3()), hit.position) };
      renderer.domElement.setPointerCapture(event.pointerId); event.stopImmediatePropagation(); callbacks.interacting(true);
    }
  }
  function pointerMove(event) { if (!dragging) return; aim(event); const point = raycaster.ray.intersectPlane(dragging.plane, new THREE.Vector3()); if (point && [point.x, point.y, point.z].every(v => Math.abs(v) <= 100000)) move(dragging.id, point); }
  function pointerUp(event) {
    if (dragging) { dragging = null; controls.enabled = true; callbacks.interacting(false); }
    else if (down && Math.hypot(event.clientX - down.x, event.clientY - down.y) < 5) { const hit = aim(event); select(hit?.userData.nodeId || '');if(down.button===2&&hit&&event.type==='pointerup'){const id=hit.userData.nodeId,point={x:event.clientX,y:event.clientY};queueMicrotask(()=>{if(!disposed)callbacks.context?.(id,renderer.domElement,point);});} }
    down = null;
  }
  function keyDown(event) {
    const ids = [...objects.keys()];
    if(event.key==='F10'&&event.shiftKey&&!event.ctrlKey&&!event.metaKey&&!event.altKey&&!event.isComposing&&selected){event.preventDefault();callbacks.context?.(selected,renderer.domElement);return;}
    if (event.key === 'Escape') { if (dragging) { move(dragging.id, dragging.original); dragging = null; controls.enabled = true; callbacks.interacting(false); } else select(''); event.preventDefault(); return; }
    if (event.key === 'Enter' && selected) { event.preventDefault(); callbacks.open(selected); return; }
    if (!['ArrowUp', 'ArrowDown', 'ArrowLeft', 'ArrowRight', 'PageUp', 'PageDown'].includes(event.key)) return;
    event.preventDefault();
    if (event.altKey && graph.canSave && selected) { const p = objects.get(selected).position.clone(), amount = event.shiftKey ? 50 : 10; if (event.key === 'ArrowLeft') p.x -= amount; if (event.key === 'ArrowRight') p.x += amount; if (event.key === 'ArrowUp') p.y += amount; if (event.key === 'ArrowDown') p.y -= amount; if (event.key === 'PageUp') p.z += amount; if (event.key === 'PageDown') p.z -= amount; move(selected, p); }
    else { const direction = ['ArrowLeft', 'ArrowUp', 'PageUp'].includes(event.key) ? -1 : 1; select(ids[(ids.indexOf(selected) + direction + ids.length) % ids.length]); }
  }
  renderer.domElement.addEventListener('pointerdown', pointerDown, true);
  renderer.domElement.addEventListener('pointermove', pointerMove);
  renderer.domElement.addEventListener('pointerup', pointerUp);
  renderer.domElement.addEventListener('pointercancel', pointerUp);
  renderer.domElement.addEventListener('keydown', keyDown);
  renderer.domElement.addEventListener('dblclick', () => { if (selected) callbacks.open(selected); });
  controls.addEventListener('start', () => callbacks.interacting(true));
  controls.addEventListener('end', () => callbacks.interacting(false));
  controls.addEventListener('change', render);
  const resize = new ResizeObserver(() => { if (disposed) return; const w = element.clientWidth, h = element.clientHeight; camera.aspect = w / Math.max(1, h); camera.updateProjectionMatrix(); renderer.setSize(w, h); render(); });
  resize.observe(element);
  function appearance() { scene.background = new THREE.Color(palette().getPropertyValue('--paper').trim());grid.material.color.set(palette().getPropertyValue('--line').trim()); for (const sprite of labels) { sprite.material.map.dispose(); sprite.material.dispose(); scene.remove(sprite); } labels.length = 0; for (const node of graph.nodes) label({ ...node, position: objects.get(node.id).position }); render(); }
  document.addEventListener('teleport-appearance', appearance);
  appearance();
  if (retained.camera && retained.target) { camera.position.fromArray(retained.camera); controls.target.fromArray(retained.target); controls.update(); } else fit();
  select(selected);
  return { select, move, zoom, fit, snapshot: () => ({ camera: camera.position.toArray(), target: controls.target.toArray(), selected }), dispose() { disposed = true; resize.disconnect(); document.removeEventListener('teleport-appearance', appearance); controls.dispose(); scene.traverse(object => { object.geometry?.dispose(); if (object.material) { object.material.map?.dispose(); object.material.dispose(); } }); renderer.dispose(); renderer.forceContextLoss(); renderer.domElement.remove(); } };
}
