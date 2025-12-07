import React, { useState, useEffect, useRef, useMemo } from 'react';
import { Canvas } from '@react-three/fiber';
import 'bootstrap/dist/css/bootstrap.min.css';
import { Dialog, DialogTitle, DialogContent, DialogActions, TextField, Button, Menu, MenuItem, Checkbox, Switch, FormControlLabel } from '@mui/material';

import Scene from './components/Scene';
import HeightView from './components/HeightView';

function App() {
  const tagsRef = useRef({});
  const [tagIds, setTagIds] = useState([]);
  const [displayedTags, setDisplayedTags] = useState([]);
  
  const [status, setStatus] = useState('Disconnected');
  const [mapConfig, setMapConfig] = useState({ width: 100, height: 100, url: null });
  const [mapOffsetX, setMapOffsetX] = useState(0);
  const [mapOffsetY, setMapOffsetY] = useState(0);
  const [is2D, setIs2D] = useState(true);
  const [viewMode, setViewMode] = useState('map'); // 'map' | 'height'
  const [referenceTagId, setReferenceTagId] = useState(null);
  
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

  // Selection/Filter State
  const [selectedTags, setSelectedTags] = useState(new Set());
  const [showSelectedOnly, setShowSelectedOnly] = useState(false);

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
          if (msg.pressure) console.log("Got Pressure:", msg.id, msg.pressure);
          if (msg.id) {
            // Keep raw data for debugging
            msg.rawX = msg.x;
            msg.rawY = msg.y;

            // Apply offset for visualization
            msg.x -= mapOffsetX;
            msg.y -= mapOffsetY;

            // Hard gate: ignore obviously off-map points to avoid visual outliers
            const margin = 20; // meters
            if (msg.x < -margin || msg.y < -margin || msg.x > mapConfig.width + margin || msg.y > mapConfig.height + margin) {
              return;
            }

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
      const currentTags = Object.values(tagsRef.current);
      // Sort tags by ID for consistent ordering
      currentTags.sort((a, b) => a.id - b.id);
      setDisplayedTags(currentTags);
    }, 50);
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

  const handleToggleSelection = (id) => {
    const newSet = new Set(selectedTags);
    if (newSet.has(id)) {
        newSet.delete(id);
    } else {
        newSet.add(id);
    }
    setSelectedTags(newSet);
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

  const handleSetReference = () => {
    setReferenceTagId(menuTagId);
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

  // Compute Visible Tags for Map
  const visibleTagIds = useMemo(() => {
      const ids = showSelectedOnly ? tagIds.filter(id => selectedTags.has(id)) : tagIds;
      if (searchTerm.trim().length === 0) return ids;
      const upper = searchTerm.toUpperCase();
      return ids.filter(id => id.toString(16).toUpperCase().includes(upper));
  }, [tagIds, showSelectedOnly, selectedTags, searchTerm]);

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

            <div className="btn-group me-2">
                <button 
                  className={`btn btn-sm ${viewMode === 'map' ? 'btn-primary' : 'btn-outline-secondary'}`}
                  onClick={() => setViewMode('map')}
                >
                  Map
                </button>
                <button 
                  className={`btn btn-sm ${viewMode === 'height' ? 'btn-primary' : 'btn-outline-secondary'}`}
                  onClick={() => {
                      if (viewMode !== 'height') {
                          if (!referenceTagId) {
                              alert("Please select a Reference Tag (Right Click on Tag in List) to calculate relative height.");
                              return;
                          }
                      }
                      setViewMode('height');
                  }}
                >
                  Height
                </button>
            </div>

            {viewMode === 'map' && (
                <>
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
                </>
            )}
            <span className={`badge ${status === 'Connected' ? 'bg-success' : 'bg-danger'}`}>
              {status}
            </span>
          </div>
        </div>
      </nav>

      <div className="flex-grow-1 d-flex flex-row" style={{ minHeight: 0 }}>
        <div className="bg-light border-end overflow-hidden d-flex flex-column p-3" style={{ width: '300px', flexShrink: 0 }}>
          <div className="d-flex justify-content-between align-items-center mb-2">
            <h5 className="mb-0">Active Tags ({filteredTags.length})</h5>
          </div>
          
          <div className="mb-2">
            <FormControlLabel 
                control={
                    <Switch 
                        size="small"
                        checked={showSelectedOnly} 
                        onChange={e => setShowSelectedOnly(e.target.checked)} 
                    />
                } 
                label={<span style={{fontSize: '0.9rem'}}>Show Confirmed Only</span>} 
            />
          </div>

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
                    className="list-group-item list-group-item-action d-flex align-items-center"
                    style={{ cursor: 'context-menu', paddingLeft: '5px' }}
                    onClick={() => setFocusTagId(tag.id)}
                    onContextMenu={(e) => handleContextMenu(e, tag.id)}
                    title={`Raw: ${tag.rawX?.toFixed(2)}, ${tag.rawY?.toFixed(2)}`}
                >
                    <Checkbox 
                        edge="start"
                        checked={selectedTags.has(tag.id)}
                        onChange={() => handleToggleSelection(tag.id)}
                        onClick={(e) => e.stopPropagation()}
                        size="small"
                    />
                    <div className="flex-grow-1 ms-2 d-flex justify-content-between align-items-center">
                        <div>
                            <strong>{tag.id.toString(16).toUpperCase()}</strong>
                            {enabledTrails.has(tag.id) && <span className="ms-2 badge bg-info" style={{fontSize: '0.6em'}}>Plot</span>}
                            <br />
                            <small className="text-muted">
                                {tag.x.toFixed(2)}, {tag.y.toFixed(2)}, {tag.z.toFixed(2)}
                            </small>
                        </div>
                        <span className="badge bg-primary rounded-pill">L{tag.layer}</span>
                    </div>
                </li>
                ))}
            </ul>
          </div>
        </div>
        
        <div className="flex-grow-1 position-relative bg-secondary">
          {viewMode === 'map' ? (
              <>
                  <Canvas camera={{ position: [mapConfig.width / 2, 60, mapConfig.height / 2], fov: 50 }} style={{ width: '100%', height: '100%' }}>
                    <Scene 
                        tagIds={visibleTagIds} 
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
              </>
          ) : (
              <HeightView tags={displayedTags} referenceTagId={referenceTagId} />
          )}
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
        <MenuItem onClick={handleSetReference}>Set as Reference (Baro)</MenuItem>
        <MenuItem onClick={handleToggleTrail}>
            {menuTagId && enabledTrails.has(menuTagId) ? "Disable Trajectory" : "Enable Trajectory"}
        </MenuItem>
        <MenuItem onClick={handleClearTrail}>Clear Trajectory</MenuItem>
      </Menu>
    </div>
  );
}
export default App;
