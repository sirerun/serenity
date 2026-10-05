import { useEffect, useRef } from "react";
import * as THREE from "three";
import { OrbitControls } from "three/addons/controls/OrbitControls.js";

const shades = { entity: "#c1798c", fact: "#eee0e2", claim: "#d9c0c7", source: "#d7c49a" };
const types = ["entity", "fact", "claim", "source"];
const titleFor = (node) => node?.type === "entity" ? node.label : node?.text || node?.label || node?.kind;

function labelSprite(value) {
  const label = String(value || "Memory").slice(0, 34);
  const canvas = document.createElement("canvas");
  canvas.width = Math.min(640, Math.max(190, Math.ceil(label.length * 31 + 40)));
  canvas.height = 76;
  const context = canvas.getContext("2d");
  context.fillStyle = "rgba(91,54,67,.97)";
  context.strokeStyle = "rgba(247,213,222,.9)";
  context.lineWidth = 2;
  context.beginPath();
  context.roundRect(2, 2, canvas.width - 4, 72, 15);
  context.fill();
  context.stroke();
  context.fillStyle = "#fff8f4";
  context.font = "600 31px Georgia, serif";
  context.textBaseline = "middle";
  context.fillText(label, 19, 39, canvas.width - 38);
  const texture = new THREE.CanvasTexture(canvas);
  texture.colorSpace = THREE.SRGBColorSpace;
  const material = new THREE.SpriteMaterial({ map: texture, transparent: true, depthWrite: false, sizeAttenuation: true });
  const sprite = new THREE.Sprite(material);
  const height = .52;
  sprite.scale.set(Math.max(1.4, Math.min(3.5, (canvas.width / canvas.height) * height)), height, 1);
  return sprite;
}

// Rose-lit, instanced constellation. OrbitControls supports mouse/touch orbit,
// pan, wheel and pinch. Parent keeps the keyboard-operable SVG and list fallback.
export default function Graph3D({ nodes, edges, positions, selectedId, onSelect, onReady, onFailure, reducedMotion }) {
  const hostRef = useRef(null);
  const callbacks = useRef({ onSelect, onReady, onFailure });
  callbacks.current = { onSelect, onReady, onFailure };
  const selectedRef = useRef(selectedId);
  selectedRef.current = selectedId;
  const sceneRef = useRef(null);
  const rendererRef = useRef(null);
  const controlsRef = useRef(null);
  const nodePositionsRef = useRef(new Map());
  const nodesRef = useRef(nodes);
  nodesRef.current = nodes;
  const keyboardRingRef = useRef(null);
  const selectedLabelRef = useRef(null);
  const renderRef = useRef(() => {});

  useEffect(() => {
    const host = hostRef.current;
    if (!host) return undefined;
    callbacks.current.onReady?.(false);
    let renderer;
    let controls;
    let camera;
    let observer;
    let frame = 0;
    let disposed = false;
    let requestMotionFrame = () => {};
    let controlChanged = () => {};
    let move = () => {};
    let pointerDown = () => {};
    let pointerUp = () => {};
    let keydown = () => {};
    const scene = new THREE.Scene();
    const dimensions = host.getBoundingClientRect();
    const rawPoints = nodes.map((node) => positions.get(node.id)).filter(Boolean);
    const rawXHalf = Math.max(.15, ...rawPoints.map((p) => Math.abs(p[0] * 9)));
    const rawYHalf = Math.max(.15, ...rawPoints.map((p) => Math.abs(p[1] * 9)));
    const aspect = dimensions.width / Math.max(1, dimensions.height);
    const horizontalSpread = Math.max(.5, rawYHalf * aspect / rawXHalf);
    const nodePositions = new Map(nodes.map((node) => {
      const p = positions.get(node.id);
      return [node.id, p ? new THREE.Vector3(p[0] * 9 * horizontalSpread, p[1] * 9, (p[2] || 0) * 9) : null];
    }));
    const pickMeshes = [];
    const geometries = new Set();
    const materials = new Set();
    const textures = new Set();
    const pointer = new THREE.Vector2();
    const raycaster = new THREE.Raycaster();
    let downPoint = null;

    const renderOnce = () => { if (!disposed && renderer && camera) renderer.render(scene, camera); };
    const setPointer = (event) => {
      const rect = renderer.domElement.getBoundingClientRect();
      pointer.x = ((event.clientX - rect.left) / rect.width) * 2 - 1;
      pointer.y = -(((event.clientY - rect.top) / rect.height) * 2 - 1);
    };
    pointerDown = (event) => { downPoint = { x: event.clientX, y: event.clientY }; };
    pointerUp = (event) => {
      if (!downPoint || Math.hypot(event.clientX - downPoint.x, event.clientY - downPoint.y) > 5) { downPoint = null; return; }
      downPoint = null;
      setPointer(event);
      raycaster.setFromCamera(pointer, camera);
      const hit = raycaster.intersectObjects(pickMeshes, false)[0];
      const id = hit && hit.object.userData.nodeIds?.[hit.instanceId];
      if (id) callbacks.current.onSelect?.(id);
    };
    keydown = (event) => {
      const order = nodes.map((node) => node.id);
      const current = order.indexOf(selectedRef.current);
      if (["ArrowRight", "ArrowDown", "ArrowLeft", "ArrowUp"].includes(event.key)) {
        event.preventDefault();
        const delta = event.key === "ArrowRight" || event.key === "ArrowDown" ? 1 : -1;
        const next = current < 0 ? 0 : (current + delta + order.length) % order.length;
        if (order[next]) callbacks.current.onSelect?.(order[next]);
      } else if (event.key === "Enter" && selectedRef.current) {
        callbacks.current.onSelect?.(selectedRef.current);
      } else if (event.key === "Home") {
        event.preventDefault();
        controls.reset();
      }
      renderOnce();
    };

    try {
      renderer = new THREE.WebGLRenderer({ antialias: true, alpha: true, powerPreference: "high-performance" });
      renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 1.65));
      renderer.outputColorSpace = THREE.SRGBColorSpace;
      renderer.toneMapping = THREE.ACESFilmicToneMapping;
      renderer.toneMappingExposure = 1.2;
      renderer.domElement.className = "graph-canvas";
      renderer.domElement.setAttribute("role", "application");
      renderer.domElement.setAttribute("aria-label", `Interactive three dimensional memory map with ${nodes.length} loaded notes. Drag to orbit, pinch or scroll to zoom, arrow keys to select, Enter to inspect, and Home to reset.`);
      renderer.domElement.setAttribute("aria-describedby", "map-keyboard-help");
      renderer.domElement.tabIndex = 0;
      host.replaceChildren(renderer.domElement);

      scene.add(new THREE.HemisphereLight(0xfff6f1, 0x5c3847, 2.1));
      const keyLight = new THREE.PointLight(0xe8a9b9, 110, 32, 1.8);
      keyLight.position.set(-4, 5, 8);
      scene.add(keyLight);
      const fillLight = new THREE.PointLight(0xffeadf, 62, 35, 1.8);
      fillLight.position.set(7, -5, 5);
      scene.add(fillLight);
      const initialWidth = host.clientWidth || 1;
      const initialHeight = host.clientHeight || 1;
      camera = new THREE.PerspectiveCamera(39, initialWidth / initialHeight, .1, 150);
      controls = new OrbitControls(camera, renderer.domElement);
      controls.enableDamping = !reducedMotion;
      controls.dampingFactor = reducedMotion ? 0 : .075;
      controls.enablePan = true;
      controls.screenSpacePanning = true;
      controls.minDistance = 4;
      controls.maxDistance = 38;
      controls.rotateSpeed = .78;
      controls.zoomSpeed = .86;
      controls.panSpeed = .72;
      let userMovedView = false;
      controls.addEventListener("start", () => { userMovedView = true; });

      const pointValues = [...nodePositions.values()].filter(Boolean);
      const bounds = new THREE.Box3().setFromPoints(pointValues);
      const center = bounds.isEmpty() ? new THREE.Vector3() : bounds.getCenter(new THREE.Vector3());
      const frameBounds = (bounds.isEmpty() ? new THREE.Box3().setFromCenterAndSize(center, new THREE.Vector3(2, 2, 1)) : bounds.clone())
        .expandByVector(new THREE.Vector3(3.1, .72, 0));
      const fitInitialCamera = (width, height) => {
        camera.aspect = width / Math.max(1, height);
        camera.updateProjectionMatrix();
        const frameCenter = frameBounds.getCenter(new THREE.Vector3());
        const size = frameBounds.getSize(new THREE.Vector3());
        const verticalTan = Math.tan(THREE.MathUtils.degToRad(camera.fov / 2));
        const horizontalTan = verticalTan * camera.aspect;
        const fitDistance = Math.max(size.y / (2 * verticalTan), size.x / (2 * horizontalTan)) / .84 + size.z / 2;
        camera.position.set(frameCenter.x, frameCenter.y, frameCenter.z + Math.max(8, fitDistance));
        controls.target.copy(frameCenter);
        controls.update();
        controls.saveState();
      };
      fitInitialCamera(initialWidth, initialHeight);

      const linePositions = [];
      const lineColors = [];
      for (const edge of edges) {
        const from = nodePositions.get(edge.from);
        const to = nodePositions.get(edge.to);
        if (!from || !to) continue;
        linePositions.push(from.x, from.y, from.z, to.x, to.y, to.z);
        const tint = new THREE.Color(edge.kind === "source" ? "#bca270" : edge.kind === "history" ? "#d48ca0" : "#bd8e9a");
        const gain = edge.kind === "source" ? .45 : .78;
        lineColors.push(tint.r * gain, tint.g * gain, tint.b * gain, tint.r * gain, tint.g * gain, tint.b * gain);
      }
      if (linePositions.length) {
        const geometry = new THREE.BufferGeometry();
        geometry.setAttribute("position", new THREE.Float32BufferAttribute(linePositions, 3));
        geometry.setAttribute("color", new THREE.Float32BufferAttribute(lineColors, 3));
        const material = new THREE.LineBasicMaterial({ vertexColors: true, transparent: true, opacity: .52, depthWrite: false });
        geometries.add(geometry); materials.add(material);
        scene.add(new THREE.LineSegments(geometry, material));
      }

      const dummy = new THREE.Object3D();
      const shapeGeometry = {
        entity: new THREE.SphereGeometry(1, 28, 22),
        fact: new THREE.SphereGeometry(1, 22, 18),
        claim: new THREE.OctahedronGeometry(1, 0),
        source: new THREE.BoxGeometry(1, 1, 1),
      };
      Object.values(shapeGeometry).forEach((geometry) => geometries.add(geometry));
      let labelCount = 0;
      for (const type of types) {
        for (const context of [false, true]) {
          const batch = nodes.filter((node) => node.type === type && Boolean(node.isContext) === context && nodePositions.get(node.id));
          if (!batch.length) continue;
          const material = new THREE.MeshPhysicalMaterial({ color: shades[type], roughness: .24, metalness: type === "source" ? .17 : .035, clearcoat: .8, clearcoatRoughness: .18, emissive: type === "entity" ? "#552837" : "#35262b", emissiveIntensity: type === "entity" ? .3 : .11, transparent: context, opacity: context ? .52 : 1 });
          materials.add(material);
          const instances = new THREE.InstancedMesh(shapeGeometry[type], material, batch.length);
          instances.userData.nodeIds = [];
          batch.forEach((node, index) => {
            dummy.position.copy(nodePositions.get(node.id));
            dummy.scale.setScalar(type === "entity" ? .43 : type === "source" ? .15 : .19);
            dummy.rotation.set(0, 0, 0);
            dummy.updateMatrix();
            instances.setMatrixAt(index, dummy.matrix);
            instances.userData.nodeIds[index] = node.id;
          });
          instances.instanceMatrix.needsUpdate = true;
          instances.computeBoundingSphere();
          scene.add(instances);
          pickMeshes.push(instances);
          if (type === "entity") {
            const ringGeometry = new THREE.TorusGeometry(.58, .013, 5, 56);
            geometries.add(ringGeometry);
            const ringMaterial = new THREE.MeshBasicMaterial({ color: "#c68998", transparent: true, opacity: context ? .14 : .28 });
            materials.add(ringMaterial);
            const rings = new THREE.InstancedMesh(ringGeometry, ringMaterial, batch.length);
            batch.forEach((node, index) => {
              dummy.position.copy(nodePositions.get(node.id));
              dummy.rotation.set(.7, .2, -.22);
              dummy.scale.setScalar(1);
              dummy.updateMatrix();
              rings.setMatrixAt(index, dummy.matrix);
            });
            rings.instanceMatrix.needsUpdate = true;
            rings.computeBoundingSphere();
            scene.add(rings);
          }
          if (type === "entity" && labelCount < 12) {
            for (const node of batch.slice(0, 12 - labelCount)) {
              const point = nodePositions.get(node.id);
              const label = labelSprite(titleFor(node));
              label.position.set(point.x + .86, point.y + .28, point.z + .05);
              scene.add(label);
              textures.add(label.material.map); materials.add(label.material);
              labelCount += 1;
            }
          }
        }
      }

      const ringGeometry = new THREE.TorusGeometry(.62, .022, 8, 56);
      const ringMaterial = new THREE.MeshBasicMaterial({ color: "#ed9cb1", transparent: true, opacity: .95, depthTest: false });
      geometries.add(ringGeometry); materials.add(ringMaterial);
      const keyboardRing = new THREE.Mesh(ringGeometry, ringMaterial);
      keyboardRing.visible = false;
      keyboardRing.renderOrder = 20;
      scene.add(keyboardRing);
      keyboardRingRef.current = keyboardRing;
      nodePositionsRef.current = nodePositions;
      nodesRef.current = nodes;
      sceneRef.current = scene;
      rendererRef.current = renderer;
      controlsRef.current = controls;

      const resize = () => {
        const { width, height } = host.getBoundingClientRect();
        if (!width || !height) return;
        renderer.setSize(width, height, false);
        if (!userMovedView) fitInitialCamera(width, height);
        else {
          camera.aspect = width / height;
          camera.updateProjectionMatrix();
        }
        renderOnce();
      };
      observer = new ResizeObserver(resize);
      observer.observe(host);
      resize();
      controls.addEventListener("change", renderOnce);
      let animate = () => {};
      requestMotionFrame = () => {
        if (disposed || frame) return;
        frame = requestAnimationFrame(animate);
      };
      animate = () => {
        frame = 0;
        if (disposed) return;
        const moving = controls.update();
        renderer.render(scene, camera);
        if (moving && !reducedMotion) requestMotionFrame();
      };
      controlChanged = () => { renderOnce(); if (!reducedMotion) requestMotionFrame(); };
      controls.addEventListener("change", controlChanged);
      move = () => requestMotionFrame();
      renderer.domElement.addEventListener("pointerdown", move);
      renderer.domElement.addEventListener("pointermove", move);
      renderer.domElement.addEventListener("wheel", move, { passive: true });
      renderer.domElement.addEventListener("pointerdown", pointerDown);
      renderer.domElement.addEventListener("pointerup", pointerUp);
      renderer.domElement.addEventListener("keydown", keydown);
      renderRef.current = renderOnce;
      const selectedPoint = nodePositions.get(selectedRef.current);
      if (selectedPoint) { keyboardRing.visible = true; keyboardRing.position.copy(selectedPoint); }
      renderOnce();
      callbacks.current.onReady?.(true);
    } catch {
      disposed = true;
      callbacks.current.onFailure?.();
    }

    return () => {
      disposed = true;
      cancelAnimationFrame(frame);
      observer?.disconnect();
      controls?.removeEventListener("change", renderOnce);
      controls?.removeEventListener("change", controlChanged);
      renderer?.domElement.removeEventListener("pointerdown", move);
      renderer?.domElement.removeEventListener("pointermove", move);
      renderer?.domElement.removeEventListener("wheel", move);
      renderer?.domElement.removeEventListener("pointerdown", pointerDown);
      renderer?.domElement.removeEventListener("pointerup", pointerUp);
      renderer?.domElement.removeEventListener("keydown", keydown);
      for (const texture of textures) texture.dispose();
      for (const geometry of geometries) geometry.dispose();
      for (const material of materials) material.dispose();
      selectedLabelRef.current?.material.map?.dispose();
      selectedLabelRef.current?.material.dispose();
      controls?.dispose();
      renderer?.dispose();
      renderer?.forceContextLoss();
      renderer?.domElement.remove();
      scene.clear();
      sceneRef.current = null;
      rendererRef.current = null;
      controlsRef.current = null;
      nodePositionsRef.current = new Map();
      keyboardRingRef.current = null;
      selectedLabelRef.current = null;
      renderRef.current = () => {};
    };
  }, [nodes, edges, positions, reducedMotion]);

  useEffect(() => {
    const scene = sceneRef.current;
    const point = nodePositionsRef.current.get(selectedId);
    const ring = keyboardRingRef.current;
    if (ring) { ring.visible = Boolean(point); if (point) ring.position.copy(point); }
    if (selectedLabelRef.current && scene) {
      scene.remove(selectedLabelRef.current);
      selectedLabelRef.current.material.map?.dispose();
      selectedLabelRef.current.material.dispose();
      selectedLabelRef.current = null;
    }
    if (scene && point) {
      const node = nodesRef.current.find((item) => item.id === selectedId);
      const label = labelSprite(titleFor(node));
      label.position.set(point.x + .55, point.y + .3, point.z + .15);
      scene.add(label);
      selectedLabelRef.current = label;
    }
    renderRef.current();
  }, [selectedId, nodes]);

  return <div className="graph-3d" ref={hostRef} />;
}
