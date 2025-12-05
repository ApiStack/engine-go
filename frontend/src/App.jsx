import React, { useState, useEffect, useMemo, useRef } from 'react';
import { Canvas, useLoader, useFrame } from '@react-three/fiber';
import { OrbitControls, Html, Line } from '@react-three/drei';
import * as THREE from 'three';
import 'bootstrap/dist/css/bootstrap.min.css';
import { Dialog, DialogTitle, DialogContent, DialogActions, TextField, Button, Menu, MenuItem } from '@mui/material';

// Explicitly enforcing standard 2D-like coordinate system in 3D World:
// World Origin (0,0,0) corresponds to Map Image Top-Left corner.
// World X+ (Right) corresponds to Map Image Right.
// World Z+ (Forward/Down in top-view) corresponds to Map Image Down.
// This aligns with standard CSS/SVG coordinates where (0,0) is Top-Left.

function MapPlane({ mapConfig }) {
  const texture = useLoader(THREE.TextureLoader, mapConfig.url || '/Map/gongsi.png');
  
  // Texture is loaded with standard settings (flipY=true).
  // PlaneGeometry UVs map (0,0) to bottom-left and (1,1) to top-right.
  // To align Image Top-Left with World (0,0,0) and Image Right/Down with X+/Z+:
  // 1. Create PlaneGeometry of size (W, H). Local coords: [-W/2, W/2], [-H/2, H/2].
  // 2. Rotate -90 degrees around X-axis.
  //    Local +Y (Top) becomes World -Z (North).
  //    Local -Y (Bottom) becomes World +Z (South).
  //    This would put Image Top at -Z. We want Image Top at Z=0.
  // 3. Position the plane such that its Top-Left corner is at (0,0,0).
  //    Center X = W/2. Center Z = H/2.
  //    Position = [W/2, -0.1, H/2].
  
  const centerX = mapConfig.width / 2;
  const centerZ = mapConfig.height / 2;

  return (
    <mesh rotation={[-Math.PI / 2, 0, 0]} position={[centerX, -0.1, centerZ]}>
      <planeGeometry args={[mapConfig.width, mapConfig.height]} />
      <meshBasicMaterial map={texture} toneMapped={false} side={THREE.DoubleSide} />
    </mesh>
  );
}

function TagMarker({ id, tagsRef, trailsRef, isTrailEnabled }) {
  const ref = useRef();
  const [layer, setLayer] = useState(0);
  const [trailPoints, setTrailPoints] = useState([]);

  useFrame(() => {
    const tag = tagsRef.current[id];
    if (tag && ref.current) {
      const targetX = tag.x;
      const targetY = tag.z || 0.5;
      const targetZ = tag.y;
      
      ref.current.position.set(targetX, targetY, targetZ);
      
      if (tag.layer !== layer) {
         setLayer(tag.layer);
      }

      // Trail Logic
      if (isTrailEnabled && trailsRef.current[id]) {
          const pts = trailsRef.current[id];
          // Check if update is needed: Length change OR Last point moved
          // This is necessary because the buffer might be full (fixed length), so length check isn't enough.
          let shouldUpdate = false;
          if (pts.length !== trailPoints.length) {
              shouldUpdate = true;
          } else if (pts.length > 0) {
              const lastNew = pts[pts.length - 1];
              const lastOld = trailPoints[trailPoints.length - 1];
              // Simple coordinate comparison
              if (!lastOld || lastNew[0] !== lastOld[0] || lastNew[1] !== lastOld[1] || lastNew[2] !== lastOld[2]) {
                  shouldUpdate = true;
              }
          }

          if (shouldUpdate) {
              setTrailPoints([...pts]);
          }
      } else if (!isTrailEnabled && trailPoints.length > 0) {
          setTrailPoints([]);
      }
    }
  });

  // Removed smoothedPath calculation to strictly follow 'connects adjacent history positions' (Polyline)
  
  return (
    <>
      <group ref={ref}>
        {/* Pin Shape: Cone pointing down + Sphere on top */}
        <group position={[0, 0.25, 0]}>
           {/* Cone: RadiusTop, RadiusBottom, Height, Segments */}
           <mesh position={[0, 0, 0]}>
              <cylinderGeometry args={[0.2, 0, 0.5, 16]} />
              <meshStandardMaterial color="#1976d2" roughness={0.3} metalness={0.5} />
           </mesh>
           {/* Sphere on top */}
           <mesh position={[0, 0.25, 0]}>
              <sphereGeometry args={[0.15, 16, 16]} />
              <meshStandardMaterial color="#1976d2" roughness={0.3} metalness={0.5} />
           </mesh>
        </group>
        
        <Html position={[0, 1.2, 0]} center>
          <div style={{ 
              color: 'black', 
              background: 'rgba(255,255,255,0.85)', 
              padding: '4px 8px', 
              borderRadius: '12px', 
              fontSize: '12px',
              fontWeight: 'bold',
              whiteSpace: 'nowrap',
              cursor: 'pointer',
              border: '1px solid #ccc',
              boxShadow: '0 2px 4px rgba(0,0,0,0.2)'
          }}>
            {id.toString(16).toUpperCase()} (L{layer})
          </div>
        </Html>
      </group>
      {isTrailEnabled && trailPoints.length > 1 && (
         <Line points={trailPoints} color="#ff5722" lineWidth={2} />
      )}
    </>
  );
}

function Scene({ tagIds, tagsRef, mapConfig, is2D, focusTagId, setFocusTagId, trailsRef, enabledTrails }) {
  const controlsRef = useRef();

  const newMapCenterX = mapConfig.width / 2;
  const newMapCenterZ = mapConfig.height / 2;

  useEffect(() => {
    if (controlsRef.current) {
      const ctrl = controlsRef.current;
      const camera = ctrl.object;
      
      // Reset standard state
      ctrl.reset(); 

      if (is2D) {
        // 2D Mode: Strictly Top-Down, North-Up
        ctrl.enableRotate = false;
        ctrl.screenSpacePanning = true;
        
        // Fix Camera Orientation:
        // We want to look down the Y axis (at the XZ plane).
        // We want Screen Top to align with -Z (North).
        // Standard LookAt(0,-1,0) with Up(0,1,0) is degenerate.
        // We use Up(0,0,-1).
        camera.up.set(0, 0, -1);
        
        camera.position.set(newMapCenterX, Math.max(mapConfig.width, mapConfig.height) * 1.2, newMapCenterZ);
        camera.lookAt(newMapCenterX, 0, newMapCenterZ);
        
        // Lock to Top-Down View (North-Up)
        ctrl.minPolarAngle = Math.PI / 2;
        ctrl.maxPolarAngle = Math.PI / 2;
      } else {
        // 3D Mode: Standard Perspective
        ctrl.enableRotate = true;
        ctrl.screenSpacePanning = false;
        
        // Restore Standard Up
        camera.up.set(0, 1, 0);
        
        // Angled view (South-East looking North-West)
        camera.position.set(newMapCenterX, 50, newMapCenterZ + 50);
        camera.lookAt(newMapCenterX, 0, newMapCenterZ);
        
        ctrl.minPolarAngle = 0;
        ctrl.maxPolarAngle = Math.PI / 2 - 0.1; // Don't go below ground
      }
      ctrl.target.set(newMapCenterX, 0, newMapCenterZ);
      ctrl.update();
    }
  }, [is2D, newMapCenterX, newMapCenterZ, mapConfig.width, mapConfig.height]);

  // Focus effect
  useEffect(() => {
    if (focusTagId && controlsRef.current && tagsRef.current[focusTagId]) {
        const tag = tagsRef.current[focusTagId];
        const targetX = tag.x;
        const targetZ = tag.y; // Aox Y is World Z

        if (is2D) {
           const currentY = controlsRef.current.object.position.y;
           controlsRef.current.object.position.set(targetX, currentY, targetZ);
        }
        
        controlsRef.current.target.set(targetX, 0, targetZ);
        controlsRef.current.update();
        
        // Reset focus request
        setFocusTagId(null);
    }
  }, [focusTagId, setFocusTagId, tagsRef, is2D]);

  return (
    <>
      <ambientLight intensity={0.8} />
      <pointLight position={[10, 10, 10]} />
      <OrbitControls 
        ref={controlsRef}
        mouseButtons={{
          LEFT: is2D ? THREE.MOUSE.PAN : THREE.MOUSE.ROTATE,
          MIDDLE: THREE.MOUSE.DOLLY,
          RIGHT: is2D ? THREE.MOUSE.ROTATE : THREE.MOUSE.PAN,
        }}
      />
      <gridHelper args={[Math.max(mapConfig.width, mapConfig.height) * 1.5, Math.max(mapConfig.width, mapConfig.height) / 10]} position={[newMapCenterX, -0.2, newMapCenterZ]} />
      <axesHelper args={[5]} position={[0, 0.1, 0]} /> 
      
      {mapConfig.url && (
        <MapPlane mapConfig={mapConfig} />
      )}

      {tagIds.map((id) => (
        <TagMarker 
            key={id} 
            id={id} 
            tagsRef={tagsRef} 
            trailsRef={trailsRef}
            isTrailEnabled={enabledTrails && enabledTrails.has(id)}
        />
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
  const [mapOffsetX, setMapOffsetX] = useState(0);
  const [mapOffsetY, setMapOffsetY] = useState(0);
  const [is2D, setIs2D] = useState(true);
  
  // Config State
  const [isConfigOpen, setIsConfigOpen] = useState(false);
  const [configTagId, setConfigTagId] = useState('');
  const [configCmdId, setConfigCmdId] = useState('1');
  const [configData, setConfigData] = useState('');

  // Search/Filter State
  const [searchTerm, setSearchTerm] = useState('');
  const [focusTagId, setFocusTagId] = useState(null);

  // Trajectory State
  const [enabledTrails, setEnabledTrails] = useState(new Set());
  const trailsRef = useRef({});
  const trailRecordingEnabledRef = useRef(new Set());

  // Context Menu State
  const [contextMenu, setContextMenu] = useState(null);
  const [menuTagId, setMenuTagId] = useState(null);

  useEffect(() => {
    fetch('/project.xml')
      .then(res => res.text())
      .then(str => {
        const parser = new DOMParser();
        const xmlDoc = parser.parseFromString(str, "text/xml");
        const mapItem = xmlDoc.getElementsByTagName("mapItem")[0];
        if (mapItem) {
          const url = mapItem.getAttribute("url");
          const w = parseFloat(mapItem.getAttribute("width")) / 100.0;
          const h = parseFloat(mapItem.getAttribute("height")) / 100.0;
          const xOffset = parseFloat(mapItem.getAttribute("x-topleft")) / 100.0;
          const yOffset = parseFloat(mapItem.getAttribute("y-topleft")) / 100.0;
          
          setMapConfig({ width: w, height: h, url: `/Map/${url}` });
          setMapOffsetX(xOffset);
          setMapOffsetY(yOffset);
        }
      })
      .catch(err => console.error("Failed to load config", err));
  }, []);

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
        // Clear trails on new connection to avoid stale/mismatched data
        trailsRef.current = {};
        setEnabledTrails(new Set());
        trailRecordingEnabledRef.current = new Set();

        fetch('/api/tags')
          .then(res => res.json())
          .then(initialTags => {
             if (Array.isArray(initialTags)) {
                initialTags.forEach(tag => {
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
            // Keep raw data for debugging
            msg.rawX = msg.x;
            msg.rawY = msg.y;

            // Apply offset for visualization
            msg.x -= mapOffsetX;
            msg.y -= mapOffsetY;

            const isNew = !tagsRef.current[msg.id];
            tagsRef.current[msg.id] = msg;
            if (isNew) {
               setTagIds(prev => [...prev, msg.id]);
            }

            // Record Trajectory if enabled
            if (trailRecordingEnabledRef.current.has(msg.id)) {
                if (!trailsRef.current[msg.id]) trailsRef.current[msg.id] = [];
                
                const worldX = msg.x;
                // Force Flat Trajectory: Ignore height noise, keep it just above map
                const worldY = 0.2; 
                const worldZ = msg.y;
                
                // Validate Coordinates
                if (Number.isFinite(worldX) && Number.isFinite(worldZ) &&
                   !(Math.abs(worldX) < 0.001 && Math.abs(worldZ) < 0.001)) {
                    
                    const currentPath = trailsRef.current[msg.id];
                    const lastPt = currentPath.length > 0 ? currentPath[currentPath.length - 1] : null;
                    
                    let isValid = true;
                    // let resetHistory = false;

                    if (lastPt) {
                        const dist = Math.sqrt(
                            Math.pow(lastPt[0] - worldX, 2) + 
                            Math.pow(lastPt[2] - worldZ, 2)
                        );
                        // Filter 1: Dedup (too close)
                        if (dist < 0.05) isValid = false;
                        
                        // removed resetHistory logic to satisfy "no clear unless user clear"
                        // Long jumps will draw a line. 
                    }

                    if (isValid) {
                        currentPath.push([worldX, worldY, worldZ]);
                        
                        // Limit history window (Timeline view) - increased to 5000
                        if (currentPath.length > 5000) {
                            currentPath.shift();
                        }
                    }
                }
            }
          }
        } catch (e) {
          console.error("Parse error", e);
        }
      };
    };

    if (mapOffsetX !== 0 || mapOffsetY !== 0 || mapConfig.url) {
        connect();
    }
    
    // Debug Log for Offsets
    console.log("Map Config:", mapConfig, "Offsets:", mapOffsetX, mapOffsetY);

    return () => {
      shouldReconnect = false;
      if (ws) ws.close();
      if (reconnectTimer) clearTimeout(reconnectTimer);
    };
  }, [mapOffsetX, mapOffsetY, mapConfig.url]);

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
      if (res.ok) {
          alert("Config sent!");
          setIsConfigOpen(false);
      }
      else res.text().then(t => alert("Error: " + t));
    })
    .catch(err => alert("Network error: " + err));
  };

  const handleContextMenu = (event, id) => {
    event.preventDefault();
    setMenuTagId(id);
    setContextMenu({
      mouseX: event.clientX + 2,
      mouseY: event.clientY - 6,
    });
  };

  const handleCloseMenu = () => {
    setContextMenu(null);
    setMenuTagId(null);
  };

  const handleToggleTrail = () => {
    if (menuTagId) {
        const newSet = new Set(enabledTrails);
        if (newSet.has(menuTagId)) {
            newSet.delete(menuTagId);
            trailRecordingEnabledRef.current.delete(menuTagId);
        } else {
            newSet.add(menuTagId);
            trailRecordingEnabledRef.current.add(menuTagId);
        }
        setEnabledTrails(newSet);
    }
    handleCloseMenu();
  };

  const handleClearTrail = () => {
    if (menuTagId && trailsRef.current[menuTagId]) {
        trailsRef.current[menuTagId] = [];
    }
    handleCloseMenu();
  };

  const handleToggleAllPlots = () => {
    // Check if all currently displayed tags are already enabled
    const allEnabled = displayedTags.length > 0 && displayedTags.every(tag => enabledTrails.has(tag.id));

    if (allEnabled) {
        // Disable all
        setEnabledTrails(new Set());
        trailRecordingEnabledRef.current.clear();
    } else {
        // Enable all
        const newSet = new Set();
        displayedTags.forEach(tag => {
            newSet.add(tag.id);
            trailRecordingEnabledRef.current.add(tag.id);
        });
        setEnabledTrails(newSet);
    }
  };

  const handleClearScreen = () => {
     // Clear data for all
     Object.keys(trailsRef.current).forEach(k => {
         trailsRef.current[k] = [];
     });
     // We do not disable the recording, just clear the history.
  };

  const filteredTags = displayedTags.filter(tag => 
    tag.id.toString(16).toUpperCase().includes(searchTerm.toUpperCase())
  );

  // Helper to determine toggle button text
  const areAllEnabled = displayedTags.length > 0 && displayedTags.every(tag => enabledTrails.has(tag.id));

  return (
    <div className="d-flex flex-column vh-100 overflow-hidden">
      <nav className="navbar navbar-dark bg-dark flex-shrink-0 px-3">
        <div className="d-flex align-items-center w-100">
          <span className="navbar-brand mb-0 h1 me-auto">AOX Engine Web</span>
          <div className="d-flex gap-2 align-items-center">
            <Button 
                variant="outlined" 
                color={areAllEnabled ? "secondary" : "info"} 
                size="small"
                onClick={handleToggleAllPlots}
                style={{ color: 'white', borderColor: 'white' }}
            >
                {areAllEnabled ? "Disable All Plots" : "Enable All Plots"}
            </Button>
            <Button 
                variant="outlined" 
                color="warning" 
                size="small"
                onClick={handleClearScreen}
            >
                Clear Screen
            </Button>

            <Button 
                variant="contained" 
                color="primary" 
                size="small"
                onClick={() => setIsConfigOpen(true)}
                style={{ marginRight: '10px' }}
            >
                Config
            </Button>

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
        <div className="bg-light border-end overflow-hidden d-flex flex-column p-3" style={{ width: '300px', flexShrink: 0 }}>
          <h5>Active Tags ({filteredTags.length})</h5>
          
          <div className="mb-3">
              <input 
                type="text" 
                className="form-control" 
                placeholder="Search Tag ID..." 
                value={searchTerm}
                onChange={e => setSearchTerm(e.target.value)}
              />
          </div>

          <div className="overflow-auto flex-grow-1">
            <ul className="list-group">
                {filteredTags.map(tag => (
                <li 
                    key={tag.id} 
                    className="list-group-item list-group-item-action d-flex justify-content-between align-items-center"
                    style={{ cursor: 'context-menu' }}
                    onClick={() => setFocusTagId(tag.id)}
                    onContextMenu={(e) => handleContextMenu(e, tag.id)}
                    title={`Raw: ${tag.rawX?.toFixed(2)}, ${tag.rawY?.toFixed(2)}`}
                >
                    <div>
                    <strong>{tag.id.toString(16).toUpperCase()}</strong>
                    {enabledTrails.has(tag.id) && <span className="ms-2 badge bg-info" style={{fontSize: '0.6em'}}>Plot</span>}
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
        </div>
        
        <div className="flex-grow-1 position-relative bg-secondary">
          <Canvas camera={{ position: [mapConfig.width / 2, 60, mapConfig.height / 2], fov: 50 }} style={{ width: '100%', height: '100%' }}>
            <Scene 
                tagIds={tagIds} 
                tagsRef={tagsRef} 
                mapConfig={mapConfig} 
                is2D={is2D} 
                focusTagId={focusTagId}
                setFocusTagId={setFocusTagId}
                trailsRef={trailsRef}
                enabledTrails={enabledTrails}
            />
          </Canvas>
          <div className="position-absolute bottom-0 start-0 p-2 text-light small" style={{ background: 'rgba(0,0,0,0.5)' }}>
            Map: {mapConfig.width.toFixed(1)}m x {mapConfig.height.toFixed(1)}m @ (0.0, 0.0)
          </div>
        </div>
      </div>

      {/* Config Dialog */}
      <Dialog open={isConfigOpen} onClose={() => setIsConfigOpen(false)}>
        <DialogTitle>Tag Configuration</DialogTitle>
        <DialogContent>
            <div className="mt-2">
                <TextField
                    autoFocus
                    margin="dense"
                    label="Tag ID (Hex)"
                    type="text"
                    fullWidth
                    variant="outlined"
                    value={configTagId}
                    onChange={e => setConfigTagId(e.target.value)}
                    placeholder="e.g. 1A2B"
                />
                <TextField
                    margin="dense"
                    label="Cmd ID (Int)"
                    type="number"
                    fullWidth
                    variant="outlined"
                    value={configCmdId}
                    onChange={e => setConfigCmdId(e.target.value)}
                />
                <TextField
                    margin="dense"
                    label="Data (Hex)"
                    type="text"
                    fullWidth
                    variant="outlined"
                    value={configData}
                    onChange={e => setConfigData(e.target.value)}
                    placeholder="e.g. 010203"
                />
            </div>
        </DialogContent>
        <DialogActions>
            <Button onClick={() => setIsConfigOpen(false)}>Cancel</Button>
            <Button onClick={handleSendConfig} variant="contained" color="primary">Send Config</Button>
        </DialogActions>
      </Dialog>
      
      {/* Context Menu */}
      <Menu
        open={contextMenu !== null}
        onClose={handleCloseMenu}
        anchorReference="anchorPosition"
        anchorPosition={
          contextMenu !== null
            ? { top: contextMenu.mouseY, left: contextMenu.mouseX }
            : undefined
        }
      >
        <MenuItem onClick={handleToggleTrail}>
            {menuTagId && enabledTrails.has(menuTagId) ? "Disable Trajectory" : "Enable Trajectory"}
        </MenuItem>
        <MenuItem onClick={handleClearTrail}>Clear Trajectory</MenuItem>
      </Menu>
    </div>
  );
}
export default App;