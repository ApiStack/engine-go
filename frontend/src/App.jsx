import React, { useState, useEffect, useMemo } from 'react';
import { Canvas, useLoader } from '@react-three/fiber';
import { OrbitControls, Plane, Text, Html } from '@react-three/drei';
import * as THREE from 'three';
import 'bootstrap/dist/css/bootstrap.min.css';

function MapPlane({ mapUrl, width, height }) {
  const texture = useLoader(THREE.TextureLoader, mapUrl);
  // Map center correction:
  // If map is WxH, and origin (0,0) is Top-Left.
  // Three.js Plane is centered at (0,0,0).
  // We want (0,0) to be Top-Left relative to the markers?
  // Usually markers are in meters relative to (0,0) Top-Left.
  // So if we place Plane at (W/2, -H/2, 0) (assuming Z is up? No, usually Y is up in Three.js).
  // Let's use X/Z plane. Y is height.
  // Map (0,0) is Top-Left. X+, Z+.
  // Plane geometry is centered.
  // We shift Plane by (W/2, 0, H/2).
  // NOTE: Texture needs to be rotated? Standard texture maps UV (0,0) bottom-left to (1,1) top-right.
  // Images are usually displayed top-down.
  
  return (
    <mesh rotation={[-Math.PI / 2, 0, 0]} position={[width / 2, -0.1, height / 2]}>
      <planeGeometry args={[width, height]} />
      <meshBasicMaterial map={texture} toneMapped={false} />
    </mesh>
  );
}

function TagMarker({ id, x, y, z }) {
  // x, y, z are in meters.
  // Assuming Y is 2D Y (depth in map), so it maps to Three.js Z.
  // Z is height, so it maps to Three.js Y.
  return (
    <group position={[x, z || 0.5, y]}>
      <mesh>
        <sphereGeometry args={[0.3, 16, 16]} />
        <meshStandardMaterial color="hotpink" />
      </mesh>
      <Html position={[0, 1, 0]} center>
        <div style={{ color: 'white', background: 'rgba(0,0,0,0.5)', padding: '2px 5px', borderRadius: '4px', fontSize: '12px' }}>
          {id.toString(16).toUpperCase()}
        </div>
      </Html>
    </group>
  );
}

function Scene({ tags, mapConfig }) {
  return (
    <>
      <ambientLight intensity={0.8} />
      <pointLight position={[10, 10, 10]} />
      <OrbitControls target={[mapConfig.width / 2, 0, mapConfig.height / 2]} />
      <gridHelper args={[100, 100]} position={[50, -0.2, 50]} />
      <axesHelper args={[5]} />
      
      {mapConfig.url && (
        <MapPlane mapUrl={mapConfig.url} width={mapConfig.width} height={mapConfig.height} />
      )}

      {Object.values(tags).map((tag) => (
        <TagMarker key={tag.id} {...tag} />
      ))}
    </>
  );
}

function App() {
  const [tags, setTags] = useState({});
  const [status, setStatus] = useState('Disconnected');
  const [mapConfig, setMapConfig] = useState({ width: 78.6, height: 70.2, url: '/Map/gongsi.png' });

  // Fetch project.xml to get map info (Prototype: XML parsing in JS)
  useEffect(() => {
    fetch('/project.xml')
      .then(res => res.text())
      .then(str => {
        const parser = new DOMParser();
        const xmlDoc = parser.parseFromString(str, "text/xml");
        const mapItem = xmlDoc.getElementsByTagName("mapItem")[0];
        if (mapItem) {
          const url = mapItem.getAttribute("url");
          const w = parseFloat(mapItem.getAttribute("width")) / 100.0; // cm to m
          const h = parseFloat(mapItem.getAttribute("height")) / 100.0;
          setMapConfig({ width: w, height: h, url: `/Map/${url}` });
        }
      })
      .catch(err => console.error("Failed to load config", err));
  }, []);

  useEffect(() => {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const host = window.location.host; 
    // If running via Vite proxy, window.location.host is localhost:5173, proxies to localhost:8080.
    // Direct ws://localhost:8080/ws bypasses Vite if hardcoded? 
    // Vite proxy handles /ws -> target.
    // So we use ws://window.location.host/ws.
    
    const ws = new WebSocket(`${protocol}//${host}/ws`);

    ws.onopen = () => {
      setStatus('Connected');
      console.log('WS Connected');
    };

    ws.onclose = () => {
      setStatus('Disconnected');
      console.log('WS Disconnected');
    };

    ws.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data);
        if (msg.id) {
          setTags(prev => ({
            ...prev,
            [msg.id]: msg
          }));
        }
      } catch (e) {
        console.error("Parse error", e);
      }
    };

    return () => {
      ws.close();
    };
  }, []);

  const activeTags = useMemo(() => {
    const now = Date.now();
    // Filter tags older than 5 seconds? Or just show all.
    // Let's just show all for now.
    return Object.values(tags);
  }, [tags]);

  return (
    <div className="container-fluid vh-100 d-flex flex-column p-0">
      <nav className="navbar navbar-dark bg-dark">
        <div className="container-fluid">
          <span className="navbar-brand mb-0 h1">AOX Engine Web</span>
          <span className={`badge ${status === 'Connected' ? 'bg-success' : 'bg-danger'}`}>
            {status}
          </span>
        </div>
      </nav>

      <div className="row flex-grow-1 g-0">
        <div className="col-md-3 bg-light border-end overflow-auto p-3">
          <h5>Active Tags ({activeTags.length})</h5>
          <ul className="list-group">
            {activeTags.map(tag => (
              <li key={tag.id} className="list-group-item d-flex justify-content-between align-items-center">
                <div>
                  <strong>{tag.id.toString(16).toUpperCase()}</strong>
                  <br />
                  <small className="text-muted">
                    {tag.x.toFixed(2)}, {tag.y.toFixed(2)}, {tag.z.toFixed(2)}
                  </small>
                </div>
                <span className="badge bg-primary rounded-pill">L{tag.layer}</span>
              </li>
            ))}
          </ul>
        </div>
        <div className="col-md-9 position-relative" style={{ background: '#eee' }}>
          <Canvas camera={{ position: [40, 60, 40], fov: 50 }}>
            <Scene tags={tags} mapConfig={mapConfig} />
          </Canvas>
          <div className="position-absolute bottom-0 start-0 p-2 text-muted small">
            Map: {mapConfig.width.toFixed(1)}m x {mapConfig.height.toFixed(1)}m
          </div>
        </div>
      </div>
    </div>
  );
}

export default App;