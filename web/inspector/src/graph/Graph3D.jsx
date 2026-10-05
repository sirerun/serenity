import { useEffect, useRef } from "react";
import * as THREE from "three";

const shades = { entity: "#c1798c", fact: "#eee0e2", claim: "#d9c0c7", source: "#d7c49a" };

// Rose-lit, low-motion Three.js rendering of the injected graph. The SVG layer
// above this canvas provides the matching keyboard-operable node controls.
export default function Graph3D({ nodes, edges, positions, onFailure }) {
  const hostRef = useRef(null);
  const failureRef = useRef(onFailure);
  failureRef.current = onFailure;

  useEffect(() => {
    const host = hostRef.current;
    if (!host) return undefined;
    let renderer;
    let observer;
    let scene;
    let camera;
    let disposed = false;
    const geometries = [];
    const materials = [];
    try {
      renderer = new THREE.WebGLRenderer({ antialias: true, alpha: true, powerPreference: "low-power" });
      renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 1.5));
      renderer.outputColorSpace = THREE.SRGBColorSpace;
      renderer.toneMapping = THREE.ACESFilmicToneMapping;
      renderer.toneMappingExposure = 1.12;
      host.replaceChildren(renderer.domElement);
      scene = new THREE.Scene();
      camera = new THREE.PerspectiveCamera(35, 1, .1, 100);
      camera.position.set(0, 0, 19.5);
      scene.add(new THREE.HemisphereLight(0xfff6f1, 0x5c3847, 2.1));
      const roseLight = new THREE.PointLight(0xe8a9b9, 110, 32, 1.8);
      roseLight.position.set(-4, 5, 9);
      scene.add(roseLight);
      const pearlLight = new THREE.PointLight(0xffeadf, 62, 35, 1.8);
      pearlLight.position.set(7, -5, 5);
      scene.add(pearlLight);
      const collection = new THREE.Group();
      scene.add(collection);
      const world = (point, id) => {
        let seed = 0;
        for (const char of id) seed = Math.imul(seed ^ char.charCodeAt(0), 16777619);
        const depth = ((seed >>> 0) % 1000 / 1000 - .5) * 1.3;
        return new THREE.Vector3(point[0] * 9.2, -point[1] * 9.0, depth);
      };
      for (const edge of edges) {
        const a = positions.get(edge.from);
        const b = positions.get(edge.to);
        if (!a || !b) continue;
        const start = world(a, edge.from);
        const end = world(b, edge.to);
        const arch = new THREE.Vector3((start.x + end.x) / 2, (start.y + end.y) / 2 + .22, (start.z + end.z) / 2 - .24);
        const curve = new THREE.QuadraticBezierCurve3(start, arch, end);
        const geometry = new THREE.TubeGeometry(curve, 22, edge.kind === "source" ? .012 : .018, 5, false);
        const material = new THREE.MeshBasicMaterial({ color: edge.kind === "source" ? "#bca270" : "#bd8e9a", transparent: true, opacity: edge.kind === "source" ? .29 : .24 });
        geometries.push(geometry); materials.push(material);
        collection.add(new THREE.Mesh(geometry, material));
      }
      for (const node of nodes) {
        const point = positions.get(node.id);
        if (!point) continue;
        const entity = node.type === "entity";
        const size = entity ? .43 : node.type === "source" ? .16 : .19;
        const geometry = node.type === "claim" ? new THREE.OctahedronGeometry(size, 0) : node.type === "source" ? new THREE.BoxGeometry(size * 1.5, size * 1.5, size * 1.5) : new THREE.SphereGeometry(size, 24, 18);
        const material = new THREE.MeshPhysicalMaterial({ color: shades[node.type] || shades.fact, roughness: .27, metalness: node.type === "source" ? .18 : .03, clearcoat: .7, clearcoatRoughness: .22, emissive: entity ? "#552837" : "#35262b", emissiveIntensity: entity ? .24 : .1 });
        geometries.push(geometry); materials.push(material);
        const mesh = new THREE.Mesh(geometry, material);
        mesh.position.copy(world(point, node.id));
        collection.add(mesh);
        if (entity) {
          const haloGeometry = new THREE.TorusGeometry(.58, .014, 5, 64);
          const haloMaterial = new THREE.MeshBasicMaterial({ color: "#c68998", transparent: true, opacity: .28 });
          geometries.push(haloGeometry); materials.push(haloMaterial);
          const halo = new THREE.Mesh(haloGeometry, haloMaterial);
          halo.position.copy(mesh.position);
          halo.rotation.set(.7, .2, -.22);
          collection.add(halo);
        }
      }
      const resize = () => {
        const { width, height } = host.getBoundingClientRect();
        if (!width || !height) return;
        renderer.setSize(width, height, false);
        camera.aspect = width / height;
        camera.position.z = width < 600 ? 24 : 19.5;
        camera.updateProjectionMatrix();
      };
      resize();
      renderer.render(scene, camera);
      observer = new ResizeObserver(() => { resize(); renderer.render(scene, camera); });
      observer.observe(host);
    } catch {
      disposed = true;
      failureRef.current?.();
    }
    return () => {
      disposed = true;
      observer?.disconnect();
      for (const geometry of geometries) geometry.dispose();
      for (const material of materials) material.dispose();
      renderer?.dispose();
      renderer?.domElement.remove();
    };
  }, [nodes, edges, positions]);

  return <div className="graph-3d" ref={hostRef} aria-hidden="true" />;
}
