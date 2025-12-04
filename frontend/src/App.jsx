import React, { useState, useEffect, useMemo, useRef } from 'react';
import { Canvas, useLoader, useFrame } from '@react-three/fiber';
import { OrbitControls, Html } from '@react-three/drei';
import * as THREE from 'three';
import 'bootstrap/dist/css/bootstrap.min.css';

function MapPlane({ mapConfig }) {
  const texture = useLoader(THREE.TextureLoader, mapConfig.url || '/Map/gongsi.png');
  
  // Map coordinates (in meters)
  // x, y are top-left origin.
  // Three.js PlaneGeometry is centered.
  // Width extends from x to x+w
  // Height extends from y to y+h (Map Y corresponds to World Z)
  
  const centerX = mapConfig.width / 2;
  const centerZ = mapConfig.height / 2;

  return (
    <mesh rotation={[-Math.PI / 2, 0, 0]} position={[centerX, -0.1, centerZ]}>
      <planeGeometry args={[mapConfig.width, mapConfig.height]} />
      <meshBasicMaterial map={texture} toneMapped={false} />
    </mesh>
  );
}

function TagMarker({ id, tagsRef }) {
  const ref = useRef();
  const textRef = useRef();
  const [layer, setLayer] = useState(0);

  useFrame(() => {
    const tag = tagsRef.current[id];
    if (tag && ref.current) {
      // Coordinates: Aox X -> Three X, Aox Y -> Three Z (Depth), Aox Z -> Three Y (Up)
      const targetX = tag.x;
      const targetY = tag.z || 0.5;
      const targetZ = tag.y;
      
      ref.current.position.set(targetX, targetY, targetZ);
      
      if (tag.layer !== layer) {
         setLayer(tag.layer);
      }
    }
  });

  return (
    <group ref={ref}>
      <mesh>
        <sphereGeometry args={[0.3, 16, 16]} />
        <meshStandardMaterial color="hotpink" />
      </mesh>
      <Html position={[0, 1, 0]} center>
        <div style={{ 
            color: 'white', 
            background: 'rgba(0,0,0,0.5)', 
            padding: '2px 5px', 
            borderRadius: '4px', 
            fontSize: '12px',
            whiteSpace: 'nowrap' 
        }}>
          {id.toString(16).toUpperCase()} (L{layer})
        </div>
      </Html>
    </group>
  );
}

function Scene({ tagIds, tagsRef, mapConfig, is2D }) {
  const controlsRef = useRef();

  // Calculate new center for target
  const newMapCenterX = mapConfig.width / 2;
  const newMapCenterZ = mapConfig.height / 2; // Map Y is World Z

  useEffect(() => {
    if (controlsRef.current) {
      controlsRef.current.reset();
      const camera = controlsRef.current.object;
      if (is2D) {
        camera.position.set(newMapCenterX, Math.max(mapConfig.width, mapConfig.height), newMapCenterZ); // Top-down view
        camera.rotation.set(-Math.PI / 2, 0, 0); // Look straight down
      } else {
        camera.position.set(newMapCenterX + 40, 60, newMapCenterZ + 40); // Diagonal 3D view
        camera.lookAt(newMapCenterX, 0, newMapCenterZ);
      }
    }
  }, [is2D, newMapCenterX, newMapCenterZ, mapConfig.width, mapConfig.height]); // Include mapConfig in dependencies

  return (
    <>
      <ambientLight intensity={0.8} />
      <pointLight position={[10, 10, 10]} />
      <OrbitControls 
        ref={controlsRef}
        target={[newMapCenterX, 0, newMapCenterZ]} 
        enableRotate={!is2D}
        maxPolarAngle={is2D ? 0.01 : Math.PI} // Lock overhead view in 2D (small epsilon to avoid gimbal lock)
        minPolarAngle={is2D ? 0.01 : 0}
        mouseButtons={{
          LEFT: is2D ? THREE.MOUSE.PAN : THREE.MOUSE.ROTATE,
          MIDDLE: THREE.MOUSE.DOLLY,
          RIGHT: is2D ? THREE.MOUSE.ROTATE : THREE.MOUSE.PAN, // Swap right click for 2D pan if left is rotate
        }}
        screenSpacePanning={is2D} // Enables panning across the screen without Z changes
      />
      <gridHelper args={[Math.max(mapConfig.width, mapConfig.height) * 1.5, Math.max(mapConfig.width, mapConfig.height) / 10]} position={[newMapCenterX, -0.2, newMapCenterZ]} />
      <axesHelper args={[5]} />
      
      {mapConfig.url && (
        <MapPlane mapConfig={mapConfig} />
      )}

      {tagIds.map((id) => (
        <TagMarker key={id} id={id} tagsRef={tagsRef} />
      ))}
    </>
  );
}

function App() {
  const tagsRef = useRef({});
  const [tagIds, setTagIds] = useState([]);
  const [displayedTags, setDisplayedTags] = useState([]);
  
  const [status, setStatus] = useState('Disconnected');
  const [mapConfig, setMapConfig] = useState({ width: 100, height: 100, url: null });
  const [mapOffsetX, setMapOffsetX] = useState(0); // Store original x-topleft
  const [mapOffsetY, setMapOffsetY] = useState(0); // Store original y-topleft
  const [is2D, setIs2D] = useState(true); // Default 2D
  
  const [configTagId, setConfigTagId] = useState('');
  const [configCmdId, setConfigCmdId] = useState('1');
  const [configData, setConfigData] = useState('');

  // Fetch Map Config
  useEffect(() => {
    fetch('/project.xml')
      .then(res => res.text())
      .then(str => {
        const parser = new DOMParser();
        const xmlDoc = parser.parseFromString(str, "text/xml");
        const mapItem = xmlDoc.getElementsByTagName("mapItem")[0];
        if (mapItem) {
          const url = mapItem.getAttribute("url");
          // Parse cm to meters
          const w = parseFloat(mapItem.getAttribute("width")) / 100.0;
          const h = parseFloat(mapItem.getAttribute("height")) / 100.0;
          const xOffset = parseFloat(mapItem.getAttribute("x-topleft")) / 100.0;
          const yOffset = parseFloat(mapItem.getAttribute("y-topleft")) / 100.0;
          
          setMapConfig({ width: w, height: h, url: `/Map/${url}` }); // Map's local coords start at 0,0
          setMapOffsetX(xOffset);
          setMapOffsetY(yOffset);
        }
      })
      .catch(err => console.error("Failed to load config", err));
  }, []);

  // WebSocket
  useEffect(() => {
    let ws = null;
    let shouldReconnect = true;
    let reconnectTimer = null;

    const connect = () => {
      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      const host = window.location.host; 
      ws = new WebSocket(`${protocol}//${host}/ws`);

      ws.onopen = () => {
        setStatus('Connected');
        console.log('WS Connected');
        fetch('/api/tags')
          .then(res => res.json())
          .then(initialTags => {
             if (Array.isArray(initialTags)) {
                initialTags.forEach(tag => {
                   // Adjust tag position by map offsets
                   tag.x -= mapOffsetX;
                   tag.y -= mapOffsetY;
                   tagsRef.current[tag.id] = tag;
                });
                setTagIds(Object.keys(tagsRef.current).map(k => parseInt(k)));
             }
          })
          .catch(e => console.error("Failed to fetch snapshot", e));
      };

      ws.onclose = () => {
        setStatus('Disconnected');
        console.log('WS Disconnected, reconnecting in 2s...');
        if (shouldReconnect) {
          reconnectTimer = setTimeout(connect, 2000);
        }
      };

      ws.onmessage = (event) => {
        try {
          const msg = JSON.parse(event.data);
          if (msg.id) {
            // Adjust tag position by map offsets
            msg.x -= mapOffsetX;
            msg.y -= mapOffsetY;

            const isNew = !tagsRef.current[msg.id];
            tagsRef.current[msg.id] = msg;
            if (isNew) {
               setTagIds(prev => [...prev, msg.id]);
            }
          }
        } catch (e) {
          console.error("Parse error", e);
        }
      };
    };

    // Reconnect only if offsets are available. Initial load might try to connect before map config is fetched.
    if (mapOffsetX !== 0 || mapOffsetY !== 0 || mapConfig.url) { // Trigger effect when map config is loaded
        connect();
    }


    return () => {
      shouldReconnect = false;
      if (ws) ws.close();
      if (reconnectTimer) clearTimeout(reconnectTimer);
    };
  }, [mapOffsetX, mapOffsetY, mapConfig.url]); // Re-run effect if offsets or map URL change

  useEffect(() => {
    const interval = setInterval(() => {
      setDisplayedTags(Object.values(tagsRef.current));
    }, 500);
    return () => clearInterval(interval);
  }, []);

  const handleSendConfig = () => {
    if (!configTagId || !configCmdId || !configData) {
      alert("Please fill all config fields");
      return;
    }
    
    const payload = {
      tag_id: parseInt(configTagId, 16),
      cmd_id: parseInt(configCmdId),
      data_hex: configData
    };

    fetch('/api/lora/config', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json'
      },
      body: JSON.stringify(payload)
    })
    .then(res => {
      if (res.ok) alert("Config sent!");
      else res.text().then(t => alert("Error: " + t));
    })
    .catch(err => alert("Network error: " + err));
  };

  return (
    <div className="d-flex flex-column vh-100 overflow-hidden">
      <nav className="navbar navbar-dark bg-dark flex-shrink-0 px-3">
        <div className="d-flex align-items-center w-100">
          <span className="navbar-brand mb-0 h1 me-auto">AOX Engine Web</span>
          <div className="d-flex gap-2 align-items-center">
            <button 
              className={`btn btn-sm ${is2D ? 'btn-primary' : 'btn-outline-secondary'}`}
              onClick={() => setIs2D(true)}
            >
              2D
            </button>
            <button 
              className={`btn btn-sm ${!is2D ? 'btn-primary' : 'btn-outline-secondary'}`}
              onClick={() => setIs2D(false)}
            >
              3D
            </button>
            <span className={`badge ${status === 'Connected' ? 'bg-success' : 'bg-danger'}`}>
              {status}
            </span>
          </div>
        </div>
      </nav>

      <div className="flex-grow-1 d-flex flex-row" style={{ minHeight: 0 }}>
        <div className="bg-light border-end overflow-auto p-3" style={{ width: '300px', flexShrink: 0 }}>
          <h5>Active Tags ({displayedTags.length})</h5>
          <ul className="list-group mb-4">
            {displayedTags.map(tag => (
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

          <hr />
          <h5>Configuration</h5>
          <div className="mb-3">
            <label className="form-label">Tag ID (Hex)</label>
            <input 
              type="text" 
              className="form-control" 
              value={configTagId} 
              onChange={e => setConfigTagId(e.target.value)} 
              placeholder="e.g. 1A2B"
            />
          </div>
          <div className="mb-3">
            <label className="form-label">Cmd ID (Int)</label>
            <input 
              type="number" 
              className="form-control" 
              value={configCmdId} 
              onChange={e => setConfigCmdId(e.target.value)} 
            />
          </div>
          <div className="mb-3">
            <label className="form-label">Data (Hex)</label>
            <input 
              type="text" 
              className="form-control" 
              value={configData} 
              onChange={e => setConfigData(e.target.value)} 
              placeholder="e.g. 010203"
            />
          </div>
          <button className="btn btn-primary w-100" onClick={handleSendConfig}>
            Send Config
          </button>
        </div>
        <div className="flex-grow-1 position-relative bg-secondary">
          <Canvas camera={{ position: [mapConfig.width / 2, 60, mapConfig.height / 2], fov: 50 }} style={{ width: '100%', height: '100%' }}>
            <Scene tagIds={tagIds} tagsRef={tagsRef} mapConfig={mapConfig} is2D={is2D} />
          </Canvas>
          <div className="position-absolute bottom-0 start-0 p-2 text-light small" style={{ background: 'rgba(0,0,0,0.5)' }}>
            Map: {mapConfig.width.toFixed(1)}m x {mapConfig.height.toFixed(1)}m @ (0.0, 0.0)
          </div>
        </div>
      </div>
    </div>
  );
}
export default App;